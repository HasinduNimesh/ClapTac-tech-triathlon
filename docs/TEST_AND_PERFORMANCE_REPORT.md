# Waypoint — Test and Performance Report

Date: 21 October 2026 (Asia/Colombo) · Target: https://waypoint.claptac.dev · Production commit: `main` after PR #160

This report says what has been tested, what the results were, how fast the system answers (p50 / p95 / p99), what is **not** yet tested, and the tests a production-grade release still needs.

## 1. Summary

| Area | Result |
|---|---|
| Automated tests (unit and service integration) | **1,379 tests, 0 failures** (Go 345, web 364, driver app 524 + 19 skipped, loader app 105, assistants 41) |
| Static analysis and security scans | Clean: `flutter analyze` (both apps), `govulncheck` (no vulnerabilities), secret scan, CodeQL (Go, JavaScript, Python). `npm audit`: 2 moderate (react-router, fix is a breaking upgrade) |
| Browser end-to-end on production (4 roles, phone) | Passed on 20 Oct (order → plan → load → shortfall decision → driver delivery with offline sync → receipt → audit). Re-run on 21 Oct: order placement passed; the plan-to-delivery steps wait for a date without a published plan (section 6) |
| Every page, desktop and 375 px | 19 pages × 2 widths: no console errors, no 5xx, no raw JSON; **one** sideways-scroll defect found and fixed (section 7) |
| Forced failures (500 / 409 / offline) | Plain messages on all pages (PR #158) |
| Latency, signed-in users | Dispatcher: p50 232 ms, p95 650 ms, p99 936 ms. Store manager: p50 218 ms, p95 413 ms, p99 553 ms (section 4). **A short burst of HTTP 500 errors occurred during the dispatcher run (section 6, item 1)** |
| Latency, public pages and sign-in gate | p50 ≈ 110 ms, p95 ≈ 200 ms, p99 ≈ 400–480 ms at 12 concurrent users, 0 failed requests |
| Capacity limit | **Not measured.** Production is a 2-CPU VM that is shared with another application; only a gentle load was applied (section 5) |

## 2. Environment and method

- **Production:** one Azure VM, 2 vCPU, 8 GB RAM, Docker Compose; the SMS gateway application runs on the same machine and was already using more than one CPU during the tests (load average 2–3 on 2 CPUs before any load was applied).
- **Test client:** a computer in Colombo. The **network round trip to production is about 100–110 ms** (a trivial authenticated call, `GET /shared/profiles/me`, answers in ~110 ms). Every latency below includes that, so server time is roughly the figure minus 100 ms.
- **Latency of signed-in calls** was measured inside real browser sessions (dispatcher and store manager signed in). Each page was loaded repeatedly and the time of every API call was recorded in the page. No credentials were exported.
- **Public-path load** was generated with k6 (`tests/load/public.js`): read-only requests, no credentials, ramp 4 → 8 → 12 virtual users over 6.5 minutes, with automatic abort if more than 5% of requests failed or p95 exceeded 3 s.
- Percentiles use the nearest-rank method on the raw samples.

## 3. Functional test results

### 3.1 Automated suites (run 21 Oct, clean checkout of `main`)

| Suite | Tests | Result | Notes |
|---|---|---|---|
| Go services and packages (`go test ./... -count=1`) | 345 (58 packages) | Pass | Includes database-backed end-to-end tests for delivery, fleet and loading services |
| Web app (`npm test`, then strict `npm run build`) | 364 | Pass | Build also runs the type check |
| Driver app (Flutter, `flutter test`) | 524 pass, 19 skipped | Pass | `flutter analyze`: no issues |
| Loader app (Flutter) | 105 | Pass | `flutter analyze`: no issues |
| Assistants service (Python) | 41 | Pass | Covers order text reading, product matching, dashboard assistant |
| `govulncheck ./...` | – | No vulnerabilities | |
| `npm audit --omit=dev` | – | 2 moderate | react-router open-redirect and constructor-injection advisories; fixed by a breaking upgrade to v7.18+ — planned, not urgent |

Coverage of Go packages ranges from 100% (depot, fleet service logic) to under 15% (some stores and clients that need a live database). Coverage is not enforced in CI today.

### 3.2 End-to-end workflow on production

| Step | 20 Oct | 21 Oct |
|---|---|---|
| Sign-in, all four roles; role gating | Pass | Pass (dispatcher and store manager re-signed in) |
| Store manager places an order (order helper fills the form, submit, confirmation, list, timeline) | Pass | **Pass** — helper read "basmati rice 2 bales and red dhal 3 bales" correctly and the order was accepted |
| Dispatcher creates, validates and locks a plan | Pass | Not repeated — today's plan is already locked and its trip delivered (section 6) |
| Loader acknowledges, loads, reports a shortfall | Pass | Not repeated |
| Dispatcher decides the shortfall; driver is told | Pass | Not repeated |
| Driver app: load check, deliver, proof, partial delivery, offline save, reconnect and sync | Pass (phone) | Production build re-installed and checked on the phone |
| Store confirms receipt; dispatcher sees the audit trail | Pass | Not repeated |

### 3.3 Interface sweep

- 11 dispatcher pages and 8 store-manager pages, each at 1366 px and 375 px, checked for console errors, 5xx responses, raw JSON or `undefined`/`NaN` text, broken images and sideways scrolling. One defect (store "Create a new dashboard", Send button pushed off screen at 375 px) was found and fixed in this branch.
- Every button on every page was clicked with writes blocked (71 clicks on 20 Oct): nothing harmful happened; the raw-error findings were fixed.
- Failure injection (500, 409, network offline) on all dispatcher and store pages: plain-language messages, no raw JSON.

### 3.4 Assistants and automations (production)

| Item | Result |
|---|---|
| Order helper | Pass after PR #160: "basmati rice" stays basmati, an unknown product becomes a "not in your product list" question, quantities and units are converted in code. Root cause of the earlier outage: the provider rejected a very long product-family enum (HTTP 400) |
| Dispatcher assistant | Pass: fleet count, order deferral reason and plan questions answered with a named source; no tool-call loop |
| Dashboard assistant | Pass |
| Automations | Pass: a routine fired at its minute, recorded its run, showed order numbers, paused, resumed, and deleting asks for confirmation |
| Habit helper, priority reviews | Unit-tested only (need weeks of order history to trigger) |
| SMS | 3 messages sent through the cellular gateway |

## 4. Latency of signed-in calls (production)

Measured from Colombo, includes ~100 ms network round trip. Dispatcher: 78 page loads, 1,221 successful calls in 105 s (≈ 11.6 calls/s). Store manager: 86 page loads, 1,403 calls in 106 s (≈ 13 calls/s).

| Role | Calls | p50 | p95 | p99 |
|---|---|---|---|---|
| Dispatcher, all endpoints | 1,221 | 232 ms | 650 ms | 936 ms |
| Store manager, all endpoints | 1,403 | 218 ms | 413 ms | 553 ms |

### 4.1 Dispatcher, per endpoint (slowest p95 first)

| Endpoint | Calls | p50 | p95 | p99 | Max | Error responses |
|---|---|---|---|---|---|---|
| GET /delivery/trips | 10 | 676 | 1,118 | 1,118 | 1,118 | 0 |
| GET /delivery/trips/:id | 10 | 513 | 960 | 960 | 960 | 0 |
| GET /planning/plans | 105 | 539 | 936 | 1,037 | 1,099 | 32 (**HTTP 500**, one burst; see section 6) |
| GET /loading/trips | 82 | 430 | 885 | 1,115 | 1,115 | 25 (**HTTP 500**, same burst) |
| GET /fleet/incidents | 195 | 268 | 500 | 629 | 650 | 0 |
| GET /fleet/fuel/ledger | 10 | 194 | 392 | 392 | 392 | 0 |
| GET /orders | 98 | 232 | 379 | 468 | 468 | 0 |
| GET /orders/forecast | 20 | 238 | 369 | 383 | 383 | 0 |
| GET /fleet/availability | 29 | 225 | 357 | 393 | 393 | 0 |
| GET /orders/receipt-issues | 97 | 191 | 356 | 556 | 556 | 0 |
| GET /delivery/sync-conflicts | 87 | 168 | 355 | 471 | 471 | 0 |
| GET /loading/alerts | 87 | 164 | 350 | 427 | 427 | 0 |
| GET /shared/audit/events | 96 | 202 | 343 | 379 | 379 | 0 |
| GET /fleet/vehicles | 40 | 170 | 296 | 382 | 382 | 0 |
| GET /shared/outlets | 40 | 233 | 278 | 311 | 311 | 0 |
| GET /shared/automations/habits | 78 | 161 | 273 | 392 | 392 | 0 |
| GET /orders/cutoff | 10 | 164 | 263 | 263 | 263 | 0 |
| GET /shared/profiles/me | 78 | 111 | 142 | 308 | 308 | 0 |

(Other endpoints, with 9–10 calls each, answered at p95 below 320 ms.) All values in milliseconds. Latency percentiles are for successful calls only. **The run also hit 500 responses** on `GET /planning/plans`, `GET /loading/trips` and the planning service's internal trip calls — about 94 in a burst lasting under a minute (Prometheus: 94 server errors in that 5-minute window, none since); see section 6, item 1.

### 4.2 Store manager, per endpoint

| Endpoint | Calls | p50 | p95 | p99 | Max |
|---|---|---|---|---|---|
| GET /orders/:id/tracking | 860 | 277 | 445 | 861 | 1,174 |
| GET /agent/assistants/status | 14 | 201 | 499 | 499 | 499 |
| GET /shared/outlets/:id/notifications | 100 | 137 | 289 | 391 | 448 |
| GET /orders | 128 | 142 | 286 | 340 | 460 |
| GET /shared/products | 14 | 160 | 277 | 277 | 277 |
| GET /orders/cutoff | 44 | 147 | 251 | 340 | 340 |
| GET /shared/dashboards | 15 | 148 | 251 | 251 | 251 |
| GET /shared/automations/habits | 86 | 127 | 239 | 255 | 255 |
| GET /shared/profiles/me | 86 | 108 | 202 | 586 | 586 |
| GET /shared/automations/inbox | 28 | 123 | 182 | 255 | 255 |

No error responses and no 5xx.

### 4.3 Public pages and the sign-in gate (k6, 12 virtual users)

Run length 6 min 30 s, 683 iterations, 3,416 requests (8.7 per second), **0 failed requests**, all 3,415 checks passed.

| Request | p50 | p95 | p99 | Max |
|---|---|---|---|---|
| Web shell (`/`, `/login`) | 110 ms | 216 ms | 480 ms | 998 ms |
| Health (`/api/v1/health`) | 109 ms | 205 ms | 385 ms | 1.2 s |
| API sign-in gate (unauthenticated call answered 401) | 110 ms | 195 ms | 411 ms | 1.6 s |
| All requests together, including the 840 KB JavaScript bundle (214 KB compressed) | 114 ms | 2.88 s | 4.96 s | 15.3 s |

The "all requests" tail comes from downloading the main JavaScript bundle on every iteration at ~1.5 MB/s of test-client bandwidth, which is a first-visit cost, not a per-request cost (browsers keep the file). It is reported so the figure is not mistaken for API latency.

### 4.4 Reading the numbers

- Against a proposed service target of **p95 under 1 s and p99 under 2 s for read calls** (measured from a Colombo client), every call meets p99, and every call meets p95 except `GET /delivery/trips` (1.12 s) which is at the line, and `GET /planning/plans` (0.94 s) which is close.
- The slowest calls are the ones that join several services: delivery trips, plans, loading trips.
- Each dispatcher page fires about eight background calls (incidents, plans, audit events, loading trips, habits, receipt issues, alerts, sync conflicts) in addition to its own data. The store-manager order list makes one tracking call per order (860 calls in the sample). Combining these would cut load more than any server tuning.
- These are results at **light load** (one signed-in browser per role plus the public-page test). They show the system is responsive for normal use. They are **not** a statement of how many users it can carry.

## 5. What the load test did and did not cover

Done: sustained read traffic on the public surface (12 users, 6.5 minutes) and signed-in read traffic from one dispatcher and one store-manager browser for ~105 s each (about 12 calls per second each, run one after the other), with no errors from the store manager and a short burst of 500 errors on the dispatcher side (section 6, item 1).

Not done on purpose: the production VM has two CPUs, is shared with another application and was already near its CPU limit, and the product is in live use. A stress test, spike test or soak test would risk an outage. Those tests need a separate staging copy of the stack (section 8).

Run it again:

```
docker run --rm -i -v "$PWD/tests/load:/s:ro" -e BASE=https://waypoint.claptac.dev -e PEAK=12 grafana/k6 run /s/public.js
```

Keep `PEAK` low against production. The script aborts itself when failures exceed 5% or p95 exceeds 3 s.

## 6. Known gaps found during this run

1. **A burst of HTTP 500 errors, cause unknown.** At 09:58 (Colombo) during the dispatcher latency run, `GET /planning/plans`, `GET /loading/trips` and the planning service's internal trip calls answered 500 for about a minute (94 responses across the services in Prometheus; none before or since, and the same pages answered 200 immediately afterwards). The failures returned in 10–70 ms, so they were not timeouts, and the database log shows nothing. The services logged the status but **not the reason**, so the cause could not be found. This needs: (a) logging the underlying error whenever a request ends in 5xx, and (b) a repeat of the run in staging while watching the new "Server errors" panel. Until then, treat the planning/loading read path as not proven under rapid repeated page loads. (An earlier version of this report called these responses expected empty-state 4xx; that was wrong. A date with no plan correctly answers 404.)
2. **"Revise published plan" has no confirmation.** It reopens a locked plan with one click. The plan can then be reset and generated again, and a reset deletes the plan's trips and allocations; nothing checks whether a trip has already run. It should ask for confirmation, and a reset should be refused once a trip has started. For this reason the plan-to-delivery steps were not repeated for a date that already has a delivered trip; they will be run on a date without a plan.
3. **No order cancellation** in the product; a mistaken order can only be deferred by the dispatcher.
4. **Hashed JavaScript files have no long-term cache header** (only ETag and Last-Modified), so a returning browser revalidates every file. Adding `Cache-Control: public, max-age=31536000, immutable` for `/assets/` is a one-line change in the web server configuration.
5. Fractional order quantities (2.5) are silently rounded down; error sentences are English only; the driver app shows "5 units on board" after a 4-unit partial load.
6. Two moderate npm advisories (react-router) await a planned major upgrade.

## 7. Defects found and fixed in this campaign (all verified on production unless noted)

Offline-queue stall from a stale trip (#151, #154); deploy preflight problems (#150, #152, #153); dispatcher decision not reaching the driver before a run exists, shortfalls on later-date trips, decided shortfalls still shown as "needs action", plan lockable for a passed window, loader app not refreshing (#156, #157); duplicate order-tracking calls and Redis rate limits and caches (#155); raw server errors on ~20 pages, raw JSON on Audit, Sign-out off screen on phones, stale product list in the order helper, dispatcher assistant null tool schema, habit helper looking for the wrong event name, internal ids in automation results, delete without confirmation (#158); phone-width overflow on Master data and Audit, ISO times shown raw in audit details, dispatcher assistant tool loop (#159); order helper rejected by the model provider (#160); store "Create a new dashboard" sideways scroll (this branch).

## 8. Tests a production-grade release still needs

| Test type | Status | What is needed |
|---|---|---|
| Unit and service integration | **Have** | Raise coverage on stores/clients; enforce a floor in CI |
| Contract tests against the OpenAPI files in `contracts/` | Gap | Generate requests from the specification and fail CI on drift |
| Database migration tests | Gap | Apply all migrations to an empty database and to a copy of last release; check role grants |
| Automated browser end-to-end in CI | Gap | The workflow in `tools/playwright-mcp/WORKFLOWS.md` is run by an agent on demand; turn it into a Playwright test run against a staging stack on every release |
| Device end-to-end (driver app on a phone) | Manual | Keep a device checklist; add an emulator run in CI |
| Role × endpoint authorisation matrix | Gap | A table-driven test that every endpoint answers 401/403 for the wrong role and wrong outlet |
| Load: baseline, stress, spike, soak | **Partly** (baseline, public) | Staging stack with seeded data; ramp to failure, a 4 PM order-cutoff spike (all stores ordering at once), a 2-hour soak; record p50/p95/p99 and CPU/memory |
| Write-path performance | Gap | Order submission, plan generation and driver sync at volume |
| Resilience / chaos | Gap | Stop one service, stop Redis (fail-open is unit-tested), stop the model provider; confirm degraded but working screens |
| Backup and restore drill | Gap | Restore last night's database into staging and run the smoke test |
| Security: dependency, secret and code scans | **Have** (govulncheck, secret scan, CodeQL, Dependabot) | Add container image scan |
| Security: dynamic scan and external penetration test | Gap | OWASP ZAP baseline on staging; third-party test before launch |
| Accessibility | Partial (skip link, languages) | Automated axe checks per page; keyboard and screen-reader pass |
| Language / layout (Sinhala, Tamil) | Partial | Screenshot comparison per language and per width |
| Assistant quality (model) | Partial | A fixed set of 100+ order phrasings with expected results, run on every prompt or model change; prompt-injection cases |
| Offline and sync (driver) | **Have** (unit and phone run) | Soak: 8 hours with repeated connection loss |
| Observability | Partial | Alert rules tested by forcing a failure (service down, error rate, queue stall) |

## 9. Sign-off checklist for a release

- [ ] All automated suites pass (section 3.1) on the release commit
- [ ] Security scans clean or each finding accepted in writing
- [ ] Staging: end-to-end workflow passes for all four roles and the phone
- [ ] Staging: load baseline within the targets (p95 < 1 s, p99 < 2 s for reads; < 1% errors)
- [ ] Backup restore drill within the last 30 days
- [ ] Rollback tested (previous images still available)
