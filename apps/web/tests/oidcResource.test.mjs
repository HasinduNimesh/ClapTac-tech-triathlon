import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = (path) => readFileSync(new URL(`../src/auth/${path}`, import.meta.url), "utf8");

test("sign-in asks for a token for the Waypoint API, from the build-time audience", () => {
  assert.match(source("oidc.ts"), /resource: import\.meta\.env\.VITE_OIDC_AUDIENCE \?\? "waypoint-api"/);
  const manager = source("userManager.ts");
  assert.match(manager, /resource: config\.resource/);
  assert.match(manager, /extraTokenParams: \{ resource: config\.resource \}/);
});
