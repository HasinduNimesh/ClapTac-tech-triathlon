import test from "node:test";
import assert from "node:assert/strict";
import { deferralExplanation, deferralMessages } from "../src/store-manager/deferralMessage.mjs";
import { translate } from "../src/locale.mjs";

const plannerCodes = ["NO_ELIGIBLE_VEHICLE", "WEIGHT_CAPACITY_EXCEEDED", "VOLUME_CAPACITY_EXCEEDED", "REFRIGERATION_REQUIRED", "VAN_REQUIRED", "DEPOT_MISMATCH", "DELIVERY_WINDOW_CONFLICT", "FUEL_QUOTA_EXCEEDED", "TRIP_LIMIT_REACHED", "VEHICLE_UNAVAILABLE", "MANUAL_DISPATCHER_DEFERRAL"];

test("every planner reason code has a business-language message and next action", () => {
  for (const code of plannerCodes) {
    const entry = deferralMessages[code];
    assert.ok(entry?.message && entry?.nextAction, `missing explanation for ${code}`);
    assert.doesNotMatch(entry.message, /[A-Z]{3,}_[A-Z]{3,}/, "no raw solver code in the message");
  }
});

test("unknown or empty reason codes fall back to a safe generic message", () => {
  assert.match(deferralExplanation("SOMETHING_NEW").message, /could not be delivered/);
  assert.match(deferralExplanation(undefined).nextAction, /dispatch/i);
  assert.equal(deferralExplanation("refrigeration_required"), deferralMessages.REFRIGERATION_REQUIRED);
});

test("deferral explanations are translated to Sinhala and Tamil", () => {
  for (const entry of [...Object.values(deferralMessages), deferralExplanation("x")]) {
    for (const text of [entry.message, entry.nextAction]) {
      assert.notEqual(translate("si", text), text, `missing Sinhala for ${text}`);
      assert.notEqual(translate("ta", text), text, `missing Tamil for ${text}`);
    }
  }
  assert.notEqual(translate("si", "What happens next"), "What happens next");
});
