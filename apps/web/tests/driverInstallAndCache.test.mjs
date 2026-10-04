import assert from "node:assert/strict";
import test from "node:test";
import { installCardState, manualInstallSteps } from "../src/driver/installPrompt.mjs";
import { otherDayTripIds } from "../src/offline/driverDataPrivacy.mjs";

test("the install card shows once, never inside the installed app", () => {
  assert.equal(installCardState({ canPrompt: true }), "prompt");
  assert.equal(installCardState({}), "manual");
  assert.equal(installCardState({ dismissed: true, canPrompt: true }), "hidden");
  assert.equal(installCardState({ standalone: true }), "hidden");
  assert.match(manualInstallSteps("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)"), /Share/);
  assert.match(manualInstallSteps("Mozilla/5.0 (Linux; Android 14)"), /browser menu/);
});

test("only today's route stays on the device; unsent work and undated trips are kept", () => {
  const trips = [
    { tripId: "today", deliveryDate: "2026-10-04" },
    { tripId: "yesterday", deliveryDate: "2026-10-03" },
    { tripId: "yesterday-unsent", deliveryDate: "2026-10-03" },
    { tripId: "undated" },
  ];
  const details = [{ tripId: "old-detail-only", run: { deliveryDate: "2026-10-01" } }];
  assert.deepEqual(otherDayTripIds({ trips, details, queuedTripIds: ["yesterday-unsent"], today: "2026-10-04" }), ["old-detail-only", "yesterday"]);
});
