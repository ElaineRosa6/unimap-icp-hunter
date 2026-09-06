import assert from "node:assert/strict";
import test from "node:test";
import { parseHTML } from "linkedom";
import { renderBridgeStatus } from "../src/status.js";

test("status is explicit about unknown, stale and recent polling without exposing credentials", () => {
  const { document } = parseHTML('<div id="bridgeStatus"></div>');
  const el = document.getElementById("bridgeStatus");
  renderBridgeStatus(el, "0.4.22", {}, 100000);
  assert.match(el.textContent, /尚无成功轮询/);
  renderBridgeStatus(el, "0.4.22", { paired:true, last_poll_at:99999, last_error:"SECRET", token:"SECRET" }, 100000);
  assert.match(el.textContent, /0.4.22/);
  assert.match(el.textContent, /最近轮询成功/);
  assert.doesNotMatch(el.textContent, /SECRET/);
  renderBridgeStatus(el, "0.4.22", { paired:true, last_poll_at:1 }, 100000);
  assert.match(el.textContent, /状态已过期/);
  renderBridgeStatus(el, "0.4.22", { paired:true, last_poll_at:99999, last_error_at:100000 }, 100000);
  assert.match(el.textContent, /最近一次操作失败/);
  renderBridgeStatus(el, "0.4.22", { paired:true, last_poll_at:100001 }, 100000);
  assert.match(el.textContent, /状态已过期/);
});


test("real options page loads running version and refreshes stored polling state", async () => {
  const { readFileSync } = await import("node:fs");
  const vm = await import("node:vm");
  const { document } = parseHTML(readFileSync(new URL("../src/options.html", import.meta.url), "utf8"));
  let state = {};
  const code = readFileSync(new URL("../src/options.js", import.meta.url), "utf8").replace(/^import .*;$/gm, "");
  vm.runInNewContext(code, {
    document, renderBridgeStatus, URLSearchParams, location:{ search:"" },
    normalizeLoopbackAPIBaseURL: x => x, loadAPIBaseURL: async () => "http://127.0.0.1:8448",
    saveAPIBaseURL: async () => {}, setTimeout: () => {},
    chrome: { runtime: {getManifest: () => ({version:"test-running-version"})}, storage: {local:{get:async () => ({bridgeRuntimeState:state})}} }
  });
  await new Promise(resolve => setImmediate(resolve));
  assert.match(document.getElementById("bridgeStatus").textContent, /test-running-version/);
  assert.match(document.getElementById("bridgeStatus").textContent, /尚无成功轮询/);
  state = { last_poll_at:Date.now(), last_error:"SECRET" };
  document.getElementById("refreshStatusBtn").click();
  await new Promise(resolve => setImmediate(resolve));
  assert.match(document.getElementById("bridgeStatus").textContent, /最近轮询成功/);
  assert.doesNotMatch(document.getElementById("bridgeStatus").textContent, /SECRET/);
});

test("toolbar opens options without starting a second polling loop", async () => {
  const { readFileSync } = await import("node:fs");
  const vm = await import("node:vm");
  let clicked; let opened = 0;
  const code = readFileSync(new URL("../src/background.js", import.meta.url), "utf8")
    .replace(/^import .*;$/gm, "").replace(/^bridgeLoop\(\);$/gm, "");
  vm.runInNewContext(code, {chrome:{
    runtime:{onInstalled:{addListener(){}},onStartup:{addListener(){}},openOptionsPage(){opened++;}},
    action:{onClicked:{addListener(fn){clicked=fn;}}}
  }});
  assert.equal(typeof clicked, "function");
  clicked();
  assert.equal(opened, 1);
});


test("background records successful polling only after the request succeeds", async () => {
  const { readFileSync } = await import("node:fs");
  const vm = await import("node:vm");
  const code = readFileSync(new URL("../src/background.js", import.meta.url), "utf8")
    .replace(/^import .*;$/gm, "").replace(/^bridgeLoop\(\);$/gm, "");
  for (const fail of [false, true]) {
    const states = []; let errors = 0; let polled = false;
    const stop = new Error("test iteration complete");
    const context = vm.createContext({
      chrome:{runtime:{onInstalled:{addListener(){}},onStartup:{addListener(){}}},
        action:{onClicked:{addListener(){}}},storage:{local:{get:async () => ({usingAdminToken:false})}}},
      cleanupTabPool:async () => {},
      loadSessionToken:async () => ({token:"fixture-only",expireAt:Date.now()+600000}),
      isTokenExpired:() => false,
      saveRuntimeState:async state => {assert.equal(polled,true);states.push(state);},
      saveLastError:async () => {errors++;},
      apiGet:async () => {
        assert.equal(states.length,0);
        polled = true;
        if (fail) throw new Error("network fixture failed");
        return {task:null};
      },
      setTimeout:() => {throw stop;}
    });
    vm.runInContext(code, context);
    await assert.rejects(vm.runInContext("bridgeLoop()",context), error => error === stop);
    assert.equal(states.length, fail ? 0 : 1);
    assert.equal(errors, fail ? 1 : 0);
    if (!fail) assert.ok(states[0].last_poll_at > 0);
  }
});
