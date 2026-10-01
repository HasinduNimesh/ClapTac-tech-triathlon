#!/usr/bin/env python3
"""Small repeatable authenticated API load check for the local Waypoint stack."""

import argparse
import base64
import concurrent.futures
import hashlib
import json
import os
import queue
import secrets
import statistics
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, response, code, message, headers, new_url):
        return None


def request_token(args):
    username = os.environ.get("WAYPOINT_OIDC_USER", "")
    password = os.environ.get("WAYPOINT_OIDC_PASSWORD", "")
    if not username or not password:
        raise SystemExit("Set WAYPOINT_OIDC_USER and WAYPOINT_OIDC_PASSWORD in the environment.")

    verifier = secrets.token_urlsafe(48)
    challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()
    state = secrets.token_urlsafe(18)
    redirect_uri = args.redirect_uri
    authorize_url = args.oidc_url.rstrip("/") + "/oauth2/authorize?" + urllib.parse.urlencode({
        "response_type": "code",
        "client_id": args.client_id,
        "redirect_uri": redirect_uri,
        "state": state,
        "code_challenge": challenge,
        "code_challenge_method": "S256",
    })
    opener = urllib.request.build_opener(NoRedirect)
    opener.open(authorize_url, timeout=5).close()

    form = urllib.parse.urlencode({
        "username": username,
        "password": password,
        "redirect_uri": redirect_uri,
        "state": state,
        "code_challenge": challenge,
        "client_id": args.client_id,
    }).encode()
    try:
        opener.open(urllib.request.Request(
            args.oidc_url.rstrip("/") + "/oauth2/authorize",
            data=form,
            headers={"Content-Type": "application/x-www-form-urlencoded"},
        ), timeout=5)
    except urllib.error.HTTPError as response:
        if response.code not in (301, 302, 303, 307, 308):
            raise SystemExit(f"Local OIDC authorization failed with HTTP {response.code}.")
        location = response.headers.get("Location", "")
    else:
        raise SystemExit("Local OIDC did not return an authorization redirect.")

    params = urllib.parse.parse_qs(urllib.parse.urlparse(location).query)
    if params.get("state", [""])[0] != state or not params.get("code"):
        raise SystemExit("Local OIDC returned an invalid authorization response.")
    token_form = urllib.parse.urlencode({
        "grant_type": "authorization_code",
        "client_id": args.client_id,
        "redirect_uri": redirect_uri,
        "code": params["code"][0],
        "code_verifier": verifier,
    }).encode()
    request = urllib.request.Request(
        args.oidc_url.rstrip("/") + "/oauth2/token",
        data=token_form,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
    )
    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            token = json.loads(response.read())["access_token"]
    except (urllib.error.URLError, KeyError, json.JSONDecodeError) as error:
        raise SystemExit(f"Local OIDC token exchange failed ({type(error).__name__}).") from None
    return token


def percentile(values, fraction):
    if not values:
        return 0.0
    ordered = sorted(values)
    index = max(0, min(len(ordered) - 1, int((len(ordered) - 1) * fraction + 0.999999)))
    return ordered[index]


def check_statuses(statuses, expected_status, additional_allowed=(), required=()):
    """Return unexpected and missing HTTP statuses for a bounded load run."""
    allowed = {expected_status, *additional_allowed}
    observed = set(statuses)
    unexpected = sum(status not in allowed for status in statuses)
    missing = set(required) - observed
    return unexpected, missing


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://localhost/api/v1/orders/forecast")
    parser.add_argument("--oidc-url", default="http://localhost:8090")
    parser.add_argument("--client-id", default="waypoint-web")
    parser.add_argument("--redirect-uri", default="http://localhost/auth/callback")
    parser.add_argument("--concurrency", type=int, default=10)
    parser.add_argument("--seconds", type=int, default=30)
    parser.add_argument("--rate-per-second", type=float, default=10.0,
                        help="open-loop request cap; keep below the local NGINX 20 r/s edge limit")
    parser.add_argument("--expected-status", type=int, default=200)
    parser.add_argument("--allow-status", type=int, action="append", default=[],
                        help="additional HTTP status accepted for this scenario (repeatable)")
    parser.add_argument("--require-status", type=int, action="append", default=[],
                        help="require this HTTP status to appear at least once (repeatable)")
    parser.add_argument("--max-p95-ms", type=float, default=500.0,
                        help="provisional local budget, not a production SLA")
    parser.add_argument("--max-error-percent", type=float, default=0.5)
    args = parser.parse_args()
    if args.concurrency < 1 or args.seconds < 1 or args.rate_per_second <= 0:
        parser.error("concurrency, seconds, and rate-per-second must be positive")

    token = request_token(args)
    results = queue.SimpleQueue()
    started = time.monotonic()
    deadline = started + args.seconds
    next_start = [started]
    schedule_lock = threading.Lock()

    def worker():
        request = urllib.request.Request(args.url, headers={"Authorization": "Bearer " + token})
        while True:
            with schedule_lock:
                scheduled = max(next_start[0], time.monotonic())
                if scheduled >= deadline:
                    return
                next_start[0] = scheduled + 1 / args.rate_per_second
            delay = scheduled - time.monotonic()
            if delay > 0:
                time.sleep(delay)
            if time.monotonic() >= deadline:
                return
            started = time.perf_counter()
            try:
                with urllib.request.urlopen(request, timeout=10) as response:
                    response.read(65536)
                    status = response.status
            except urllib.error.HTTPError as response:
                status = response.code
                response.close()
            except Exception as error:  # Include timeouts/connection failures in the error rate.
                results.put((time.perf_counter() - started, None, type(error).__name__))
                continue
            results.put((time.perf_counter() - started, status, ""))

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as executor:
        futures = [executor.submit(worker) for _ in range(args.concurrency)]
        for future in futures:
            future.result()
    elapsed = time.monotonic() - started

    latencies = []
    status_counts = {}
    transport_errors = {}
    observed_statuses = []
    transport_error_count = 0
    while not results.empty():
        latency, status, error = results.get()
        latencies.append(latency * 1000)
        if status is None:
            transport_error_count += 1
            transport_errors[error] = transport_errors.get(error, 0) + 1
        else:
            observed_statuses.append(status)
            status_counts[status] = status_counts.get(status, 0) + 1

    count = len(latencies)
    status_errors, missing_statuses = check_statuses(
        observed_statuses,
        args.expected_status,
        args.allow_status,
        args.require_status,
    )
    # Transport failures and unapproved HTTP statuses count against the run.
    errors = transport_error_count + status_errors
    error_percent = errors * 100 / count if count else 100.0
    p95 = percentile(latencies, 0.95)
    print(f"requests={count} elapsed_s={elapsed:.1f} concurrency={args.concurrency} target_rps={args.rate_per_second:.1f} actual_rps={count / elapsed:.1f}")
    print(f"latency_ms p50={statistics.median(latencies) if latencies else 0:.1f} p95={p95:.1f} p99={percentile(latencies, .99):.1f}")
    accepted_statuses = sorted({args.expected_status, *args.allow_status})
    print(f"errors={errors} error_percent={error_percent:.2f} accepted_statuses={accepted_statuses} required_statuses={sorted(args.require_status)} missing_required_statuses={sorted(missing_statuses)} statuses={status_counts} transport_errors={transport_errors}")
    if not count or p95 > args.max_p95_ms or error_percent > args.max_error_percent or missing_statuses:
        print("RESULT=FAIL (provisional local budget; not a production capacity claim)")
        raise SystemExit(1)
    print("RESULT=PASS (provisional local budget; not a production capacity claim)")


if __name__ == "__main__":
    main()
