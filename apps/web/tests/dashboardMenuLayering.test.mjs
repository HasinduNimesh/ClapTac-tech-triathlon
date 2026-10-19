import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { translate } from "../src/locale.mjs";

const css = readFileSync(new URL("../src/index.css", import.meta.url), "utf8");
const smCss = readFileSync(new URL("../src/store-manager/storeManager.css", import.meta.url), "utf8");
const dashboardsSource = readFileSync(new URL("../src/store-manager/dashboardRequest.mjs", import.meta.url), "utf8");
const CARD_CATALOGUE_TITLES = [...dashboardsSource.matchAll(/^  \{ id: "\w+", title: "([^"]+)"/gm)].map((m) => m[1]);
const menu = readFileSync(new URL("../src/store-manager/DashboardMenu.tsx", import.meta.url), "utf8");

const rule = (source, selector) => {
  const start = source.indexOf(`${selector} {`);
  assert.notEqual(start, -1, `${selector} rule exists`);
  return source.slice(start, source.indexOf("}", start));
};
const zIndex = (body) => Number(/z-index:\s*(\d+)/.exec(body)?.[1]);

test("the hero does not clip its Dashboard menu", () => {
  const hero = rule(css, ".sm-hero");
  assert.doesNotMatch(hero, /overflow\s*:\s*(hidden|clip)/, "an overflow clip on the hero cuts off the dropdown");
  assert.doesNotMatch(hero, /z-index/, "the hero must not create a stacking context of its own");
  assert.match(rule(css, ".sm-hero::before"), /border-radius:\s*inherit/, "the wash keeps the hero's rounded corners without clipping");
});

test("the hero actions (and the open menu) stack above the stat cards pulled up over the hero", () => {
  assert.ok(zIndex(rule(css, ".sm-hero-actions")) > zIndex(rule(css, ".sm-stat-row")));
  const list = rule(smCss, ".sm-dash-list");
  assert.match(list, /position:\s*absolute/);
  assert.ok(zIndex(list) >= 40);
});

test("Escape closes the Dashboard menu and returns focus to its button", () => {
  assert.match(menu, /e\.key === "Escape"[^}]*setOpen\(false\)[^}]*trigger\.current\?\.focus\(\)/);
  assert.match(menu, /ref=\{trigger\}/);
});

test("the hand-pick card titles are translated", () => {
  assert.equal(CARD_CATALOGUE_TITLES.length, 8);
  for (const title of CARD_CATALOGUE_TITLES) {
    assert.notEqual(translate("si", title), title, `Sinhala for ${title}`);
    assert.notEqual(translate("ta", title), title, `Tamil for ${title}`);
  }
});

test("the unavailable-helper line is translated", () => {
  for (const key of ["The helper isn't available right now — you can fill this in by hand.", "Pick the cards by hand"]) {
    assert.notEqual(translate("si", key), key);
    assert.notEqual(translate("ta", key), key);
  }
});
