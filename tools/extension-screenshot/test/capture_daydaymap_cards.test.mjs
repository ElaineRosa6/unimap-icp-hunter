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

test("DayDayMap cards preserve ports and ignore summaries and duplicate headers", async () => {
 const card = value => `<div class="ant-pro-card"><div class="ant-pro-card-header"><div class="ant-pro-card-title"><span>${value}</span></div></div></div>`;
 const html = `<html><body><table><tbody><tr><td>Country</td><td>42</td></tr></tbody></table>${card('https://192.0.2.1:80')}${card('https://192.0.2.1:80')}${card('http://192.0.2.1:8080')}${card('https://999.1.1.1:80')}<aside>https://192.0.2.99:443</aside></body></html>`;
 const result = await extractFromHTML(html, 'https://www.daydaymap.com/searchResult?keyword=x');
 assert.equal(result.items.length, 2);
 assert.deepEqual(result.items.map(x => [x.ip,x.port,x.protocol]), [['192.0.2.1',80,'https'],['192.0.2.1',8080,'http']]);
});

for (const [name, endpoint, expected] of [
 ['default_https','https://192.0.2.3',['192.0.2.3',443,'https']],
 ['ipv6','http://[2001:db8::1]:8080',['2001:db8::1',8080,'http']],
 ['zero_port','https://192.0.2.3:0',null],
 ['large_port','https://192.0.2.3:65536',null],
 ['credentials','https://fixture:fixture@192.0.2.3',null],
 ['hostname','https://example.test:443',null],
]) test(`DayDayMap card endpoint ${name}`, async () => {
 const html = `<html><body><div class="ant-pro-card"><div class="ant-pro-card-header"><div class="ant-pro-card-title"><span>${endpoint}</span></div></div></div></body></html>`;
 const result = await extractFromHTML(html, 'https://www.daydaymap.com/searchResult?keyword=x');
 assert.equal(result.items.length, expected ? 1 : 0);
 if(expected) assert.deepEqual([result.items[0].ip,result.items[0].port,result.items[0].protocol],expected);
});
