import test from "node:test";
import assert from "node:assert/strict";
import { startLoginRedirect } from "../src/routes/startLoginRedirect.mjs";

test("login redirect failure is surfaced to the page instead of becoming an unhandled rejection", async () => {
  const failures = [];
  const started = [];
  const result = await startLoginRedirect(async () => { throw new Error("identity provider unavailable"); }, () => failures.push("shown"), () => started.push("connecting"));

  assert.equal(result, false);
  assert.deepEqual(started, ["connecting"]);
  assert.deepEqual(failures, ["shown"]);
});

test("successful login redirect does not show an error", async () => {
  const failures = [];
  const started = [];
  const completed = [];
  const result = await startLoginRedirect(async () => undefined, () => failures.push("shown"), () => started.push("connecting"), () => completed.push("done"));

  assert.equal(result, true);
  assert.deepEqual(started, ["connecting"]);
  assert.deepEqual(completed, ["done"]);
  assert.deepEqual(failures, []);
});
