import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const app = await readFile(new URL("../src/App.tsx", import.meta.url), "utf8");

test("unknown URLs render the not-found page, in the workspace for store-manager paths", () => {
  assert.match(app, /path="\/store-manager\/\*" element=\{<NotFoundPage inWorkspace \/>\}/);
  assert.match(app, /path="\*" element=\{<NotFoundPage \/>\}/);
});

test("the store-manager catch-all comes after the specific workspace routes", () => {
  assert.ok(app.indexOf('path="/store-manager/notifications"') < app.indexOf('path="/store-manager/*"'));
});
