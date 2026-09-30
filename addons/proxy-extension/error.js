const urlParams = new URLSearchParams(window.location.search);
const targetUrl = urlParams.get("target") || "";

const errorTarget = document.getElementById("errorTarget");
if (targetUrl) {
  errorTarget.textContent = targetUrl;
} else {
  errorTarget.style.display = "none";
}

document.getElementById("retryBtn").addEventListener("click", () => {
  if (targetUrl) {
    window.location.href = targetUrl;
  } else {
    window.history.back();
  }
});

document.getElementById("disableBtn").addEventListener("click", () => {
  chrome.runtime.sendMessage({ action: "disconnect" }, () => {
    if (targetUrl) {
      window.location.href = targetUrl;
    } else {
      window.location.reload();
    }
  });
});