/**
 * ST-3: speak the order instead of typing it. The browser's own speech recognition turns speech into
 * text in the order box; the manager still reads it and presses "Fill in for me". Nothing is sent
 * anywhere by this module, and browsers without speech recognition simply do not show the button.
 */

/** Recognition language for the interface language: Sri Lankan Sinhala and Tamil, Indian English as the nearest English model. */
export function speechLanguage(locale) {
  return locale === "si" ? "si-LK" : locale === "ta" ? "ta-LK" : "en-IN";
}

/** The browser's SpeechRecognition constructor, or undefined when there is none (Firefox, older Safari). */
export function speechRecognitionCtor(scope = globalThis) {
  return scope?.SpeechRecognition || scope?.webkitSpeechRecognition;
}

/** Joins what was already typed with what was just said, with one space between. */
export function appendSpoken(existing, spoken) {
  const said = String(spoken ?? "").replace(/\s+/g, " ").trim();
  if (!said) return existing;
  const before = String(existing ?? "").replace(/\s+$/, "");
  return before ? `${before} ${said}` : said;
}

/** A plain-language message for a recognition error code, or "" when nothing needs saying. */
export function speechErrorMessage(code) {
  switch (code) {
    case "not-allowed":
    case "service-not-allowed":
      return "Microphone access is blocked. Allow the microphone for this site, or type the order.";
    case "audio-capture":
      return "No microphone was found. Type the order instead.";
    case "network":
      return "Speech could not be turned into text without a connection. Type the order instead.";
    case "language-not-supported":
      return "This browser cannot listen in the chosen language. Switch to English or type the order.";
    case "no-speech":
      return "Nothing was heard. Tap Speak and try again.";
    default:
      return "";
  }
}
