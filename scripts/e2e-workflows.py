#!/usr/bin/env python3
"""Live end-to-end check of Waypoint workflows W1-W15 (+A1/A2) through NGINX with dev-OIDC tokens.
Local stack only. Read/exercise job: places orders, builds plans, drives loader/driver flows.
Usage: python scripts/e2e-workflows.py [BASE_DATE_MONDAY e.g. 2026-10-19]
"""
import base64, hashlib, json, secrets, urllib.parse, urllib.request, urllib.error, sys, subprocess, uuid
from datetime import date, timedelta, datetime, timezone

OIDC, API, PASSWORD = "http://localhost:8090", "http://localhost/api/v1", "waypoint"
REPO = r"C:\Users\aDMIN\OneDrive\Documents\GitHub\wt-integ"
PNG = base64.b64decode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **k):
        return None


def login(user):
    verifier = secrets.token_urlsafe(48)
    challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
    form = urllib.parse.urlencode({"username": user, "password": PASSWORD, "redirect_uri": "http://localhost/auth/callback",
                                   "state": "s", "code_challenge": challenge, "client_id": "waypoint-web"}).encode()
    try:
        urllib.request.build_opener(NoRedirect).open(urllib.request.Request(OIDC + "/oauth2/authorize", data=form))
        raise SystemExit("expected redirect")
    except urllib.error.HTTPError as e:
        code = urllib.parse.parse_qs(urllib.parse.urlparse(e.headers["Location"]).query)["code"][0]
    body = urllib.parse.urlencode({"grant_type": "authorization_code", "code": code, "code_verifier": verifier,
                                   "client_id": "waypoint-web", "redirect_uri": "http://localhost/auth/callback"}).encode()
    return json.load(urllib.request.urlopen(OIDC + "/oauth2/token", data=body))["access_token"]


def call(method, path, token=None, body=None, headers=None, raw=False):
    data = None
    if body is not None:
        data = body if isinstance(body, (bytes, bytearray)) else json.dumps(body).encode()
    req = urllib.request.Request(API + path, method=method, data=data)
    if not (headers and "Content-Type" in headers):
        req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            b = r.read()
            if raw:
                return r.status, b.decode(), dict(r.headers)
            return r.status, (json.loads(b) if b else {})
    except urllib.error.HTTPError as e:
        b = e.read()
        if raw:
            return e.code, b.decode(), dict(e.headers)
        try:
            return e.code, json.loads(b)
        except ValueError:
            return e.code, b.decode()


def psql(sql):
    out = subprocess.run(["docker", "compose", "-p", "waypoint-integ", "exec", "-T", "postgres", "psql", "-U", "waypoint", "-d", "waypoint", "-At", "-F", "|", "-c", sql],
                         cwd=REPO, capture_output=True, text=True)
    return out.stdout.strip() + (("\nERR:" + out.stderr.strip()) if out.returncode else "")


def pj(x, n=400):
    s = json.dumps(x, default=str)
    return s if len(s) <= n else s[:n] + "..."


def multipart(fields, fname, content, ctype="image/png"):
    bd = "----wp" + uuid.uuid4().hex
    out = b""
    for k, v in fields.items():
        out += ("--%s\r\nContent-Disposition: form-data; name=\"%s\"\r\n\r\n%s\r\n" % (bd, k, v)).encode()
    out += ("--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\nContent-Type: %s\r\n\r\n" % (bd, fname, ctype)).encode() + content + b"\r\n"
    out += ("--%s--\r\n" % bd).encode()
    return out, {"Content-Type": "multipart/form-data; boundary=" + bd}


RESULTS = {}  # workflow -> list of (ok, label, detail)


def chk(w, label, cond, detail=""):
    RESULTS.setdefault(w, []).append((bool(cond), label, "" if cond else str(detail)))
    print(("PASS " if cond else "FAIL ") + "[%s] %s" % (w, label) + ("" if cond else "  -> " + str(detail)[:600]))
    return bool(cond)


def note(w, label, detail):
    print("NOTE [%s] %s: %s" % (w, label, str(detail)[:600]))


def stage(name):
    def deco(fn):
        def run(*a, **k):
            print("\n=== %s" % name)
            try:
                return fn(*a, **k)
            except Exception as e:  # noqa
                import traceback
                traceback.print_exc()
                chk("ERR", name + " crashed", False, repr(e))
        return run
    return deco


def trk(user, order):
    s, b = call("GET", "/orders/%s/tracking" % order["id"], T[user])
    return s, (b.get("tracking") if isinstance(b, dict) and "tracking" in b else b) or {}


def d(base, n):
    return (base + timedelta(days=n)).isoformat()


T = {}
S = {}  # shared state


def place(user, day, units, temp, w="W1"):
    s, b = call("POST", "/orders", T[user], {"requestedDeliveryDate": day, "orderUnits": units, "orderWeightKg": units * 5, "orderVolumeM3": round(units * .05, 2), "temperatureRequirement": temp})
    chk(w, "%s places %s order %su for %s -> 201" % (user, temp, units, day), s == 201 and b["order"]["requestedDeliveryDate"] == day, (s, b))
    return b["order"] if s == 201 else None


def make_plan(day):
    s, b = call("POST", "/planning/plans", T["dispatcher"], {"deliveryDate": day})
    chk("W2", "create plan %s" % day, s in (200, 201), (s, b))
    return b["plan"]["id"]


def detail(pid):
    return call("GET", "/planning/plans/%s" % pid, T["dispatcher"])[1]


def confirm_and_ack(pid, day):
    s, b = call("POST", "/planning/plans/%s/confirm" % pid, T["dispatcher"])
    chk("W2", "confirm plan %s -> 200 confirmed" % day, s == 200 and b.get("status") == "confirmed", (s, b))
    dt = detail(pid)
    pub = dt.get("publication", {})
    chk("W2", "published version >= 1 (v%s)" % pub.get("version"), pub.get("version", 0) >= 1 and dt["plan"]["status"] == "confirmed", pub)
    ver = pub.get("version", 1)
    for u in ("loader", "driver"):
        s, b = call("POST", "/planning/plans/%s/acknowledgements" % pid, T[u], {"version": ver})
        chk("W2", "%s acknowledges v%s" % (u, ver), s == 200 and b.get("status") == "acknowledged", (s, b))
    acks = detail(pid).get("publication", {}).get("acknowledgements", [])
    chk("W2", "publication lists loader+driver acknowledgements", {a["actorRole"] for a in acks} >= {"LOADER", "DRIVER"}, acks)
    return ver


def outbox(where):
    rows = psql("select event_type,outlet_id,status,body from shared.notification_outbox where %s order by id" % where)
    return rows


def load_trip(day, load_all=True, ver=1):
    s, b = call("GET", "/loading/trips?date=%s" % day, T["loader"])
    trips = b.get("items", []) if s == 200 else []
    chk("W2", "loader sees %s trip(s) for %s" % (len(trips), day), s == 200 and trips, (s, b))
    tid = trips[0]["tripId"]
    for t in trips:  # use the first trip that actually has orders; the planner may split small orders over several trips
        sd, det = call("GET", "/loading/trips/%s" % t["tripId"], T["loader"])
        if sd == 200 and det.get("orders"):
            tid = t["tripId"]
            break
    chk("W2", "no empty trip is published to the loader for %s" % day, all(call("GET", "/loading/trips/%s" % t["tripId"], T["loader"])[1].get("orders") for t in trips), "empty trip present")
    s, b = call("POST", "/loading/trips/%s/start" % tid, T["loader"], headers={"If-Match": str(ver)})
    chk("W2", "loader starts loading session (planVersion %s)" % ver, s == 200 and b.get("status") == "in_progress", (s, b))
    orders = b.get("orders", [])
    if load_all:
        for o in orders:
            s, bb = call("PUT", "/loading/trips/%s/orders/%s/loaded" % (tid, o["orderId"]), T["loader"])
            chk("W2", "loader marks %s loaded" % o["orderRef"], s == 200, (s, bb))
    return tid, orders


def load_trip_id(tid, ver=1):
    s, b = call("POST", "/loading/trips/%s/start" % tid, T["loader"], headers={"If-Match": str(ver)})
    chk("W2", "loader starts loading session (planVersion %s)" % ver, s == 200 and b.get("status") == "in_progress", (s, b))
    for o in b.get("orders", []):
        s, bb = call("PUT", "/loading/trips/%s/orders/%s/loaded" % (tid, o["orderId"]), T["loader"])
        chk("W2", "loader marks %s loaded" % o["orderRef"], s == 200, (s, bb))
    return tid, b.get("orders", [])


def ready(tid):
    s, b = call("POST", "/loading/trips/%s/ready" % tid, T["loader"], {"chilledTemperatureC": 3, "sealNumber": "WP-E2E-%s" % tid[:4]})
    chk("W2", "loader marks trip ready (temp+seal)", s == 200 and b.get("status") == "ready", (s, b))


def sync(user, ops):
    s, b = call("POST", "/delivery/sync", T[user], {"operations": ops})
    return s, b


def result_status(b, opid):
    for r in b.get("results", []):
        if r.get("operationId") == opid:
            return r
    return {}


# ------------------------------------------------------------------ stages
@stage("A1/A2 agent assistants status")
def stage_agents():
    s, b = call("GET", "/agent/assistants/status", T["store-manager"])
    chk("A", "GET /agent/assistants/status -> 200 both on", s == 200 and b == {"orderAssistant": True, "dashboardAssistant": True}, (s, b))
    chk("A", "unauthenticated status -> 401", call("GET", "/agent/assistants/status")[0] == 401)
    s, b = call("POST", "/agent/order-assistant/drafts", T["store-manager"], {"message": "rice 10 bags, oil 24 bottles"})
    chk("A", "A1 draft returns lines and submitted=false", s == 200 and b.get("submitted") is False and len(b.get("lines", [])) > 0, (s, b))
    s, b = call("POST", "/agent/dashboard-assistant/drafts", T["store-manager"], {"message": "what arrived short this week and which receipts I still need to confirm", "locale": "en"})
    chk("A", "A2 draft builds cards, saved=false", s == 200 and len(b.get("draft", {}).get("cards", [])) > 0 and b.get("saved") is False, (s, b))


@stage("Part 1: D1 W1/W2 orders, plan, publish, acks")
def part1(D1):
    A = {}
    A["A1"] = place("store-manager", D1, 10, "chilled")
    A["A2"] = place("store-manager-b", D1, 10, "ambient")
    A["A3"] = place("store-manager", D1, 6, "ambient")
    A["A4"] = place("store-manager-b", D1, 4, "ambient")
    S["A"] = A
    s, b = call("GET", "/orders", T["dispatcher"])
    refs = {o["orderRef"]: o for o in b.get("items", [])} if s == 200 else {}
    chk("W1", "dispatcher order queue lists all 4 new orders with status", all(a["orderRef"] in refs and refs[a["orderRef"]].get("status") for a in A.values()), (s, [o.get("orderRef") for o in b.get("items", [])][:10]))
    note("W1", "queue statuses", {k: refs.get(v["orderRef"], {}).get("status") for k, v in A.items()})
    s, b = call("GET", "/orders", T["store-manager"])
    chk("W1", "store-manager list scoped to own outlet", s == 200 and all(o["outletId"] == "OUT034" for o in b["items"]), (s, pj(b)))
    chk("W1", "store-manager-b cannot read A1 (403)", call("GET", "/orders/%s" % A["A1"]["id"], T["store-manager-b"])[0] == 403)
    s, b = trk("store-manager", A["A1"])
    chk("W1", "tracking: stage CONFIRMED before planning", s == 200 and b.get("stage") in ("CONFIRMED", "PLANNED"), (s, pj(b)))

    pid = make_plan(D1)
    S["pid1"] = pid
    s, b = call("POST", "/planning/plans/%s/generate" % pid, T["dispatcher"])
    chk("W2", "generate allocates all 4 orders", s == 200 and b.get("allocated") == 4 and b.get("unallocated") == 0, (s, b))
    dt = detail(pid)
    trips = dt["trips"]
    note("W2", "trips", [(t["vehicleId"], t["tripNumber"]) for t in trips])
    chk("W2", "all allocations on VEH001 (the seeded driver's vehicle)", all(a["vehicleId"] == "VEH001" for a in dt["allocations"]), [a["vehicleId"] for a in dt["allocations"]])
    ver = confirm_and_ack(pid, D1)
    s, b = call("POST", "/planning/plans/%s/acknowledgements" % pid, T["driver"], {"version": ver + 1})
    chk("W2", "ack of a stale/unknown version -> 409", s == 409, (s, b))
    chk("W2", "store-manager cannot acknowledge -> 403", call("POST", "/planning/plans/%s/acknowledgements" % pid, T["store-manager"], {"version": ver})[0] == 403)
    s, b = trk("store-manager", A["A1"])
    chk("W2", "store tracking shows planned arrival after publish", s == 200 and b.get("planning", {}).get("plannedArrivalAt"), (s, pj(b)))
    s_, b_ = call("GET", "/loading/trips?date=%s" % D1, T["loader"])
    for tr in sorted(b_.get("items", []), key=lambda t: t["tripNumber"]):
        tid, orders = load_trip_id(tr["tripId"])
        ready(tid)


@stage("Part 2: D1 driver run (W5 checkout, W7 location, W9 ETA, W8 data, W11 conflict, W13 return)")
def part2(D1):
    A, pid = S["A"], S["pid1"]
    now_z = lambda: datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    s, b = call("GET", "/delivery/trips?date=%s" % D1, T["driver"])
    trips = sorted(b.get("items", []), key=lambda t: t["tripNumber"]) if s == 200 else []
    chk("W5", "driver lists %d ready trip(s)" % len(trips), s == 200 and trips, (s, b))
    first = True
    last_tid, last_stops = None, None
    for ti, tr in enumerate(trips):
        tid = tr["tripId"]
        s, b = call("POST", "/delivery/trips/%s/prepare" % tid, T["driver"])
        chk("W5", "trip %s: driver prepares run from loading READY snapshot" % tr["tripNumber"], s == 200 and b.get("run", {}).get("status") == "prepared", (s, pj(b)))
        tstops = {st["orderRef"]: st for st in b.get("stops", [])}
        ids = [st["orderId"] for st in b["stops"]]
        s, bb = call("POST", "/delivery/trips/%s/start" % tid, T["driver"], headers={"Idempotency-Key": "e2e-start-pre-" + tid})
        chk("W5", "start before checkout -> 409", s == 409, (s, bb))
        if len(ids) > 1:
            s, bb = call("POST", "/delivery/trips/%s/checkout" % tid, T["driver"], {"planVersion": 1, "confirmedOrderIds": ids[:-1]})
            chk("W5", "checkout missing one order -> blocked", s == 200 and bb.get("checkout", {}).get("status") == "blocked", (s, bb))
            s, bb = call("GET", "/delivery/trips/%s/checkout" % tid, T["loader"])
            chk("W5", "loader (DEPOT_NORTH profile) sees checkout alert for missing goods", s == 200 and bb.get("checkout") and bb["checkout"].get("missingOrderIds"), (s, bb))
            s, bb = call("GET", "/delivery/trips/%s/checkout" % tid, T["dispatcher"])
            chk("W5", "dispatcher sees checkout alert", s == 200 and bb.get("checkout") and bb["checkout"].get("missingOrderIds"), (s, bb))
        s, bb = call("POST", "/delivery/trips/%s/checkout" % tid, T["driver"], {"planVersion": 1, "confirmedOrderIds": ids})
        chk("W5", "checkout all onboard -> confirmed", s == 200 and bb.get("checkout", {}).get("status") == "confirmed", (s, bb))
        s, bb = call("POST", "/delivery/trips/%s/start" % tid, T["driver"], headers={"Idempotency-Key": "e2e-start-" + tid})
        chk("W5", "driver start after checkout -> 200 in_progress", s == 200 and bb.get("run", {}).get("status") == "in_progress", (s, pj(bb)))

        if first:
            first = False
            s, bb = call("GET", "/delivery/trips/%s" % tid, T["dispatcher"])
            chk("W8", "dispatcher trip detail has run.startedAt + stops", s == 200 and bb.get("run", {}).get("startedAt") and bb.get("stops"), (s, pj(bb.get("run"))))
            note("W8", "run.startedAt", bb.get("run", {}).get("startedAt"))
            pt = {"latitude": 6.9271, "longitude": 79.8612, "timestamp": now_z()}
            s, bb = call("POST", "/delivery/trips/%s/location" % tid, T["driver"], pt)
            chk("W7", "driver posts live location -> 200", s == 200 and bb.get("location", {}).get("latitude") == 6.9271, (s, bb))
            s, bb = call("GET", "/delivery/trips/%s/location" % tid, T["dispatcher"])
            chk("W7", "dispatcher reads latest location", s == 200 and (bb.get("location") or {}).get("latitude") == 6.9271, (s, bb))
            chk("W7", "store-manager direct location read -> 403", call("GET", "/delivery/trips/%s/location" % tid, T["store-manager"])[0] == 403)
            probe = next((r for r in A if A[r]["orderRef"] in tstops), None)
            if probe:
                s, bb = trk("store-manager" if A[probe]["outletId"] == "OUT034" else "store-manager-b", A[probe])
                chk("W7", "store tracking of %s: OUT_FOR_DELIVERY + location" % probe, s == 200 and bb.get("stage") == "OUT_FOR_DELIVERY" and (bb.get("delivery") or {}).get("location"), (s, pj(bb.get("delivery")), bb.get("stage")))
            chk("W7", "other store cannot read A1 tracking -> 403", call("GET", "/orders/%s/tracking" % A["A1"]["id"], T["store-manager-b"])[0] == 403)
            p1 = sorted(tstops.values(), key=lambda x: x["stopSequence"])[0]
            po = next((A[r] for r in A if A[r]["orderRef"] == p1["orderRef"]), None)
            eta0 = datetime.now(timezone.utc) + timedelta(hours=1)
            base = {"planVersion": 1, "lateRisk": "On track"}
            fz = lambda t: t.isoformat().replace("+00:00", "Z")
            u = "/delivery/trips/%s/stops/%s/arrival-prediction" % (tid, p1["id"])
            s1, b1 = call("POST", u, T["dispatcher"], dict(base, estimatedArrivalAt=fz(eta0)))
            chk("W9", "dispatcher publishes baseline ETA -> 200", s1 == 200, (s1, b1))
            s2, b2 = call("POST", u, T["dispatcher"], dict(base, estimatedArrivalAt=fz(eta0 + timedelta(minutes=45)), lateRisk="Window at risk"))
            chk("W9", "ETA +45min -> 200 and store notified (notifiedArrivalAt set)", s2 == 200 and (b2.get("prediction") or {}).get("notifiedArrivalAt"), (s2, b2))
            ob = outbox("event_type='ARRIVAL_CHANGE'")
            note("W9", "outbox ARRIVAL_CHANGE rows", ob or "(none)")
            chk("W9", "ARRIVAL_CHANGE row exists in shared.notification_outbox", "ARRIVAL_CHANGE" in ob, "none (shared-service enqueue returned 500; DB check constraint notification_outbox_event_type_check allows only DEFERRAL/MAJOR_DELAY/LOAD_SHORTFALL)")
            if po:
                s, bb = trk("store-manager" if po["outletId"] == "OUT034" else "store-manager-b", po)
                chk("W9", "store tracking carries arrivalPrediction", (bb.get("delivery") or {}).get("arrivalPrediction"), pj(bb.get("delivery")))
            chk("W9", "driver cannot publish ETA -> 403", call("POST", u, T["driver"], dict(base, estimatedArrivalAt=fz(eta0)))[0] == 403)
            s, bb = call("GET", "/delivery/trips/%s/lateness-history" % tid, T["dispatcher"])
            note("W15", "lateness-history", (s, pj(bb, 500)))
            chk("W15", "lateness-history answers for dispatcher; driver 403", s == 200 and call("GET", "/delivery/trips/%s/lateness-history" % tid, T["driver"])[0] == 403, (s, bb))
            s, bb = call("GET", "/orders/forecast", T["dispatcher"])
            chk("W15", "forecast exposes serviceEstimateVersion (client uses it for the fallback banner)", s == 200 and (bb.get("forecast") or {}).get("serviceEstimateVersion"), (s, pj(bb)))

        for st in sorted(tstops.values(), key=lambda x: x["stopSequence"]):
            ref = st["orderRef"]
            who = next((r for r in A if A[r]["orderRef"] == ref), "extra")
            s, bb = call("POST", "/delivery/trips/%s/stops/%s/arrive" % (tid, st["id"]), T["driver"], {"occurredAt": now_z()}, headers={"Idempotency-Key": "e2e-arr-" + st["id"]})
            chk("W5", "arrive stop %s (%s)" % (ref, who), s == 200, (s, pj(bb)))
            if who == "A1":
                s, bb = call("GET", "/delivery/trips/%s" % tid, T["dispatcher"])
                chk("W8", "stop carries arrivedAt after arrival (last stop event for silent-trip watch)", any(x.get("arrivedAt") for x in bb.get("stops", [])), pj(bb["stops"][0]))
            if who in ("A3", "A4"):
                units = 6 if who == "A3" else 4
                if who == "A3":
                    s, bb = call("POST", "/delivery/trips/%s/stops/%s/outcome" % (tid, st["id"]), T["driver"], {"code": "REFUSED", "reason": "GOODS_REJECTED"}, headers={"Idempotency-Key": "e2e-out-bad-" + st["id"]})
                    chk("W13", "REFUSED without returnedGoods -> 400", s == 400, (s, bb))
                s, bb = call("POST", "/delivery/trips/%s/stops/%s/outcome" % (tid, st["id"]), T["driver"],
                             {"code": "REFUSED", "reason": "GOODS_REJECTED", "returnedGoods": {"goods": "Rejected cartons", "units": units, "resolution": "NEXT_RUN" if who == "A3" else "REQUEST_DEFERRAL"}},
                             headers={"Idempotency-Key": "e2e-out-" + st["id"]})
                chk("W13", "%s REFUSED + returnedGoods (%s) -> 200" % (who, "NEXT_RUN" if who == "A3" else "REQUEST_DEFERRAL"), s == 200, (s, pj(bb)))
                continue
            body, h = multipart({"type": "SIGNATURE", "mimeType": "image/png", "receiverName": "Receiver " + ref}, "proof.png", PNG)
            h["Idempotency-Key"] = "e2e-proof-" + st["id"]
            s, bb = call("POST", "/delivery/trips/%s/stops/%s/proofs" % (tid, st["id"]), T["driver"], body, headers=h)
            chk("W5", "proof uploaded for %s" % ref, s == 201, (s, bb))
            exp = st.get("expectedUnits") or 1
            code, units = ("PARTIAL", 8) if who == "A2" else ("DELIVERED", exp)
            s, bb = call("POST", "/delivery/trips/%s/stops/%s/outcome" % (tid, st["id"]), T["driver"], {"code": code, "deliveredUnits": units, "dependsOnOperationId": "e2e-proof-" + st["id"]}, headers={"Idempotency-Key": "e2e-out-" + st["id"]})
            chk("W5", "%s outcome %s (%s units)" % (ref, code, units), s == 200 and code in json.dumps(bb), (s, pj(bb)))
        last_tid, last_stops = tid, tstops

    # W11: make plan v2 while the last trip is still in progress, then sync an offline v1 record
    s, bb = call("POST", "/planning/plans/%s/revise" % pid, T["dispatcher"])
    chk("W11", "dispatcher opens revision on in-progress plan", s == 200, (s, bb))
    imp = {"version": 1, "sourceSystem": "e2e_w11", "orders": [{"externalOrderId": "E2E-W11-" + D1, "outletId": "OUT019", "brand": "Style", "requestedDeliveryDate": D1, "orderUnits": 2, "orderWeightKg": 10, "orderVolumeM3": 0.1, "temperatureRequirement": "ambient"}]}
    s, bb = call("POST", "/orders/import", T["dispatcher"], imp)
    extra_id = None
    if s == 200:
        res = bb.get("results") or bb.get("items") or []
        if res:
            extra_id = (res[0].get("order") or res[0]).get("id")
    chk("W11", "late order (late-window outlet) imported by dispatcher", extra_id, (s, pj(bb)))
    assigned = False
    for tn in (1, 2):
        s, bb = call("POST", "/planning/plans/%s/allocations" % pid, T["dispatcher"], {"orderId": extra_id, "vehicleId": "VEH001", "tripNumber": tn, "reason": "e2e late add to create plan v2"})
        if s == 201:
            assigned = True
            break
    chk("W11", "late order assigned to VEH001", assigned, (s, pj(bb)))
    s, bb = call("POST", "/planning/plans/%s/confirm" % pid, T["dispatcher"])
    ver = detail(pid).get("publication", {}).get("version")
    chk("W11", "plan republished as v2 (got v%s)" % ver, s == 200 and ver == 2, (s, bb, ver))
    anystop = sorted(last_stops.values(), key=lambda x: x["stopSequence"])[0]
    s, bb = sync("driver", [{"operationId": ("e2e-offline-v1-" + D1), "type": "INCIDENT_REPORT", "tripId": last_tid, "stopId": anystop["id"], "planVersion": 1,
                             "occurredAt": now_z(), "payload": {"category": "OUTLET", "description": "Gate locked, recorded offline on v1"}}])
    r = result_status(bb, ("e2e-offline-v1-" + D1))
    cf = r.get("conflict") or (r.get("payload") or {}).get("conflict") or {}
    chk("W11", "offline v1 record while plan is v2 -> APPLIED + conflict{recorded 1,current 2}", s == 200 and r.get("status") == "APPLIED" and cf.get("recordedPlanVersion") == 1 and cf.get("currentPlanVersion") == 2, (s, pj(bb, 900)))
    s, bb = call("GET", "/delivery/sync-conflicts?date=%s" % D1, T["dispatcher"])
    items = bb.get("items", []) if s == 200 else []
    mine = [i for i in items if i.get("operationId") == ("e2e-offline-v1-" + D1)]
    chk("W11", "dispatcher lists the conflict at GET /delivery/sync-conflicts", s == 200 and len(mine) == 1, (s, pj(bb)))
    chk("W11", "driver GET sync-conflicts -> 403", call("GET", "/delivery/sync-conflicts", T["driver"])[0] == 403)
    if mine:
        s, bb = call("POST", "/delivery/sync-conflicts/%s/settle" % mine[0]["id"], T["dispatcher"])
        chk("W11", "dispatcher settles conflict (settledBy set)", s == 200 and "settledBy" in json.dumps(bb), (s, pj(bb)))
        s, bb = call("GET", "/delivery/sync-conflicts?status=open", T["dispatcher"])
        chk("W11", "settled conflict no longer in status=open", ("e2e-offline-v1-" + D1) not in json.dumps(bb), pj(bb))
    for tr in trips:
        s, bb = call("POST", "/delivery/trips/%s/complete" % tr["tripId"], T["driver"], {}, headers={"Idempotency-Key": "e2e-complete-" + tr["tripId"]})
        chk("W5", "driver completes trip %s" % tr["tripNumber"], s == 200, (s, pj(bb)))
    chk("W7", "location after completion -> null for dispatcher", (call("GET", "/delivery/trips/%s/location" % trips[0]["tripId"], T["dispatcher"])[1] or {}).get("location") is None)

    s, bb = trk("store-manager", A["A3"])
    rg = (bb.get("delivery") or {}).get("returnedGoods") or {}
    chk("W13", "A3 tracking returnedGoods has followupOrderRef/date (NEXT_RUN)", rg.get("followupOrderRef") and rg.get("followupDate"), (s, pj(bb.get("delivery")), bb.get("stage")))
    s, bb = trk("store-manager-b", A["A4"])
    rg4 = (bb.get("delivery") or {}).get("returnedGoods") or {}
    chk("W13", "A4 tracking returnedGoods followupOrderRef (REQUEST_DEFERRAL)", rg4.get("followupOrderRef"), (s, pj(bb.get("delivery"))))
    s, bb = call("GET", "/orders", T["store-manager"])
    refs = [o["orderRef"] for o in bb.get("items", [])]
    chk("W13", "follow-up order for A3 visible in store-manager's order list", rg.get("followupOrderRef") in refs, (rg.get("followupOrderRef"), refs))
    ob = outbox("event_type='DELIVERY_REJECTED'")
    note("W13", "DELIVERY_REJECTED outbox", ob or "(none)")
    chk("W13", "DELIVERY_REJECTED store notice queued in outbox", "DELIVERY_REJECTED" in ob, "none queued (see W9: outbox check constraint excludes the type)")
    n = psql("select count(*) from delivery.trip_messages where sent_by='system:returned-goods'")
    chk("W13", "dispatcher notice (system:returned-goods trip message) created", n.isdigit() and int(n) >= 1, n)



@stage("Part 3: receipts (W12) + store confirmation")
def part3():
    A = S["A"]
    s, b = call("GET", "/orders/receipts/pending", T["store-manager"])
    items = b.get("items", []) if s == 200 else []
    a1 = [i for i in items if i["order"]["orderRef"] == A["A1"]["orderRef"]]
    chk("W12", "receipts/pending lists A1 with reportBy + reportState", s == 200 and a1 and a1[0].get("reportBy") and a1[0].get("reportState"), (s, pj(b)))
    if a1:
        note("W12", "A1 reportBy/reportState", (a1[0].get("reportBy"), a1[0].get("reportState"), a1[0]["tracking"].get("delivery", {}).get("completedAt")))
    s, b = call("POST", "/orders/%s/receipt/confirm" % A["A1"]["id"], T["store-manager"], {"receivedUnits": 9})
    chk("W12", "store counts 9 of delivered 10 without issue -> 400", s == 400, (s, b))
    s, b = call("POST", "/orders/%s/receipt/confirm" % A["A1"]["id"], T["store-manager"], {"receivedUnits": 10})
    chk("W12", "store confirms A1 receipt 10/10 -> status confirmed", s in (200, 201) and (b.get("receipt") or {}).get("status") == "confirmed", (s, pj(b)))
    s, b = call("POST", "/orders/%s/receipt/confirm" % A["A1"]["id"], T["store-manager"], {"receivedUnits": 10})
    chk("W12", "duplicate confirm is idempotent", s in (200, 201) and (b.get("receipt") or {}).get("status") == "confirmed", (s, pj(b)))
    s, b = call("GET", "/orders/receipts/pending", T["store-manager-b"])
    a2 = [i for i in b.get("items", []) if i["order"]["orderRef"] == A["A2"]["orderRef"]]
    chk("W12", "A2 (driver PARTIAL 8) in store-b pending with reportBy", s == 200 and a2 and a2[0].get("reportBy"), (s, pj(b)))
    s, b = call("POST", "/orders/%s/receipt/confirm" % A["A2"]["id"], T["store-manager-b"], {"receivedUnits": 10})
    chk("W12", "store count 10 != driver's 8 without discrepancy issue -> 400", s == 400, (s, b))
    s, b = call("POST", "/orders/%s/receipt/confirm" % A["A2"]["id"], T["store-manager-b"], {"receivedUnits": 8, "issue": {"issueType": "MISSING", "affectedUnits": 2, "note": "Two cartons short", "idempotencyKey": "e2e-a2-short-" + A["A2"]["id"][:8]}})
    chk("W12", "store reports shortfall with issue -> confirmed_with_issue", s in (200, 201) and (b.get("receipt") or {}).get("status") == "confirmed_with_issue", (s, pj(b)))
    s, b = call("GET", "/orders/receipt-issues", T["dispatcher"])
    chk("W12", "dispatcher sees discrepancy at receipt-issues", s == 200 and A["A2"]["orderRef"] in json.dumps(b), (s, pj(b)))
    s, b = trk("store-manager-b", A["A2"])
    chk("W12", "tracking stage RECEIPT_CONFIRMED_WITH_ISSUE", b.get("stage") == "RECEIPT_CONFIRMED_WITH_ISSUE", (s, b.get("stage")))


@stage("Part 4: D2/D3 W3 deferral + W4 shortfall + W5 blocked checkout")
def part4(D2, D3):
    for o in ("OUT034", "OUT021"):
        s, b = call("GET", "/shared/outlets/%s/notification-preferences" % o, T["dispatcher"])
        ver = (b.get("preferences") or {}).get("version", 0) if s == 200 else 0
        call("PUT", "/shared/outlets/%s/notification-preferences" % o, T["dispatcher"], {"phoneE164": "+9471000%04d" % int(o[3:]), "consentEnabled": True, "deferralsEnabled": True, "majorDelaysEnabled": True, "locale": "en", "version": ver})
    B1 = place("store-manager-b", D2, 5, "ambient", "W3")
    B2 = place("store-manager", D2, 6, "chilled", "W4")
    B3 = place("store-manager-b", D3, 5, "ambient", "W3")
    pid2 = make_plan(D2)
    s, b = call("POST", "/planning/plans/%s/generate" % pid2, T["dispatcher"])
    chk("W3", "generate D2 (2 orders allocated)", s == 200 and b.get("allocated") == 2, (s, b))
    al = [a for a in detail(pid2)["allocations"] if a["orderId"] == B1["id"]]
    s, b = call("DELETE", "/planning/plans/%s/allocations/%s" % (pid2, al[0]["id"]), T["dispatcher"]) if al else (0, "no alloc")
    chk("W3", "dispatcher unassigns B1 before deferring", s in (200, 204), (s, b))
    dp = "/planning/plans/%s/deferrals" % pid2
    nxt = d(date.fromisoformat(D2), 1)
    s, b = call("POST", dp, T["dispatcher"], {"orderId": B1["id"], "reasonCode": "NO_ELIGIBLE_VEHICLE"})
    chk("W3", "deferral without nextRunTarget -> 400", s == 400, (s, b))
    s, b = call("POST", dp, T["dispatcher"], {"orderId": B1["id"], "nextRunTarget": nxt})
    chk("W3", "deferral without reasonCode -> 400", s == 400, (s, b))
    s, b = call("POST", dp, T["dispatcher"], {"orderId": B1["id"], "reasonCode": "NO_ELIGIBLE_VEHICLE", "nextRunTarget": D2})
    chk("W3", "next run not after plan date -> 400", s == 400, (s, b))
    s, b = call("POST", dp, T["driver"], {"orderId": B1["id"], "reasonCode": "NO_ELIGIBLE_VEHICLE", "nextRunTarget": nxt})
    chk("W3", "driver cannot defer -> 403", s == 403, (s, b))
    s, b = call("POST", dp, T["dispatcher"], {"orderId": B1["id"], "reasonCode": "NO_ELIGIBLE_VEHICLE", "comment": "No spare vehicle", "nextRunTarget": nxt})
    chk("W3", "valid deferral (reason + next run) -> 201", s == 201, (s, b))
    rows = outbox("event_type='DEFERRAL' and body like '%%%s%%'" % B1["orderRef"])
    note("W3", "deferral store notice", rows)
    nd = date.fromisoformat(nxt)
    chk("W3", "store notice mentions the reason", "vehicle" in rows.lower() or "NO_ELIGIBLE" in rows, rows)
    chk("W3", "store notice mentions the next-run date", nxt in rows or nd.strftime("%d %b") in rows or nd.strftime("%-d") in rows or nd.strftime("%B") in rows, rows)
    dt2 = detail(pid2)
    chk("W3", "plan detail lists the deferral with nextRunTarget", any(x.get("orderId") == B1["id"] and x.get("nextRunTarget") == nxt for x in dt2.get("deferrals", [])), pj(dt2.get("deferrals")))
    ver = confirm_and_ack(pid2, D2)
    # priority on next plan
    pid3 = make_plan(D3)
    o3 = [o for o in detail(pid3)["orders"] if o["id"] == B3["id"]]
    chk("W3", "next plan flags outlet deferred last run (deferredLastRun)", o3 and o3[0].get("deferredLastRun") is True, pj(o3))
    s, b = call("POST", "/planning/plans/%s/deferrals" % pid3, T["dispatcher"], {"orderId": B3["id"], "reasonCode": "NO_ELIGIBLE_VEHICLE", "nextRunTarget": d(date.fromisoformat(D3), 1)})
    chk("W3", "second consecutive deferral for the outlet -> 201", s == 201, (s, b))
    o3 = [o for o in detail(pid3)["orders"] if o["id"] == B3["id"]]
    chk("W3", "priorityNextPlan=true on the 2nd consecutive deferral", o3 and o3[0].get("priorityNextPlan") is True, pj(o3))

    # W4
    tid, orders = load_trip(D2, load_all=False)
    ln = orders[0]
    iid_s, bb = call("POST", "/loading/trips/%s/orders/%s/issues" % (tid, ln["orderId"]), T["loader"], {"type": "MISSING", "affectedUnits": 2, "note": "2 cartons short"}, headers={"Idempotency-Key": "e2e-short-" + tid})
    chk("W4", "loader raises shortfall issue -> 201", iid_s == 201, (iid_s, bb))
    iid = bb["issue"]["id"]
    s, bb = call("POST", "/loading/trips/%s/ready" % tid, T["loader"], {"chilledTemperatureC": 3, "sealNumber": "WP-E2E-D2"})
    chk("W4", "ready blocked until dispatcher decides (409)", s == 409, (s, bb))
    dec = "/loading/trips/%s/orders/%s/issues/%s/decision" % (tid, ln["orderId"], iid)
    chk("W4", "loader cannot decide -> 403", call("POST", dec, T["loader"], {"decision": "PARTIAL_LOAD"})[0] == 403)
    s, bb = call("POST", dec, T["dispatcher"], {"decision": "PARTIAL_LOAD", "note": "leave on time"})
    chk("W4", "dispatcher decides PARTIAL_LOAD -> 200", s == 200 and bb["issue"]["decision"] == "PARTIAL_LOAD", (s, bb))
    rows = outbox("event_type='LOAD_SHORTFALL' and body like '%%%s%%'" % ln["orderRef"])
    chk("W4", "store gets LOAD_SHORTFALL notice naming order + units short", rows and ln["orderRef"] in rows and "2" in rows, rows)
    note("W4", "notice", rows)
    ready(tid)
    # W5 with shortfall
    s, bb = call("POST", "/delivery/trips/%s/prepare" % tid, T["driver"])
    ids = [st["orderId"] for st in bb.get("stops", [])]
    s, bb = call("POST", "/delivery/trips/%s/checkout" % tid, T["driver"], {"planVersion": 1, "confirmedOrderIds": ids})
    ck = bb.get("checkout", {})
    chk("W5", "checkout with every order on the truck after dispatcher PARTIAL_LOAD approval -> confirmed (departure possible)", s == 200 and ck.get("status") == "confirmed", (s, bb))
    s, bb = call("POST", "/delivery/trips/%s/start" % tid, T["driver"], headers={"Idempotency-Key": "e2e-start-d2-" + tid})
    chk("W5", "driver can depart a dispatcher-approved partial load", s == 200, (s, pj(bb)))
    s, bb = call("GET", "/delivery/trips/%s/checkout" % tid, T["dispatcher"])
    note("W5", "dispatcher checkout view", (s, pj(bb)))


@stage("Part 5: D4 W10 breakdown recovery + W14 audit")
def part5(D4):
    imp = {"version": 1, "sourceSystem": "e2e_w10", "orders": [
        {"externalOrderId": "E2E-W10-AMB-" + D4, "outletId": "OUT006", "brand": "Fresh", "requestedDeliveryDate": D4, "orderUnits": 8, "orderWeightKg": 40, "orderVolumeM3": 0.4, "temperatureRequirement": "ambient"},
        {"externalOrderId": "E2E-W10-CHL-" + D4, "outletId": "OUT019", "brand": "Style", "requestedDeliveryDate": D4, "orderUnits": 8, "orderWeightKg": 40, "orderVolumeM3": 0.4, "temperatureRequirement": "chilled"}]}
    s, b = call("POST", "/orders/import", T["dispatcher"], imp)
    chk("W10", "dispatcher imports an ambient (early window) + chilled (late window) order", s == 200 and json.dumps(b).count("created") >= 1, (s, pj(b)))
    pid = make_plan(D4)
    s, b = call("POST", "/planning/plans/%s/generate" % pid, T["dispatcher"])
    chk("W10", "generate D4", s == 200 and b.get("allocated") == 2, (s, b))
    dt = detail(pid)
    veh = dt["allocations"][0]["vehicleId"] if dt["allocations"] else None
    note("W10", "alloc seq/vehicle", [(a["sequence"], a["vehicleId"]) for a in dt["allocations"]])
    ver = confirm_and_ack(pid, D4)
    s, b = call("GET", "/planning/plans/%s/breakdowns/proposals?vehicleId=%s" % (pid, veh), T["dispatcher"])
    items = b.get("items", []) if s == 200 else []
    stops = items[0]["stops"] if items else []
    chk("W10", "proposals ranked chilled-first (rank1 chilled) and severity critical", s == 200 and stops and stops[0]["chilled"] is True and stops[0]["urgencyRank"] == 1 and b.get("severity") == "critical", (s, pj(b, 900)))
    note("W10", "stop order by seq vs rank", [(x["stopSequence"], x["urgencyRank"], x["chilled"]) for x in stops])
    opts = [o for o in (items[0]["options"] if items else []) if o["valid"]]
    chk("W10", "at least one valid replacement vehicle", opts, items and items[0]["options"][:3])
    if opts:
        s, b = call("POST", "/planning/plans/%s/breakdowns/reassign" % pid, T["dispatcher"], {"sourceVehicleId": veh, "replacementVehicleId": opts[0]["vehicleId"], "tripNumber": items[0]["tripNumber"]})
        chk("W10", "confirm reassign -> 200 confirmed, new plan version", s == 200 and b.get("status") == "confirmed" and b.get("planVersion", 0) >= 2, (s, pj(b)))
        chk("W10", "response vehicleInWorkshop=true", b.get("vehicleInWorkshop") is True, pj(b))
        s, av = call("GET", "/fleet/availability?date=%s" % D4, T["dispatcher"])
        note("W10", "fleet availability", pj(av, 400))
        row = psql("select vehicle_id,date,status,reason from fleet.vehicle_availability where vehicle_id='%s' and date='%s'" % (veh, D4))
        chk("W10", "fleet availability: %s in_workshop on %s" % (veh, D4), "in_workshop" in row, row or "no row")
    s, b = call("GET", "/shared/audit/events?action=BREAKDOWN_RECOVERY_CONFIRMED", T["dispatcher"])
    chk("W10", "audit event BREAKDOWN_RECOVERY_CONFIRMED present", s == 200 and b.get("total", 0) >= 1, (s, pj(b)))


@stage("Part 6: W14 audit export")
def part6():
    s, b, h = call("GET", "/shared/audit/export.csv", T["dispatcher"], raw=True)
    chk("W14", "dispatcher export.csv -> 200 text/csv", s == 200 and "text/csv" in h.get("Content-Type", ""), (s, h.get("Content-Type"), b[:200]))
    note("W14", "export rows/header", (b.splitlines()[0] if b else None, len(b.splitlines()), h.get("X-Export-Total")))
    chk("W14", "csv has expected columns and rows", b.startswith("event_id,timestamp,actor_id") and len(b.splitlines()) > 5, b[:200])
    chk("W14", "driver export -> 403", call("GET", "/shared/audit/export.csv", T["driver"], raw=True)[0] == 403)
    chk("W14", "store-manager export -> 403", call("GET", "/shared/audit/export.csv", T["store-manager"], raw=True)[0] == 403)
    s, b = call("GET", "/shared/audit/events?limit=100", T["dispatcher"])
    actions = {e["action"] for e in b.get("items", [])} if s == 200 else set()
    note("W14", "audit actions seen", sorted(actions))
    for a in ("PLAN_CONFIRMED", "ORDER_DEFERRED", "LOADING_SHORTFALL_DECIDED"):
        chk("W14", "audit contains %s" % a, call("GET", "/shared/audit/events?action=%s" % a, T["dispatcher"])[1].get("total", 0) >= 1, a)


def seed_calendar():
    """shared.operating_calendar is empty in the stock stack; give it Mon-Sat operating days (via PUT /shared/calendar)."""
    n = 0
    day = date(2026, 10, 5)
    while day <= date(2026, 12, 12):
        s, b = call("PUT", "/shared/calendar/%s" % day.isoformat(), T["dispatcher"], {"isOperating": day.weekday() != 6, "expectedVersion": 0})
        n += 1 if s == 200 else 0
        day += timedelta(days=1)
    print("seeded %d calendar days" % n)


def main():
    base = date.fromisoformat(sys.argv[1]) if len(sys.argv) > 1 and not sys.argv[1].startswith("--") else date(2026, 10, 19)
    for u in ("store-manager", "store-manager-b", "dispatcher", "loader", "driver"):
        T[u] = login(u)
    D1, D2, D3, D4 = (d(base, i) for i in (0, 3, 4, 5))  # D1+1 is where W13 follow-up orders land
    if "--seed-calendar" in sys.argv:
        seed_calendar()
    stage_agents()
    part1(D1)
    part2(D1)
    part3()
    part4(D2, D3)
    part5(D4)
    part6()
    print("\n===== SUMMARY")
    for w in sorted(RESULTS):
        ok = sum(1 for r in RESULTS[w] if r[0])
        print("%-4s %d/%d" % (w, ok, len(RESULTS[w])), "; FAILED: " + " | ".join(r[1] for r in RESULTS[w] if not r[0]) if ok < len(RESULTS[w]) else "")


if __name__ == "__main__":
    main()
