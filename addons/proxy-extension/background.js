let proxyConfig = {
  host: "vpn.homerouter.io",
  port: 443,
  username: "",
  password: ""
};

let testCredentials = null;
let testAuthChallenges = 0;
let testAuthRejected = false;

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

chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
  if (request.action === "connect") {
    const config = request.payload || proxyConfig;
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
    clearProxy((error) => {
      if (error) {
        sendResponse({ status: "error", error: error });
        return;
      }
      chrome.storage.local.set({ connected: false });
      sendResponse({ status: "disconnected" });
    });
    return true;
  }

  if (request.action === "test-connection") {
    const { host, port, username, password } = request.payload || {};
    if (!host || !Number.isInteger(port) || port < 1 || port > 65535) {
      sendResponse({ success: false, error: "Angiv en gyldig server og port." });
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
            testCredentials = null;
            sendResponse(result);
            return;
          }

          testCredentials = wasConnected
            ? { username: previousConfig.username, password: previousConfig.password }
            : null;

          const restoreComplete = (error) => {
            testCredentials = null;
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
          let result;

          try {
            const res = await fetch("https://1.1.1.1/cdn-cgi/trace", {
              method: "GET",
              cache: "no-store",
              signal: controller.signal
            });

            if (res.status === 204 || res.ok) {
              result = {
                success: true,
                credentialsVerified: testAuthChallenges > 0
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
          }

          finishTest(result);
        });
      }
    );
    return true;
  }
});

// Autentificering
chrome.webRequest.onAuthRequired.addListener(
  (details) => {
    if (details.isProxy) {
      if (testCredentials) {
        testAuthChallenges += 1;
        if (testAuthChallenges > 1) {
          testAuthRejected = true;
          return { cancel: true };
        }
      }
      const activeAuth = testCredentials || proxyConfig;
      return {
        authCredentials: {
          username: activeAuth.username,
          password: activeAuth.password
        }
      };
    }
  },
  { urls: ["<all_urls>"] },
  ["blocking"]
);

// Fang fejl når tunnelen eller proxyen dør og vis custom fejlside
chrome.webRequest.onErrorOccurred.addListener(
  (details) => {
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