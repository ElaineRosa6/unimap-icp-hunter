// Tab pool for reuse - limits memory usage
let tabPool = [];
const MAX_TAB_POOL_SIZE = 3;
const TAB_REUSE_TIMEOUT_MS = 30000;
let lastTabReuseTime = 0;

/**
 * Extract the origin (scheme + host) from a URL for same-domain matching.
 * Returns "" for invalid/empty URLs.
 * @param {string} url
 * @returns {string}
 */
function extractOrigin(url) {
  try {
    const u = new URL(url);
    return u.origin;
  } catch {
    return "";
  }
}

/**
 * Detect search engine type from the page URL.
 * @param {string} url - The page URL
 * @returns {string} Engine name or "unknown"
 */
export function detectEngine(url) {
  if (!url) return "unknown";
  const lower = url.toLowerCase();
  if (lower.includes("fofa.info")) return "fofa";
  if (lower.includes("hunter.qianxin.com")) return "hunter";
  if (lower.includes("zoomeye.org") || lower.includes("zoomeye.com") || lower.includes("zoomeye.ai")) return "zoomeye";
  if (lower.includes("quake.360.cn") || lower.includes("quake.360.net")) return "quake";
  if (lower.includes("shodan.io")) return "shodan";
  if (lower.includes("censys.io")) return "censys";
  if (lower.includes("daydaymap.com")) return "daydaymap";
  return "unknown";
}

// Known cookie names that indicate login state per engine.
const LOGIN_COOKIE_NAMES = {
  fofa: ["fofa_token", "fofa_user", "session", "fofasession"],
  hunter: ["HSESSION", "hunter_token", "sessionid", "jwt", "token", "User-Center", "csrf_token"],
  zoomeye: ["session", "jwt", "zmsession", "ctoken", "token"],
  quake: ["session", "jwt", "token", "QSESSIONID", "cert_common"],
  shodan: ["session", "session_id", "flash", "reveal", "token"],
  censys: ["session", "jwt", "token", "censys_session"],
  daydaymap: ["session", "token", "jwt", "daydaymap_token"]
};

/**
 * Check login cookies for a domain and return diagnostic info.
 * @param {string} url - Page URL to derive domain from
 * @returns {Promise<{has_login_cookies: boolean, cookie_count: number, cookie_names: string[], engine: string}>}
 */
export async function checkLoginCookies(url) {
  const engine = detectEngine(url);
  let origin = "";
  try {
    origin = new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return { has_login_cookies: false, cookie_count: 0, cookie_names: [], engine };
  }

  try {
    const cookies = await chrome.cookies.getAll({ domain: origin });
    const names = cookies.map(c => c.name);
    const knownLogin = LOGIN_COOKIE_NAMES[engine] || [];
    const hasLogin = knownLogin.some(kn => names.some(n => n.toLowerCase().includes(kn.toLowerCase())));
    return {
      has_login_cookies: hasLogin,
      cookie_count: cookies.length,
      cookie_names: names,
      engine
    };
  } catch {
    return { has_login_cookies: false, cookie_count: 0, cookie_names: [], engine };
  }
}

/**
 * Open or create a tab with the target URL.
 * Prioritizes reusing same-domain tabs to preserve login cookies.
 * @param {string} targetUrl - URL to open
 * @returns {Promise<number>} Tab ID
 */
export async function ensureTab(targetUrl) {
  const targetOrigin = extractOrigin(targetUrl);
  const tabs = await chrome.tabs.query({});

  // 1. Pool reuse — prefer same-domain pooled tab (preserves cookies)
  const now = Date.now();
  if (tabPool.length > 0 && now - lastTabReuseTime < TAB_REUSE_TIMEOUT_MS) {
    let reused = false;
    // First pass: same-domain match (best — keeps cookies alive)
    for (let i = tabPool.length - 1; i >= 0; i--) {
      const pooled = tabPool[i];
      if (pooled && pooled.id && extractOrigin(pooled.url) === targetOrigin) {
        try {
          await chrome.tabs.get(pooled.id);
          tabPool.splice(i, 1);
          await chrome.tabs.update(pooled.id, { url: targetUrl, active: true });
          reused = true;
          return pooled.id;
        } catch {
          tabPool.splice(i, 1);
        }
      }
    }
    // Second pass: any pooled tab (cookies lost but tab exists)
    if (!reused && tabPool.length > 0) {
      const fallback = tabPool.pop();
      if (fallback && fallback.id) {
        try {
          await chrome.tabs.get(fallback.id);
          await chrome.tabs.update(fallback.id, { url: targetUrl, active: true });
          return fallback.id;
        } catch {
          tabPool = tabPool.filter(t => t.id !== fallback.id);
        }
      }
    }
  }

  // 2. Exact URL match
  const exact = tabs.find((t) => t.url === targetUrl);
  if (exact && exact.id) {
    await chrome.tabs.update(exact.id, { active: true });
    return exact.id;
  }

  // 3. Same-domain tab — reuse it to keep cookies alive
  if (targetOrigin) {
    const sameDomain = tabs.find((t) => {
      if (!t.url || t.url.startsWith("chrome://") || t.url.startsWith("about:")) return false;
      return extractOrigin(t.url) === targetOrigin;
    });
    if (sameDomain && sameDomain.id) {
      await chrome.tabs.update(sameDomain.id, { url: targetUrl, active: true });
      return sameDomain.id;
    }
  }

  // 4. Create fresh tab
  const created = await chrome.tabs.create({ url: targetUrl, active: true });
  return created.id;
}

/**
 * Return tab to pool for reuse, or close if pool is full.
 * IMPORTANT: Do NOT navigate away from the current URL — that destroys cookies.
 * The tab stays at its last URL so same-domain reuse preserves login state.
 * @param {number} tabId - Tab ID to release
 */
export async function releaseTab(tabId) {
  try {
    const tab = await chrome.tabs.get(tabId);
    if (!tab) return;

    if (tabPool.length < MAX_TAB_POOL_SIZE) {
      tabPool.push({ id: tabId, url: tab.url });
      lastTabReuseTime = Date.now();
      // Do NOT navigate to about:blank — it destroys session cookies.
      // The tab stays at its last URL; ensureTab() picks it by origin match.
    } else {
      await chrome.tabs.remove(tabId);
    }
  } catch (e) {
    tabPool = tabPool.filter(t => t.id !== tabId);
  }
}

/**
 * Clean up stale tabs from pool.
 */
export async function cleanupTabPool() {
  const now = Date.now();
  if (now - lastTabReuseTime > TAB_REUSE_TIMEOUT_MS) {
    for (const pooledTab of tabPool) {
      try {
        await chrome.tabs.remove(pooledTab.id);
      } catch (e) { /* ignore */ }
    }
    tabPool = [];
  }
}

/**
 * Wait for page to be ready with multiple strategies.
 * @param {number} tabId - Tab ID
 * @param {string} strategy - "load", "delay", "networkidle", "spa"
 * @param {number} timeoutMs - Timeout in milliseconds
 */
export async function waitForPageReady(tabId, strategy, timeoutMs) {
  const timeout = Math.max(1000, timeoutMs || 15000);

  if (strategy === "delay") {
    await new Promise((resolve) => setTimeout(resolve, timeout));
    return;
  }

  // For SPA-heavy pages (search engines), use a hybrid approach:
  // 1. Wait for tab status "complete"
  // 2. Then wait extra time for dynamic content rendering
  const current = await chrome.tabs.get(tabId);
  if (current && current.status === "complete" && strategy === "load") {
    // For search engines, always wait extra for SPA rendering
    await new Promise((resolve) => setTimeout(resolve, 2000));
    return;
  }

  if (strategy === "spa" || strategy === "networkidle") {
    // SPA strategy: give the page time to start rendering
    await new Promise((resolve) => setTimeout(resolve, Math.min(timeout, 5000)));
    // If the tab already reached "complete" during the SPA delay,
    // the onUpdated listener below would never fire — resolve now.
    const afterDelay = await chrome.tabs.get(tabId);
    if (afterDelay && afterDelay.status === "complete") {
      await new Promise((resolve) => setTimeout(resolve, 3000));
      return;
    }
  }

  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup();
      reject(new Error("plugin_timeout: page load timeout"));
    }, timeout);

    function onUpdated(updatedTabId, info) {
      if (updatedTabId === tabId && info.status === "complete") {
        cleanup();
        // Extra wait for SPA rendering
        setTimeout(resolve, strategy === "spa" ? 3000 : 1000);
      }
    }

    function cleanup() {
      clearTimeout(timer);
      chrome.tabs.onUpdated.removeListener(onUpdated);
    }

    chrome.tabs.onUpdated.addListener(onUpdated);
  });
}

/**
 * Capture visible tab as PNG data URL.
 * @returns {Promise<string>} Data URL
 */
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

/**
 * Build screenshot result payload.
 */
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

/**
 * Build collect result payload with structured data.
 */
export function normalizeCollectPayload(items, title, requestId, startedAt) {
  const durationMs = Math.max(1, Date.now() - startedAt);
  return {
    request_id: requestId,
    success: true,
    image_path: "",
    image_data: "",
    duration_ms: durationMs,
    collected_data: title || "",
    structured_collected_data: {
      title: title || "",
      items: items || [],
      total: items ? items.length : 0,
      has_more: false
    }
  };
}

/**
 * DOM selector configurations per engine.
 * These can be updated without redeploying the extension.
 */
export const ENGINE_SELECTORS = {
  fofa: {
    // FOFA uses Vue SPA with hsxa-* prefixed class names.
    // Live DOM (2026-08-21): row .hsxa-meta-data-item; span.hsxa-host is IP only;
    // port is span.hsxa-port / qbase64=cG9ydD0. Do not reuse hsxa-host as host.
    row: [
      ".hsxa-meta-data-item",
      "[class*='meta-data-item']",
      ".result-card", "[class*='result-card']",
      "[class*='result-item']", ".result-item"
    ],
    cells: {
      ip: { selector: "span.hsxa-host, .hsxa-ip a, a[href*='qbase64=aXA9']" },
      port: { selector: "span.hsxa-port, a[href*='qbase64=cG9ydD0']" },
      protocol: { selector: "a[href*='qbase64=cHJvdG9jb2w9'], a[href*='qbase64=cHJvdG9jb2xf']" },
      host: { selector: "a[href*='qbase64=ZG9tYWluPS'], a[href*='qbase64=aG9zdD0']" },
      title: { selector: ".hsxa-title a, .hsxa-title, [class*='title'] a, [class*='title'] span" },
      country_code: { selector: "a[href*='qbase64=Y291bnRyeT0']" },
      asn: { selector: "a[href*='qbase64=YXNuPS']" },
      org: { selector: "a[href*='qbase64=b3JnPS']" },
      banner: { selector: "a[href*='qbase64=YmFubmVyX2hhc2g9'], pre, [class*='banner-content']" }
    },
    total: [".total-count", ".total_count", "[class*='total']", "[class*='count']"],
    nextPage: [".next", ".next-page", "[class*='next']", ".el-pagination__next"]
  },
  hunter: {
    // Hunter uses Quasar UI. Data stored in .q-tooltip spans inside <div class="cell"> wrappers.
    // Columns: 1=序号, 2=IP, 3=域名, 4=端口/服务, 5=标题, 6=状态码, 7=ICP, 8=应用, 9=标签, 10=地区, 11=更新时间
    row: [
      ".q-table tbody tr",
      ".q-table__body tr",
      ".list-table tbody tr",
      ".page-list-body_table tr",
      ".result-list > .result-item",
      ".result-item",
      "div[class*='result-item']",
      "[class*='result-list'] > div",
      "[class*='result'] > div",
      ".page-list-body > div"
    ],
    cells: {
      ip: { selector: "td:nth-child(2)" },
      host: { selector: "td:nth-child(3)" },
      port: { selector: "td:nth-child(4)" },
      protocol: { selector: "td:nth-child(4)" },
      title: { selector: "td:nth-child(5)" },
      status_code: { selector: "td:nth-child(6)" },
      org: { selector: "td:nth-child(7)" },
      product: { selector: "td:nth-child(8)" },
      country_code: { selector: "td:nth-child(10)" },
    },
    total: [".total-count", ".total", "[class*='total-count']", "[class*='total']", ".page-list-body_statistic"],
    nextPage: [".next", ".q-pagination button", "[class*='next']", ".pagination-next", ".page-list-pagination button"]
  },
  zoomeye: {
    // ZoomEye uses card-based layout (2026-06-15 verified, 2026-06-16 fixed hashed class).
    // Container: div.search-result-item-container (each result block)
    // Header: div.header-bar contains IP:Port in div.url-container
    // Port/Protocol: div.protocol-port-box button elements
    // IP: div.url-container span or div.ip-detail-box span (stable selectors)
    // Pagination: ul.ant-pagination
    row: [
      ".search-result-item-container",
      "[class*='search-result-item-container']",
      ".search-result-list > .search-result-item-container",
      ".search-result-item"
    ],
    cells: {
      // IP: prefer div.url-container text (stable), fallback to div.ip-detail-box span
      ip: { selector: "div.url-container span, div.ip-detail-box span, div.header-bar span" },
      // Port is first button in div.protocol-port-box (e.g. "8888")
      port: { selector: "div.protocol-port-box button:first-child, div.protocol-port-box button" },
      // Protocol is second button's span in div.protocol-port-box (e.g. "http")
      protocol: { selector: "div.protocol-port-box button:last-child span, div.protocol-box span" },
      // Host/domain from header-bar link
      host: { selector: "div.header-bar a[href], div.url-container a" },
      // Banner from pre tab panels
      banner: { selector: "div.ant-tabs-tabpane-active pre, pre" }
      // NOTE: title/org/asn/isp/country_code/timestamp are extracted by the
      // ZoomEye-specific router-container traversal below (not via cells selectors),
      // because ZoomEye stores them as <span>label:</span> + url-container value blocks,
      // and [class*='title'] matches unrelated elements (.search-result-item-tabs etc.).
    },
    total: ["li.ant-pagination-total-text span", ".total", "[class*='total']", "[class*='count']"],
    nextPage: ["li.ant-pagination-next:not(.ant-pagination-disabled) a", ".ant-pagination-next a", ".next", "[class*='next']"]
  },
  quake: {
    // Quake 360 search results (Vue SPA, Element UI).
    // Result container: div.item-container
    // IP: span.copy_btn with data-clipboard-text="IP:port"
    // Port: span.port.common-tag
    // Protocol: span.server-protocol.common-tag
    // Country: span.country-container span.address
    // Title: span.ellipse-text inside div.title-line
    // ASN/Org/ISP: div.item span.label + span.ellipse-text
    row: [
      ".item-container:has(span.port)",
      ".item-container:has(div.ip)",
      ".content-container .item-container",
      ".item-container",
      "[class*='result-item']",
      "[class*='result-card']",
      ".el-table tbody tr",
      ".el-table__body tr"
    ],
    cells: {
      ip: { selector: "div.ip span.copy_btn, [data-clipboard-text]", attr: "data-clipboard-text", extract: "ip_from_hostport" },
      port: { selector: "span.port" },
      protocol: { selector: "span.server-protocol" },
      title: { selector: ".title-line span.ellipse-text, [class*='title']" },
      country_code: { selector: ".country-container .address" },
      asn: { selector: ".item .label + .ellipse-text" },
    },
    total: [".total-count", ".total", "[class*='total']", ".pagination-info"],
    nextPage: [".next", ".next-page", "[class*='next']", ".el-pagination__next"]
  },
  shodan: {
    // Shodan search results (verified 2026-06-17).
    // Result: div.result > div.heading + div.result-details + div.banner-data
    // HTML structure:
    //   <div class="heading">
    //     <a href="/host/X.X.X.X" class="title text-dark">TITLE</a>
    //     <a href="http://X.X.X.X:PORT" class="text-danger">
    //     <div class="timestamp">...</div>
    //   </div>
    //   <div class="result-details">
    //     <li><img class="flag"> <a>COUNTRY</a></li>
    //     <a class="filter-link filter-org">ORG</a>
    //     ...
    //   </div>
    //   <div class="banner-data"><pre>BANNER_CONTENT</pre></div>
    // IP: first a.title href → extract IP from /host/IP path
    // Port: second a.text-danger href → extract port from http://IP:PORT URL
    // Title: a.title text content
    // Timestamp: div.timestamp text (Shodan-specific)
    // Org: a.filter-link.filter-org text
    // Country: .flag img + sibling a text
    // Banner: div.banner-data pre text
    row: [
      // 2026-08-21: results live under .l-search-results without a .row wrapper.
      ".l-search-results .result",
      ".row.l-search-results .result",
      // Main result div (narrow to avoid false positives)
      "div.result",
      // Card-based layout match
      "[class*='search-result']",
      "[class*='result-item']",
      "div[class*='host']",
      // Fallback: any div that contains a heading with /host/ link
      "div:has(a[href*='/host/'])",
      // Generic fallback
      "[class*='result'] > div",
      ".list-group-item"
    ],
    cells: {
      ip: {
        selector: "div.heading a.title, a[href*='/host/'], div[class*='heading'] a[href*='/host/']",
        attr: "href",
        extract: "ip_from_path"
      },
      port: {
        selector: "div.heading a.text-danger, div[class*='heading'] a[href^='http://'], a[href^='https://']",
        attr: "href",
        extract: "port_from_url"
      },
      title: { selector: "div.heading a.title, a[href*='/host/'], .host-title, [class*='title']" },
      last_seen: { selector: "div.heading div.timestamp, .timestamp, [class*='timestamp'], time" },
      org: { selector: ".result-details a.filter-link.filter-org, a.filter-org, .org, [class*='org']" },
      country_code: { selector: "img.flag + a, .result-details img.flag + a, [class*='country']" },
      banner: { selector: "div.banner-data pre, div[data-banner] pre, .banner pre, pre" },
      server: { selector: "pre" },
      host: { selector: "li.hostnames" },
    },
    total: ["h4.total-results", ".total-results", ".total", "[class*='total']", ".result-count", "[class*='result-count']", "div[class*='summary']"],
    nextPage: [".next", ".pagination-next", "[class*='next']", "a[rel='next']", "nav ul li:last-child a"]
  },
  censys: {
    // Censys uses a modern SPA layout with result cards.
    row: [
      "[data-search-results-layout] a[href*='/hosts/']",
      "a[href*='/hosts/']",
      "[class*='result-card']", "[class*='search-result']",
      "[class*='result-list'] > div",
      "table tbody tr"
    ],
    cells: {
      ip: { selector: "a[href*='/hosts/']", attr: "href", extract: "ip_from_path" },
      port: { selector: "[class*='port'], [data-port]" },
      host: { selector: "[class*='hostname'], [class*='domain']" },
      title: { selector: "[class*='title'], h2, h3" },
      country_code: { selector: "[class*='country'], [class*='location']" },
      org: { selector: "[class*='org'], [class*='organization']" }
    },
    total: ["[class*='total']", "[class*='count']"],
    nextPage: ["[class*='next']", "button[aria-label='next']"]
  },
  daydaymap: {
    // Live 2026-08-21: Ant Design table, 10 data rows as tr.ant-table-row.
    // table tbody tr also matches the hidden ant-table-measure-row (no assets).
    // Columns: 序号, IP, 域名/访问链接, 端口, 服务, 传输...
    row: [
      ".ant-pro-card > .ant-pro-card-header .ant-pro-card-title",
      "tr.ant-table-row",
      ".ant-table-row",
      "[class*='table-row']",
      "[class*='result-item']",
      "[class*='result-card']",
      "[class*='result-list'] > div",
      ".el-table__row",
      "table tbody tr"
    ],
    cells: {
      ip: { selector: "td:nth-child(2), [class*='ip'], [data-ip]" },
      host: { selector: "td:nth-child(3), [class*='domain'], [class*='host']" },
      port: { selector: "td:nth-child(4), [class*='port'], [data-port]" },
      protocol: { selector: "td:nth-child(5), [class*='protocol'], [class*='service']" },
      title: { selector: "td:nth-child(7), [class*='title'], [class*='name']" },
      country_code: { selector: "[class*='country'], [class*='location']" },
      org: { selector: "[class*='org'], [class*='company']" }
    },
    total: ["[class*='StyleSummary']", ".search-result-header", "[class*='total']", "[class*='count']"],
    nextPage: [".ant-pagination-next:not(.ant-pagination-disabled)", "[class*='next']", ".el-pagination__next"]
  }
};

/**
 * Safely query a single element using the first matching selector.
 * @param {Document|Element} root - Root element to query
 * @param {string|string[]} selectors - CSS selector(s)
 * @returns {Element|null}
 */
function queryOne(root, selectors) {
  const list = Array.isArray(selectors) ? selectors : [selectors];
  for (const sel of list) {
    const el = root.querySelector(sel);
    if (el) return el;
  }
  return null;
}

/**
 * Query all matching elements across multiple selector variants.
 * @param {Document|Element} root
 * @param {string|string[]} selectors
 * @returns {NodeListOf<Element>|Element[]}
 */
function queryAll(root, selectors) {
  const list = Array.isArray(selectors) ? selectors : [selectors];
  for (const sel of list) {
    const els = root.querySelectorAll(sel);
    if (els.length > 0) return els;
  }
  return [];
}

/**
 * Check if the page looks like a login wall.
 * @param {Document} doc
 * @returns {boolean}
 */
function isLoginWall(doc) {
  const text = doc.body.textContent.toLowerCase();
  const loginKeywords = [
    "请登录", "请先登录", "login required", "sign in to continue",
    "session expired", "session expired", "please log in",
    "登录", "登入", "サインイン", "로그인"
  ];
  // Only trigger if the page is short (likely a login form, not a full results page)
  if (text.length > 5000) return false;
  return loginKeywords.some(kw => text.includes(kw));
}

/**
 * Extract structured assets from a search engine result page DOM.
 * This is the KEY function for collect mode.
 * @param {number} tabId - Chrome tab ID
 * @returns {Promise<{items: Array, total: number, has_more: boolean, title: string, engine: string, is_login_wall: boolean, error?: string}>}
 */
export async function extractEngineAssets(tabId) {
  const tab = await chrome.tabs.get(tabId);
  const engine = detectEngine(tab?.url);

  try {
    const results = await chrome.scripting.executeScript({
      target: { tabId },
      func: (eng, selectors) => {
        const items = [];
        let total = 0;
        let hasMore = false;
        const title = document.title || "";
        const bodyText = (document.body?.innerText || "").toLowerCase();
        const loginRequired = /登录|登陆|请先登录|login|sign in|signin|unauthorized/.test(bodyText + " " + title.toLowerCase());

        // Dedicated interstitial titles are not empty search results.
        if (/^just a moment(?:\.\.\.)?$/i.test(title.trim()) || /^attention required!?\s*\|\s*cloudflare$/i.test(title.trim())) {
          return { items: [], total: 0, has_more: false, title, engine: eng, is_login_wall: false, browser_challenge: true, extraction_method: "browser_challenge" };
        }

        // Check for login wall first
        if (isLoginWallFn(document)) {
          return { items: [], total: 0, has_more: false, title, engine: eng, is_login_wall: true, extraction_method: "login_wall" };
        }

        const engineSelectors = selectors[eng];
        if (!engineSelectors) {
          return fallbackExtraction();
        }

        // Try each row selector variant
        let rows = [];
        let rowSelectorUsed = "";
        for (const rowSel of engineSelectors.row) {
          try {
            rows = document.querySelectorAll(rowSel);
            if (rows.length > 0) {
              rowSelectorUsed = rowSel;
              break;
            }
          } catch {
            rows = [];
          }
        }

        if (rows.length === 0) {
          // No rows found — try fallback extraction
          return fallbackExtraction();
        }

        // Cell text extractor with attribute support
        function extractCellText(row, cellConfig) {
          const selectors = cellConfig.selector.split(/,\s*/);
          let el = null;
          for (const sel of selectors) {
            try {
              if (row.matches && row.matches(sel)) {
                el = row;
                break;
              }
            } catch { /* invalid selector for matches() */ }
            el = row.querySelector(sel);
            if (el) break;
          }
          if (!el && cellConfig.fallback) {
            const fbs = cellConfig.fallback.split(/,\s*/);
            for (const fb of fbs) {
              el = row.querySelector(fb);
              if (el) break;
            }
          }
          if (!el) return "";

          // Support attribute extraction (e.g. href, src, data-*)
          if (cellConfig.attr) {
            const val = el.getAttribute(cellConfig.attr) || "";
            // Post-process: extract IP or port from URL paths
            if (cellConfig.extract) {
              if (cellConfig.extract === "ip_from_path") {
                const markers = ["/hosts/", "/host/"];
                for (const marker of markers) {
                  const idx = val.indexOf(marker);
                  if (idx < 0) continue;
                  const rest = val.slice(idx + marker.length);
                  const cut = rest.search(/[/?#]/);
                  const ip = cut >= 0 ? rest.slice(0, cut) : rest;
                  try { return decodeURIComponent(ip); } catch { return ip; }
                }
                const m = val.match(/\/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/);
                return m ? m[1] : val;
              }
              if (cellConfig.extract === "port_from_url") {
                const br = val.lastIndexOf("]:");
                if (br >= 0) {
                  const n = parseInt(val.slice(br + 2), 10);
                  if (n) return String(n);
                }
                // Match port at end of URL: http://IP:PORT  OR  with path: http://IP:PORT/path
                const m = val.match(/:(\d{1,5})(\/|$)/);
                if (m) return m[1];
                // Fallback: #PORT format (some engines use hash fragments)
                const m2 = val.match(/#(\d{1,5})/);
                if (m2) return m2[1];
                return "";
              }
              if (cellConfig.extract === "ip_from_hostport") {
                const m = val.match(/^(\d{1,3}(?:\.\d{1,3}){3})(?::\d{1,5})?$/);
                return m ? m[1] : "";
              }
            }
            return val.trim();
          }

          return el.textContent.trim();
        }

        // Clean Hunter UI filter labels from text
        function cleanHunterText(text) {
          if (!text) return "";
          text = text.replace(/只看该[^\s]*不看该[^\s]*/g, "");
          text = text.replace(/只看空[^\s]*不看空[^\s]*/g, "");
          text = text.replace(/看相似(网站|icon)/g, "");
          text = text.replace(/访问[^\s]*/g, "");
          text = text.replace(/复制/g, "");
          text = text.replace(/云厂商/g, "");
          text = text.replace(/\s+/g, " ").trim();
          return text;
        }

        // Clean ZoomEye title: remove metadata prefixes like "CN 北京 公司名 AS12345 "
        // The title field from ZoomEye DOM may contain country/city/org/asn before the actual title.
        function cleanZoomEyeTitle(text) {
          if (!text) return "";
          // Remove patterns like: CN, US (2-letter country codes at start)
          text = text.replace(/^[A-Z]{2}\s+/, "");
          // Remove city names (common Chinese cities)
          text = text.replace(/^(北京|上海|广州|深圳|杭州|成都|武汉|南京|西安|重庆|天津|苏州|长沙|郑州|青岛|大连|厦门|宁波|东莞|无锡|佛山)\s+/, "");
          // Remove ASN pattern (AS followed by digits)
          text = text.replace(/^AS\d+\s+/, "");
          // Remove organization patterns (ending with company/org keywords)
          text = text.replace(/^[^\s]+(公司|集团|有限|股份|科技|网络|信息|技术|企业|机构|组织)\s+/, "");
          return text.trim();
        }

        // Extract data from each row/card
        const seenKeys = new Set();
        rows.forEach((row) => {
          if (String(row.className || "").includes("measure-row")) return;
          if (eng === "daydaymap" && row.matches(".ant-pro-card-title")) {
            for (const node of row.querySelectorAll("span, a")) {
              const text = node.textContent.trim();
              if (!/^https?:\/\//i.test(text) || /\s/.test(text)) continue;
              try {
                const endpoint = new URL(text);
                if (endpoint.username || endpoint.password) continue;
                const ip = endpoint.hostname.replace(/^\[|\]$/g, "");
                const ipv4 = /^(\d{1,3}\.){3}\d{1,3}$/.test(ip) && ip.split(".").every(part => Number(part) <= 255);
                const ipv6 = endpoint.hostname.startsWith("[") && ip.includes(":");
                if (!ipv4 && !ipv6) continue;
                const port = endpoint.port ? Number(endpoint.port) : (endpoint.protocol === "https:" ? 443 : 80);
                if (port < 1 || port > 65535) continue;
                const key = endpoint.protocol + "|" + ip + "|" + port;
                if (seenKeys.has(key)) continue;
                seenKeys.add(key);
                items.push({ip, port, protocol: endpoint.protocol.slice(0, -1)});
              } catch { /* Not an asset endpoint URL. */ }
            }
            return;
          }
          const cells = row.querySelectorAll("td");
          const item = {};
          const cellConfig = engineSelectors.cells;

          Object.keys(cellConfig).forEach((key) => {
            const cfg = cellConfig[key];
            if (cells.length > 0 && /td:nth-child/.test(cfg.selector || "")) {
              item[key] = extractCellTextFromCells(cells, cfg);
            } else {
              item[key] = extractCellText(row, cfg);
            }
          });

          // Clean Hunter UI text from title/ip/host
          if (typeof item.title === "string") item.title = cleanHunterText(item.title);
          if (typeof item.ip === "string") item.ip = cleanHunterText(item.ip);
          if (typeof item.host === "string") item.host = cleanHunterText(item.host);

          // ZoomEye: extract title/org/asn/host/isp/country/timestamp from labelled
          // router-container blocks inside search-result-item-info.
          // ZoomEye cards store metadata as <span>label:</span> + <div class="url-container">value</div>,
          // NOT as concatenated text. The generic cells title selector ([class*='title'])
          // matches unrelated elements, so we override with precise DOM traversal.
          if (eng === "zoomeye") {
            const infoEl = row.querySelector("div.search-result-item-info");
            if (infoEl) {
              infoEl.querySelectorAll("div.router-container").forEach((rc) => {
                const label = rc.querySelector("span.whitespace-nowrap");
                const valueEl = rc.querySelector("div.url-container span");
                if (!label || !valueEl) return;
                const labelText = label.textContent.trim();
                const value = valueEl.textContent.trim();
                if (!value) return;
                if (labelText.startsWith("标题:")) { if (!item.title) item.title = cleanZoomEyeTitle(value); }
                else if (labelText.startsWith("组织:")) { if (!item.org) item.org = value; }
                else if (labelText.startsWith("ASN:")) { if (!item.asn) item.asn = value; }
                else if (labelText.startsWith("主机名:")) { if (!item.host) item.host = value; }
                else if (labelText.startsWith("ISP:")) { if (!item.isp) item.isp = value; }
              });
              // Country: extract from flag-XX class (e.g. flag-cn → CN)
              if (!item.country_code) {
                const flagEl = infoEl.querySelector("span.flag");
                if (flagEl) {
                  const fm = (flagEl.className || "").match(/flag-([a-z]{2})/i);
                  if (fm) item.country_code = fm[1].toUpperCase();
                }
              }
              // Timestamp: search-result-icon-time paragraph
              if (!item.last_seen) {
                const timeEl = infoEl.querySelector("p.search-result-icon-time");
                if (timeEl) item.last_seen = timeEl.textContent.trim();
              }
            }
          }

          function looksLikeIPv4(value) {
            return /^\d{1,3}(?:\.\d{1,3}){3}$/.test(String(value || ""));
          }
          function looksLikeIPv6(value) {
            return /:/.test(String(value || "")) && /[0-9a-f]/i.test(String(value || "")) && !/^\d{1,3}(?:\.\d{1,3}){3}:\d+$/.test(String(value || ""));
          }
          function normalizeEndpoint(target) {
            const raw = String(target.ip || "").trim();
            if (!raw) return;
            let m = raw.match(/^(\d{1,3}(?:\.\d{1,3}){3}):(\d{1,5})$/);
            if (m) {
              target.ip = m[1];
              if (!target.port) target.port = parseInt(m[2], 10) || 0;
              return;
            }
            m = raw.match(/^([A-Za-z0-9.-]+\.[A-Za-z]{2,}):(\d{1,5})$/);
            if (m) {
              target.host = target.host && target.host !== raw ? target.host : m[1];
              target.ip = looksLikeIPv4(m[1]) ? m[1] : "";
              if (!target.port) target.port = parseInt(m[2], 10) || 0;
              return;
            }
            if (!looksLikeIPv4(raw) && !looksLikeIPv6(raw) && /[A-Za-z]/.test(raw) && raw.indexOf(":") < 0) {
              if (!target.host || target.host === raw) target.host = raw;
              target.ip = "";
            }
          }
          if (item.host === "--" || item.host === "-" || item.host === "—") item.host = "";
          if (item.ip && !looksLikeIPv4(item.ip) && !looksLikeIPv6(item.ip)) {
            const ipOnly = String(item.ip).match(/(\d{1,3}(?:\.\d{1,3}){3})/);
            if (ipOnly) item.ip = ipOnly[1];
          }
          normalizeEndpoint(item);
          if (item.host && item.ip && item.host === item.ip && looksLikeIPv4(item.host)) item.host = "";

          if (eng === "quake") {
            if (!row.querySelector("div.ip, span.port, span.copy_btn, [data-clipboard-text]")) return;
            const copyButton = row.querySelector("div.ip span.copy_btn[data-clipboard-text], [data-clipboard-text]");
            const endpoint = copyButton?.getAttribute("data-clipboard-text") || "";
            const endpointMatch = endpoint.match(/^(\d{1,3}(?:\.\d{1,3}){3})(?::(\d{1,5}))?$/);
            if (endpointMatch) {
              item.ip = endpointMatch[1];
              if (!item.port && endpointMatch[2]) item.port = parseInt(endpointMatch[2], 10) || 0;
            } else if (/^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/i.test(endpoint)) {
              // Quake's visible card header can be a domain while the IP is masked.
              // Preserve that domain without inventing or resolving the hidden IP.
              if (!item.host) item.host = endpoint.toLowerCase();
            }
            row.querySelectorAll(".item").forEach((detail) => {
              const label = (detail.querySelector(".label")?.textContent || "").trim().toLowerCase();
              const value = (detail.querySelector(".ellipse-text")?.textContent || "").trim();
              if (!value || value === "--" || value === "-") return;
              if (/host|domain|主机|域名/.test(label)) item.host = value;
              else if (/^asn$|自治系统号/.test(label)) item.asn = value;
              else if (/organization|组织|机构|公司/.test(label)) item.org = value;
              else if (/^isp$|运营商/.test(label)) item.isp = value;
            });
          }
          if (item.host === "--" || item.host === "-" || item.host === "—") item.host = "";

          // Clean Shodan country/org: extract from multi-line result-details
          if (typeof item.country_code === "string" && item.country_code.includes("\n")) {
            const lines = item.country_code.split(/\n/).map(l => l.trim()).filter(l => l.length > 1);
            // Country: look for "Country, City" pattern
            for (const l of lines) {
              if (/^[A-Z][a-z]+,\s*[A-Z]/.test(l)) {
                item.country_code = l.trim();
                break;
              }
              // Chinese locations
              if (/^中国/.test(l) || /^[\u4e00-\u9fa5]{2,}省/.test(l)) {
                item.country_code = l.trim();
                break;
              }
            }
            // Fallback: line after org
            if (!item.country_code || item.country_code.includes("\n")) {
              const orgIdx = lines.findIndex(l => /Cloud|Inc|Ltd|Corp|Company/.test(l));
              if (orgIdx >= 0 && orgIdx + 1 < lines.length) {
                item.country_code = lines[orgIdx + 1].trim();
              }
            }
          }
          if (typeof item.org === "string" && item.org.includes("\n")) {
            const lines = item.org.split(/\n/).map(l => l.trim()).filter(l => l.length > 3 && !/^\d/.test(l));
            const orgLine = lines.find(l => l !== item.ip && /Cloud|Inc|Ltd|Corp|Company|LLC|University/.test(l));
            if (orgLine) item.org = orgLine.trim();
          }

          // Port: ensure number, fallback to protocol
          if (typeof item.port === "string") item.port = parseInt(item.port, 10) || 0;
          if (!item.port && item.protocol) {
            const pm = String(item.protocol).match(/(\d{1,5})/);
            if (pm) item.port = parseInt(pm[1], 10);
          }

          // Hunter-specific: protocol may contain port number (e.g. "8081 http")
          // Extract only the known protocol name
          if (eng === "shodan") {
            const names = Array.from(row.querySelectorAll("li.hostnames")).map((el) => (el.textContent || "").trim()).filter(Boolean);
            const domain = names.find((n) => n !== item.ip && /[A-Za-z]/.test(n));
            item.host = domain || "";
          }

          if (eng === "hunter" && item.protocol) {
            const protoMatch = String(item.protocol).match(/\b(http|https|tls|ssl|tcp|udp|ssh|ftp|smtp|pop3|imap|mysql|rdp|smb|dns)\b/i);
            if (protoMatch) {
              item.protocol = protoMatch[1].toLowerCase();
            } else if (/^\d+$/.test(String(item.protocol))) {
              item.protocol = ""; // Pure port number, not a protocol
            }
          }

          // Post-fix: if ip is empty but host contains IP, move it
          if (!item.ip && item.host) {
            const m = String(item.host).match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/);
            if (m) item.ip = m[1];
          }

          if (eng === "hunter") {
            const ipOk = looksLikeIPv4(item.ip) || looksLikeIPv6(item.ip);
            const hostOk = !!String(item.host || "").split(/\s+/)[0] && /^(?:[a-z0-9-]+\.)+[a-z]{2,}$/i.test(String(item.host).split(/\s+/)[0]);
            if (!ipOk && !hostOk) return;
          }

          // Skip completely empty rows
          const hasAnyValue = Object.values(item).some(v => v !== "" && v !== 0);
          if (!hasAnyValue) return;
          if (!item.ip && !item.host) return;

          // Deduplicate by ip:port (Hunter shows duplicate summary+detail rows)
          if (item.ip) {
            const dedupKey = item.ip + ":" + (item.port || 0);
            if (seenKeys.has(dedupKey)) return;
            seenKeys.add(dedupKey);
          }

          items.push(item);
        });

        // Extract pagination info
        const totalEl = queryOne(document, engineSelectors.total);
        if (totalEl) {
          const raw = totalEl.textContent.trim();
          const labeled = raw.match(/共有\s*([\d,]+)\s*个/) || raw.match(/([\d,]+)/);
          total = labeled ? parseInt(labeled[1].replace(/,/g, ""), 10) || 0 : 0;
        }

        const nextEl = queryOne(document, engineSelectors.nextPage);
        hasMore = !!nextEl && !nextEl.classList.contains("disabled");

        return { items, total, has_more: hasMore, title, engine: eng, is_login_wall: false, row_selector_used: rowSelectorUsed, rows_found: rows.length, extraction_method: "selector" };

        function isLoginWallFn(doc) {
          const pageTitle = (doc.title || "").trim();
          // Anchor to a dedicated login title; a navigation login link is not a wall.
          if (/^(?:登录|登陆|登入|log in|login|sign in)(?:\s*[-–—|:]|$)/i.test(pageTitle)) return true;
          const text = (doc.body?.textContent || "").toLowerCase();
          const loginKeywords = [
            "请登录", "请先登录", "login required", "sign in to continue",
            "session expired", "please log in"
          ];
          if (text.length > 5000) return false;
          return loginKeywords.some(kw => text.includes(kw));
        }

        // queryOne MUST be defined inside the injected function — chrome.scripting
        // .executeScript serializes only the function body, so module-level helpers
        // are undefined in the page scope and throw ReferenceError (same class of bug
        // as the extractCellText scope fix). Missing this caused selector-based
        // extraction to throw at the pagination step → empty items → 0 assets.
        function queryOne(root, selectors) {
          if (!selectors) return null;
          const list = Array.isArray(selectors) ? selectors : [selectors];
          for (const sel of list) {
            const el = root.querySelector(sel);
            if (el) return el;
          }
          return null;
        }

        function extractCellTextFromCells(cells, cfg) {
          const match = cfg.selector.match(/td:nth-child\((\d+)\)/);
          if (!match) return "";
          const idx = parseInt(match[1], 10) - 1;
          if (idx < 0 || idx >= cells.length) return "";
          const target = cells[idx];
          // Get raw cell text, remove UI artifacts
          let text = target.textContent.trim();
          text = text.replace(/只看该[^\s]*/g, "").replace(/不看该[^\s]*/g, "");
          text = text.replace(/只看空[^\s]*/g, "").replace(/不看空[^\s]*/g, "");
          text = text.replace(/看相似[^\s]*/g, "").replace(/访问[^\s]*/g, "");
          text = text.replace(/复制[^\s]*/g, "").replace(/云厂商/g, "");
          text = text.replace(/高危|中危|低危/g, "");
          text = text.replace(/\s+/g, " ").trim();
          if (text === "-" || text === "—") text = "";
          return text;
        }

        function fallbackExtraction() {
          // Try table-based extraction first
          const fallbackItems = [];
          const tables = document.querySelectorAll("table");
          tables.forEach((table) => {
            const tRows = table.querySelectorAll("tbody tr, tr");
            tRows.forEach((row) => {
              const tCells = row.querySelectorAll("td");
              if (tCells.length >= 2) {
                const item = {};
                tCells.forEach((cell, idx) => {
                  item[`col_${idx}`] = cell.textContent.trim().substring(0, 200);
                });
                fallbackItems.push(item);
              }
            });
          });

          if (fallbackItems.length > 0) {
            return { items: fallbackItems, total: 0, has_more: false, title, engine: eng, is_login_wall: false, extraction_method: "table_fallback" };
          }

          // Try card-based extraction using link patterns
          const cardResult = cardBasedExtraction();
          if (cardResult.items.length > 0) return cardResult;

          return { items: [], total: 0, has_more: false, title, engine: eng, is_login_wall: false, extraction_method: "no_match" };
        }

        function cardBasedExtraction() {
          const cardItems = [];

          // Find potential result cards by looking for elements with IP-like content
          const allLinks = Array.from(document.querySelectorAll("a"));
          const ipLinks = allLinks.filter(a => {
            const href = a.href || "";
            const text = a.textContent.trim();
            return /\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/.test(text) ||
                   href.includes("qbase64=aXA9") ||
                   href.includes("ip=") ||
                   (a.className && a.className.includes("ip"));
          });

          if (ipLinks.length === 0) {
            // Try finding card containers
            const cardSelectors = [
              "[class*='result-card']", "[class*='result-item']",
              "[class*='asset-item']", "[class*='data-item']",
              ".result-list > div", ".list_content > div"
            ];
            for (const sel of cardSelectors) {
              const cards = document.querySelectorAll(sel);
              if (cards.length > 0) {
                cards.forEach((card) => {
                  const item = {};
                  const links = Array.from(card.querySelectorAll("a"));
                  if (links.length >= 2) {
                    // Heuristic: first link with IP pattern is IP, second is port, etc.
                    for (const link of links) {
                      const text = link.textContent.trim();
                      if (/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/.test(text)) {
                        if (!item.ip) item.ip = text;
                      } else if (/^\d{1,5}$/.test(text) && !item.port) {
                        item.port = text;
                      } else if (!item.title && text.length > 3 && text.length < 100) {
                        item.title = text;
                      }
                    }
                    if (Object.keys(item).length > 0) cardItems.push(item);
                  }
                });
                break;
              }
            }
          } else {
            // Group IP links into items (each IP link + nearby links = one item)
            for (const ipLink of ipLinks.slice(0, 100)) {
              const item = {};
              const ipText = ipLink.textContent.trim();
              item.ip = /\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/.test(ipText) ? ipText : "";

              // Look for nearby elements for port, protocol, etc.
              const parent = ipLink.closest("div, li, tr, section, article");
              if (parent) {
                const siblings = Array.from(parent.querySelectorAll("a, span, div"));
                for (const sib of siblings) {
                  const text = sib.textContent.trim();
                  if (!text || text === ipText) continue;
                  if (/^\d{1,5}$/.test(text) && !item.port) item.port = text;
                  else if (/^[a-zA-Z][\w-]*$/.test(text) && text.length < 30 && !item.protocol) item.protocol = text;
                  else if (text.length > 3 && text.length < 100 && !item.title && !item.banner) item.title = text;
                }
              }

              if (Object.keys(item).length > 0) cardItems.push(item);
            }
          }

          return { items: cardItems, total: 0, has_more: false, title, engine: eng, is_login_wall: false, extraction_method: "card_fallback" };
        }
        return {
          items,
          total,
          has_more: hasMore,
          title,
          engine: eng,
          is_login_wall: false,
          login_required: loginRequired && items.length === 0
        };
      },
      args: [engine, ENGINE_SELECTORS]
    });

    if (results && results[0]) {
      if (results[0].result) {
        return results[0].result;
      }
      if (results[0].error) {
        return { items: [], total: 0, has_more: false, title: "", engine, login_required: false, error: "injection_error: " + String(results[0].error) };
      }
    }
    return { items: [], total: 0, has_more: false, title: "", engine, login_required: false, error: "no_injection_result" };
  } catch (err) {
    // DOM extraction failed — return empty result, let caller handle
    return { items: [], total: 0, has_more: false, title: "", engine, login_required: false, error: String(err) };
  }
}
