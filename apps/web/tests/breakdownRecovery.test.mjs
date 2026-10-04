import assert from "node:assert/strict";
import test from "node:test";
import { isRecoveryConfirmed, isWorkshopPending, runRecovery } from "../src/dispatcher/breakdownRecovery.mjs";

const pending = () => Object.assign(new Error(JSON.stringify({ status: 502, code: "workshop_pending", detail: "workshop_pending: VEH001" })), { status: 502 });
const confirmed = (planVersion) => ({ status: "confirmed", planVersion, vehicleInWorkshop: true, noticeDrafts: [] });

test("only the stable workshop_pending code on a 502 is treated as a retryable unresolved state", () => {
  assert.equal(isWorkshopPending(pending()), true);
  assert.equal(isWorkshopPending(Object.assign(new Error("{}"), { status: 502 })), false);
  assert.equal(isWorkshopPending(Object.assign(new Error("not json"), { status: 502 })), false);
  assert.equal(isWorkshopPending(Object.assign(new Error(JSON.stringify({ code: "workshop_pending" })), { status: 409 })), false);
  assert.equal(isWorkshopPending(undefined), false);
});

test("a result is confirmed only when the vehicle is also in the workshop", () => {
  assert.equal(isRecoveryConfirmed(confirmed(2)), true);
  assert.equal(isRecoveryConfirmed({ status: "confirmed" }), false);
  assert.equal(isRecoveryConfirmed({ status: "confirmed", vehicleInWorkshop: false }), false);
  assert.equal(isRecoveryConfirmed({ status: "reassignment_saved_plan_not_published", vehicleInWorkshop: true }), false);
  assert.equal(isRecoveryConfirmed(null), false);
});

test("a workshop failure never reports completion and a retry finishes only the missing trip", async () => {
  const trips = [{ tripNumber: 1, replacement: "VEH002" }, { tripNumber: 2, replacement: "VEH003" }];
  const calls = [];
  let tripTwoAttempts = 0;
  const reassign = async (trip) => {
    calls.push(trip.tripNumber);
    if (trip.tripNumber === 2 && ++tripTwoAttempts === 1) throw pending();
    return confirmed(trip.tripNumber + 1);
  };
  const first = await runRecovery(trips, [], reassign);
  assert.equal(first.state, "workshop_pending");
  assert.deepEqual(first.done, [1]);
  assert.equal(first.results.length, 1);
  const retry = await runRecovery(trips, first.done, reassign);
  assert.equal(retry.state, "complete");
  assert.deepEqual(retry.done, [1, 2]);
  assert.deepEqual(calls, [1, 2, 2], "trip 1 must not be re-sent: the server would no longer find it affected");
});

test("other errors and unconfirmed results stop the run as failures, not as workshop retries", async () => {
  const trips = [{ tripNumber: 1, replacement: "VEH002" }];
  const conflict = await runRecovery(trips, [], async () => { throw Object.assign(new Error("conflict"), { status: 409 }); });
  assert.equal(conflict.state, "failed");
  const unconfirmed = await runRecovery(trips, [], async () => ({ status: "confirmed" }));
  assert.equal(unconfirmed.state, "failed");
  assert.deepEqual(unconfirmed.done, []);
});

test("trips without a chosen replacement are skipped", async () => {
  const calls = [];
  const out = await runRecovery([{ tripNumber: 1, replacement: "" }, { tripNumber: 2, replacement: "VEH003" }], [], async (trip) => { calls.push(trip.tripNumber); return confirmed(2); });
  assert.deepEqual(calls, [2]);
  assert.equal(out.state, "complete");
});

test("the recovery drawer drives the reassign through runRecovery, shows the unresolved state with a Retry, and only claims success when complete", async () => {
  const { readFileSync } = await import("node:fs");
  const source = readFileSync(new URL("../src/dispatcher/LiveOperationsPage.tsx", import.meta.url), "utf8").replace(/\r\n/g, "\n");
  assert.match(source, /runRecovery\(trips, doneTrips,/);
  assert.match(source, /data-testid="workshop-pending"/);
  assert.match(source, /workshopPending \? `↻ \$\{t\("Retry workshop update"\)\}`/);
  assert.match(source, /if \(outcome\.state === "complete"\) onDone\(/);
  assert.equal((source.match(/Replacement confirmed · a new plan version was published/g) || []).length, 1);
});
