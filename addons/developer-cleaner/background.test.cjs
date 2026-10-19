const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");
const core = require("./core.js");

function worker(settings = core.defaults(), remove = async () => {}) {
  const handlers = {};
  const data = { cleanerSettings: settings };
  const calls = [];
  const alarms = [];
  const tabs = new Map([[1, { id: 1, url: "https://example.com/path", title: "Example", active: true }]]);
  const event = name => ({ addListener(listener) { handlers[name] = listener; } });
  const chrome = {
    runtime: { id: "test", getURL: value => `chrome-extension://test/${value}`, onMessage: event("message"), onInstalled: event("installed"), onStartup: event("startup"), openOptionsPage: async () => {} },
    storage: {
      local: { get: async () => ({ ...data }), set: async values => Object.assign(data, values) },
      sync: { set: async () => {}, remove: async () => {} }, onChanged: event("storage")
    },
    tabs: { get: async id => tabs.get(id), query: async () => [...tabs.values()], reload: async () => calls.push("reload"), onRemoved: event("tabRemoved") },
    windows: { getAll: async () => [{ id: 2 }], remove: async id => calls.push(["close", id]) },
    alarms: { clear: async () => {}, create: async (name, options) => alarms.push({ name, options }), onAlarm: event("alarm") },
    action: { setBadgeText: async () => {}, setBadgeBackgroundColor: async () => {} },
    browsingData: { remove: async (options, types) => { calls.push({ options, types }); await remove(options, types); }, settings: async () => ({}) },
    contextMenus: { removeAll: async () => {}, create() {}, onClicked: event("contextMenu") },
    commands: { onCommand: event("command") }
  };
  let sequence = 0;
  const context = vm.createContext({ chrome, CleanerCore: core, importScripts() {}, URL, TextEncoder, console, crypto: { randomUUID: () => `token-${++sequence}` } });
  vm.runInContext(fs.readFileSync(path.join(__dirname, "background.js"), "utf8"), context);
  const send = (message, sender = { id: "test", url: "chrome-extension://test/popup.html" }) => new Promise(resolve => {
    const accepted = handlers.message(message, sender, resolve);
    if (!accepted) resolve({ ok: false });
  });
  return { send, calls, data, tabs, alarms, context, handlers };
}

test("disabled mode rejects preview and execution without touching data", async () => {
  const app = worker();
  assert.equal((await app.send({ action: "preview" })).ok, false);
  assert.equal((await app.send({ action: "clean" })).ok, false);
  assert.equal((await app.send({ action: "execute", token: "guess", confirmed: true })).ok, false);
  assert.equal(app.calls.length, 0);
});

test("only own extension pages may issue cleanup requests", async () => {
  const app = worker({ ...core.defaults(), enabled: true });
  assert.equal((await app.send({ action: "preview" }, { id: "other", url: "https://example.com" })).ok, false);
});

test("fresh preview executes exact calls once, then reloads", async () => {
  const app = worker({ ...core.defaults(), enabled: true });
  const preview = await app.send({ action: "preview", tabId: 1 });
  assert.equal(preview.ok, true);
  assert.equal(app.calls.length, 0);
  const message = { action: "execute", token: preview.data.token, confirmed: true };
  assert.equal((await app.send(message)).ok, true);
  assert.deepEqual(Object.keys(app.calls[0].types), ["cache"]);
  assert.deepEqual(app.calls[0].options.origins, ["https://example.com"]);
  assert.equal(app.calls[2], "reload");
  assert.equal((await app.send(message)).ok, false);
});

test("navigation or settings changes invalidate the reviewed operation", async () => {
  const app = worker({ ...core.defaults(), enabled: true });
  const first = await app.send({ action: "preview", tabId: 1 });
  app.tabs.set(1, { id: 1, url: "https://different.example" });
  assert.equal((await app.send({ action: "execute", token: first.data.token, confirmed: true })).ok, false);
  const second = await app.send({ action: "preview", tabId: 1 });
  await app.send({ action: "save", settings: core.defaults() });
  assert.equal((await app.send({ action: "execute", token: second.data.token, confirmed: true })).ok, false);
  assert.equal(app.calls.length, 0);
});

test("partial failure stops subsequent types, reload and window closing", async () => {
  const settings = core.defaults();
  settings.enabled = true;
  settings.presets[0].closeWindows = true;
  const app = worker(settings, async (_options, types) => { if (types.cacheStorage) throw new Error("Policy denied"); });
  const preview = await app.send({ action: "preview", tabId: 1 });
  const result = await app.send({ action: "execute", token: preview.data.token, confirmed: true });
  assert.equal(result.ok, false);
  assert.match(result.error, /Completed: cache/);
  assert.equal(app.calls.length, 2);
  assert.equal(app.data.activity[0].ok, false);
});

test("unsafe or active-tab automation queues review and never deletes", async () => {
  for (const types of [["cookies"], ["cache"]]) {
    const settings = core.defaults();
    settings.enabled = true;
    settings.automation.minutes = 5;
    settings.presets[0].types = types;
    const app = worker(settings);
    await vm.runInContext('automate("schedule")', app.context);
    assert.equal(app.calls.length, 0);
    assert.equal(app.data.pendingReview.source, "schedule");
  }
});

test("explicit-domain cache automation runs only while enabled", async () => {
  const settings = core.defaults();
  settings.enabled = true;
  settings.automation.minutes = 5;
  Object.assign(settings.presets[0], { scope: "include", origins: ["https://example.com"] });
  const app = worker(settings);
  await vm.runInContext('automate("schedule")', app.context);
  assert.equal(app.calls.length, 2);
  assert.deepEqual(app.calls[0].options.origins, ["https://example.com"]);
  app.data.cleanerSettings.enabled = false;
  await vm.runInContext('automate("schedule")', app.context);
  assert.equal(app.calls.length, 2);
});

test("expired previews and missing confirmation fail closed", async () => {
  const app = worker({ ...core.defaults(), enabled: true });
  const first = await app.send({ action: "preview", tabId: 1 });
  vm.runInContext('for (const ticket of tickets.values()) ticket.expires = 0', app.context);
  assert.equal((await app.send({ action: "execute", token: first.data.token, confirmed: true })).ok, false);
  const second = await app.send({ action: "preview", tabId: 1 });
  assert.equal((await app.send({ action: "execute", token: second.data.token })).ok, false);
  assert.equal(app.calls.length, 0);
});

test("fixed origins work even with no normal target tab", async () => {
  const settings = core.defaults();
  settings.enabled = true;
  Object.assign(settings.presets[0], { scope: "include", origins: ["https://example.com"] });
  const app = worker(settings);
  app.tabs.clear();
  const preview = await app.send({ action: "preview" });
  assert.equal(preview.ok, true);
  assert.equal((await app.send({ action: "execute", token: preview.data.token, confirmed: true })).ok, true);
  assert.equal(app.calls.length, 2);
});

test("oversized sync data is rejected before any local setting changes", async () => {
  const app = worker();
  const settings = core.defaults();
  settings.syncEnabled = true;
  settings.presets = Array.from({ length: 20 }, (_, index) => ({
    ...settings.presets[0], id: `preset-${index}`, name: "Preset name ".repeat(6), scope: "include",
    origins: Array.from({ length: 20 }, (_, origin) => `https://site-${origin}.example.com`)
  }));
  settings.activePreset = "preset-0";
  settings.automation.presetId = "preset-0";
  const result = await app.send({ action: "save", settings });
  assert.equal(result.ok, false);
  assert.match(result.error, /8 KB/);
  assert.equal(app.data.cleanerSettings.syncEnabled, false);
});

test("incoming synced presets never enable developer mode or automation", async () => {
  const settings = core.defaults();
  settings.enabled = true;
  settings.syncEnabled = true;
  settings.automation.minutes = 5;
  const app = worker(settings);
  const incoming = core.defaults();
  incoming.enabled = true;
  incoming.automation.minutes = 10;
  incoming.presets[0].name = "Remote preset";
  app.handlers.storage({ cleanerShared: { newValue: incoming } }, "sync");
  await vm.runInContext("mutation", app.context);
  assert.equal(app.data.cleanerSettings.enabled, false);
  assert.equal(app.data.cleanerSettings.automation.minutes, 0);
  assert.equal(app.data.cleanerSettings.presets[0].name, "Remote preset");
});

test("turning developer mode off during cleanup stops remaining removals and follow-up actions", async () => {
  let app;
  const settings = core.defaults();
  settings.enabled = true;
  settings.presets[0].closeWindows = true;
  app = worker(settings, async () => { app.data.cleanerSettings.enabled = false; });
  const preview = await app.send({ action: "preview", tabId: 1 });
  const result = await app.send({ action: "execute", token: preview.data.token, confirmed: true });
  assert.equal(result.ok, false);
  assert.match(result.error, /turned off/);
  assert.equal(app.calls.length, 1);
});

test("one-click cleanup runs saved cache settings on the actual active tab", async () => {
  const app = worker({ ...core.defaults(), enabled: true });
  const response = await app.send({ action: "clean", tabId: 999, presetId: "untrusted" });
  assert.equal(response.ok, true);
  assert.equal(response.data.requiresConfirmation, false);
  assert.equal(app.calls.length, 3);
  assert.deepEqual(app.calls[0].options.origins, ["https://example.com"]);
  assert.equal(app.calls[2], "reload");
});

test("one-click cleanup follows the saved selected preset rather than the first preset", async () => {
  const settings = core.defaults();
  settings.enabled = true;
  settings.presets.push({ ...settings.presets[0], id: "custom", name: "Custom cache", types: ["cacheStorage"], reload: false });
  settings.activePreset = "custom";
  const app = worker(settings);
  const response = await app.send({ action: "clean" });
  assert.equal(response.ok, true);
  assert.equal(response.data.result.name, "Custom cache");
  assert.equal(app.calls.length, 1);
  assert.deepEqual(Object.keys(app.calls[0].types), ["cacheStorage"]);
});

test("one-click cleanup never bypasses confirmation for risky presets", async () => {
  for (const overrides of [
    { types: ["cookies"] }, { types: ["localStorage"] }, { scope: "all", types: ["history"] },
    { closeWindows: true }, { scope: "all", protectedWeb: true }, { scope: "all", extension: true }
  ]) {
    const settings = core.defaults();
    settings.enabled = true;
    Object.assign(settings.presets[0], overrides);
    const app = worker(settings);
    const response = await app.send({ action: "clean" });
    assert.equal(response.ok, true);
    assert.equal(response.data.requiresConfirmation, true);
    assert.ok(response.data.token);
    assert.equal(app.calls.length, 0);
    assert.equal((await app.send({ action: "execute", token: response.data.token, confirmed: true })).ok, true);
  }
});

test("one-click cleanup on browser-internal pages fails without widening scope", async () => {
  const app = worker({ ...core.defaults(), enabled: true });
  app.tabs.set(1, { id: 1, active: true, url: "chrome://extensions" });
  assert.equal((await app.send({ action: "clean" })).ok, false);
  assert.equal(app.calls.length, 0);
});

test("one-click cleanup failure reports partial removal and skips reload", async () => {
  const app = worker({ ...core.defaults(), enabled: true }, async (_options, types) => {
    if (types.cacheStorage) throw new Error("Policy denied");
  });
  const response = await app.send({ action: "clean" });
  assert.equal(response.ok, false);
  assert.match(response.error, /Completed: cache/);
  assert.equal(app.calls.length, 2);
});