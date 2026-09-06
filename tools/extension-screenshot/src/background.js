import { apiGet, apiPostBridgeSigned, bridgeRotateToken } from "./api.js";
import { ensureTab, waitForPageReady, captureVisible, normalizeImagePayload, releaseTab, cleanupTabPool, normalizeCollectPayload, extractEngineAssets, checkLoginCookies } from "./capture.js";
import { loadSessionToken, isTokenExpired, saveSessionToken, saveRuntimeState, saveLastError, loadAdminToken } from "./storage.js";
import { pairAndStore } from "./pairing.js";
import { resolveTabFinalURL } from "./tab_url.js";
import { prepareDayDayMapSearch } from "./daydaymap.js";
import { readBrowserCredentials } from "./credentials.js";

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
  const resp = await apiGet("/api/v1/screenshot/bridge/tasks/next", token);
  return resp?.task || null;
}

async function reportTaskResult(result, token) {
  await apiPostBridgeSigned("/api/v1/screenshot/bridge/mock/result", result, token);
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
  const action = task.action || "screenshot";
  let tabId = null;

  async function captureWithFocus(tid, windowId) {
    await waitForCaptureSlot();
    await chrome.tabs.update(tid, { active: true });
    await chrome.windows.update(windowId, { focused: true });
    await new Promise((resolve) => setTimeout(resolve, 300));
    return captureVisible();
  }

  async function reportResult(result) {
    result.batch_id = task.batch_id || "";
    result.url = task.url || "";
    if (tabId) {
      try {
        result.final_url = await resolveTabFinalURL(tabId);
      } catch {
        if (result.success) {
          throw new Error("plugin_final_url_unavailable");
        }
        result.final_url = "";
      }
    }
    await reportTaskResult(result, token);
  }

  async function reportFailure(err) {
    const durationMs = Math.max(1, Date.now() - startedAt);
    const errorText = String(err || "plugin_task_failed");
    await reportResult({
      request_id: requestId,
      success: false,
      image_path: "",
      image_data: "",
      duration_ms: durationMs,
      error_code: "plugin_task_failed",
      error: errorText
    });
  }

  try {
    // Cookie-read tasks don't need a tab — read directly via chrome.cookies API.
    if (action === "get_cookies") {
	  let cookieDomain = task.url;
	  try {
		cookieDomain = new URL(task.url).hostname;
	  } catch {
		// Backward compatibility for an older loopback server that sent a bare domain.
	  }
	  const cookies = await chrome.cookies.getAll({ domain: cookieDomain });
      const durationMs = Math.max(1, Date.now() - startedAt);
      await reportResult({
        request_id: requestId,
        success: true,
        image_path: "",
        image_data: "",
        collected_data: JSON.stringify(cookies),
        structured_collected_data: { cookies: cookies },
        duration_ms: durationMs
      });
      return;
    }

    if (action === "get_browser_credentials") {
      const credentials = await readBrowserCredentials(task.url);
      const durationMs = Math.max(1, Date.now() - startedAt);
      await reportResult({
        request_id: requestId,
        success: true,
        image_path: "",
        image_data: "",
        final_url: credentials.finalURL,
        collected_data: "",
        structured_collected_data: { cookies: credentials.cookies, storage: credentials.storage },
        duration_ms: durationMs
      });
      return;
    }

    tabId = await ensureTab(task.url);

    // Check login cookies AFTER navigation (preserved by same-domain reuse)
    let loginDiagnostics = null;
    try {
      loginDiagnostics = await checkLoginCookies(task.url);
    } catch {
      // Diagnostics are non-fatal
    }

    // Choose wait strategy based on action type
    let waitStrategy = task.wait_strategy || "load";
    // Use longer timeout for collect action (SPA rendering)
    const effectiveTimeout = action === "collect"
      ? Math.max(task.timeout_ms || 30000, 30000)
      : (task.timeout_ms || 15000);

    await waitForPageReady(tabId, waitStrategy, effectiveTimeout);

    if (task.query && new URL(task.url).hostname.endsWith("daydaymap.com")) {
      await prepareDayDayMapSearch(tabId, task.query);
      await waitForPageReady(tabId, "spa", effectiveTimeout);
    }

    // Extra render wait for collect action (SPA search results take time to render)
    if (action === "collect" || action === "screenshot" || action === "collect_and_capture") {
      // 固定15秒等待，确保所有引擎（尤其是Hunter）的登录态和页面内容完全加载
      await new Promise((resolve) => setTimeout(resolve, 15000));

      // 滚动到页面底部触发懒加载内容，再滚动回顶部
      try {
        await chrome.tabs.executeScript(tabId, {
          code: `
            window.scrollTo(0, document.body.scrollHeight);
            setTimeout(() => window.scrollTo(0, 0), 500);
          `
        });
      } catch (e) {
        // 忽略滚动错误
      }

      // 滚动后再等待2秒，确保内容稳定
      await new Promise((resolve) => setTimeout(resolve, 2000));
    }

    if (action === "open") {
      // Only open the page, no screenshot or data collection
      const durationMs = Math.max(1, Date.now() - startedAt);
      await reportResult({
        request_id: requestId,
        success: true,
        image_path: "",
        image_data: "",
        duration_ms: durationMs
      });
    } else if (action === "collect") {
      // Extract structured DOM data from the page
      const assets = await extractEngineAssets(tabId);

      // Handle login wall detection
      if (assets.is_login_wall || assets.browser_challenge) {
        const durationMs = Math.max(1, Date.now() - startedAt);
        const wallResult = {
          request_id: requestId,
          success: false,
          image_path: "",
          image_data: "",
          duration_ms: durationMs,
          error_code: assets.browser_challenge ? "browser_challenge" : "login_required",
          error: `${assets.browser_challenge ? "browser challenge" : "login wall"} detected on ${assets.engine || "unknown"}`,
          collected_data: "",
          structured_collected_data: {
            title: assets.title || "",
            items: [],
            total: 0,
            has_more: false,
            is_login_wall: !!assets.is_login_wall,
            browser_challenge: !!assets.browser_challenge,
            engine: assets.engine
          }
        };
        if (loginDiagnostics) {
          wallResult.structured_collected_data.login_diagnostics = {
            has_login_cookies: loginDiagnostics.has_login_cookies,
            cookie_count: loginDiagnostics.cookie_count,
            engine: loginDiagnostics.engine
          };
        }
        await reportResult(wallResult);
      } else {
        const result = normalizeCollectPayload(
          assets.items,
          assets.title,
          requestId,
          startedAt
        );

        // Override total and has_more from extraction result
        if (result.structured_collected_data) {
          result.structured_collected_data.total = assets.total || assets.items.length;
          result.structured_collected_data.has_more = assets.has_more || false;
          result.structured_collected_data.engine = assets.engine || "unknown";
          // Diagnostic fields — report which selector worked and how
          result.structured_collected_data.extraction_method = assets.extraction_method || "unknown";
          result.structured_collected_data.row_selector_used = assets.row_selector_used || "";
          result.structured_collected_data.rows_found = assets.rows_found || 0;
          if (assets.error) {
            result.structured_collected_data.extraction_error = assets.error;
          }
        }

        // Include raw error info if extraction failed but didn't login wall
        if (assets.error && assets.items.length === 0) {
          result.structured_collected_data.extraction_error = assets.error;
        }

        // Attach login diagnostics
        if (loginDiagnostics && result.structured_collected_data) {
          result.structured_collected_data.login_diagnostics = {
            has_login_cookies: loginDiagnostics.has_login_cookies,
            cookie_count: loginDiagnostics.cookie_count,
            engine: loginDiagnostics.engine
          };
        }

        await reportResult(result);
      }
    } else if (action === "collect_and_capture") {
      // Combined: collect structured data + take screenshot in one navigation
      const assets = await extractEngineAssets(tabId);

      // Handle login wall detection
      if (assets.is_login_wall || assets.browser_challenge) {
        const durationMs = Math.max(1, Date.now() - startedAt);
        const wallResult2 = {
          request_id: requestId,
          success: false,
          image_path: "",
          image_data: "",
          duration_ms: durationMs,
          error_code: assets.browser_challenge ? "browser_challenge" : "login_required",
          error: `${assets.browser_challenge ? "browser challenge" : "login wall"} detected on ${assets.engine || "unknown"}`,
          collected_data: "",
          structured_collected_data: {
            title: assets.title || "",
            items: [],
            total: 0,
            has_more: false,
            is_login_wall: !!assets.is_login_wall,
            browser_challenge: !!assets.browser_challenge,
            engine: assets.engine
          }
        };
        if (loginDiagnostics) {
          wallResult2.structured_collected_data.login_diagnostics = {
            has_login_cookies: loginDiagnostics.has_login_cookies,
            cookie_count: loginDiagnostics.cookie_count,
            engine: loginDiagnostics.engine
          };
        }
        await reportResult(wallResult2);
      } else {
        let captureDataUrl = null;
        await waitForCaptureSlot();
        const tab = await chrome.tabs.get(tabId);
        try {
          captureDataUrl = await captureWithFocus(tabId, tab.windowId);
        } catch (captureErr) {
          await waitForCaptureSlot();
          captureDataUrl = await captureWithFocus(tabId, tab.windowId);
        }

        const collectResult = normalizeCollectPayload(
          assets.items, assets.title, requestId, startedAt
        );

        if (captureDataUrl) {
          const imagePayload = normalizeImagePayload(captureDataUrl, requestId, startedAt);
          collectResult.image_path = imagePayload.image_path || "";
          collectResult.image_data = imagePayload.image_data || "";
        }

        if (collectResult.structured_collected_data) {
          collectResult.structured_collected_data.total = assets.total || assets.items.length;
          collectResult.structured_collected_data.has_more = assets.has_more || false;
          collectResult.structured_collected_data.engine = assets.engine || "unknown";
          collectResult.structured_collected_data.extraction_method = assets.extraction_method || "unknown";
          collectResult.structured_collected_data.row_selector_used = assets.row_selector_used || "";
          collectResult.structured_collected_data.rows_found = assets.rows_found || 0;
          if (assets.error) {
            collectResult.structured_collected_data.extraction_error = assets.error;
          }
          // Attach login diagnostics
          if (loginDiagnostics) {
            collectResult.structured_collected_data.login_diagnostics = {
              has_login_cookies: loginDiagnostics.has_login_cookies,
              cookie_count: loginDiagnostics.cookie_count,
              engine: loginDiagnostics.engine
            };
          }
        }

        await reportResult(collectResult);
      }
    } else {
      // "screenshot" or unknown action — capture screenshot
      await waitForCaptureSlot();
      const tab = await chrome.tabs.get(tabId);
      let dataUrl;
      try {
        dataUrl = await captureWithFocus(tabId, tab.windowId);
      } catch (captureErr) {
        await waitForCaptureSlot();
        dataUrl = await captureWithFocus(tabId, tab.windowId);
      }
      const result = normalizeImagePayload(dataUrl, requestId, startedAt);
      // Attach login diagnostics to screenshot result
      if (loginDiagnostics) {
        result.login_diagnostics = {
          has_login_cookies: loginDiagnostics.has_login_cookies,
          cookie_count: loginDiagnostics.cookie_count,
          engine: loginDiagnostics.engine
        };
      }
      await reportResult(result);
    }

    await saveRuntimeState({
      last_task_id: requestId,
      last_success_at: Date.now()
    });

    // Release tab after successful task
    await releaseTab(tabId);
    tabId = null;
  } catch (err) {
    await reportFailure(err);

    // Release tab after error
    if (tabId) {
      await releaseTab(tabId);
      tabId = null;
    }
    throw err;
  }
}

async function acquireToken(session, usingAdmin) {
  // 1. Valid bridge token from pairing — use it
  if (session.token && !isTokenExpired(session.expireAt)) {
    return { token: session.token, isAdmin: usingAdmin };
  }

  // 2. Try pairing to get a fresh bridge token
  try {
    const pair = await pairAndStore(chrome.runtime.id, "dev-pair");
    await chrome.storage.local.set({ usingAdminToken: false });
    return { token: pair.token, isAdmin: false };
  } catch (pairErr) {
    // Pairing failed — fall through to admin token
  }

  // 3. Fallback: use admin token (static, survives server restarts)
  const adminToken = await loadAdminToken();
  if (adminToken) {
    await saveSessionToken(adminToken, 24 * 3600);
    await chrome.storage.local.set({ usingAdminToken: true });
    return { token: adminToken, isAdmin: true };
  }

  return { token: "", isAdmin: false };
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
      const adminData = await chrome.storage.local.get(["usingAdminToken"]);
      const usingAdmin = !!adminData.usingAdminToken;
      const { token, isAdmin } = await acquireToken(session, usingAdmin);
      if (!token) {
        await saveRuntimeState({ paired: false });
        await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
        continue;
      }

      // Rotate bridge tokens before expiry (skip for admin token — it's static)
      if (!isAdmin && shouldRotateSoon(session.expireAt)) {
        try {
          const rotated = await bridgeRotateToken(token);
          const newToken = rotated?.token || "";
          const expiresIn = Number(rotated?.expires_in || 600);
          if (newToken) {
            await saveSessionToken(newToken, expiresIn);
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
        const wasAdmin = (await chrome.storage.local.get(["usingAdminToken"])).usingAdminToken;
        await saveSessionToken("", 1);
        await chrome.storage.local.set({ usingAdminToken: false });
        await saveRuntimeState({ paired: false });
        // If admin token was used and rejected, clear it to avoid an infinite retry loop.
        // The user should re-enter the correct token in the extension options page.
        if (wasAdmin) {
          await chrome.storage.local.remove("adminToken");
        }

        // Retry pairing immediately so the extension recovers from transient auth
        // failures (e.g. server restart that invalidates in-memory bridge tokens).
        try {
          const pair = await pairAndStore(chrome.runtime.id, "dev-pair");
          await saveRuntimeState({ paired: true });
          continue;
        } catch (rePairErr) {
          // Re-pairing also failed; fall through to the next loop iteration.
        }
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
