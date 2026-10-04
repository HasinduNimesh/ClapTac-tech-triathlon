import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { receiptDiscrepancy } from "../src/store-manager/receiptDiscrepancy.mjs";
import { translate } from "../src/locale.mjs";

test("driver recorded 8 of 10 and the store counts 10: a discrepancy is required", () => {
  const d = receiptDiscrepancy({ ordered: 10, received: 10, driverDelivered: 8 });
  assert.equal(d.short, 0);
  assert.equal(d.differsFromDriver, true);
  assert.equal(d.needsIssue, true);
  assert.equal(d.reason, "driver_mismatch");
  assert.equal(d.affectedUnits, 2);
  assert.equal(d.suggestedIssueType, "QUANTITY_MISMATCH");
});

test("driver recorded 8 of 10 and the store counts 8: still short of the order, issue stays required like the backend", () => {
  const d = receiptDiscrepancy({ ordered: 10, received: 8, driverDelivered: 8 });
  assert.equal(d.differsFromDriver, false);
  assert.equal(d.short, 2);
  assert.equal(d.needsIssue, true);
  assert.equal(d.reason, "shortage");
  assert.equal(d.affectedUnits, 2);
  assert.equal(d.suggestedIssueType, "MISSING");
});

test("driver recorded 10 of 10 and the store counts 9: shortage issue", () => {
  const d = receiptDiscrepancy({ ordered: 10, received: 9, driverDelivered: 10 });
  assert.equal(d.needsIssue, true);
  assert.equal(d.short, 1);
  assert.equal(d.differsFromDriver, true);
  assert.equal(d.reason, "shortage");
  assert.equal(d.affectedUnits, 1);
});

test("driver 10 of 10 and the store counts 10: no issue", () => {
  const d = receiptDiscrepancy({ ordered: 10, received: 10, driverDelivered: 10 });
  assert.equal(d.needsIssue, false);
  assert.equal(d.reason, null);
  assert.equal(d.affectedUnits, 0);
});

test("unknown driver count: only a shortage against the order needs an issue", () => {
  for (const unknown of [undefined, null]) {
    const full = receiptDiscrepancy({ ordered: 10, received: 10, driverDelivered: unknown });
    assert.equal(full.driverKnown, false);
    assert.equal(full.differsFromDriver, false);
    assert.equal(full.needsIssue, false);
    const short = receiptDiscrepancy({ ordered: 10, received: 7, driverDelivered: unknown });
    assert.equal(short.needsIssue, true);
    assert.equal(short.short, 3);
    assert.equal(short.reason, "shortage");
  }
});

test("receipt page uses the shared decision for the payload, issue controls and submit guard", () => {
  const src = readFileSync(new URL("../src/store-manager/ReceiptConfirmPage.tsx", import.meta.url), "utf8");
  assert.match(src, /receiptDiscrepancy\(/);
  assert.match(src, /if \(disc\.needsIssue\) payload\.issue = \{ issueType, affectedUnits: disc\.affectedUnits/);
  assert.match(src, /\{disc\.needsIssue && <>/);
  assert.doesNotMatch(src, /if \(short > 0\) payload\.issue/);
});

test("Sinhala and Tamil translate the driver-mismatch receipt labels", () => {
  for (const key of ["Differs from driver record", "Count differs from the driver's record", "you counted", "Report the difference so the dispatcher can review it before you confirm.", "Confirm receipt & report difference"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});
