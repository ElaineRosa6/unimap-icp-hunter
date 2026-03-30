import { apiGet, apiPost } from "./api.js";
import { ensureTab, waitForPageReady, captureVisible, normalizeImagePayload } from "./capture.js";
import { loadSessionToken, isTokenExpired, saveRuntimeState, saveLastError } from "./storage.js";
import { pairAndStore } from "./pairing.js";

const POLL_INTERVAL_MS = 1000;
let loopStarted = false;

async function pollTaskOnce(token) {
  const resp = await apiGet("/api/screenshot/bridge/tasks/next", token);
  return resp?.task || null;
}

async function reportTaskResult(result, token) {
  // Day 5/6 local mock callback endpoint.
  await apiPost("/api/screenshot/bridge/mock/result", result, token);
}

async function handleTask(task, token) {
  const startedAt = Date.now();
  const requestId = task.request_id;
  const tabId = await ensureTab(task.url);
  const tab = await chrome.tabs.get(tabId);
  await waitForPageReady(tabId, task.wait_strategy || "load", task.timeout_ms || 15000);
  const dataUrl = await captureVisible(tab.windowId);
  const result = normalizeImagePayload(dataUrl, requestId, startedAt);
  result.batch_id = task.batch_id || "";
  result.url = task.url || "";
  await reportTaskResult(result, token);
  await saveRuntimeState({
    last_task_id: requestId,
    last_success_at: Date.now()
  });
}

async function bridgeLoop() {
  if (loopStarted) {
    return;
  }
  loopStarted = true;

  for (;;) {
    try {
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
      }

      await saveRuntimeState({ paired: true });
      const task = await pollTaskOnce(token);
      if (task && task.request_id && task.url) {
        await handleTask(task, token);
      }
    } catch (err) {
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
