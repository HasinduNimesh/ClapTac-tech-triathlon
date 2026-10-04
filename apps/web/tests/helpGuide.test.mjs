import assert from "node:assert/strict";
import test from "node:test";
import { DISPATCHER_GUIDE, STORE_MANAGER_GUIDE } from "../src/help/guideContent.mjs";
import { translate } from "../src/locale.mjs";

const strings = (guide) => guide.flatMap((section) => [section.title, ...section.items.flatMap((item) => [item.title, item.text])]);

test("every Help & Guide heading and explanation has Sinhala and Tamil text", () => {
  for (const key of [...strings(STORE_MANAGER_GUIDE), ...strings(DISPATCHER_GUIDE)]) {
    assert.notEqual(translate("si", key), key, `missing Sinhala translation for ${key}`);
    assert.notEqual(translate("ta", key), key, `missing Tamil translation for ${key}`);
  }
});

test("the guides cover the features added for the audit", () => {
  const store = strings(STORE_MANAGER_GUIDE).join(" ");
  const dispatcher = strings(DISPATCHER_GUIDE).join(" ");
  for (const topic of ["Speak your order", "Temperature on arrival", "The bell", "Report by the deadline"]) assert.ok(store.includes(topic), topic);
  for (const topic of ["Ten-week forecast", "Road and weather risks", "Alerts grouped by trip", "Typed order numbers", "low-data mode"]) assert.ok(dispatcher.includes(topic), topic);
});
