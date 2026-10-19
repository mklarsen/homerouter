"use strict";

const core = globalThis.CleanerCore;
const $ = id => document.getElementById(id);
const isPopup = document.body.dataset.view === "popup";
let state;
let ticket;
let dirty = false;
let busy = false;

document.getElementById("app").innerHTML = `
  <div class="toolbar">
    <span id="mode-label" class="mode-indicator" role="status" aria-live="polite">Inactive</span>
    <button class="icon popup-only" id="options" title="Open setup" aria-label="Open setup"><img src="icons/settings.svg" alt=""></button>
  </div>
  <header>
    <img src="icons/icon48.png" alt="homerouter.io">
    <div><div class="brand">homerouter.io</div><h1>DeClean</h1></div>
    <span class="badge">POC / 0.1</span>
  </header>
  <div class="mode options-only"><div><strong>Developer mode</strong></div>
    <input class="switch" type="checkbox" id="enabled" aria-label="Developer mode">
  </div>
  <div id="pending" class="pending" hidden></div>
  <p id="status" role="status" aria-live="polite"></p>
  <div class="layout"><div>
    <section>
      <div class="options-only">
        <div class="section-heading"><h2>Cleanup</h2></div>
        <label>Preset<select id="preset"></select></label>
        <label>Target tab<select id="target"></select></label>
      </div>
      <div class="clean-frame popup-only"><button class="primary" id="clean"><img src="icons/trash-2.svg" alt=""><span id="clean-label">Clean</span></button></div>
      <div id="summary" class="summary"></div>
      <button class="primary options-only" id="preview"><img src="icons/trash-2.svg" alt="">Review cleanup</button>
    </section>
    <section class="options-only">
      <div class="section-heading"><h2>Preset settings</h2><div class="tools">
        <button class="icon" id="add" title="Add preset" aria-label="Add preset"><img src="icons/plus.svg" alt=""></button>
        <button class="icon" id="duplicate" title="Duplicate preset" aria-label="Duplicate preset"><img src="icons/copy.svg" alt=""></button>
        <button class="icon danger" id="delete" title="Delete preset" aria-label="Delete preset"><img src="icons/trash-2.svg" alt=""></button>
      </div></div>
      <form id="editor">
        <label>Name<input id="name" maxlength="80" required></label>
        <div class="row">
          <label>Scope<select id="scope"><option value="active">Target tab origin</option><option value="include">Only listed origins</option><option value="exclude">All except listed origins</option><option value="all">Whole browser</option></select></label>
          <label>Time range<select id="range"><option value="quarter">Last 15 minutes</option><option value="hour">Last hour</option><option value="day">Last 24 hours</option><option value="week">Last 7 days</option><option value="month">Last 30 days</option><option value="all">All time</option></select></label>
        </div>
        <label id="origins-label">Origins<textarea id="origins" placeholder="https://example.com&#10;localhost:3000" spellcheck="false"></textarea></label>
        <h3>Data types</h3><div id="types" class="checks"></div>
        <h3>Per-type rules</h3><div id="rules" class="rules"></div>
        <h3>After clearing</h3><div class="checks">
          <label class="check"><input type="checkbox" id="reload">Reload target tab</label>
          <label class="check risk"><input type="checkbox" id="closeWindows">Close all normal windows</label>
        </div>
        <h3>Additional origin types</h3><div class="checks">
          <label class="check risk"><input type="checkbox" id="protectedWeb">Installed web apps</label>
          <label class="check risk"><input type="checkbox" id="extension">Extension / packaged app data</label>
        </div>
        <button class="primary" type="submit"><img src="icons/check.svg" alt="">Save preset</button>
      </form>
    </section>
  </div><aside>
    <section class="options-only">
      <div class="section-heading"><h2>Automation</h2></div>
      <form id="automation-form">
        <label>Preset<select id="auto-preset"></select></label>
        <label>Interval (minutes; 0 = off)<input type="number" id="minutes" min="0" max="10080" value="0"></label>
        <label class="check"><input type="checkbox" id="startup">On browser startup</label>
        <label class="check"><input type="checkbox" id="tabClose">After a tab closes</label>
        <button type="submit"><img src="icons/check.svg" alt="">Save automation</button>
      </form>
      <p id="auto-safety" class="warning muted"></p>
    </section>
    <section class="options-only">
      <div class="section-heading"><h2>Preferences</h2></div>
      <label>Appearance<select id="theme"><option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option></select></label>
      <label class="check"><input type="checkbox" id="sync">Sync presets &amp; appearance</label>
      <div class="tools">
        <button class="icon" id="export" title="Export settings" aria-label="Export settings"><img src="icons/download.svg" alt=""></button>
        <button class="icon" id="import" title="Import settings" aria-label="Import settings"><img src="icons/upload.svg" alt=""></button>
        <button class="icon" id="browser" title="Import browser cleanup settings" aria-label="Import browser cleanup settings"><img src="icons/rotate-cw.svg" alt=""></button>
      </div>
      <input type="file" id="import-file" accept="application/json,.json" hidden>
    </section>
    <section class="options-only"><div class="section-heading"><h2>Recent activity</h2></div><div id="activity"></div></section>
    <section class="options-only" aria-labelledby="about-title">
      <div class="section-heading"><h2 id="about-title">About DeClean</h2></div>
      <p class="summary"><strong>DeClean</strong> is short for <strong>Developer Cleaner</strong>.</p>
      <p class="muted">By homerouter.io.</p>
    </section>
  </aside></div>
  <footer class="popup-only"><span>by homerouter.io</span><span>POC / 0.1</span></footer>
  <dialog id="review" aria-labelledby="review-title">
    <div class="section-heading"><h2 id="review-title">Confirm cleanup</h2></div>
    <div id="review-calls"></div><div id="review-warnings" class="warning"></div>
    <label class="check"><input type="checkbox" id="ack">Permanently remove the listed data. This cannot be undone.</label>
    <div class="row"><button id="cancel">Cancel</button><button class="primary" id="confirm" disabled><img src="icons/trash-2.svg" alt="">Clear now</button></div>
  </dialog>`;

function status(message, error = false) {
  $("status").textContent = message;
  $("status").dataset.error = String(error);
}

async function request(action, values = {}) {
  if (!globalThis.chrome?.runtime?.sendMessage) throw new Error("Load this folder as an unpacked Chrome extension to connect to browser data.");
  const response = await chrome.runtime.sendMessage({ action, ...values });
  if (!response?.ok) throw new Error(response?.error || "The extension worker did not respond.");
  return response.data;
}

function option(select, value, text) {
  const entry = document.createElement("option");
  entry.value = value;
  entry.textContent = text;
  select.append(entry);
}

function selected() {
  return state.settings.presets.find(preset => preset.id === state.settings.activePreset);
}

function list(value) {
  return value.split(/[\n,]+/).map(entry => entry.trim()).filter(Boolean);
}

function updateScope() {
  $("origins-label").hidden = !["include", "exclude"].includes($("scope").value);
}

function fillEditor() {
  const preset = selected();
  for (const key of ["name", "scope", "range"]) $(key).value = preset[key];
  $("origins").value = preset.origins.join("\n");
  for (const key of ["reload", "closeWindows", "protectedWeb", "extension"]) $(key).checked = preset[key];
  $("types").replaceChildren();
  $("rules").replaceChildren();
  for (const [type, name] of Object.entries(core.TYPES)) {
    const label = document.createElement("label");
    label.className = `check${["history", "downloads", "formData"].includes(type) ? " risk" : ""}`;
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.id = `type-${type}`;
    checkbox.checked = preset.types.includes(type);
    label.append(checkbox, document.createTextNode(name));
    $("types").append(label);
    const row = document.createElement("div");
    row.className = "rule";
    row.id = `rule-${type}`;
    const title = document.createElement("span");
    title.textContent = name;
    const scope = document.createElement("select");
    scope.id = `scope-${type}`;
    scope.setAttribute("aria-label", `${name} scope`);
    for (const [value, text] of [["inherit", "Use preset scope"], ["active", "Target origin"], ["include", "Only listed"], ["exclude", "All except listed"], ["all", "Whole browser"]]) option(scope, value, text);
    scope.value = preset.typeRules[type]?.scope || "inherit";
    const origins = document.createElement("input");
    origins.id = `origins-${type}`;
    origins.placeholder = "Origins, comma-separated";
    origins.setAttribute("aria-label", `${name} origins`);
    origins.value = preset.typeRules[type]?.origins.join(", ") || "";
    const update = () => {
      row.hidden = !checkbox.checked;
      origins.hidden = !["include", "exclude"].includes(scope.value);
    };
    checkbox.addEventListener("change", update);
    scope.addEventListener("change", update);
    row.append(title, scope, origins);
    $("rules").append(row);
    update();
  }
  dirty = false;
  updateScope();
}

function renderMode() {
  const mode = !state.settings.enabled ? "inactive" : state.running ? "busy" : state.pendingReview ? "pending" : "active";
  document.body.dataset.mode = mode;
  $("mode-label").textContent = { inactive: "Inactive", active: "Active", busy: "Clearing", pending: "Needs review" }[mode];
  $("clean-label").textContent = state.running ? "Cleaning..." : "Clean";
  $("clean").disabled = !state.settings.enabled || state.running;
}

function render() {
  const settings = state.settings;
  document.documentElement.dataset.theme = settings.theme;
  $("enabled").checked = settings.enabled;
  renderMode();
  $("preview").disabled = !settings.enabled || state.running;
  $("preset").replaceChildren();
  $("auto-preset").replaceChildren();
  for (const preset of settings.presets) {
    option($("preset"), preset.id, preset.name);
    option($("auto-preset"), preset.id, preset.name);
  }
  $("preset").value = settings.activePreset;
  $("auto-preset").value = settings.automation.presetId;
  const previousTarget = $("target").value;
  $("target").replaceChildren();
  if (!state.tabs.length) option($("target"), "", "No website tabs");
  for (const tab of state.tabs) option($("target"), String(tab.id), `${new URL(tab.url).host} | ${tab.title || "Untitled"}`);
  if (state.tabs.some(tab => String(tab.id) === previousTarget)) $("target").value = previousTarget;
  else if (state.tabs.some(tab => tab.active)) $("target").value = String(state.tabs.find(tab => tab.active).id);
  $("summary").textContent = isPopup ? selected().name : `${selected().types.map(type => core.TYPES[type]).join(", ")} / ${selected().scope === "active" ? "target origin" : selected().scope === "all" ? "whole browser" : `${selected().scope} origins`}`;
  $("theme").value = settings.theme;
  $("sync").checked = settings.syncEnabled;
  $("minutes").value = settings.automation.minutes;
  $("startup").checked = settings.automation.startup;
  $("tabClose").checked = settings.automation.tabClose;
  $("auto-safety").textContent = "Only cache-only presets without target-tab rules run unattended. Other triggers queue manual approval (!).";
  $("pending").hidden = !state.pendingReview;
  if (state.pendingReview) $("pending").textContent = isPopup ? "Automation approval pending in setup." : `Approval pending: ${state.pendingReview.source}. Select the preset and review cleanup.`;
  $("activity").replaceChildren();
  if (!state.activity.length) $("activity").textContent = "No cleanups yet.";
  for (const item of state.activity.slice(0, document.body.dataset.view === "popup" ? 1 : 10)) {
    const row = document.createElement("div");
    row.className = "activity-item";
    row.dataset.result = item.ok ? "success" : "error";
    const title = document.createElement("strong");
    title.textContent = `${item.ok ? "Completed" : "Failed"}: ${item.name}`;
    const time = document.createElement("time");
    time.textContent = `${new Date(item.at).toLocaleString()} / ${item.source}`;
    row.append(title, time);
    if (item.error) row.append(document.createElement("br"), document.createTextNode(item.error));
    $("activity").append(row);
  }
  $("delete").disabled = settings.presets.length === 1;
  fillEditor();
}

async function refresh() {
  state = await request("state");
  render();
}

async function save(settings, message = "Saved.") {
  try { await request("save", { settings }); }
  finally { await refresh(); }
  status(message);
}

function on(id, event, handler) {
  $(id).addEventListener(event, async event => {
    event.preventDefault();
    const ownsLock = id !== "enabled";
    if (busy && ownsLock) return;
    if (ownsLock) busy = true;
    try { await handler(event); }
    catch (error) { status(error.message, true); }
    finally { if (ownsLock) busy = false; }
  });
}

on("enabled", "change", async () => {
  await save({ ...state.settings, enabled: $("enabled").checked }, $("enabled").checked ? "Developer mode enabled." : "Developer mode disabled. Automation stopped.");
});
on("options", "click", () => chrome.runtime.openOptionsPage());
on("preset", "change", async () => {
  if (dirty && !confirm("Discard unsaved preset changes?")) { $("preset").value = state.settings.activePreset; return; }
  await save({ ...state.settings, activePreset: $("preset").value });
});
$("editor").addEventListener("input", () => { dirty = true; });
$("scope").addEventListener("change", updateScope);
on("editor", "submit", async () => {
  const preset = { ...selected(), name: $("name").value, scope: $("scope").value, range: $("range").value, origins: list($("origins").value), types: [], typeRules: {} };
  for (const key of ["reload", "closeWindows", "protectedWeb", "extension"]) preset[key] = $(key).checked;
  for (const type of Object.keys(core.TYPES)) {
    if (!$(`type-${type}`).checked) continue;
    preset.types.push(type);
    const scope = $(`scope-${type}`).value;
    if (scope !== "inherit") preset.typeRules[type] = { scope, origins: list($(`origins-${type}`).value) };
  }
  core.preset(preset);
  await save({ ...state.settings, presets: state.settings.presets.map(entry => entry.id === preset.id ? preset : entry) });
});
for (const action of ["add", "duplicate"]) on(action, "click", async () => {
  const preset = { ...(action === "add" ? core.defaults().presets[0] : selected()), id: crypto.randomUUID(), name: action === "add" ? "New preset" : `${selected().name.slice(0, 70)} copy` };
  await save({ ...state.settings, activePreset: preset.id, presets: [...state.settings.presets, preset] });
});
on("delete", "click", async () => {
  if (!confirm(`Delete preset "${selected().name}"?`)) return;
  const presets = state.settings.presets.filter(preset => preset.id !== selected().id);
  const automation = { ...state.settings.automation };
  if (automation.presetId === selected().id) Object.assign(automation, { presetId: presets[0].id, minutes: 0, startup: false, tabClose: false });
  await save({ ...state.settings, presets, activePreset: presets[0].id, automation });
});
on("theme", "change", () => save({ ...state.settings, theme: $("theme").value }));
on("sync", "change", async () => {
  if ($("sync").checked && !confirm("Sync preset names, origin lists and appearance through your Chrome account? Developer mode and automation stay local.")) { $("sync").checked = false; return; }
  await save({ ...state.settings, syncEnabled: $("sync").checked });
});
on("automation-form", "submit", async () => {
  const automation = { presetId: $("auto-preset").value, minutes: Number($("minutes").value), startup: $("startup").checked, tabClose: $("tabClose").checked };
  if ((automation.minutes || automation.startup || automation.tabClose) && !confirm("Enable these cleanup triggers? Cache-only presets without target-tab rules can permanently clear cache without another prompt. Other presets require manual review.")) return;
  await save({ ...state.settings, automation });
});
on("browser", "click", async () => {
  const preset = await request("import-browser");
  await save({ ...state.settings, presets: [...state.settings.presets, preset], activePreset: preset.id }, "Browser settings imported. Time range mapped to the nearest available range; review before use.");
});
on("export", "click", () => {
  const settings = { ...state.settings, enabled: false, syncEnabled: false, automation: { ...core.defaults().automation, presetId: state.settings.activePreset } };
  const url = URL.createObjectURL(new Blob([JSON.stringify(settings, null, 2)], { type: "application/json" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "declean-settings.json";
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
});
on("import", "click", () => $("import-file").click());
on("import-file", "change", async () => {
  const file = $("import-file").files[0];
  if (!file) return;
  if (file.size > 100000) throw new Error("Settings file must be under 100 KB.");
  const imported = core.settings(JSON.parse(await file.text()));
  if (!confirm("Replace local presets and preferences? Developer mode, sync and automation will be disabled.")) return;
  await save({ ...imported, enabled: false, syncEnabled: false, automation: { ...core.defaults().automation, presetId: imported.activePreset } }, "Imported. Developer mode and automation are off.");
  $("import-file").value = "";
});
on("preview", "click", async () => {
  if (dirty) throw new Error("Save preset changes before reviewing cleanup.");
  showReview(await request("preview", { presetId: selected().id, tabId: $("target").value ? Number($("target").value) : undefined }));
});

function showReview(review) {
  ticket = review;
  $("review-calls").replaceChildren();
  for (const call of ticket.plan.calls) {
    const row = document.createElement("div");
    row.className = "review-call";
    const title = document.createElement("strong");
    title.textContent = core.TYPES[call.type];
    const scope = call.options.origins ? `Only: ${call.options.origins.join(", ")}` : call.options.excludeOrigins ? `ALL sites EXCEPT: ${call.options.excludeOrigins.join(", ")}` : "WHOLE BROWSER";
    row.append(title, document.createTextNode(scope), document.createElement("br"), document.createTextNode(call.options.since ? `Since ${new Date(call.options.since).toLocaleString()}` : "All time"));
    $("review-calls").append(row);
  }
  $("review-warnings").replaceChildren();
  for (const warning of ticket.plan.warnings) {
    const paragraph = document.createElement("p");
    paragraph.textContent = warning;
    $("review-warnings").append(paragraph);
  }
  $("ack").checked = false;
  $("confirm").disabled = true;
  $("review").showModal();
}

$("ack").addEventListener("change", () => { $("confirm").disabled = !$("ack").checked; });
on("cancel", "click", () => { ticket = null; $("review").close(); });
on("confirm", "click", async () => {
  if (!$("ack").checked || !ticket) return;
  $("confirm").disabled = true;
  $("review").close();
  await performCleanup("execute", { token: ticket.token, confirmed: true });
});
on("clean", "click", () => performCleanup("clean"));

async function performCleanup(action, values = {}) {
  $("preview").disabled = true;
  state.running = true;
  renderMode();
  status(action === "clean" ? "Preparing cleanup..." : "Clearing data...");
  try {
    const response = await request(action, values);
    if (response.requiresConfirmation) {
      showReview(response);
      status("");
    } else {
      ticket = null;
      await refresh();
      status(response.result?.note || response.note || "Cleanup completed.");
    }
  } finally {
    state.running = false;
    renderMode();
    $("preview").disabled = !state.settings.enabled;
  }
}

refresh().catch(error => {
  state = { settings: core.defaults(), tabs: [], activity: [], pendingReview: null, running: false };
  render();
  document.querySelectorAll("button, input, select, textarea").forEach(control => { control.disabled = true; });
  status(error.message, true);
});