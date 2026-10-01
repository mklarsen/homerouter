const btn = document.getElementById("toggleBtn");
const connectionBadge = document.getElementById("connectionBadge");
const statusDot = document.getElementById("statusDot");
const statusText = document.getElementById("statusText");

const settingsBtn = document.getElementById("toggleSettings");
const settingsPanel = document.getElementById("settingsPanel");
const saveSettingsBtn = document.getElementById("saveSettingsBtn");
const feedback = document.getElementById("settingsFeedback");

const cfgHost = document.getElementById("cfgHost");
const cfgPort = document.getElementById("cfgPort");
const cfgUser = document.getElementById("cfgUser");
const cfgPass = document.getElementById("cfgPass");
document.getElementById("currentVersion").textContent =
  "v" + chrome.runtime.getManifest().version;

function saveProxySettings(host, port, username, password, testResult) {
  if (!testResult.success || testResult.credentialsVerified !== true) {
    saveSettingsBtn.disabled = false;
    feedback.className = "feedback-error";
    feedback.textContent = testResult.error || "Proxy credentials were not verified. Settings were not saved.";
    return;
  }

  feedback.className = "feedback-loading";
  feedback.textContent = "Saving settings...";

  chrome.storage.local.set(
    {
      proxyHost: host,
      proxyPort: port,
      proxyUser: username,
      proxyPass: password
    },
    () => {
      saveSettingsBtn.disabled = false;

      if (chrome.runtime.lastError) {
        feedback.className = "feedback-error";
        feedback.textContent = "Could not save settings: " + chrome.runtime.lastError.message;
      } else {
        feedback.className = "feedback-success";
        feedback.textContent = "Saved. Credentials and proxy connection are verified.";
      }
    }
  );
}

chrome.storage.local.get(
  ["connected", "proxyHost", "proxyPort", "proxyUser", "proxyPass"],
  (result) => {
    if (chrome.runtime.lastError) {
      console.error("[homerouter] Failed to load saved settings:", chrome.runtime.lastError.message);
      updateUI(false);
      return;
    }

    const stored = result || {};
    const hasCredentials = typeof stored.proxyUser === "string" && stored.proxyUser.trim().length > 0 &&
      typeof stored.proxyPass === "string" && stored.proxyPass.length >= 12;
    updateUI(Boolean(stored.connected && hasCredentials));
    cfgHost.value = stored.proxyHost || "vpn.homerouter.io";
    cfgPort.value = stored.proxyPort || "443";
    cfgUser.value = stored.proxyUser || "";
    cfgPass.value = stored.proxyPass || "";
  }
);

settingsBtn.addEventListener("click", () => {
  feedback.textContent = "";
  feedback.className = "";
  settingsPanel.style.display =
    settingsPanel.style.display === "block" ? "none" : "block";
});

saveSettingsBtn.addEventListener("click", () => {
  const host = cfgHost.value.trim() || "vpn.homerouter.io";
  const port = parseInt(cfgPort.value.trim(), 10) || 443;
  const username = cfgUser.value.trim();
  const password = cfgPass.value;

  if (!username || password.length < 12) {
    feedback.className = "feedback-error";
    feedback.textContent = "Enter a username and a password of at least 12 characters.";
    return;
  }

  saveSettingsBtn.disabled = true;
  feedback.className = "feedback-loading";
  feedback.textContent = "Testing connection...";

  chrome.runtime.sendMessage(
    {
      action: "test-connection",
      payload: { host, port, username, password }
    },
    (res) => {
      if (chrome.runtime.lastError) {
        saveProxySettings(host, port, username, password, {
          success: false,
          error: chrome.runtime.lastError.message
        });
        return;
      }

      saveProxySettings(host, port, username, password, {
        success: Boolean(res && res.success),
        credentialsVerified: Boolean(res && res.credentialsVerified),
        error: res && res.error ? res.error : "No response from the proxy test."
      });
    }
  );
});

btn.addEventListener("click", () => {
  chrome.storage.local.get(
    ["connected", "proxyHost", "proxyPort", "proxyUser", "proxyPass"],
    (result) => {
      if (chrome.runtime.lastError) {
        showConnectionError(chrome.runtime.lastError.message);
        return;
      }

      if (result.connected) {
        const config = {
          username: result.proxyUser || "",
          password: result.proxyPass || ""
        };
        btn.disabled = true;
        chrome.runtime.sendMessage({ action: "disconnect", payload: config }, (response) => {
          btn.disabled = false;
          if (chrome.runtime.lastError) {
            showConnectionError(chrome.runtime.lastError.message);
            return;
          }
          if (!response || response.status !== "disconnected") {
            showConnectionError(response && response.error ? response.error : "Could not disconnect the tunnel.");
            return;
          }
          updateUI(false);
          if (response.remoteCloseConfirmed) {
            feedback.className = "feedback-success";
            feedback.textContent = `Proxy disconnected; ${response.closedConnections} server connections closed.`;
          } else {
            feedback.className = "feedback-warning";
            feedback.textContent = "Proxy disconnected locally; the server could not confirm closure of existing connections.";
          }
        });
        return;
      }

      const config = {
        host: result.proxyHost || "vpn.homerouter.io",
        port: parseInt(result.proxyPort, 10) || 443,
        username: result.proxyUser || "",
        password: result.proxyPass || ""
      };

      if (!config.username.trim() || config.password.length < 12) {
        showConnectionError("Enter and save a proxy username and a password of at least 12 characters.");
        return;
      }

      btn.disabled = true;
      feedback.className = "feedback-loading";
      feedback.textContent = "Verifying credentials before connecting...";

      chrome.runtime.sendMessage(
        { action: "test-connection", payload: config },
        (testResponse) => {
          if (chrome.runtime.lastError) {
            btn.disabled = false;
            showConnectionError(chrome.runtime.lastError.message);
            return;
          }

          if (!testResponse || !testResponse.success || testResponse.credentialsVerified !== true) {
            btn.disabled = false;
            showConnectionError(
              testResponse && testResponse.error
                ? testResponse.error
                : "The proxy connection could not be verified."
            );
            return;
          }

          chrome.runtime.sendMessage(
            { action: "connect", payload: config },
            (connectResponse) => {
              btn.disabled = false;
              if (chrome.runtime.lastError) {
                showConnectionError(chrome.runtime.lastError.message);
                return;
              }

              if (!connectResponse || connectResponse.status !== "connected") {
                showConnectionError(
                  connectResponse && connectResponse.error
                    ? connectResponse.error
                    : "Could not enable the proxy."
                );
                return;
              }

              feedback.className = "feedback-success";
              feedback.textContent = "Credentials verified; proxy connected.";
              updateUI(true);
            }
          );
        }
      );
    }
  );
  });

function showConnectionError(message) {
  settingsPanel.style.display = "block";
  feedback.className = "feedback-error";
  feedback.textContent = message;
  updateUI(false);
}

function updateUI(connected) {
  if (connected) {
    btn.textContent = "DISCONNECT TUNNEL";
    btn.className = "action-btn connected";
    statusDot.className = "dot active";
    connectionBadge.className = "badge online";
    statusText.textContent = "CONNECTED";
  } else {
    btn.textContent = "CONNECT TUNNEL";
    btn.className = "action-btn disconnected";
    statusDot.className = "dot inactive";
    connectionBadge.className = "badge offline";
    statusText.textContent = "DISCONNECTED";
  }
}