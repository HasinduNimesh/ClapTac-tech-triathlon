import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), "utf8");

test("a loader who signs in lands in the loader app, not the old loader pages", () => {
  const callback = source("auth/CallbackPage.tsx");
  assert.match(callback, /role === "LOADER"\) \{[^}]*window\.location\.replace\(LOADER_APP_PATH\)/s);
  assert.doesNotMatch(callback, /"\/loader\/loading"/);
  assert.match(source("loader/LoaderAppRedirect.tsx"), /LOADER_APP_PATH = "\/loader-app\/"/);
});

test("the loader's navigation link and the old loader routes lead to the loader app", () => {
  assert.match(source("components/Layout.tsx"), /role === "LOADER" && <a href="\/loader-app\/">/);
  const app = source("App.tsx");
  assert.match(app, /path="\/loader" element=\{<RoleGate role="LOADER"><LoaderAppRedirect \/>/);
  assert.match(app, /path="\/loader\/loading" element=\{<RoleGate role="LOADER"><LoaderAppRedirect \/>/);
});

test("the service worker does not answer /loader-app/ with this app's page", () => {
  const config = readFileSync(new URL("../vite.config.ts", import.meta.url), "utf8");
  assert.match(config, /navigateFallbackDenylist: \[[^\]]*\/\^\\\/loader-app\//);
});
