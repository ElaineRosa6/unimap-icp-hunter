import assert from "node:assert/strict";
import test from "node:test";
import { parseHTML } from "linkedom";

import { extractEngineAssets, ENGINE_SELECTORS } from "../src/capture.js";

async function extractFromHTML(html, url) {
  const { document, window } = parseHTML(html);
  const prev = { document: globalThis.document, window: globalThis.window, chrome: globalThis.chrome };
  globalThis.document = document;
  globalThis.window = window;
  globalThis.chrome = {
    tabs: { get: async () => ({ url }) },
    scripting: {
      executeScript: async ({ func, args }) => [{ result: func(...(args || [])) }],
    },
  };
  try {
    return await extractEngineAssets(1);
  } finally {
    globalThis.document = prev.document;
    globalThis.window = prev.window;
    globalThis.chrome = prev.chrome;
  }
}

test("FOFA live layout uses hsxa-host IP and hsxa-port, not host=IP", async () => {
  const html = `<!doctype html><html><body>
    <div class="hsxa-meta-data-item">
      <span class="hsxa-copy-btn" data-clipboard-text="203.0.113.8"></span>
      <span class="hsxa-host">203.0.113.8</span>
      <span class="hsxa-port">80</span>
      <a href="/result?qbase64=cG9ydD0iODAi">80</a>
      <a href="/result?qbase64=cHJvdG9jb2w9aHR0cA">http</a>
      <a href="/result?qbase64=Y291bnRyeT0iQ04i">CN</a>
      <a href="/result?qbase64=b3JnPSJCSVQgQlYi">BIT BV</a>
      <div class="hsxa-title"><a>Example FOFA Service</a></div>
    </div>
  </body></html>`;
  const result = await extractFromHTML(html, "https://fofa.info/result?qbase64=cG9ydD0iODAi");
  assert.equal(result.row_selector_used, ".hsxa-meta-data-item");
  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].ip, "203.0.113.8");
  assert.equal(result.items[0].port, 80);
  assert.notEqual(result.items[0].host, "203.0.113.8");
});

test("Hunter skips ICP junk rows and extracts tls protocol", async () => {
  const html = `<!doctype html><html><body>
    <table class="q-table"><tbody>
      <tr>
        <td><div class="cell">1</div></td>
        <td><div class="cell">198.51.100.20</div></td>
        <td><div class="cell">hunter.example.test</div></td>
        <td><div class="cell">443 tls</div></td>
        <td><div class="cell">Hunter Title</div></td>
        <td><div class="cell">200</div></td>
        <td><div class="cell">京ICP备1号</div></td>
        <td><div class="cell">nginx</div></td>
        <td><div class="cell">web</div></td>
        <td><div class="cell">CN Beijing</div></td>
        <td><div class="cell">2026-08-21</div></td>
      </tr>
      <tr>
        <td><div class="cell"></div></td>
        <td><div class="cell">粤ICP备05128807号-8</div></td>
        <td><div class="cell">查看</div></td>
        <td><div class="cell">页面无备案号</div></td>
        <td><div class="cell">--</div></td>
      </tr>
    </tbody></table>
  </body></html>`;
  const result = await extractFromHTML(html, "https://hunter.qianxin.com/home/list?search=x");
  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].ip, "198.51.100.20");
  assert.equal(result.items[0].protocol, "tls");
  assert.equal(result.items[0].port, 443);
});

test("ZoomEye hostname:port is stored as host, not ip", async () => {
  const html = `<!doctype html><html><body>
    <div class="search-result-item-container">
      <div class="url-container"><span>spinvolcano7x.top:80</span></div>
      <div class="protocol-port-box"><button>80</button><button><span>http</span></button></div>
      <div class="search-result-item-info">
        <div class="router-container"><span class="whitespace-nowrap">标题:</span><div class="url-container"><span>Index of /</span></div></div>
      </div>
    </div>
  </body></html>`;
  const result = await extractFromHTML(html, "https://www.zoomeye.org/searchResult?q=port%3A80");
  assert.equal(result.row_selector_used, ".search-result-item-container");
  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].ip, "");
  assert.equal(result.items[0].host, "spinvolcano7x.top");
  assert.equal(result.items[0].port, 80);
  assert.equal(result.items[0].protocol, "http");
});

test("ZoomEye loading header is not treated as a result row", async () => {
  const html = `<!doctype html><html><body>
    <div class="header"><div class="header-content">个人资料 退出登录</div></div>
    <div class="search-result"><div class="search-result-loading">loading</div></div>
  </body></html>`;
  const result = await extractFromHTML(html, "https://www.zoomeye.org/searchResult?q=port%3A80");
  assert.equal(result.items.length, 0);
  assert.notEqual(result.row_selector_used, "[class*='result'] > div");
});

test("Quake aggregation item-container without host/port is ignored", async () => {
  const html = `<!doctype html><html><body>
    <div class="total-count">共 10 条</div>
    <div class="aggregations-lists-collapse"><div class="item-container"><div class="item-title">Ports</div></div></div>
    <div class="item-container">
      <div class="ip"><span class="copy_btn" data-clipboard-text="203.0.113.10:443"></span></div>
      <span class="port">443</span>
      <span class="server-protocol">https</span>
      <div class="item"><span class="label">Host</span><span class="ellipse-text">--</span></div>
    </div>
  </body></html>`;
  const result = await extractFromHTML(html, "https://quake.360.net/quake/#/searchResult?searchVal=port%3A80");
  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].ip, "203.0.113.10");
  assert.ok(!result.items[0].host);
});

test("Censys hashed cards extract unique /hosts/ links", async () => {
  const html = `<!doctype html><html><body>
    <div data-search-results-layout="true">
      <div class="_card_abc123"><a href="/hosts/198.51.100.77">198.51.100.77Host</a></div>
      <div class="_card_abc123"><a href="/hosts/198.51.100.77">198.51.100.77Host</a></div>
      <div class="_card_def456"><a href="/hosts/203.0.113.9">203.0.113.9Host</a></div>
    </div>
  </body></html>`;
  const result = await extractFromHTML(html, "https://platform.censys.io/search?q=host.services.port%3A%2280%22");
  assert.ok(result.items.length >= 2, JSON.stringify(result));
  const ips = result.items.map((item) => item.ip).sort();
  assert.ok(ips.includes("198.51.100.77"));
  assert.ok(ips.includes("203.0.113.9"));
});

test("DayDayMap ant-table-row extracts IP/port and skips measure-row", async () => {
  const html = `<!doctype html><html><body>
    <div class="style__StyleSummary-sc-x">共有 2,163,417,935 个检索结果，独立IP 1,773,193,421 个</div>
    <table class="ant-table">
      <thead class="ant-table-thead"><tr><th>序号</th><th>IP</th><th>域名/访问链接</th><th>端口</th><th>服务</th><th>传输</th></tr></thead>
      <tbody class="ant-table-tbody">
        <tr class="ant-table-measure-row" aria-hidden="true"><td></td><td></td><td></td><td></td><td></td><td></td></tr>
        <tr class="ant-table-row ant-table-row-level-0" data-row-key="1">
          <td class="ant-table-cell"><span class="ant-table-cell-content">1</span></td>
          <td class="ant-table-cell"><span class="ant-table-cell-content"><div class="ant-flex"><span class="ellipsis cp">203.0.113.90</span></div>视频监控设备+0</span></td>
          <td class="ant-table-cell"><span class="ant-table-cell-content">dayday.example.test</span></td>
          <td class="ant-table-cell"><span class="ant-table-cell-content">80</span></td>
          <td class="ant-table-cell"><span class="ant-table-cell-content">http</span></td>
          <td class="ant-table-cell"><span class="ant-table-cell-content">tcp</span></td>
        </tr>
      </tbody>
    </table>
  </body></html>`;
  const result = await extractFromHTML(html, "https://www.daydaymap.com/searchResult?keyword=cG9ydD0iODAi");
  assert.equal(result.row_selector_used, "tr.ant-table-row");
  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].ip, "203.0.113.90");
  assert.equal(result.items[0].port, 80);
  assert.equal(result.items[0].protocol, "http");
  assert.equal(result.items[0].host, "dayday.example.test");
});

test("DayDayMap empty summary is not a result row", async () => {
  const html = `<!doctype html><html><body>
    <div class="search-result-header"><div class="style__StyleSummary-sc-x">共有0个检索结果，独立IP0个</div></div>
    <div class="ant-empty-description">暂无数据</div>
  </body></html>`;
  const result = await extractFromHTML(html, "https://www.daydaymap.com/searchResult?keyword=port%3D80");
  assert.equal(result.items.length, 0);
});

test("Shodan hostnames that duplicate the IP are not copied into host", async () => {
  const html = `<!doctype html><html><body>
    <h4 class="total-results">10</h4>
    <div class="l-search-results">
      <div class="result">
        <div class="heading">
          <a class="title" href="/host/1.2.3.4">nginx</a>
          <a class="text-danger" href="http://1.2.3.4:80"></a>
        </div>
        <ul><li class="hostnames">www.example.com</li></ul>
      </div>
    </div>
  </body></html>`;
  const result = await extractFromHTML(html, "https://www.shodan.io/search?query=port%3A80");
  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].ip, "1.2.3.4");
  assert.equal(result.items[0].host, "www.example.com");
});

test("ENGINE_SELECTORS do not put generic result>div ahead of engine rows", () => {
  for (const [name, sel] of Object.entries(ENGINE_SELECTORS)) {
    const idxGeneric = sel.row.indexOf("[class*='result'] > div");
    const idxPrimary = sel.row.findIndex((s) => !s.includes("[class*='result'] > div"));
    if (idxGeneric >= 0) {
      assert.ok(idxGeneric > idxPrimary, `${name} generic selector is too early`);
    }
  }
  assert.equal(ENGINE_SELECTORS.zoomeye.row.includes("[class*='result'] > div"), false);
  assert.equal(ENGINE_SELECTORS.daydaymap.row.includes("[class*='result'] > div"), false);
});

test("Quake dedicated login title is a login wall", async () => {
  const result = await extractFromHTML('<html><head><title>登录 - 360网络空间资产测绘</title></head><body><input type="password"><button>登录</button></body></html>', 'https://quake.360.net/quake/#/searchResult');
  assert.equal(result.is_login_wall, true);
  assert.equal(result.items.length, 0);
});

test("Shodan challenge page is not an empty successful collection", async () => {
  const result = await extractFromHTML('<html><head><title>Just a moment...</title></head><body>Verify you are human</body></html>', 'https://www.shodan.io/search?query=port%3A80');
  assert.equal(result.browser_challenge, true);
  assert.equal(result.extraction_method, 'browser_challenge');
  assert.equal(result.items.length, 0);
});

test("Login link on a result page does not mark the page as a login wall", async () => {
  const result = await extractFromHTML('<html><head><title>Search results</title></head><body><a>登录</a><div class="hsxa-meta-data-item"><span class="hsxa-host">203.0.113.8</span><span class="hsxa-port">80</span></div></body></html>', 'https://fofa.info/result');
  assert.equal(result.is_login_wall, false);
  assert.equal(result.items.length, 1);
});

test("Quake preserves visible domain card headers when the IP is masked", async () => {
  const cards = Array.from({length:10}, (_,i)=>`<div class="item-container"><div class="ip"><span class="copy_btn" data-clipboard-text="asset${i}.example.test">asset${i}.example.test</span></div><span class="port">80</span><span class="server-protocol">http</span><span>122.*.*.*</span></div>`).join('');
  const result = await extractFromHTML('<html><body>'+cards+'</body></html>', 'https://quake.360.net/quake/#/searchResult');
  assert.equal(result.items.length,10);
  assert.equal(result.items[0].host,'asset0.example.test');
  assert.equal(result.items[0].ip || '', '');
  assert.equal(result.items[0].port,80);
});

test("Quake CDP script preserves visible domain headers without fabricating IPs", async () => {
  const {readFileSync} = await import('node:fs');
  const {runInNewContext} = await import('node:vm');
  const source = readFileSync(new URL('../../../internal/screenshot/dom_selectors.go',import.meta.url),'utf8');
  const script = source.match(/const extractQuakeJS = `([\s\S]*?)`/)[1];
  const {document} = parseHTML('<html><body><div class="item-container"><div class="ip"><span class="copy_btn" data-clipboard-text="asset.example.test">asset.example.test</span></div><span class="port">80</span></div></body></html>');
  const result = JSON.parse(runInNewContext(script,{document}));
  assert.equal(result.assets.length,1);
  assert.equal(result.assets[0].host,'asset.example.test');
  assert.equal(result.assets[0].ip || '', '');
  assert.equal(result.assets[0].port,80);
});
