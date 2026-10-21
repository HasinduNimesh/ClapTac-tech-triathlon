// Read-only load test for the public surface: web shell, static assets, health, and the
// sign-in gate on the API (an unauthenticated call must be answered 401 quickly).
// It never writes data and never sends credentials.
//
//   docker run --rm -i -v "$PWD/tests/load:/s" -e BASE=https://waypoint.claptac.dev \
//     -e PEAK=15 grafana/k6 run /s/public.js
//
// BASE   target origin (default https://waypoint.claptac.dev)
// PEAK   peak virtual users (default 15; keep it low on the 2-CPU production VM)
import http from "k6/http";
import { check, sleep } from "k6";

const BASE = __ENV.BASE || "https://waypoint.claptac.dev";
const PEAK = Number(__ENV.PEAK || 15);

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 399 }, 401));

export const options = {
  stages: [
    { duration: "1m", target: Math.max(1, Math.round(PEAK / 3)) },
    { duration: "2m", target: Math.max(2, Math.round((PEAK * 2) / 3)) },
    { duration: "3m", target: PEAK },
    { duration: "30s", target: 0 },
  ],
  thresholds: {
    // Stop the run early if the site starts to struggle.
    http_req_failed: [{ threshold: "rate<0.05", abortOnFail: true, delayAbortEval: "30s" }],
    http_req_duration: [{ threshold: "p(95)<3000", abortOnFail: true, delayAbortEval: "60s" }],
    "http_req_duration{page:shell}": ["p(95)<1500"],
    "http_req_duration{page:health}": ["p(95)<1000"],
    "http_req_duration{page:api_gate}": ["p(95)<1500"],
  },
  summaryTrendStats: ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"],
};

export function setup() {
  const html = http.get(`${BASE}/login`).body || "";
  const m = html.match(/assets\/index-[^"]+\.js/);
  return { script: m ? `/${m[0]}` : "/" };
}

export default function (data) {
  check(http.get(`${BASE}/`, { tags: { page: "shell" } }), { "shell 200": (r) => r.status === 200 });
  check(http.get(`${BASE}/login`, { tags: { page: "shell" } }), { "login 200": (r) => r.status === 200 });
  check(http.get(`${BASE}${data.script}`, { tags: { page: "asset" } }), { "asset 200": (r) => r.status === 200 });
  check(http.get(`${BASE}/api/v1/health`, { tags: { page: "health" } }), { "health 200": (r) => r.status === 200 });
  check(http.get(`${BASE}/api/v1/orders`, { tags: { page: "api_gate" } }), { "gate 401": (r) => r.status === 401 });
  sleep(1);
}
