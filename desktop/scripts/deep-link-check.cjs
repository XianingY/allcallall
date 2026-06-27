const assert = require("assert");

const { createRouteHelpers, normalizeBaseURL } = require("../route-utils.cjs");

const helpers = createRouteHelpers("https://desktop.example.com/app/");

assert.strictEqual(normalizeBaseURL("https://desktop.example.com/app/"), "https://desktop.example.com/app");
assert.strictEqual(helpers.routeURL("/meetings"), "https://desktop.example.com/app/meetings");
assert.strictEqual(helpers.routeURL("meetings/42"), "https://desktop.example.com/app/meetings/42");

assert.strictEqual(
  helpers.normalizeRouteTarget("allcallall://rooms/42"),
  "https://desktop.example.com/app/meetings/42"
);
assert.strictEqual(
  helpers.normalizeRouteTarget("allcallall://rooms/42?utm=test"),
  "https://desktop.example.com/app/meetings/42"
);
assert.strictEqual(
  helpers.normalizeRouteTarget("allcallall://conversations/99"),
  "https://desktop.example.com/app/conversations/99"
);
assert.strictEqual(
  helpers.normalizeRouteTarget("allcallall://meetings"),
  "https://desktop.example.com/app/meetings"
);
assert.strictEqual(
  helpers.normalizeRouteTarget("/conversations/7"),
  "https://desktop.example.com/app/conversations/7"
);
assert.strictEqual(
  helpers.normalizeRouteTarget("/rooms/7"),
  "https://desktop.example.com/app/meetings/7"
);
assert.strictEqual(
  helpers.normalizeRouteTarget("https://desktop.example.com/app/meetings/5"),
  "https://desktop.example.com/app/meetings/5"
);

assert.strictEqual(helpers.normalizeRouteTarget("https://evil.example.com/app/meetings/5"), null);
assert.strictEqual(helpers.normalizeRouteTarget("allcallall://settings"), null);
assert.strictEqual(helpers.isInternalWebURL("https://desktop.example.com/app/meetings/5"), true);
assert.strictEqual(helpers.isInternalWebURL("https://desktop.example.com/other/meetings/5"), false);

console.log("[desktop-deep-link-check] passed");
