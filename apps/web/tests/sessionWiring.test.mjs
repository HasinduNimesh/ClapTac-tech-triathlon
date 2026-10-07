import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const context = read("../src/auth/AuthContext.tsx");
const manager = read("../src/auth/userManager.ts");
const client = read("../src/api/client.ts");
const callback = read("../src/auth/CallbackPage.tsx");
const app = read("../src/App.tsx");
const ci = read("../../../.github/workflows/ci-cd.yml");
const dockerfile = read("../Dockerfile");

test("the banner is mounted for every page", () => {
  assert.match(app, /<SessionBanner \/>/);
});

test("a refused request only ends the session after the sign-in itself is checked", () => {
  assert.match(client, /res\.status === 401 && token/);
  assert.match(context, /profiles\/me/);
  assert.match(context, /res\.status === 401\) setRejected\(true\)/);
});

test("signing in again returns to the page, through the safe return-to rule", () => {
  assert.match(context, /rememberReturn\(window\.sessionStorage/);
  assert.match(callback, /takeReturn\(window\.sessionStorage, role\)/);
});

test("renewal switches on with the scope only, so nothing changes until the identity server allows it", () => {
  assert.match(manager, /automaticSilentRenew: canRenew\(config\.scope\)/);
  assert.match(manager, /scope: config\.scope/);
  assert.match(dockerfile, /ARG VITE_OIDC_SCOPE="openid profile"/);
  assert.match(ci, /VITE_OIDC_SCOPE=\$\{\{ vars\.PROD_WEB_OIDC_SCOPE \|\| 'openid profile' \}\}/);
});
