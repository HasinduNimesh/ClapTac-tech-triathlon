import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const map = readFileSync(new URL("../src/components/LiveLocationMap.tsx", import.meta.url), "utf8");
const base = readFileSync(new URL("../src/components/WaypointMap.tsx", import.meta.url), "utf8");

test("the live location is drawn on the street map, not a placeholder grid", () => {
  assert.match(map, /<WaypointMap /);
  assert.doesNotMatch(map, /<svg/);
  assert.doesNotMatch(map, /\* 6000/, "no position relative to the first report");
});

test("a report is stale only after five minutes, because a parked truck reports every 90 seconds", () => {
  assert.match(map, /!isFresh\(point\.timestamp, now\)/);
  assert.doesNotMatch(map, /> 60_000/);
});

test("a moving truck is followed without resetting the zoom", () => {
  assert.match(map, /follow/);
  assert.match(base, /m\.panTo\(markers\[0\]\.at\)/);
  assert.match(base, /fitted\.current !== key/);
});

test("an unavailable position says so instead of drawing a map", () => {
  assert.match(map, /\{point && <WaypointMap/);
  assert.match(map, /Live location unavailable\./);
});
