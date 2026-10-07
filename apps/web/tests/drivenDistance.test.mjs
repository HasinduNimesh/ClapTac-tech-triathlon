import test from "node:test";
import assert from "node:assert/strict";
import { drivenKm, estimatedLitres, formatKm, formatLitres, isFresh, livePoint } from "../src/dispatcher/drivenDistance.mjs";

const now = Date.UTC(2026, 9, 7, 9, 0, 0);
const ago = (seconds) => new Date(now - seconds * 1000).toISOString();

test("a position is live for five minutes, then the truck is drawn at its last stop", () => {
  assert.equal(isFresh(ago(10), now), true);
  assert.equal(isFresh(ago(299), now), true);
  assert.equal(isFresh(ago(301), now), false);
  assert.equal(isFresh("not a date", now), false);
  assert.equal(isFresh(new Date(now + 30_000).toISOString(), now), true, "a clock a little ahead is still live");
  assert.equal(isFresh(new Date(now + 10 * 60_000).toISOString(), now), false, "a timestamp far in the future is not trusted");
});

test("livePoint returns coordinates only for a fresh, valid report", () => {
  assert.deepEqual(livePoint({ latitude: 6.93, longitude: 79.86, timestamp: ago(20) }, now), [6.93, 79.86]);
  assert.equal(livePoint({ latitude: 6.93, longitude: 79.86, timestamp: ago(900) }, now), null);
  assert.equal(livePoint({ latitude: NaN, longitude: 79.86, timestamp: ago(20) }, now), null);
  assert.equal(livePoint(null, now), null);
});

test("distance is in kilometres and never negative", () => {
  assert.equal(drivenKm(12400), 12.4);
  assert.equal(drivenKm(0), 0);
  assert.equal(drivenKm(-5), 0);
  assert.equal(drivenKm(undefined), 0);
});

test("fuel is an estimate from the rated km per litre, and missing without one", () => {
  assert.equal(estimatedLitres(47000, 4.7), 10);
  assert.equal(estimatedLitres(47000, 0), null);
  assert.equal(estimatedLitres(47000, undefined), null);
  assert.equal(estimatedLitres(0, 4.7), 0);
});

test("formatting keeps one decimal for short distances", () => {
  assert.equal(formatKm(0), "0.0 km");
  assert.equal(formatKm(4.25), "4.3 km");
  assert.equal(formatKm(124.6), "125 km");
  assert.equal(formatLitres(2.634), "2.6 L");
});
