import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const page = await readFile(new URL("../src/store-manager/StoreManagerNotificationsPage.tsx", import.meta.url), "utf8");

test("deferral acknowledgements are keyed to the deferral occurrence, not just the order", () => {
  assert.match(page, /deferralKey = \(row: Tracking\) => `\$\{row\.order\.id\}\|\$\{row\.planning\.planRef \?\? ""\}\|\$\{row\.planning\.reasonCode \?\? ""\}`/);
  assert.match(page, /acknowledged\.has\(deferralKey\(row\)\)/);
  assert.doesNotMatch(page, /acknowledged\.has\(row\.order\.id\)/);
});

test("acknowledgements are cleared once an order leaves the deferred stage", () => {
  assert.match(page, /filter\(\(key\) => current\.has\(key\)\)/);
});
