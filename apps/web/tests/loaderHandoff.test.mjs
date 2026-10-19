import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { LOADER_HANDOFF_KEY, markLoaderHandoff } from "../src/loader/handoff.mjs";

test("stores the time under the key the loader app reads", () => {
  const store = new Map();
  const ok = markLoaderHandoff({ setItem: (k, v) => store.set(k, v) }, 1791185889262);
  assert.equal(ok, true);
  assert.equal(LOADER_HANDOFF_KEY, "waypoint.loader.handoff");
  assert.equal(store.get("waypoint.loader.handoff"), "1791185889262");
});

test("blocked storage is not an error: the loader app just shows its own Sign in", () => {
  const blocked = { setItem() { throw new Error("denied"); } };
  assert.equal(markLoaderHandoff(blocked, 1), false);
});

// Every route into the loader app must leave the marker first, or a loader meets a second sign-in. The login
// callback is the main way in, so it is checked by name.
test("the login callback marks the hand-off before it opens the loader app", () => {
  const src = readFileSync(new URL("../src/auth/CallbackPage.tsx", import.meta.url), "utf8");
  const mark = src.indexOf("markLoaderHandoff();");
  const go = src.indexOf("window.location.replace(LOADER_APP_PATH)");
  assert.ok(mark > 0 && go > 0, "both calls must be present");
  assert.ok(mark < go, "the marker must be set before the redirect");
});

test("the redirect page and the home page link mark the hand-off too", () => {
  const redirect = readFileSync(new URL("../src/loader/LoaderAppRedirect.tsx", import.meta.url), "utf8");
  assert.match(redirect, /markLoaderHandoff\(\);\s*window\.location\.replace\(LOADER_APP_PATH\)/);
  const home = readFileSync(new URL("../src/routes/HomePage.tsx", import.meta.url), "utf8");
  assert.match(home, /href=\{workspace\.href\} onClick=\{\(\) => markLoaderHandoff\(\)\}/);
});
