import assert from "node:assert/strict";
import test from "node:test";
import { badgeText, bellLabel, countUnread, unreadItems } from "../src/notifications/bellModel.mjs";
import { deriveDispatcherAlerts, deriveStoreMessages, STORE_MESSAGE_WINDOW_HOURS } from "../src/notifications/notificationItems.mjs";

test("the badge shows nothing at zero and caps at 99+, while the label says the full number", () => {
  assert.equal(badgeText(0), "");
  assert.equal(badgeText(7), "7");
  assert.equal(badgeText(140), "99+");
  assert.equal(bellLabel(140), "Notifications, 140 unread");
});

test("only items that can be read are hidden once read", () => {
  const items = [{ key: "a", title: "A", readable: true }, { key: "b", title: "B" }];
  assert.deepEqual(unreadItems(items, new Set(["a", "b"])).map((i) => i.key), ["b"], "a breakdown stays until it is resolved");
  assert.equal(countUnread(items, []), 2);
});

test("a breakdown is a critical dispatcher alert and settled sync conflicts are not counted", () => {
  const alerts = deriveDispatcherAlerts({
    incidents: [{ id: "inc-1", vehicleId: "VEH014", type: "breakdown", description: "engine", reportedAt: "2026-10-04T03:00:00Z" }],
    conflicts: [{ id: "c1" }],
  });
  assert.equal(alerts[0].tone, "red");
  assert.match(alerts[0].tag, /Critical/);
  assert.match(alerts.at(-1).title, /^1 sync conflict/);
});

test("recent store messages appear in the bell; old ones drop out", () => {
  const now = Date.parse("2026-10-04T12:00:00Z");
  const items = deriveStoreMessages([
    { id: 1, eventType: "MAJOR_DELAY", body: "ORD1 is running 75 minutes late after a breakdown.", createdAt: "2026-10-04T08:00:00Z" },
    { id: 2, eventType: "DEFERRAL", body: "ORD2 moved to Tuesday.", createdAt: "2026-10-03T13:00:00Z" },
    { id: 3, eventType: "DEFERRAL", body: "Old notice", createdAt: new Date(now - (STORE_MESSAGE_WINDOW_HOURS + 1) * 3600_000).toISOString() },
    { id: 4, eventType: "DEFERRAL", body: "", createdAt: "2026-10-04T08:00:00Z" },
  ], now);
  assert.deepEqual(items.map((i) => i.key), ["message:1", "message:2"]);
  assert.equal(items[0].tone, "red", "a breakdown delay is urgent");
  assert.equal(items[0].tag, "DELAY");
  assert.ok(items.every((i) => i.readable));
});
