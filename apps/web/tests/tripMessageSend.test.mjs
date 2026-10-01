import assert from "node:assert/strict";
import test from "node:test";
import { singleFlightMessagePost } from "../src/dispatcher/singleFlightMessagePost.mjs";

const request = { senderId: "dispatcher-a", tripId: "trip-1", stopId: "stop-1", body: "Use the south gate", token: "session" };

test("overlapping identical message submits produce one POST", async () => {
  let calls = 0;
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  const postOnce = singleFlightMessagePost(async value => { calls += 1; await gate; return value.body; });
  const first = postOnce(request);
  const second = postOnce({ ...request, body: "  Use the south gate  " });
  await Promise.resolve();
  assert.equal(calls, 1);
  release();
  assert.deepEqual(await Promise.all([first, second]), ["Use the south gate", "Use the south gate"]);
});

test("different trip, stop, sender, or message text remains a separate send", async () => {
  const sent = [];
  const postOnce = singleFlightMessagePost(async value => { sent.push(value); return value; });
  await Promise.all([
    postOnce(request),
    postOnce({ ...request, tripId: "trip-2" }),
    postOnce({ ...request, stopId: "stop-2" }),
    postOnce({ ...request, senderId: "dispatcher-b" }),
    postOnce({ ...request, body: "Use the loading bay" }),
  ]);
  assert.equal(sent.length, 5);
});

test("a failed request releases its key so the dispatcher can retry", async () => {
  let calls = 0;
  const postOnce = singleFlightMessagePost(async () => {
    calls += 1;
    if (calls === 1) throw new Error("temporary outage");
    return "sent";
  });
  await assert.rejects(postOnce(request), /temporary outage/);
  assert.equal(await postOnce(request), "sent");
  assert.equal(calls, 2);
});
