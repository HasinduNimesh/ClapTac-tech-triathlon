import test from "node:test";
import assert from "node:assert/strict";
import { findLoadOrderByCode, resolveLoadOrderCode } from "../src/loader/barcodeLookup.mjs";
import { acquireBarcodeCamera } from "../src/loader/barcodeCamera.mjs";

const orders = [{ orderId: "order-123", orderRef: "ORD-001" }, { orderId: "order-456" }];
test("scanner matches a current-trip order by case-insensitive reference or ID", () => {
  assert.equal(findLoadOrderByCode(orders, " ord-001 "), orders[0]);
  assert.equal(findLoadOrderByCode(orders, "ORDER-456"), orders[1]);
});
test("unknown and empty scans do not match another order", () => {
  assert.equal(findLoadOrderByCode(orders, "ORD-999"), undefined);
  assert.equal(findLoadOrderByCode(orders, " "), undefined);
});

test("scan confirmation rejects unknown, not-started, and shortfall orders before load", () => {
  const withIssue = [{ orderId: "order-789", orderRef: "ORD-003", issues: [{ type: "DAMAGED" }] }];
  assert.deepEqual(resolveLoadOrderCode(orders, "ORD-999", true), { kind: "not_found" });
  assert.equal(resolveLoadOrderCode(orders, "ORD-001", false).kind, "not_loading");
  assert.equal(resolveLoadOrderCode(withIssue, "ORD-003", true).kind, "unresolved_shortfall");
  assert.deepEqual(resolveLoadOrderCode(orders, "ord-001", true), { kind: "ready", order: orders[0] });
});

test("camera setup fails clearly when the browser has no camera API", async () => {
  await assert.rejects(acquireBarcodeCamera({}, {}, () => true), /camera_unavailable/);
});

test("permission denial is returned to the screen for a manual-entry fallback", async () => {
  const denied = new Error("NotAllowedError");
  await assert.rejects(acquireBarcodeCamera({ getUserMedia: async () => { throw denied; } }, {}, () => true), error => error === denied);
});

test("camera stream is stopped if the scanner closes while permission is pending", async () => {
  const track = { stopped: 0, stop() { this.stopped += 1; } };
  let active = true;
  const stream = { getTracks: () => [track] };
  const pendingGrant = acquireBarcodeCamera({ getUserMedia: async () => stream }, { srcObject: null, play: async () => {} }, () => active);
  active = false;
  assert.equal(await pendingGrant, undefined);
  assert.equal(track.stopped, 1);
});

test("successful camera setup returns idempotent cleanup and releases tracks", async () => {
  const track = { stopped: 0, stop() { this.stopped += 1; } };
  const stream = { getTracks: () => [track] };
  const video = { srcObject: null, play: async () => {} };
  const stop = await acquireBarcodeCamera({ getUserMedia: async constraints => {
    assert.equal(constraints.video.facingMode.ideal, "environment");
    assert.equal(constraints.audio, false);
    return stream;
  } }, video, () => true);
  assert.equal(video.srcObject, stream);
  stop();
  stop();
  assert.equal(track.stopped, 1);
  assert.equal(video.srcObject, null);
});

test("camera stream is released if the video cannot start", async () => {
  const track = { stopped: 0, stop() { this.stopped += 1; } };
  const stream = { getTracks: () => [track] };
  const error = new Error("play failed");
  const video = { srcObject: null, play: async () => { throw error; } };
  await assert.rejects(acquireBarcodeCamera({ getUserMedia: async () => stream }, video, () => true), value => value === error);
  assert.equal(track.stopped, 1);
  assert.equal(video.srcObject, null);
});
