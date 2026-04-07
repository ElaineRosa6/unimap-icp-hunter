// Tab pool for reuse - limits memory usage
let tabPool = [];
const MAX_TAB_POOL_SIZE = 3;
const TAB_REUSE_TIMEOUT_MS = 30000;
let lastTabReuseTime = 0;

export async function ensureTab(targetUrl) {
  const tabs = await chrome.tabs.query({});

  // Check if we have a reusable tab in the pool
  const now = Date.now();
  if (tabPool.length > 0 && now - lastTabReuseTime < TAB_REUSE_TIMEOUT_MS) {
    const reusableTab = tabPool.pop();
    if (reusableTab && reusableTab.id) {
      try {
        // Check if tab still exists
        await chrome.tabs.get(reusableTab.id);
        await chrome.tabs.update(reusableTab.id, { url: targetUrl, active: true });
        return reusableTab.id;
      } catch (e) {
        // Tab no longer exists, remove from pool
        tabPool = tabPool.filter(t => t.id !== reusableTab.id);
      }
    }
  }

  // Check for existing tab with the same URL
  const existing = tabs.find((t) => t.url === targetUrl);
  if (existing && existing.id) {
    await chrome.tabs.update(existing.id, { active: true });
    return existing.id;
  }

  // Create new tab
  const created = await chrome.tabs.create({ url: targetUrl, active: true });
  return created.id;
}

// Return tab to pool for reuse, or close if pool is full
export async function releaseTab(tabId) {
  try {
    // Check if tab still exists
    const tab = await chrome.tabs.get(tabId);
    if (!tab) return;

    if (tabPool.length < MAX_TAB_POOL_SIZE) {
      // Return to pool for reuse
      tabPool.push({ id: tabId, url: tab.url });
      lastTabReuseTime = Date.now();
      // Navigate to blank page to free memory
      await chrome.tabs.update(tabId, { url: "about:blank" });
    } else {
      // Pool full, close the tab
      await chrome.tabs.remove(tabId);
    }
  } catch (e) {
    // Tab already closed or doesn't exist
    tabPool = tabPool.filter(t => t.id !== tabId);
  }
}

// Clean up stale tabs from pool
export async function cleanupTabPool() {
  const now = Date.now();
  if (now - lastTabReuseTime > TAB_REUSE_TIMEOUT_MS) {
    // Close all pooled tabs
    for (const pooledTab of tabPool) {
      try {
        await chrome.tabs.remove(pooledTab.id);
      } catch (e) {
        // Ignore errors
      }
    }
    tabPool = [];
  }
}

export async function waitForPageReady(tabId, strategy, timeoutMs) {
  const timeout = Math.max(1000, timeoutMs || 15000);

  if (strategy === "delay") {
    await new Promise((resolve) => setTimeout(resolve, timeout));
    return;
  }

  const current = await chrome.tabs.get(tabId);
  if (current && current.status === "complete") {
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

export async function captureVisible() {
  try {
    const dataUrl = await chrome.tabs.captureVisibleTab(undefined, { format: "png" });
    return dataUrl;
  } catch (err) {
    try {
      const currentWindow = await chrome.windows.getCurrent({ populate: false });
      const dataUrl = await chrome.tabs.captureVisibleTab(currentWindow?.id, { format: "png" });
      return dataUrl;
    } catch (fallbackErr) {
      throw new Error(`plugin_capture_failed: ${String(fallbackErr || err)}`);
    }
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
