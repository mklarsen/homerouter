"use strict";

importScripts("core.js");
const core = globalThis.CleanerCore;
const ALARM = "developer-cleaner";
const tickets = new Map();
let running = false;
let mutation = Promise.resolve();

async function readSettings() {
  const { cleanerSettings } = await chrome.storage.local.get("cleanerSettings");
  return cleanerSettings ? core.settings(cleanerSettings) : core.defaults();
}

function sharedSettings(settings) {
  return {
    ...settings, enabled: false, syncEnabled: true,
    automation: { minutes: 0, startup: false, tabClose: false, presetId: settings.activePreset }
  };
}

async function configure(settings) {
  await chrome.alarms.clear(ALARM);
  if (settings.enabled && settings.automation.minutes) {
    await chrome.alarms.create(ALARM, { periodInMinutes: settings.automation.minutes });
  }
  await chrome.action.setBadgeBackgroundColor({ color: settings.enabled ? "#16866d" : "#747b79" });
  const { pendingReview } = await chrome.storage.local.get("pendingReview");
  await chrome.action.setBadgeText({ text: settings.enabled ? (pendingReview ? "!" : "DEV") : "" });
}

async function saveSettings(value) {
  const settings = core.settings(value);
  if (settings.syncEnabled && new TextEncoder().encode(JSON.stringify(sharedSettings(settings))).length + 32 > 8192) {
    throw new Error("Presets exceed Chrome's 8 KB sync item limit. Reduce origin lists or turn sync off.");
  }
  tickets.clear();
  await chrome.storage.local.set({ cleanerSettings: settings, pendingReview: null });
  await configure(settings);
  if (settings.syncEnabled) await chrome.storage.sync.set({ cleanerShared: sharedSettings(settings) });
  else await chrome.storage.sync.remove("cleanerShared");
  return settings;
}

async function targetTab(tabId) {
  if (Number.isInteger(tabId)) return chrome.tabs.get(tabId);
  const [tab] = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
  return tab;
}

function selectedPreset(settings, presetId) {
  const selected = settings.presets.find(entry => entry.id === (presetId || settings.activePreset));
  if (!selected) throw new Error("Preset not found.");
  return selected;
}

async function preview(message) {
  const settings = await readSettings();
  if (!settings.enabled) throw new Error("Developer mode is off.");
  if (running) throw new Error("A cleanup is already running.");
  const tab = await targetTab(message.tabId);
  const selected = selectedPreset(settings, message.presetId);
  const plan = core.plan(selected, tab?.url);
  const token = crypto.randomUUID();
  tickets.clear();
  const requiresActive = selected.types.some(type => (selected.typeRules[type] || selected).scope === "active");
  tickets.set(token, { plan, requiresActive, tabId: tab?.id, url: tab?.url, settings: JSON.stringify(settings), expires: Date.now() + 60000 });
  return { token, plan };
}

async function record(result) {
  const { activity = [] } = await chrome.storage.local.get("activity");
  await chrome.storage.local.set({ activity: [result, ...activity].slice(0, 20) });
}

async function removeData(plan, tabId, source) {
  if (running) throw new Error("A cleanup is already running.");
  running = true;
  const completed = [];
  let result;
  try {
    for (const call of plan.calls) {
      if (!(await readSettings()).enabled) throw new Error("Developer mode was turned off; remaining steps stopped.");
      await chrome.browsingData.remove(call.options, { [call.type]: true });
      completed.push(call.type);
    }
    result = { at: Date.now(), name: plan.name, source, completed, ok: true };
    await record(result);
    await chrome.storage.local.set({ pendingReview: null });
    const followUpEnabled = (await readSettings()).enabled;
    if (plan.closeWindows && followUpEnabled) {
      const windows = await chrome.windows.getAll({ windowTypes: ["normal"] });
      await Promise.all(windows.filter(window => !window.incognito).map(window => chrome.windows.remove(window.id)));
    } else if (plan.reload && !plan.closeWindows && followUpEnabled && Number.isInteger(tabId)) {
      try { await chrome.tabs.reload(tabId, { bypassCache: true }); }
      catch { result.note = "Data cleared; the target tab could not be reloaded."; }
    }
    return result;
  } catch (error) {
    result = { at: Date.now(), name: plan.name, source, completed, ok: false, error: error.message };
    await record(result);
    throw new Error(`${error.message} Completed: ${completed.join(", ") || "none"}. No rollback is possible.`);
  } finally {
    running = false;
    await configure(await readSettings());
  }
}

async function execute(message) {
  const ticket = tickets.get(message.token);
  tickets.delete(message.token);
  if (!ticket || ticket.expires < Date.now() || message.confirmed !== true) throw new Error("Review and confirm a fresh preview first.");
  const settings = await readSettings();
  if (!settings.enabled || JSON.stringify(settings) !== ticket.settings) throw new Error("Settings changed. Review a new preview.");
  if (ticket.requiresActive) {
    const tab = await targetTab(ticket.tabId);
    if (tab?.url !== ticket.url) throw new Error("The target tab changed. Review a new preview.");
  }
  return removeData(ticket.plan, ticket.tabId, "manual");
}

async function clean() {
  const review = await preview({});
  if (!review.plan.automaticSafe) return { requiresConfirmation: true, ...review };
  const result = await execute({ token: review.token, confirmed: true });
  return { requiresConfirmation: false, result };
}

async function automate(source) {
  const settings = await readSettings();
  if (!settings.enabled || running) return;
  if (source === "startup" && !settings.automation.startup) return;
  if (source === "tab-close" && !settings.automation.tabClose) return;
  if (source === "schedule" && !settings.automation.minutes) return;
  const selected = selectedPreset(settings, settings.automation.presetId);
  const usesActive = selected.types.some(type => (selected.typeRules[type] || selected).scope === "active");
  if (usesActive || !core.plan(selected).automaticSafe) {
    await chrome.storage.local.set({ pendingReview: { source, presetId: selected.id, at: Date.now() } });
    await chrome.action.setBadgeText({ text: "!" });
    return;
  }
  const plan = core.plan(selected);
  plan.reload = false;
  await removeData(plan, undefined, source);
}

function serialized(operation) {
  const next = mutation.then(operation);
  mutation = next.catch(() => {});
  return next;
}

async function handle(message) {
  if (message.action === "state") {
    const settings = await readSettings();
    const { activity = [], pendingReview = null } = await chrome.storage.local.get(["activity", "pendingReview"]);
    const tabs = (await chrome.tabs.query({})).filter(tab => /^https?:\/\//.test(tab.url || "") && !tab.incognito)
      .map(tab => ({ id: tab.id, title: tab.title, url: tab.url, active: tab.active }));
    return { settings, activity, pendingReview, tabs, running };
  }
  if (message.action === "save") return serialized(() => saveSettings(message.settings));
  if (message.action === "preview") return preview(message);
  if (message.action === "execute") return execute(message);
  if (message.action === "clean") return clean();
  if (message.action === "import-browser") {
    const browser = await chrome.browsingData.settings();
    const settings = await readSettings();
    const types = Object.keys(core.TYPES).filter(type => browser.dataToRemove?.[type] && browser.dataRemovalPermitted?.[type] !== false);
    if (!types.length) throw new Error("No supported browser data types were selected.");
    const since = browser.options?.since || 0;
    const range = since === 0 ? "all" : Object.keys(core.RANGES).filter(key => key !== "all")
      .sort((left, right) => Math.abs(core.RANGES[left] - (Date.now() - since)) - Math.abs(core.RANGES[right] - (Date.now() - since)))[0];
    return core.preset({ ...settings.presets[0], id: `browser-${Date.now()}`, name: "Browser settings", types,
      range, scope: "all", origins: [], typeRules: {}, reload: false, closeWindows: false, protectedWeb: false, extension: false });
  }
  throw new Error("Unknown request.");
}

chrome.runtime.onMessage.addListener((message, sender, respond) => {
  if (sender.id !== chrome.runtime.id || !sender.url?.startsWith(chrome.runtime.getURL(""))) return false;
  handle(message).then(data => respond({ ok: true, data }), error => respond({ ok: false, error: error.message }));
  return true;
});

async function openReview() {
  await chrome.runtime.openOptionsPage();
}

function reportError(error) {
  console.error("DeClean:", error);
  record({ at: Date.now(), name: "Automation", source: "background", completed: [], ok: false, error: error.message }).catch(console.error);
}

chrome.runtime.onInstalled.addListener(() => {
  serialized(async () => {
    await saveSettings(await readSettings());
    await chrome.contextMenus.removeAll();
    chrome.contextMenus.create({ id: "review-cleanup", title: "DeClean: review cleanup", contexts: ["page", "action"] });
  }).catch(reportError);
});
chrome.runtime.onStartup.addListener(() => {
  serialized(async () => { await configure(await readSettings()); await automate("startup"); }).catch(reportError);
});
chrome.alarms.onAlarm.addListener(alarm => {
  if (alarm.name === ALARM) serialized(() => automate("schedule")).catch(reportError);
});
chrome.tabs.onRemoved.addListener((_tabId, info) => {
  if (!info.isWindowClosing) serialized(() => automate("tab-close")).catch(reportError);
});
chrome.commands.onCommand.addListener(command => {
  if (command === "review-cleanup") openReview().catch(reportError);
});
chrome.contextMenus.onClicked.addListener(info => {
  if (info.menuItemId === "review-cleanup") openReview().catch(reportError);
});
chrome.storage.onChanged.addListener((changes, area) => {
  if (area !== "sync" || !changes.cleanerShared?.newValue) return;
  serialized(async () => {
    const current = await readSettings();
    if (!current.syncEnabled) return;
    const incoming = core.settings(sharedSettings(core.settings(changes.cleanerShared.newValue)));
    if (JSON.stringify(incoming) === JSON.stringify(sharedSettings(current))) return;
    tickets.clear();
    await chrome.storage.local.set({ cleanerSettings: incoming, pendingReview: null });
    await configure(incoming);
  }).catch(reportError);
});