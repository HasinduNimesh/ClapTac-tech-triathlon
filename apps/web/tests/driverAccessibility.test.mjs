import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const page = await readFile(new URL("../src/driver/DriverTripsPage.tsx", import.meta.url), "utf8");
const safeStopLifecycle = await readFile(new URL("../src/driver/safeStopLifecycle.mjs", import.meta.url), "utf8");
const css = await readFile(new URL("../src/index.css", import.meta.url), "utf8");

test("driver errors are announced and signature area has an accessible name", () => {
  assert.match(page, /className="status-bad" role="alert">\{error\}/);
  assert.match(page, /role="img"\s+aria-label=\{t\("Signature drawing area\./);
});

test("photo upload remains keyboard accessible and focus is visible", () => {
  assert.match(page, /type="file"[^>]*className="visually-hidden"/);
  assert.match(css, /\.visually-hidden:focus-visible/);
  assert.match(css, /:focus-visible\s*\{\s*outline:/);
});

test("interactive controls meet the 44px minimum target", () => {
  assert.match(css, /button, input, select\s*\{\s*min-height:\s*44px/);
  assert.match(css, /\.driver-safe-stop-toggle\s*\{[^}]*min-height:\s*44px/);
  assert.match(css, /\.driver-safe-stop-toggle input\s*\{[^}]*width:\s*1\.25rem/);
});

test("driver workflow controls stay locked until the driver confirms a safe stop", () => {
  assert.match(page, /Confirm safely stopped to continue/);
  assert.match(page, /<label className="driver-safe-stop-toggle">\s*<input type="checkbox" checked=\{safeStopped\} onChange=\{\(event\) => setSafeStopped\(event\.target\.checked\)\}/);
  assert.doesNotMatch(page, /aria-pressed=\{safeStopped\}/);
  assert.match(page, /Driver actions unlocked\. Uncheck to lock them before moving/);
  assert.match(page, /<fieldset className="driver-safe-actions" disabled=\{!safeStopped\}>/);
  assert.match(page, /installSafeStopLock\(\{ document, window, onLock: setSafeStopped \}\)/);
  assert.match(safeStopLifecycle, /const onPageHide = \(\) => onLock\(false\)/);
  assert.match(page, /this app does not detect vehicle motion/i);
  assert.match(page, /if \(!safeStopped\) return;/);
});

function rule(selector) {
  const rules = [...css.matchAll(/(^|\n)([^{}]+)\{([^}]*)\}/g)];
  const match = rules.find(([, , selectors]) => selectors.split(",").some((candidate) => candidate.trim() === selector));
  assert.ok(match, `missing CSS rule for ${selector}`);
  return match[3];
}

function color(declarations, property) {
  const match = declarations.match(new RegExp(`${property}:\\s*[^#;]*?(#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?)`));
  assert.ok(match, `missing ${property} color`);
  const rawHex = match[1].slice(1);
  const hex = rawHex.length === 3 ? [...rawHex].map((digit) => digit + digit).join("") : rawHex;
  return [0, 2, 4].map((index) => parseInt(hex.slice(index, index + 2), 16) / 255)
    .map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
}

function contrast(a, b) {
  const luminance = (rgb) => 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2];
  const [lighter, darker] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (lighter + 0.05) / (darker + 0.05);
}

test("shared text, status, guidance, and action colors meet WCAG AA contrast against their surfaces", () => {
  const root = rule(":root");
  const pageBackground = color(root, "background");
  const pageText = color(root, "color");
  const cardBackground = color(rule(".card"), "background");
  const bannerBackground = color(rule(".banner"), "background");
  const outputBackground = color(rule(".agent-panel pre"), "background");
  const approvalBackground = color(rule(".agent-approval"), "background");
  const guidanceBackground = color(rule(".stop-guidance"), "background");
  const nextStopBackground = color(rule(".next-stop"), "background");
  const tripCardBackground = color(rule(".load-item"), "background");
  const loginPaneBackground = color(rule(".login-pane"), "background");
  const loginBadgeBackground = color(rule(".login-badge"), "background");
  const pairs = [
    ["body text", pageText, pageBackground],
    ["body text on cards", pageText, cardBackground],
    ["body text on banners", pageText, bannerBackground],
    ["agent output text", pageText, outputBackground],
    ["links", color(rule("a"), "color"), pageBackground],
    ["links on cards", color(rule("a"), "color"), cardBackground],
    ["muted text", color(rule(".muted"), "color"), cardBackground],
    ["success status", color(rule(".status-ok"), "color"), cardBackground],
    ["error status", color(rule(".status-bad"), "color"), cardBackground],
    ["sync status", color(rule(".status-syncing"), "color"), cardBackground],
    ["agent approval text", pageText, approvalBackground],
    ["stop guidance values", pageText, guidanceBackground],
    ["stop guidance labels", color(rule(".stop-guidance dt"), "color"), guidanceBackground],
    ["next-stop action text", pageText, nextStopBackground],
    ["trip-card/load-item text", pageText, tripCardBackground],
    ["link-style action text", color(rule(".linkish"), "color"), cardBackground],
    ["primary button", color(rule(".tap.primary"), "color"), color(rule(".tap.primary"), "background")],
    ["skip link", color(rule(".skip-link"), "color"), color(rule(".skip-link"), "background")],
    ["login role badge text", color(rule(".login-badge"), "color"), loginBadgeBackground],
    ["login footnote link", color(rule(".login-footnote-link"), "color"), loginPaneBackground],
  ];
  for (const [label, foreground, background] of pairs) {
    assert.ok(contrast(foreground, background) >= 4.5, `${label} contrast is below 4.5:1`);
  }

  const explicitForegroundSelectors = [...css.matchAll(/(^|\n)([^{}]+)\{([^}]*)\}/g)]
    .filter(([, , , declarations]) => /(?:^|;)\s*color\s*:/.test(declarations))
    .flatMap(([, , selectors]) => selectors.split(",").map((selector) => selector.trim()));
  const testedForegroundSelectors = new Set([
    ":root", ".skip-link", "a", ".status-ok", ".status-bad", ".status-syncing",
    ".stop-guidance dt", ".muted", ".linkish", ".tap.primary",
    ".login-badge", ".login-footnote-link",
  ]);
  assert.deepEqual(
    explicitForegroundSelectors.filter((selector) => !testedForegroundSelectors.has(selector)),
    [],
    "add a contrast pair for every explicit CSS text color",
  );
});

test("keyboard focus indicator remains clearly visible against the page surface", () => {
  const focusColor = color(rule(":focus-visible"), "outline");
  const pageBackground = color(rule(":root"), "background");
  assert.ok(contrast(focusColor, pageBackground) >= 3, "focus indicator contrast is below 3:1");
});
