let proxyConfig = {
  host: "vpn.homerouter.io",
  port: 443,
  username: "",
  password: ""
};

let testCredentials = null;
let testRequestURL = null;
let testAuthChallenges = 0;
let testAuthRejected = false;
const proxyAuthAttempts = new Map();

function hasProxyCredentials(credentials) {
  return Boolean(
    credentials &&
    typeof credentials.username === "string" &&
    credentials.username.trim().length > 0 &&
    typeof credentials.password === "string" &&
    credentials.password.length >= 12
  );
}

function hasValidProxyTarget(host, port) {
  return typeof host === "string" && host.trim().length > 0 &&
    Number.isInteger(port) && port > 0 && port <= 65535;
}

function credentialsForRequest(details, temporaryCredentials, temporaryURL, savedCredentials) {
  return temporaryCredentials && details.url === temporaryURL
    ? temporaryCredentials
    : savedCredentials;
}

function loadStoredConfig() {
  chrome.storage.local.get(["proxyHost", "proxyPort", "proxyUser", "proxyPass"], (data) => {
    if (chrome.runtime.lastError) {
      console.error("[homerouter] Failed to load saved proxy settings:", chrome.runtime.lastError.message);
      return;
    }

    const stored = data || {};
    if (stored.proxyHost) proxyConfig.host = stored.proxyHost;
    if (stored.proxyPort) proxyConfig.port = parseInt(stored.proxyPort, 10);
    if (stored.proxyUser) proxyConfig.username = stored.proxyUser;
    if (stored.proxyPass) proxyConfig.password = stored.proxyPass;
  });
}

loadStoredConfig();

chrome.storage.onChanged.addListener((changes, area) => {
  if (area === "local") {
    if (changes.proxyHost) proxyConfig.host = changes.proxyHost.newValue;
    if (changes.proxyPort) proxyConfig.port = parseInt(changes.proxyPort.newValue, 10);
    if (changes.proxyUser) proxyConfig.username = changes.proxyUser.newValue;
    if (changes.proxyPass) proxyConfig.password = changes.proxyPass.newValue;
  }
});

function applyProxy(host, port, callback) {
  const config = {
    mode: "fixed_servers",
    rules: {
      singleProxy: {
        scheme: "https",
        host: host,
        port: port
      },
      bypassList: ["localhost", "127.0.0.1", "vpn.homerouter.io"]
    }
  };
  chrome.proxy.settings.set({ value: config, scope: "regular" }, () => {
    callback(chrome.runtime.lastError ? chrome.runtime.lastError.message : null);
  });
}

function clearProxy(callback) {
  chrome.proxy.settings.clear({ scope: "regular" }, () => {
    callback(chrome.runtime.lastError ? chrome.runtime.lastError.message : null);
  });
}

async function requestProxyDisconnect(credentials) {
  if (!hasProxyCredentials(credentials)) {
    return { remoteCloseConfirmed: false, closedConnections: 0 };
  }

  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 3000);
  try {
    const response = await fetch("https://vpn.homerouter.io/api/v1/proxy/disconnect", {
      method: "POST",
      cache: "no-store",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: credentials.username, password: credentials.password }),
      signal: controller.signal
    });
    const result = await response.json().catch(() => ({}));
    if (!response.ok) return { remoteCloseConfirmed: false, closedConnections: 0 };
    return {
      remoteCloseConfirmed: true,
      closedConnections: Number(result.closed_connections) || 0
    };
  } catch (_) {
    return { remoteCloseConfirmed: false, closedConnections: 0 };
  } finally {
    clearTimeout(timeoutId);
  }
}

function clearProxyWithoutCredentials() {
  chrome.storage.local.get(["connected", "proxyUser", "proxyPass"], (stored) => {
    if (chrome.runtime.lastError) return;
    if (hasProxyCredentials({ username: stored.proxyUser, password: stored.proxyPass })) return;

    chrome.proxy.settings.get({ incognito: false }, (settings) => {
      if (chrome.runtime.lastError) return;
      const extensionOwnsProxy = settings.levelOfControl === "controlled_by_this_extension";
      if (!stored.connected && !extensionOwnsProxy) return;

      clearProxy((error) => {
        if (!error) chrome.storage.local.set({ connected: false });
      });
    });
  });
}

clearProxyWithoutCredentials();

chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
  if (request.action === "connect") {
    const config = request.payload || proxyConfig;
    if (!hasValidProxyTarget(config.host, config.port) || !hasProxyCredentials(config)) {
      sendResponse({
        status: "error",
        error: "Angiv en gyldig proxy samt brugernavn og adgangskode (mindst 12 tegn)."
      });
      return;
    }
    proxyConfig = { ...config };
    applyProxy(config.host, config.port, (error) => {
      if (error) {
        sendResponse({ status: "error", error: error });
        return;
      }
      chrome.storage.local.set({ connected: true });
      sendResponse({ status: "connected" });
    });
    return true;
  }

  if (request.action === "disconnect") {
    const config = request.payload || proxyConfig;
    clearProxy((error) => {
      if (error) {
        sendResponse({ status: "error", error: error });
        return;
      }
      chrome.storage.local.set({ connected: false }, async () => {
        const remoteResult = await requestProxyDisconnect(config);
        sendResponse({ status: "disconnected", ...remoteResult });
      });
    });
    return true;
  }

  if (request.action === "test-connection") {
    const { host, port, username, password } = request.payload || {};
    if (!host || !Number.isInteger(port) || port < 1 || port > 65535) {
      sendResponse({ success: false, error: "Angiv en gyldig server og port." });
      return;
    }
    if (!hasProxyCredentials({ username, password })) {
      sendResponse({
        success: false,
        error: "Indtast et proxy-brugernavn og en adgangskode på mindst 12 tegn."
      });
      return;
    }

    testCredentials = { username, password };
    testAuthChallenges = 0;
    testAuthRejected = false;

    chrome.storage.local.get(
      ["connected", "proxyHost", "proxyPort", "proxyUser", "proxyPass"],
      (store) => {
        if (chrome.runtime.lastError) {
          testCredentials = null;
          sendResponse({ success: false, error: chrome.runtime.lastError.message });
          return;
        }

        const wasConnected = store.connected === true;
        const previousConfig = {
          host: store.proxyHost || "vpn.homerouter.io",
          port: parseInt(store.proxyPort, 10) || 443,
          username: store.proxyUser || "",
          password: store.proxyPass || ""
        };

        const finishTest = (result) => {
          if (result.success && wasConnected) {
            proxyConfig = { host, port, username, password };
            testCredentials = null;
            testRequestURL = null;
            sendResponse(result);
            return;
          }

          testCredentials = null;
          testRequestURL = null;
          if (wasConnected) proxyConfig = { ...previousConfig };

          const restoreComplete = (error) => {
            testCredentials = null;
            testRequestURL = null;
            if (error) {
              sendResponse({
                success: false,
                error: "Kunne ikke gendanne den tidligere proxy: " + error
              });
            } else {
              sendResponse(result);
            }
          };

          if (wasConnected) {
            applyProxy(previousConfig.host, previousConfig.port, restoreComplete);
          } else {
            clearProxy(restoreComplete);
          }
        };

        applyProxy(host, port, async (proxyError) => {
          if (proxyError) {
            finishTest({
              success: false,
              error: "Kunne ikke anvende proxyindstillingerne: " + proxyError
            });
            return;
          }

          const controller = new AbortController();
          const timeoutId = setTimeout(() => controller.abort(), 6000);
          testRequestURL = "https://1.1.1.1/cdn-cgi/trace";
          let result;

          try {
            const res = await fetch(testRequestURL, {
              method: "GET",
              cache: "no-store",
              signal: controller.signal
            });

            if (res.status === 204 || res.ok) {
              result = testAuthChallenges > 0
                ? { success: true, credentialsVerified: true }
                : {
                    success: false,
                    error: "Chrome genbrugte et cachet proxy-login. Luk alle browser-vinduer helt, åbn browseren igen, og test igen."
                  };
            } else if (res.status === 407) {
              result = {
                success: false,
                error: "Proxyen afviste de indtastede credentials."
              };
            } else {
              result = {
                success: false,
                error: "Proxyen svarede uventet (" + res.status + ")."
              };
            }
          } catch (err) {
            result = testAuthRejected
              ? {
                  success: false,
                  error: "Proxyen afviste de indtastede credentials."
                }
              : {
                  success: false,
                  error: "Kunne ikke forbinde via proxyen. Kontrollér server, port og netværk."
                };
          } finally {
            clearTimeout(timeoutId);
            testRequestURL = null;
          }

          finishTest(result);
        });
      }
    );
    return true;
  }
});

// Proxy credentials are supplied only after a challenge; empty credentials always cancel.
chrome.webRequest.onAuthRequired.addListener(
  (details, callback) => {
    if (!details.isProxy) {
      callback({});
      return;
    }

    const isTestRequest = Boolean(testCredentials && details.url === testRequestURL);
    const activeAuth = credentialsForRequest(details, testCredentials, testRequestURL, proxyConfig);
    if (!hasProxyCredentials(activeAuth)) {
      if (isTestRequest) testAuthRejected = true;
      callback({ cancel: true });
      return;
    }

    const attempts = proxyAuthAttempts.get(details.requestId) || 0;
    if (attempts > 0) {
      proxyAuthAttempts.delete(details.requestId);
      if (isTestRequest) testAuthRejected = true;
      callback({ cancel: true });
      return;
    }

    proxyAuthAttempts.set(details.requestId, attempts + 1);
    if (isTestRequest) testAuthChallenges += 1;
    callback({
      authCredentials: {
        username: activeAuth.username,
        password: activeAuth.password
      }
    });
  },
  { urls: ["<all_urls>"] },
  ["asyncBlocking"]
);

chrome.webRequest.onCompleted.addListener(
  details => proxyAuthAttempts.delete(details.requestId),
  { urls: ["<all_urls>"] }
);

// Fang fejl når tunnelen eller proxyen dør og vis custom fejlside
chrome.webRequest.onErrorOccurred.addListener(
  (details) => {
    proxyAuthAttempts.delete(details.requestId);
    if (details.type !== "main_frame") return;

    chrome.storage.local.get(["connected"], (store) => {
      if (!store.connected) return;

      const proxyErrors = [
        "net::ERR_PROXY_CONNECTION_FAILED",
        "net::ERR_CONNECTION_CLOSED",
        "net::ERR_CONNECTION_RESET",
        "net::ERR_TIMED_OUT",
        "net::ERR_TUNNEL_CONNECTION_FAILED",
        "net::ERR_HTTP2_PROTOCOL_ERROR"
      ];

      if (proxyErrors.includes(details.error)) {
        const errorPageUrl = chrome.runtime.getURL(
          "error.html?target=" + encodeURIComponent(details.url)
        );
        chrome.tabs.update(details.tabId, { url: errorPageUrl });
      }
    });
  },
  { urls: ["<all_urls>"] }
);