import test from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { translate } from "../src/locale.mjs";

test("Sinhala and Tamil navigation strings render and English remains fallback", () => {
  assert.equal(translate("si", "My route"), "මගේ මාර්ගය");
  assert.equal(translate("ta", "My route"), "எனது வழித்தடம்");
  assert.equal(translate("si", "New unmapped screen label"), "New unmapped screen label");
  assert.equal(translate("en", "Forecast"), "Forecast");
});

test("Sinhala and Tamil provide translated driver delivery actions and recovery controls", () => {
  for (const key of ["Driver", "Sync Now", "Acknowledge current plan", "Arrive", "Delivered", "Partial", "Failed", "Refused", "Not delivered", "End-of-day summary"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide loader workflow, shortfall, and scan fallback labels", () => {
  for (const key of ["Loader", "Start loading", "Barcode or QR verification", "Record shortfall", "Ready for departure", "Camera access is unavailable. Use the manual order-reference field or the Loaded button."]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide store order capture and order-list labels", () => {
  for (const key of ["Store Manager", "Create an order", "New Order", "Requested Delivery Date", "Temperature", "Submit Order", "Pending receipts"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide delivery tracking and receipt confirmation labels", () => {
  for (const key of ["Receipt confirmation", "Order tracking", "Received units", "Confirm Receipt", "Report Issue & Confirm Receipt", "Planned arrival", "No orders found for this outlet."]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide dispatcher order queue and import/export labels", () => {
  for (const key of ["Dispatcher", "Confirmed Orders", "Export CSV v1", "Import CSV v1", "Source system", "Filter by brand", "exact replays skipped."]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide planning, fuel, fairness, and breakdown controls", () => {
  for (const key of ["Daily Plan", "Generate", "Fairness priority signals", "Record actual fuel use", "Check replacement feasibility", "Confirm replacement and publish version", "Assign", "Order to assign", "Order to defer", "Select unallocated order", "Defer"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide audit search, KPI, and paging labels", () => {
  for (const key of ["Audit history and operational KPIs", "Plans generated", "Search audit events", "Resource ID", "Resource type", "Actor ID", "From (Sri Lanka time)", "To (Sri Lanka time)", "Sync conflicts", "Previous", "Next"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide master-data, fleet, policy, and calendar form labels", () => {
  for (const key of ["Outlet access and delivery windows", "Fleet capabilities and weekly quotas", "Save vehicle changes", "Vehicle incidents and breakdowns", "Planning policy", "Preview policy", "Operating calendar", "Save calendar"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide live operations, ETA, and trip messaging labels", () => {
  for (const key of ["Delivery status", "Estimated arrival / window risk", "Trip messages and receipts", "Send message", "Awaiting driver acknowledgement", "No driver update yet"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide dispatcher loading status headings and count labels", () => {
  for (const key of ["Loading status", "Loaded / short / pending", "Suggested", "Issues"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil translate delivery states, outcome codes, and discrepancy codes", () => {
  for (const key of ["in_progress", "arrived", "DELIVERED", "NOT_DELIVERED", "QUANTITY_MISMATCH", "OUTLET_CLOSED"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide master-data save, conflict, and incident notice phrases", () => {
  for (const key of ["saved as version", "change recorded in audit history.", "This outlet changed in another session. Reload the latest version before editing.", "This planning policy changed in another session. Reload the latest version before saving.", "Incident saved.", "review replacement feasibility before confirming changes."]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide forecast assumptions, empty states, and pressure labels", () => {
  for (const key of ["Planning estimates only. Demand uses the previous four complete weeks; sparse history may understate future demand. Confirmed orders and plan constraints remain authoritative.", "No confirmed order history is available for an estimate.", "Estimated weight", "Volume capacity", "configured_allowance_mean_v1"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("Sinhala and Tamil provide ETA risk and confidence labels", () => {
  for (const key of ["No planned ETA", "Window missed", "Window at risk", "Watch window", "ETA passed", "Low · event-based, no GPS"]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("every literal workflow translation key has Sinhala and Tamil text", () => {
  const root = fileURLToPath(new URL("../src/", import.meta.url));
  const files = [];
  function visit(directory) {
    for (const name of readdirSync(directory)) {
      const path = join(directory, name);
      if (statSync(path).isDirectory()) visit(path);
      else if (path.endsWith(".tsx")) files.push(path);
    }
  }
  visit(root);

  const keys = new Set();
  const literalCall = /\bt\(\s*"((?:\\.|[^"\\])*)"/g;
  for (const path of files) {
    let match;
    const source = readFileSync(path, "utf8");
    while ((match = literalCall.exec(source))) keys.add(JSON.parse(`"${match[1]}"`));
  }

  // Language names are proper nouns and intentionally remain self-named.
  keys.delete("English");
  for (const key of keys) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("workflow JSX has no untranslated English text nodes", () => {
  const root = fileURLToPath(new URL("../src/", import.meta.url));
  const files = [];
  function visit(directory) {
    for (const name of readdirSync(directory)) {
      const path = join(directory, name);
      if (statSync(path).isDirectory()) visit(path);
      else if (path.endsWith(".tsx")) files.push(path);
    }
  }
  visit(root);

  const untranslated = [];
  for (const path of files) {
    const source = ts.createSourceFile(path, readFileSync(path, "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
    function inspect(node) {
      if (ts.isJsxText(node)) {
        const text = node.text.trim();
        // Product name and SI unit symbol are intentionally language-neutral.
        if (/[A-Za-z]{2}/.test(text) && !["Waypoint", "kg"].includes(text)) {
          untranslated.push(`${path}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1}: ${JSON.stringify(text)}`);
        }
      }
      ts.forEachChild(node, inspect);
    }
    inspect(source);
  }
  assert.deepEqual(untranslated, [], untranslated.join("\n"));
});

test("offline recovery banners translate exact messages and queue counts", () => {
  assert.match(translate("si", "Offline · 3 queued"), /3/);
  assert.match(translate("ta", "Syncing 2 saved item(s)…"), /2/);
  assert.notEqual(translate("si", "No connection · 4 item(s) saved on this device"), "No connection · 4 item(s) saved on this device");
  assert.notEqual(translate("ta", "3 upload(s)/event(s) pending on this device"), "3 upload(s)/event(s) pending on this device");
  assert.equal(translate("en", "Offline · 3 queued"), "Offline · 3 queued");
});

test("the W15 estimate fallback banner is translated", async () => {
  const { ESTIMATES_UNAVAILABLE_MESSAGE } = await import("../src/api/estimateAvailability.mjs");
  for (const locale of ["si", "ta"]) assert.notEqual(translate(locale, ESTIMATES_UNAVAILABLE_MESSAGE), ESTIMATES_UNAVAILABLE_MESSAGE, `missing ${locale} translation for the estimate fallback banner`);
});
