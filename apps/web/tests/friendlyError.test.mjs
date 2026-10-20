import test from "node:test";
import assert from "node:assert/strict";
import { describeError, friendlyMessage, NETWORK_MESSAGE, readableDetail } from "../src/api/friendlyError.mjs";

const problem = (status, title, detail) => JSON.stringify({ type: "about:blank", title, status, detail });

test("a server failure never shows its detail, which can be an internal message", () => {
  const text = friendlyMessage(500, problem(500, "Internal Server Error", "pq: connection refused"));
  assert.equal(text, "Something went wrong on our side. Please try again in a moment.");
  assert.doesNotMatch(text, /pq|refused|\{/);
});

test("a readable detail is kept, without its machine prefix", () => {
  assert.equal(friendlyMessage(409, problem(409, "Conflict", "conflict: plan version changed")), "Plan version changed.");
  assert.equal(friendlyMessage(400, problem(400, "Bad Request", "invalid: quantity must be at least 1")), "Quantity must be at least 1.");
  assert.equal(friendlyMessage(403, problem(403, "Forbidden", "forbidden: dispatcher permission required")), "Dispatcher permission required.");
});

test("an unreadable detail falls back to a plain sentence", () => {
  assert.equal(friendlyMessage(400, problem(400, "Bad Request", "ERROR: duplicate key value violates unique constraint")), "That wasn't accepted. Check what you entered and try again.");
  assert.equal(friendlyMessage(409, "{not json"), "This changed while you were working. Reload and try again.");
  assert.equal(friendlyMessage(404, problem(404, "Not Found", "not found: plan")), "We couldn't find that. It may have been removed or moved.");
});

test("sign-in, rate limit and network have their own wording", () => {
  assert.equal(friendlyMessage(401, ""), "Your sign-in has ended. Sign in again.");
  assert.equal(friendlyMessage(429, ""), "Too many requests. Wait a moment and try again.");
  assert.equal(friendlyMessage(0, ""), NETWORK_MESSAGE);
});

test("describeError handles API errors, network errors and plain errors", () => {
  const api = Object.assign(new Error(problem(409, "Conflict", "conflict: stop already completed")), { status: 409 });
  assert.equal(describeError(api), "Stop already completed.");
  assert.equal(describeError(new TypeError("Failed to fetch")), NETWORK_MESSAGE);
  assert.equal(describeError(new Error("Add at least one item to the order.")), "Add at least one item to the order.");
  assert.equal(describeError(new Error(problem(500, "x", "pq: boom"))), "Something went wrong. Please try again.");
  assert.equal(describeError(undefined), "Something went wrong. Please try again.");
  assert.equal(describeError("Quantity is too high"), "Quantity is too high");
});

test("readableDetail refuses JSON, stack-like text and long text", () => {
  assert.equal(readableDetail('{"detail":"{\\"nested\\":1}"}'), "");
  assert.equal(readableDetail(JSON.stringify({ detail: "x".repeat(300) })), "");
  assert.equal(readableDetail(JSON.stringify({ detail: "goroutine 1 [running]" })), "");
});
