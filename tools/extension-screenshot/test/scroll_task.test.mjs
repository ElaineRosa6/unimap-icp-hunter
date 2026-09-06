import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import vm from "node:vm";

test("actual screenshot task scrolls using MV3 scripting and returns to top before capture", async () => {
  const events = [];
  const source = readFileSync(new URL("../src/background.js", import.meta.url), "utf8")
    .replace(/^import .*;$/gm, "").replace(/^bridgeLoop\(\);$/gm, "");
  const context = vm.createContext({
    chrome:{
      runtime:{onInstalled:{addListener(){}},onStartup:{addListener(){}}},
      action:{onClicked:{addListener(){}}},
      tabs:{get:async()=>({windowId:1}),update:async()=>{}},
      windows:{update:async()=>{}},
      scripting:{executeScript:async options => {
        assert.equal(options.target.tabId,7);
        assert.equal(typeof options.func,"function");
        assert.equal(options.code,undefined);
        await vm.runInNewContext(`(${options.func.toString()})()`, {
          document:{body:{scrollHeight:1400}},
          window:{scrollTo:(x,y)=>events.push([x,y])},
          setTimeout:fn=>{fn();}
        });
        return [{result:null}];
      }}
    },
    setTimeout:fn=>{fn();},
    ensureTab:async()=>7,checkLoginCookies:async()=>null,waitForPageReady:async()=>{},
    captureVisible:async()=>{events.push("capture");return "fixture-image";},
    normalizeImagePayload:()=>({success:true}),resolveTabFinalURL:async()=>"https://example.test/",
    apiPostBridgeSigned:async()=>{},saveRuntimeState:async()=>{},releaseTab:async()=>{}
  });
  vm.runInContext(source,context);
  await vm.runInContext('handleTask({request_id:"fixture",url:"https://example.test/",action:"screenshot"},"fixture-token")',context);
  assert.deepEqual(events,[[0,1400],[0,0],"capture"]);
});
