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
  feedback.className = "feedback-loading";
  feedback.textContent = "Gemmer indstillinger...";

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
        feedback.textContent = "Kunne ikke gemme: " + chrome.runtime.lastError.message;
      } else if (testResult.success && testResult.credentialsVerified === false) {
        feedback.className = "feedback-warning";
        feedback.textContent = "Proxyen virker via cachet login; credentials er ikke verificeret.";
      } else if (testResult.success) {
        feedback.className = "feedback-success";
        feedback.textContent = "Gemt. Login og proxyforbindelse er verificeret.";
      } else {
        feedback.className = "feedback-error";
        feedback.textContent = "Indstillinger gemt; testen fejlede: " + testResult.error;
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
    updateUI(stored.connected || false);
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
  const password = cfgPass.value.trim();

  saveSettingsBtn.disabled = true;
  feedback.className = "feedback-loading";
  feedback.textContent = "Tester forbindelse...";

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
        error: res && res.error ? res.error : "Ingen testsvar fra proxyen."
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
        btn.disabled = true;
        chrome.runtime.sendMessage({ action: "disconnect" }, (response) => {
          btn.disabled = false;
          if (chrome.runtime.lastError) {
            showConnectionError(chrome.runtime.lastError.message);
            return;
          }
          if (!response || response.status !== "disconnected") {
            showConnectionError(response && response.error ? response.error : "Kunne ikke afbryde tunnelen.");
            return;
          }
          updateUI(false);
        });
        return;
      }

      const config = {
        host: result.proxyHost || "vpn.homerouter.io",
        port: parseInt(result.proxyPort, 10) || 443,
        username: result.proxyUser || "",
        password: result.proxyPass || ""
      };

      btn.disabled = true;
      feedback.className = "feedback-loading";
      feedback.textContent = "Validerer login før tilslutning...";

      chrome.runtime.sendMessage(
        { action: "test-connection", payload: config },
        (testResponse) => {
          if (chrome.runtime.lastError) {
            btn.disabled = false;
            showConnectionError(chrome.runtime.lastError.message);
            return;
          }

          if (!testResponse || !testResponse.success) {
            btn.disabled = false;
            showConnectionError(
              testResponse && testResponse.error
                ? testResponse.error
                : "Proxyforbindelsen kunne ikke valideres."
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
                    : "Kunne ikke aktivere proxyen."
                );
                return;
              }

              if (testResponse.credentialsVerified === false) {
                feedback.className = "feedback-warning";
                feedback.textContent = "Proxyen er aktiv via cachet login; credentials er ikke verificeret.";
              } else {
                feedback.className = "feedback-success";
                feedback.textContent = "Login valideret; proxyen er tilsluttet.";
              }
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
    btn.textContent = "AFBRYD TUNNEL";
    btn.className = "action-btn connected";
    statusDot.className = "dot active";
    connectionBadge.className = "badge online";
    statusText.textContent = "CONNECTED";
  } else {
    btn.textContent = "TILSLUT TUNNEL";
    btn.className = "action-btn disconnected";
    statusDot.className = "dot inactive";
    connectionBadge.className = "badge offline";
    statusText.textContent = "DISCONNECTED";
  }
}