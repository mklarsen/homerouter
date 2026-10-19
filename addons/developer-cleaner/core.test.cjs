const assert = require("node:assert/strict");
const test = require("node:test");
const core = require("./core.js");

function preset(overrides = {}) {
  return { ...core.defaults().presets[0], ...overrides };
}

test("developer mode and all triggers start disabled", () => {
  const settings = core.settings(core.defaults());
  assert.equal(settings.enabled, false);
  assert.equal(settings.theme, "dark");
  assert.deepEqual(settings.automation, { minutes: 0, startup: false, tabClose: false, presetId: "cache" });
});

test("active tab cache is exact-origin scoped with the selected time range", () => {
  const result = core.plan(preset({ range: "quarter" }), "https://example.com/path", 1000000);
  assert.equal(result.calls[0].options.since, 100000);
  assert.deepEqual(result.calls[0].options.origins, ["https://example.com"]);
  assert.equal(result.automaticSafe, true);
});

test("domain lists expand bare hosts without wildcard or path guessing", () => {
  assert.deepEqual(core.origins(["example.com", "https://example.com"]), ["http://example.com", "https://example.com"]);
  for (const entry of ["*.example.com", "https://example.com/path", "file:///tmp", "https://user:pass@example.com", ""]) {
    assert.throws(() => core.origins([entry]));
  }
});

test("unsupported global data is rejected before any removal plan can be returned", () => {
  for (const type of ["history", "downloads", "formData"]) {
    assert.throws(() => core.plan(preset({ types: ["cache", type] }), "https://example.com"), /whole-browser/);
    assert.throws(() => core.plan(preset({ scope: "exclude", origins: ["example.com"], types: [type] })), /whole-browser/);
  }
});

test("per-type include/exclude rules never disappear or widen scope", () => {
  const result = core.plan(preset({
    scope: "include", origins: ["https://example.com"], types: ["cache", "cookies"],
    typeRules: { cookies: { scope: "exclude", origins: ["https://keep.example"] } }
  }));
  assert.deepEqual(result.calls[0].options.origins, ["https://example.com"]);
  assert.deepEqual(result.calls[1].options.excludeOrigins, ["https://keep.example"]);
  assert.equal(result.automaticSafe, false);
  assert.match(result.warnings.join(" "), /registrable domain/);
});

test("empty filters, invalid ranges, obsolete types and invalid automation fail closed", () => {
  assert.throws(() => core.preset(preset({ scope: "include", origins: [] })), /at least one origin/);
  assert.throws(() => core.preset(preset({ range: "forever" })));
  assert.throws(() => core.preset(preset({ types: ["passwords"] })));
  assert.throws(() => core.settings({ ...core.defaults(), automation: { minutes: -1, presetId: "cache" } }));
});

test("app/extension data and window closing are never unattended-safe", () => {
  assert.throws(() => core.plan(preset({ extension: true }), "https://example.com"), /whole-browser/);
  for (const flag of ["extension", "protectedWeb", "closeWindows"]) {
    assert.equal(core.plan(preset({ scope: "all", [flag]: true })).automaticSafe, false);
  }
});

test("invalid active tabs cannot fall back to whole-browser clearing", () => {
  for (const url of [undefined, "chrome://settings", "file:///tmp", "chrome-extension://id/popup.html"]) {
    assert.throws(() => core.plan(preset(), url));
  }
});