import { renderBridgeStatus } from "./status.js";
import { normalizeLoopbackAPIBaseURL } from "./bridge_url.js";
import { loadAPIBaseURL, saveAPIBaseURL } from "./storage.js";

const adminTokenInput = document.getElementById("adminToken");
const apiBaseURLInput = document.getElementById("apiBaseURL");
const saveBtn = document.getElementById("saveBtn");
const clearBtn = document.getElementById("clearBtn");
const statusEl = document.getElementById("status");

async function loadCurrentToken() {
  const data = await chrome.storage.local.get(["adminToken"]);
  if (data.adminToken) {
    adminTokenInput.value = data.adminToken;
  }
}

async function loadCurrentAPIBaseURL() {
  apiBaseURLInput.value = await loadAPIBaseURL();
}

function showStatus(message, isError) {
  statusEl.textContent = message;
  statusEl.className = isError ? "status error" : "status success";
  setTimeout(() => { statusEl.className = "status"; }, 3000);
}

saveBtn.addEventListener("click", async () => {
  const token = adminTokenInput.value.trim();
  let apiBaseURL;
  try {
    apiBaseURL = normalizeLoopbackAPIBaseURL(apiBaseURLInput.value);
  } catch (error) {
    showStatus(String(error.message || error), true);
    return;
  }
  await saveAPIBaseURL(apiBaseURL);
  if (token) {
    await chrome.storage.local.set({ adminToken: token });
  }
  apiBaseURLInput.value = apiBaseURL;
  showStatus(token ? "Bridge URL and admin token saved." : "Bridge URL saved.", false);
});

clearBtn.addEventListener("click", async () => {
  adminTokenInput.value = "";
  await chrome.storage.local.remove("adminToken");
  showStatus("Admin token cleared.", false);
});

loadCurrentToken();
loadCurrentAPIBaseURL();

const inspectMode = new URLSearchParams(location.search).get("inspect");
if (inspectMode === "1" || inspectMode === "2" || inspectMode === "ddm") {
  import("./inspect_live.js").then((mod) => {
    if (inspectMode === "ddm") return mod.runLiveInspectDayDayMap();
    return inspectMode === "2" ? mod.runLiveInspectPass2() : mod.runLiveInspect();
  }).catch((err) => {
    const el = document.createElement("pre");
    el.textContent = `inspect failed: ${String(err && err.stack || err)}`;
    document.body.prepend(el);
  });
}

async function refreshBridgeStatus() {
  const element = document.getElementById("bridgeStatus");
  try {
    const data = await chrome.storage.local.get(["bridgeRuntimeState"]);
    renderBridgeStatus(element, chrome.runtime.getManifest().version, data.bridgeRuntimeState || {});
  } catch {
    element.textContent = "状态读取失败，请重新加载扩展后再试。";
  }
}
document.getElementById("refreshStatusBtn").addEventListener("click", refreshBridgeStatus);
refreshBridgeStatus();
