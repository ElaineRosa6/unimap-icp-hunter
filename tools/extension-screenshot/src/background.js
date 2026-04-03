import { apiGet, apiPostBridgeSigned, bridgeRotateToken } from "./api.js";
import { ensureTab, waitForPageReady, captureVisible, normalizeImagePayload, releaseTab, cleanupTabPool } from "./capture.js";
import { loadSessionToken, isTokenExpired, saveSessionToken, saveRuntimeState, saveLastError } from "./storage.js";
import { pairAndStore } from "./pairing.js";

const POLL_INTERVAL_MS = 1000;
const CAPTURE_MIN_INTERVAL_MS = 1200;
const ROTATE_AHEAD_MS = 60 * 1000;
let loopStarted = false;
let lastCaptureAt = 0;

function shouldRotateSoon(expireAt) {
  if (!expireAt) {
    return false;
  }
  return Date.now() + ROTATE_AHEAD_MS >= expireAt;
}

function isBridgeAuthError(err) {
  const text = String(err || "").toLowerCase();
  return text.includes("unauthorized_bridge") || text.includes("401");
}

async function pollTaskOnce(token) {
  const resp = await apiGet("/api/screenshot/bridge/tasks/next", token);
  return resp?.task || null;
}

async function reportTaskResult(result, token) {
  // Day 5/6 local mock callback endpoint.
  await apiPostBridgeSigned("/api/screenshot/bridge/mock/result", result, token);
}

async function waitForCaptureSlot() {
  const elapsed = Date.now() - lastCaptureAt;
  if (elapsed < CAPTURE_MIN_INTERVAL_MS) {
    await new Promise((resolve) => setTimeout(resolve, CAPTURE_MIN_INTERVAL_MS - elapsed));
  }
  lastCaptureAt = Date.now();
}

async function handleTask(task, token) {
  const startedAt = Date.now();
  const requestId = task.request_id;
  let tabId = null;

  async function captureWithFocus(tid, windowId) {
    await waitForCaptureSlot();
    await chrome.tabs.update(tid, { active: true });
    await chrome.windows.update(windowId, { focused: true });
    await new Promise((resolve) => setTimeout(resolve, 300));
    return captureVisible();
  }

  try {
    tabId = await ensureTab(task.url);
    const tab = await chrome.tabs.get(tabId);
    await waitForPageReady(tabId, task.wait_strategy || "load", task.timeout_ms || 15000);
    let dataUrl;
    try {
      dataUrl = await captureWithFocus(tabId, tab.windowId);
    } catch (captureErr) {
      await waitForCaptureSlot();
      dataUrl = await captureWithFocus(tabId, tab.windowId);
    }
    const result = normalizeImagePayload(dataUrl, requestId, startedAt);
    result.batch_id = task.batch_id || "";
    result.url = task.url || "";
    await reportTaskResult(result, token);
    await saveRuntimeState({
      last_task_id: requestId,
      last_success_at: Date.now()
    });

    // Release tab after successful capture (reuse or close)
    await releaseTab(tabId);
    tabId = null;
  } catch (err) {
    const durationMs = Math.max(1, Date.now() - startedAt);
    const errorText = String(err || "plugin_capture_failed");
    await reportTaskResult(
      {
        request_id: requestId,
        success: false,
        image_path: "",
        image_data: "",
        duration_ms: durationMs,
        batch_id: task.batch_id || "",
        url: task.url || "",
        error_code: "plugin_capture_failed",
        error: errorText
      },
      token
    );

    // Release tab after error
    if (tabId) {
      await releaseTab(tabId);
      tabId = null;
    }
    throw err;
  }
}

async function bridgeLoop() {
  if (loopStarted) {
    return;
  }
  loopStarted = true;

  for (;;) {
    try {
      // Periodically clean up stale tabs in the pool
      await cleanupTabPool();

      const session = await loadSessionToken();
      let token = session.token;
      if (!token || isTokenExpired(session.expireAt)) {
        try {
          const pair = await pairAndStore(chrome.runtime.id, "dev-pair");
          token = pair.token;
        } catch (pairErr) {
          await saveRuntimeState({ paired: false });
          await saveLastError(pairErr);
          await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
          continue;
        }
      } else if (shouldRotateSoon(session.expireAt)) {
        try {
          const rotated = await bridgeRotateToken(token);
          const newToken = rotated?.token || "";
          const expiresIn = Number(rotated?.expires_in || 600);
          if (newToken) {
            await saveSessionToken(newToken, expiresIn);
            token = newToken;
          }
        } catch (rotateErr) {
          // Rotation failure should not stop task polling; existing token may still be valid.
          await saveLastError(rotateErr);
        }
      }

      await saveRuntimeState({ paired: true });
      const task = await pollTaskOnce(token);
      if (task && task.request_id && task.url) {
        await handleTask(task, token);
      }
    } catch (err) {
      if (isBridgeAuthError(err)) {
        await saveSessionToken("", 1);
        await saveRuntimeState({ paired: false });
      }
      await saveLastError(err);
    }

    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
  }
}

chrome.runtime.onInstalled.addListener(() => {
  bridgeLoop();
});

chrome.runtime.onStartup.addListener(() => {
  bridgeLoop();
});

bridgeLoop();
