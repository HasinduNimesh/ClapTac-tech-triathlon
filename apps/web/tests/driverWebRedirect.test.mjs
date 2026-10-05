import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (p) => readFileSync(new URL(`../src/${p}`, import.meta.url), "utf8");

// Drivers work in the Android app. The web driver screens are switched off: both driver paths must show the
// download page and must not reach the old trips screen.
test("/driver and /driver/trips both render the download page", () => {
  const app = read("App.tsx");
  assert.match(app, /path="\/driver" element=\{<RoleGate role="DRIVER"><DriverAppPage \/><\/RoleGate>\}/);
  assert.match(app, /path="\/driver\/trips" element=\{<RoleGate role="DRIVER"><DriverAppPage \/><\/RoleGate>\}/);
});

test("the old driver screens are not imported by the app any more", () => {
  const app = read("App.tsx");
  assert.doesNotMatch(app, /DriverTripsPage/);
  assert.doesNotMatch(app, /driver\/DriverPage/);
});

test("the page offers the published APK and says so when there is none", () => {
  const page = read("driver/DriverAppPage.tsx");
  assert.match(page, /href="\/downloads\/waypoint-driver\.apk"/);
  assert.match(page, /The Android app has not been published yet/);
  assert.match(page, /useDriverApk\(\)/);
});

test("the home page and the driver page share one APK lookup", () => {
  assert.match(read("routes/HomePage.tsx"), /useDriverApk\(\)/);
  assert.match(read("routes/useDriverApk.ts"), /\/downloads\/waypoint-driver\.json/);
});
