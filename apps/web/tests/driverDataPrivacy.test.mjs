import assert from "node:assert/strict";
import test from "node:test";
import { clearableCompletedTripIds, createDriverDataExport } from "../src/offline/driverDataPrivacy.mjs";

test("driver export keeps queued proof bytes and excludes authentication secrets", async () => {
  const payload = await createDriverDataExport({
    subject: "driver-1",
    trips: [{ tripId: "trip-1" }],
    details: [{ tripId: "trip-1", stops: [] }],
    queue: [{ operationId: "op-1", type: "PROOF_UPLOAD", tripId: "trip-1", createdAt: "2026-09-30T08:00:00Z", payload: { proofType: "signature", accessToken: "never-export-this" }, blob: new Blob(["proof"], { type: "image/jpeg" }) }],
  }, new Date("2026-09-30T09:00:00Z"));
  const exported = JSON.parse(payload);
  assert.equal(exported.format, "waypoint-driver-offline-export-v1");
  assert.equal(exported.subject, "driver-1");
  assert.equal(exported.pendingOperations[0].proofBlob.mimeType, "image/jpeg");
  assert.equal(exported.pendingOperations[0].proofBlob.dataUrl, "data:image/jpeg;base64,cHJvb2Y=");
  assert.equal("accessToken" in exported, false);
  assert.equal("accessToken" in exported.pendingOperations[0], false);
  assert.equal("accessToken" in exported.pendingOperations[0].payload, false);
});

test("driver export requires an authenticated subject", async () => {
  await assert.rejects(() => createDriverDataExport({ subject: "", trips: [], details: [], queue: [] }));
});

test("cache clear preserves queued work and active or unconfirmed trips", () => {
  const markers = [
    { key: "serverCompletedAt:trip%2F1", value: "2026-09-01T00:00:00Z" },
    { key: "serverCompletedAt:trip-2", value: "2026-09-01T00:00:00Z" },
    { key: "serverCompletedAt:trip-3", value: "2026-09-01T00:00:00Z" },
  ];
  const details = [
    { tripId: "trip/1", run: { status: "completed", completedAt: "2026-09-01T00:00:00Z" } },
    { tripId: "trip-2", run: { status: "in_progress", completedAt: "2026-09-01T00:00:00Z" } },
    { tripId: "trip-3", run: { status: "completed", completedAt: "different-server-time" } },
  ];
  assert.deepEqual(clearableCompletedTripIds({ pendingQueueCount: 1, markers, details }), []);
  assert.deepEqual(clearableCompletedTripIds({ pendingQueueCount: 0, markers, details }), ["trip/1"]);
  assert.deepEqual(clearableCompletedTripIds({ pendingQueueCount: Number.NaN, markers, details }), []);
});
