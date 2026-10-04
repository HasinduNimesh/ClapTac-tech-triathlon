import assert from "node:assert/strict";
import test from "node:test";
import { translate } from "../src/locale.mjs";
import { messageLabelKeys, messageStatusLabel, messageTime, messageTypeLabel } from "../src/store-manager/outletMessages.mjs";

test("every kind of notice the system sends has a readable label", () => {
  assert.equal(messageTypeLabel("LOAD_SHORTFALL"), "SHORT LOAD");
  assert.equal(messageTypeLabel("DEFERRAL"), "DEFERRED");
  assert.equal(messageTypeLabel("ARRIVAL_CHANGE"), "ETA CHANGE");
  assert.equal(messageTypeLabel("DELIVERY_REJECTED"), "DELIVERY REFUSED");
  assert.equal(messageTypeLabel("SOMETHING_NEW"), "NOTICE");
});

test("delivery status is worded neutrally: waiting messages are Queued, unsent ones Not sent", () => {
  for (const status of ["PENDING", "SENDING", "QUEUED"]) assert.equal(messageStatusLabel(status), "Queued");
  for (const status of ["FAILED", "UNKNOWN"]) assert.equal(messageStatusLabel(status), "Not sent");
  assert.equal(messageStatusLabel("SENT"), "Sent");
  assert.equal(messageStatusLabel("DELIVERED"), "Delivered");
});

test("a notice stored for a store without text-message consent is labelled as in-app only", () => {
  assert.equal(messageStatusLabel("IN_APP_ONLY"), "In your Notifications only");
  assert.equal(messageStatusLabel("SUPPRESSED"), "In your Notifications only");
  assert.ok(messageLabelKeys.includes("In your Notifications only"));
});

test("times are shown in Sri Lanka time and bad values do not throw", () => {
  assert.match(messageTime("2026-10-04T09:30:00Z"), /4 Oct.*15:00/);
  assert.equal(messageTime("not a date"), "");
});

test("the page wording that covers in-app notices is translated", () => {
  for (const key of ["Notices the system recorded for your store appear here, newest first. Some are also sent as text messages.", "No notices have been recorded for your store yet."]) {
    assert.notEqual(translate("si", key), key);
    assert.notEqual(translate("ta", key), key);
  }
});

test("all message labels have Sinhala and Tamil text", () => {
  for (const key of messageLabelKeys) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});
