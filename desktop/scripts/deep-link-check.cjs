const assert = require("assert");

const canonicalRoutes = require("../src/shared/route-utils.cjs");
const compatibilityRoutes = require("../route-utils.cjs");

assert.deepStrictEqual(Object.keys(compatibilityRoutes).sort(), Object.keys(canonicalRoutes).sort());

function checkRouteHelpers(routes) {
  const { createRouteHelpers, normalizeBaseURL } = routes;
  const helpers = createRouteHelpers("https://desktop.example.com/app/");

  assert.strictEqual(normalizeBaseURL("https://desktop.example.com/app/"), "https://desktop.example.com/app");
  assert.strictEqual(helpers.routeURL("/meetings"), "https://desktop.example.com/app/meetings");
  assert.strictEqual(helpers.routeURL("meetings/42"), "https://desktop.example.com/app/meetings/42");

  const cases = [
    ["allcallall://rooms/42", "https://desktop.example.com/app/meetings/42"],
    ["allcallall://rooms/42?utm=test", "https://desktop.example.com/app/meetings/42"],
    ["allcallall://conversations/99", "https://desktop.example.com/app/conversations/99"],
    ["allcallall://meetings", "https://desktop.example.com/app/meetings"],
    ["/conversations/7", "https://desktop.example.com/app/conversations/7"],
    ["/rooms/7", "https://desktop.example.com/app/meetings/7"],
    ["https://desktop.example.com/app/meetings/5", "https://desktop.example.com/app/meetings/5"],
    ["https://evil.example.com/app/meetings/5", null],
    ["allcallall://settings", null],
  ];

  for (const [target, expected] of cases) {
    assert.strictEqual(helpers.normalizeRouteTarget(target), expected);
  }

  assert.strictEqual(helpers.isInternalWebURL("https://desktop.example.com/app/meetings/5"), true);
  assert.strictEqual(helpers.isInternalWebURL("https://desktop.example.com/other/meetings/5"), false);

  return cases.map(([target]) => helpers.normalizeRouteTarget(target));
}

assert.deepStrictEqual(checkRouteHelpers(compatibilityRoutes), checkRouteHelpers(canonicalRoutes));

console.log("[desktop-deep-link-check] passed");
