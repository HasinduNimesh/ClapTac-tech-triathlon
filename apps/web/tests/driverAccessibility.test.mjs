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

  // These two selectors are preceded by a comment, so rule() cannot find them; pass literals directly.
  const smHeroBg = color("background: #1060d0;", "background");
  const smSidebarBg = color("background: #ffffff;", "background");
  const smNavItemHoverBg = color(rule(".sm-nav-item:hover"), "background");
  const smNavItemActiveBg = color(rule(".sm-nav-item.active"), "background");
  const smBtnPlaceOrderBg = color(rule(".sm-btn-place-order"), "background");
  const smBadgeOnRouteBg = color(rule(".sm-badge--on-route"), "background");
  const smBadgeDeferredBg = color(rule(".sm-badge--deferred"), "background");
  const smBadgeDeliveredBg = color(rule(".sm-badge--delivered"), "background");
  const smBadgeDefaultBg = color(rule(".sm-badge--default"), "background");
  const smNoticeBadgeBg = color(rule(".sm-notice-badge"), "background");
  const smTableHeadBg = color(rule(".sm-table thead tr"), "background");
  const smNotifReceiptBg = color(rule(".sm-notif-badge--receipt"), "background");
  const smNotifDeferredBg = color(rule(".sm-notif-badge--deferred"), "background");
  const smNotifActionBg = color(rule(".sm-notif-action--solid"), "background");
  const smInfoBannerBg = color(rule(".sm-info-banner"), "background");
  const notFoundActionBg = color(rule(".not-found-action"), "background");
  const smLoadErrorBg = color(rule(".sm-load-error"), "background");
  const smOrderTabActiveBg = color(rule(".sm-order-tab.active"), "background");
  const smSecondaryBtnBg = color(rule(".sm-btn-secondary"), "background");
  const smLegacySubmitBg = color(rule('.sm-legacy-body button[type="submit"]'), "background");
  const smPairs = [
    ["sm-nav-label", color(rule(".sm-nav-label"), "color"), smSidebarBg],
    ["sm-nav-item", color(rule(".sm-nav-item"), "color"), smSidebarBg],
    ["sm-nav-item:hover", color(rule(".sm-nav-item:hover"), "color"), smNavItemHoverBg],
    ["sm-nav-item.active", color(rule(".sm-nav-item.active"), "color"), smNavItemActiveBg],
    ["sm-hero-title", color(rule(".sm-hero-title"), "color"), smHeroBg],
    ["sm-hero-sub", color(rule(".sm-hero-sub"), "color"), smHeroBg],
    ["sm-btn-place-order", color(rule(".sm-btn-place-order"), "color"), smBtnPlaceOrderBg],
    ["sm-stat-label", color(rule(".sm-stat-label"), "color"), pageBackground],
    ["sm-stat-sub--orange", color(rule(".sm-stat-sub--orange"), "color"), pageBackground],
    ["sm-stat-sub--green", color(rule(".sm-stat-sub--green"), "color"), pageBackground],
    ["sm-panel-link", color(rule(".sm-panel-link"), "color"), pageBackground],
    ["sm-track-link", color(rule(".sm-track-link"), "color"), pageBackground],
    ["sm-badge--on-route", color(rule(".sm-badge--on-route"), "color"), smBadgeOnRouteBg],
    ["sm-badge--deferred", color(rule(".sm-badge--deferred"), "color"), smBadgeDeferredBg],
    ["sm-badge--delivered", color(rule(".sm-badge--delivered"), "color"), smBadgeDeliveredBg],
    ["sm-badge--default", color(rule(".sm-badge--default"), "color"), smBadgeDefaultBg],
    ["sm-notice-badge", color(rule(".sm-notice-badge"), "color"), smNoticeBadgeBg],
    ["sm-alert-action", color(rule(".sm-alert-action"), "color"), pageBackground],
    ["sm-create-order-link", color(rule(".sm-create-order-link"), "color"), pageBackground],
    ["sm-table th", color(rule(".sm-table th"), "color"), smTableHeadBg],
    ["sm-notif-badge--receipt", color(rule(".sm-notif-badge--receipt"), "color"), smNotifReceiptBg],
    ["sm-notif-badge--deferred", color(rule(".sm-notif-badge--deferred"), "color"), smNotifDeferredBg],
    ["sm-notif-action--solid", color(rule(".sm-notif-action--solid"), "color"), smNotifActionBg],
    ["sm-notif-action--outline", color(rule(".sm-notif-action--outline"), "color"), color(rule(".sm-notif-action--outline"), "background")],
    ["sm-info-banner-title", color(rule(".sm-info-banner-title"), "color"), smInfoBannerBg],
    ["sm-breadcrumb", color(rule(".sm-breadcrumb"), "color"), smHeroBg],
    ["sm-breadcrumb-link", color(rule(".sm-breadcrumb-link"), "color"), smHeroBg],
    ["not-found action", color(rule(".not-found-action"), "color"), notFoundActionBg],
    ["sm-load-error", color(rule(".sm-load-error"), "color"), smLoadErrorBg],
    ["sm-order-tab.active", color(rule(".sm-order-tab.active"), "color"), smOrderTabActiveBg],
    ["sm-btn-secondary", color(rule(".sm-btn-secondary"), "color"), smSecondaryBtnBg],
    ["sm-timeline warn label", color(rule(".sm-timeline-step--warn .sm-timeline-label"), "color"), cardBackground],
    ["sm legacy submit button", color(rule('.sm-legacy-body button[type="submit"]'), "color"), smLegacySubmitBg],
    ["sm legacy action button", color(rule('.sm-legacy-body button[type="button"]'), "color"), smLegacySubmitBg],
    ["order text helper button", color(rule(".sm-helper-actions .tap.primary"), "color"), color(rule(".sm-helper-actions .tap.primary"), "background")],
    ["order text helper link", color(rule(".sm-helper-link"), "color"), cardBackground],
    ["order helper button", color(rule(".sm-ai-fab"), "color"), color(rule(".sm-ai-fab"), "background")],
    ["order helper close", color(rule(".sm-ai-drawer-close"), "color"), color(rule(".sm-ai-drawer-close"), "background")],
  ];
  for (const [label, foreground, background] of smPairs) {
    assert.ok(contrast(foreground, background) >= 4.5, `${label} contrast is below 4.5:1`);
  }

  const explicitForegroundSelectors = [...css.matchAll(/(^|\n)([^{}]+)\{([^}]*)\}/g)]
    .filter(([, , , declarations]) => /(?:^|;)\s*color\s*:/.test(declarations))
    .flatMap(([, , selectors]) => selectors.split(",").map((selector) => selector.trim()));
  const testedForegroundSelectors = new Set([
    ":root", ".skip-link", "a", ".status-ok", ".status-bad", ".status-syncing",
    ".stop-guidance dt", ".muted", ".linkish", ".tap.primary",
    ".login-badge", ".login-footnote-link",
    ".sm-nav-label", ".sm-nav-item", ".sm-nav-item:hover", ".sm-nav-item.active",
    ".sm-hero-title", ".sm-hero-sub", ".sm-btn-place-order",
    ".sm-stat-label", ".sm-stat-sub--orange", ".sm-stat-sub--green",
    ".sm-panel-link", ".sm-track-link",
    ".sm-badge--on-route", ".sm-badge--deferred", ".sm-badge--delivered", ".sm-badge--default",
    ".sm-notice-badge", ".sm-alert-action", ".sm-create-order-link",
    ".sm-table th",
    ".sm-notif-badge--receipt", ".sm-notif-badge--deferred",
    ".sm-notif-action--solid", ".sm-notif-action--outline", ".sm-info-banner-title",
    ".not-found-action", ".sm-breadcrumb", ".sm-breadcrumb-link", ".sm-load-error", ".sm-order-tab.active",
    ".sm-btn-secondary", ".sm-timeline-step--warn .sm-timeline-label",
    '.sm-legacy-body button[type="submit"]', '.sm-legacy-body button[type="button"]',
    ".sm-helper-actions .tap.primary", ".sm-helper-link", ".sm-ai-fab", ".sm-ai-drawer-close",
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
