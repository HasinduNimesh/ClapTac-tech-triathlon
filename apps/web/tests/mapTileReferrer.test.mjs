import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const map = readFileSync(new URL("../src/components/WaypointMap.tsx", import.meta.url), "utf8");
const nginx = readFileSync(new URL("../../../infrastructure/nginx/nginx.conf", import.meta.url), "utf8");

test("map tiles send the site's origin so OpenStreetMap does not block them", () => {
  assert.match(map, /tile\.openstreetmap\.org[^)]*referrerPolicy: "strict-origin-when-cross-origin"/);
});

test("the site-wide policy stays strict; only the tile images are relaxed", () => {
  assert.match(nginx, /add_header Referrer-Policy same-origin always;/);
  assert.doesNotMatch(nginx, /Referrer-Policy (no-referrer-when-downgrade|unsafe-url|origin-when-cross-origin)/);
});

test("the content security policy still allows only the OSM tile host for images", () => {
  assert.match(nginx, /img-src 'self' data: blob: https:\/\/tile\.openstreetmap\.org;/);
});
