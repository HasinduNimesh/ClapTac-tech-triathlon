import assert from "node:assert/strict";
import test from "node:test";
import { appendSpoken, speechErrorMessage, speechLanguage, speechRecognitionCtor } from "../src/store-manager/speechInput.mjs";

test("recognition listens in the interface language", () => {
  assert.equal(speechLanguage("si"), "si-LK");
  assert.equal(speechLanguage("ta"), "ta-LK");
  assert.equal(speechLanguage("en"), "en-IN");
});

test("the speak button only appears where the browser can listen", () => {
  function Native() {}
  assert.equal(speechRecognitionCtor({ SpeechRecognition: Native }), Native);
  assert.equal(speechRecognitionCtor({ webkitSpeechRecognition: Native }), Native);
  assert.equal(speechRecognitionCtor({}), undefined);
});

test("spoken words are added after what was typed", () => {
  assert.equal(appendSpoken("", "rice 10 bags"), "rice 10 bags");
  assert.equal(appendSpoken("oil 24 bottles  ", "  rice   10 bags "), "oil 24 bottles rice 10 bags");
  assert.equal(appendSpoken("oil", "   "), "oil");
});

test("a blocked microphone gets a plain message; an ordinary stop gets none", () => {
  assert.match(speechErrorMessage("not-allowed"), /Microphone access is blocked/);
  assert.equal(speechErrorMessage("aborted"), "");
});
