import assert from "node:assert/strict";
import test from "node:test";
import { describeApk, formatBytes } from "../src/routes/apkInfo.mjs";
import { workspaceFor } from "../src/routes/workspaceLink.mjs";

const sha = "a".repeat(32) + "b".repeat(32);

test("file sizes read naturally", () => {
  assert.equal(formatBytes(512), "512 B");
  assert.equal(formatBytes(1536), "1.5 KB");
  assert.equal(formatBytes(19_300_000), "18.4 MB");
  assert.equal(formatBytes(0), "");
  assert.equal(formatBytes(Number.NaN), "");
});

test("a complete file description is shown with its version, size and checksum", () => {
  const info = describeApk({ version: "0.1.0+3", size: 19_300_000, sha256: sha.toUpperCase(), builtAt: "2026-10-04T10:00:00Z" });
  assert.equal(info.version, "0.1.0+3");
  assert.equal(info.sizeLabel, "18.4 MB");
  assert.equal(info.sha256, sha);
  assert.equal(info.shaShort, "aaaaaaaa…bbbbbbbb");
  assert.equal(info.builtOn, "4 Oct 2026");
});

test("anything incomplete means the app is not published, so no download link is offered", () => {
  assert.equal(describeApk(null), null);
  assert.equal(describeApk("nope"), null);
  assert.equal(describeApk({}), null);
  assert.equal(describeApk({ version: "1", size: 10, sha256: "short" }), null);
  assert.equal(describeApk({ version: "", size: 10, sha256: sha }), null);
  assert.equal(describeApk({ version: "1", size: 0, sha256: sha }), null);
  assert.equal(describeApk({ version: "1", size: 1.5, sha256: sha }), null);
});

test("a missing build date is left out, not invented", () => {
  assert.equal(describeApk({ version: "1", size: 10, sha256: sha }).builtOn, "");
  assert.equal(describeApk({ version: "1", size: 10, sha256: sha, builtAt: "yesterday-ish" }).builtOn, "");
});

test("each role has its own workspace; unknown roles go to sign-in", () => {
  assert.deepEqual(workspaceFor("STORE_MANAGER"), { href: "/store-manager", external: false });
  assert.deepEqual(workspaceFor("dispatcher"), { href: "/dispatcher", external: false });
  assert.deepEqual(workspaceFor("LOADER"), { href: "/loader-app/", external: true });
  assert.deepEqual(workspaceFor("DRIVER"), { href: "/driver/trips", external: false });
  assert.deepEqual(workspaceFor(undefined), { href: "/login", external: false });
});
