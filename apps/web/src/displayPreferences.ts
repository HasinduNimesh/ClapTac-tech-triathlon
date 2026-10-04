export type TextSize = "normal" | "large" | "xlarge";
export const SIZE_KEY = "waypoint.textSize";
export const CONTRAST_KEY = "waypoint.highContrast";

export function readPreference(key: string) { try { return localStorage.getItem(key); } catch { return null; } }
export function writePreference(key: string, value: string) { try { localStorage.setItem(key, value); } catch { /* storage is optional */ } }

// Applies saved display preferences to the whole document, so every role
// screen on a shared counter tablet reads at the chosen size and contrast.
export function applyDisplayPreferences() {
  document.documentElement.dataset.textSize = (readPreference(SIZE_KEY) as TextSize) || "normal";
  document.documentElement.dataset.contrast = readPreference(CONTRAST_KEY) === "true" ? "high" : "normal";
}
