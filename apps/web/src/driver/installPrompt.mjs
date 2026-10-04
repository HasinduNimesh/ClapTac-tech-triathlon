/**
 * DR-8: the first-run "Add to home screen" card on the driver web app. Shown once per device, never
 * when the app is already running from the home screen.
 */
export const INSTALL_INTRO_KEY = "waypoint.driver.installIntro.dismissed";

/** "hidden", "prompt" (the browser can install it with one tap) or "manual" (show the steps). */
export function installCardState({ standalone = false, dismissed = false, canPrompt = false } = {}) {
  if (standalone || dismissed) return "hidden";
  return canPrompt ? "prompt" : "manual";
}

/** Steps for browsers without an install prompt (iPhone Safari and others). */
export function manualInstallSteps(userAgent = "") {
  return /iphone|ipad|ipod/i.test(userAgent)
    ? "Tap Share, then Add to Home Screen."
    : "Open the browser menu, then Add to Home screen or Install app.";
}
