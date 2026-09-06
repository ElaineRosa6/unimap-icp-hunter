# CDP / Extension 分路径复验（2026-09-06）

本记录基线为 `d877386`，包含其后的 DayDayMap 卡片修复。不是实时 HEAD 或 CI 查询，不覆盖历史日期的结论。

## DayDayMap：故障与修复

独立本地 headless Chrome、标准 guarded 出口，未设置 remote debug、上游代理或持久 profile。使用本地配置中该引擎的 Cookie 数量为0，未导入现有Chrome的WebStorage。

修复前 `CollectAndCaptureSearchEngineResult` 返回0资产但PNG显示真实结果。额外诊断在准备页面3秒及再等12秒后均发现10个URL标题叶节点；旧提取器命中50个其他表格行，IPv4纯文本后备逻辑遗漏 `https://IP:端口`。本轮证据不支持单纯加长等待。

CDP 与扩展新增 `.ant-pro-card > .ant-pro-card-header .ant-pro-card-title` 路径，仅解析该标题中的 HTTP(S) IP端点。保留协议、端口，IPv6去方括号；按端点去重，拒绝带凭据、0/超范围端口以及非IP标题。保留原表格路径。未点击或请求资产目标。

## 验收层次

| 路径 | 本轮证据 | 尚未证明 |
|---|---|---|
| 原生CDP | 修复副本实测10资产、有效PNG、非挑战；原实现同固定fixture失败，修复后通过 | 七引擎全部通过、Bridge、SQLite、通知 |
| Extension 0.4.19代码 | 新卡片回归原实现失败；修复后39项Node测试通过、无skip | 用户Chrome实际加载版本、改后活页、tasks/next回传、图片通知 |
| ZoomEye原生CDP | 本轮0资产、空白PNG | 失败原因及有效业务结果 |
| Censys原生CDP | 本轮0资产，挑战页与challenge=true一致 | 原生成功、此次Bridge fallback |

Node测试以linkedom执行提取函数并模拟chrome.scripting，不能替代实际扩展注入。新版本号用于区分待加载修复；用户现有加载路径已观察到指向仓库目录，但运行时版本未核实。浏览器工具禁止访问扩展内部页面，已停止此路操作，等待用户提供遮蔽token的详情/状态页信息；不使用其他通道绕过该限制。

## 下一步

1. 完成本轮全量竞态、lint、vet、build以及补丁/回滚门禁。
2. 用户确认已安装扩展版本、路径和Bridge状态后，加载修复版本并验证DayDayMap活页非空、端口正确、无重复及Bridge任务回传。其他引擎逐项复验，分别记录。
3. ZoomEye定位导航与渲染；Censys按实际挑战状态记录，不把挑战页或空结果当成功。
4. 云端截图、证据截图和已禁用调度保持不变；云端图片送达依旧需要受控页面与DNS输入。

原始本机证据位于工作区外 `unimap-review-20260906` 的 `cdp-current-20260906`、`daydaymap-dom-diagnosis`、`round48` 目录，不包含Cookie值。截图和测试过程退出0不自动等于业务通过。
