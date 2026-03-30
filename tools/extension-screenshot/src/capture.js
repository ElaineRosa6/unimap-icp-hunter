export async function ensureTab(targetUrl) {
  const tabs = await chrome.tabs.query({});
  const existing = tabs.find((t) => t.url === targetUrl);
  if (existing && existing.id) {
    await chrome.tabs.update(existing.id, { active: true });
    return existing.id;
  }
  const created = await chrome.tabs.create({ url: targetUrl, active: true });
  return created.id;
}

export async function waitForPageReady(tabId, strategy, timeoutMs) {
  const timeout = Math.max(1000, timeoutMs || 15000);

  if (strategy === "delay") {
    await new Promise((resolve) => setTimeout(resolve, timeout));
    return;
  }

  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup();
      reject(new Error("plugin_timeout: page load timeout"));
    }, timeout);

    function onUpdated(updatedTabId, info) {
      if (updatedTabId === tabId && info.status === "complete") {
        cleanup();
        resolve();
      }
    }

    function cleanup() {
      clearTimeout(timer);
      chrome.tabs.onUpdated.removeListener(onUpdated);
    }

    chrome.tabs.onUpdated.addListener(onUpdated);
  });
}

export async function captureVisible(windowId) {
  try {
    const dataUrl = await chrome.tabs.captureVisibleTab(windowId, { format: "png" });
    return dataUrl;
  } catch (err) {
    throw new Error(`plugin_capture_failed: ${String(err)}`);
  }
}

export function normalizeImagePayload(dataUrl, requestId, startedAt) {
  const durationMs = Math.max(1, Date.now() - startedAt);
  return {
    request_id: requestId,
    success: true,
    image_path: "",
    image_data: dataUrl,
    duration_ms: durationMs
  };
}
