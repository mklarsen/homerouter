(() => {
  "use strict";

  const TYPES = {
    cache: "HTTP cache",
    cacheStorage: "Cache Storage",
    cookies: "Cookies",
    localStorage: "Local storage",
    indexedDB: "IndexedDB",
    serviceWorkers: "Service workers",
    fileSystems: "File systems",
    history: "Browsing history",
    downloads: "Download history",
    formData: "Autofill data"
  };
  const GLOBAL_TYPES = new Set(["history", "downloads", "formData"]);
  const RANGES = { quarter: 900000, hour: 3600000, day: 86400000, week: 604800000, month: 2592000000, all: 0 };
  const SCOPES = ["active", "include", "exclude", "all"];

  function defaults() {
    return {
      version: 1,
      enabled: false,
      theme: "dark",
      syncEnabled: false,
      activePreset: "cache",
      presets: [{
        id: "cache", name: "Cache refresh", scope: "active", origins: [],
        types: ["cache", "cacheStorage"], typeRules: {}, range: "hour",
        reload: true, closeWindows: false, protectedWeb: false, extension: false
      }],
      automation: { minutes: 0, startup: false, tabClose: false, presetId: "cache" }
    };
  }

  function origins(values) {
    if (!Array.isArray(values) || values.length > 100) throw new Error("Use at most 100 exact origins.");
    return [...new Set(values.flatMap(value => {
      if (typeof value !== "string" || !value.trim()) throw new Error("An origin cannot be empty.");
      const text = value.trim();
      const candidates = text.includes("://") ? [text] : [`http://${text}`, `https://${text}`];
      return candidates.map(candidate => {
        let url;
        try { url = new URL(candidate); } catch { throw new Error(`Invalid origin: ${text}`); }
        if (!["http:", "https:"].includes(url.protocol) || url.username || url.password ||
            url.pathname !== "/" || url.search || url.hash || url.hostname.includes("*")) {
          throw new Error(`Use an exact HTTP(S) origin without a path: ${text}`);
        }
        return url.origin;
      });
    }))];
  }

  function rule(value) {
    if (!value || !SCOPES.includes(value.scope)) throw new Error("Choose a valid scope.");
    const normalized = { scope: value.scope, origins: origins(value.origins || []) };
    if (["include", "exclude"].includes(normalized.scope) && !normalized.origins.length) {
      throw new Error("Domain filters must contain at least one origin.");
    }
    return normalized;
  }

  function preset(value) {
    if (!value || typeof value.id !== "string" || !/^[a-zA-Z0-9_-]{1,64}$/.test(value.id)) throw new Error("Invalid preset ID.");
    if (typeof value.name !== "string" || !value.name.trim() || value.name.length > 80) throw new Error("Use a preset name of 1-80 characters.");
    if (!Array.isArray(value.types) || !value.types.length || value.types.some(type => !Object.hasOwn(TYPES, type))) {
      throw new Error("Select at least one supported data type.");
    }
    if (!Object.hasOwn(RANGES, value.range)) throw new Error("Choose a valid time range.");
    const typeRules = {};
    for (const [type, entry] of Object.entries(value.typeRules || {})) {
      if (!Object.hasOwn(TYPES, type)) throw new Error(`Unsupported data type: ${type}`);
      typeRules[type] = rule(entry);
    }
    const result = {
      ...rule(value), id: value.id, name: value.name.trim(), types: [...new Set(value.types)], typeRules,
      range: value.range, reload: value.reload === true, closeWindows: value.closeWindows === true,
      protectedWeb: value.protectedWeb === true, extension: value.extension === true
    };
    for (const type of result.types) {
      const selected = result.typeRules[type] || result;
      if (GLOBAL_TYPES.has(type) && selected.scope !== "all") {
        throw new Error(`${TYPES[type]} requires whole-browser scope; Chrome cannot filter it by domain.`);
      }
      if ((result.extension || result.protectedWeb) && selected.scope !== "all") {
        throw new Error("App/extension data requires whole-browser scope for every selected type.");
      }
    }
    return result;
  }

  function settings(value) {
    if (!value || value.version !== 1) throw new Error("Unsupported settings format.");
    if (!Array.isArray(value.presets) || !value.presets.length || value.presets.length > 20) throw new Error("Use 1-20 presets.");
    const presets = value.presets.map(preset);
    if (new Set(presets.map(entry => entry.id)).size !== presets.length) throw new Error("Preset IDs must be unique.");
    if (!presets.some(entry => entry.id === value.activePreset)) throw new Error("Select an existing preset.");
    const automation = value.automation || {};
    const minutes = Number(automation.minutes || 0);
    if (!Number.isInteger(minutes) || (minutes !== 0 && (minutes < 1 || minutes > 10080))) throw new Error("Schedule must be off or 1-10080 minutes.");
    if (!presets.some(entry => entry.id === automation.presetId)) throw new Error("Select an automation preset.");
    if (! ["system", "light", "dark"].includes(value.theme)) throw new Error("Choose a valid theme.");
    return {
      version: 1, enabled: value.enabled === true, syncEnabled: value.syncEnabled === true,
      theme: value.theme, activePreset: value.activePreset, presets,
      automation: { minutes, startup: automation.startup === true, tabClose: automation.tabClose === true, presetId: automation.presetId }
    };
  }

  function plan(value, activeUrl, now = Date.now()) {
    const selected = preset(value);
    const since = selected.range === "all" ? 0 : now - RANGES[selected.range];
    const calls = selected.types.map(type => {
      const filter = selected.typeRules[type] || selected;
      const options = { since, originTypes: { unprotectedWeb: true, protectedWeb: selected.protectedWeb, extension: selected.extension } };
      if (filter.scope === "active") {
        let url;
        try { url = new URL(activeUrl); } catch { throw new Error("Open a normal HTTP(S) website first."); }
        if (!["http:", "https:"].includes(url.protocol)) throw new Error("The active tab is not an HTTP(S) website.");
        options.origins = [url.origin];
      } else if (filter.scope === "include") options.origins = filter.origins;
      else if (filter.scope === "exclude") options.excludeOrigins = filter.origins;
      return { type, options };
    });
    const warnings = [];
    if (selected.types.includes("cookies")) warnings.push("Cookie removal affects the entire registrable domain, including sibling subdomains, and can sign you out.");
    if (selected.types.some(type => ["localStorage", "indexedDB", "fileSystems", "serviceWorkers"].includes(type))) {
      warnings.push("Website storage may include unsaved work. Chrome may clear storage regardless of the chosen time range.");
    }
    if (selected.protectedWeb || selected.extension) warnings.push("Installed app or extension data will be permanently removed. Extension settings may also be lost.");
    if (selected.closeWindows) warnings.push("ALL normal browser windows will close after clearing. Unsaved work can be lost.");
    return {
      name: selected.name, calls, warnings, reload: selected.reload, closeWindows: selected.closeWindows,
      automaticSafe: !selected.closeWindows && !selected.extension && !selected.protectedWeb &&
        selected.types.every(type => ["cache", "cacheStorage"].includes(type))
    };
  }

  const api = { TYPES, RANGES, defaults, origins, preset, settings, plan };
  if (typeof module !== "undefined") module.exports = api;
  else globalThis.CleanerCore = api;
})();