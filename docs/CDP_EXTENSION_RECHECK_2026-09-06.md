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

1. **代码门禁已完成**：修复提交 `ededd76`，扩展CI门禁提交 `c2acd9d`；全量竞态、lint、vet、build、补丁/回滚通过。后者主CI 11/11、bridge-smoke通过，两工作流各执行39项扩展回归并保留日志。
2. 用户确认已安装扩展版本、路径和Bridge状态后，加载修复版本并验证DayDayMap活页非空、端口正确、无重复及Bridge任务回传。其他引擎逐项复验，分别记录。
3. ZoomEye定位导航与渲染；Censys按实际挑战状态记录，不把挑战页或空结果当成功。
4. 云端截图、证据截图和已禁用调度保持不变；云端图片送达依旧需要受控页面与DNS输入。

原始本机证据位于工作区外 `unimap-review-20260906` 的 `cdp-current-20260906`、`daydaymap-dom-diagnosis`、`round48` 目录，不包含Cookie值。截图和测试过程退出0不自动等于业务通过。

## 后续真实复验与环境诊断

以下均为独立本地headless Chrome，不是用户已登录Chrome，也未交接其WebStorage；不改写历史登录态的通过记录。

| 引擎 | 后续实测 | 证据所支持的结论 |
|---|---|---|
| FOFA | 导航ERR_TUNNEL_CONNECTION_FAILED，0资产、无截图 | 只读出口诊断捕获Windows connectex访问权限拒绝，发生在TCP阶段，不是选择器证据 |
| Hunter | 首次空白PNG、0资产；后续导航超时 | 暂未确定原因，不因有Cookie或PNG有效判通过 |
| Quake | 60秒导航超时，0资产、无截图 | 当前受限；未证明选择器或账号失效 |
| Shodan | 挑战页、0资产、challenge=true | 挑战识别一致；此次未验证Bridge回退 |
| ZoomEye | 当前查询URL和候选编码URL对照分别超时、连接失败 | 未有效渲染，暂不据此改变编码或域名 |

只读防火墙检查发现3条启用的 `codex_sandbox_offline_block_*` 出站规则，均有LocalUser条件；其地址范围与部分失败连接相符，但未通过精确身份/事件关联证明单条规则是唯一原因。不能据Program=Any推断用户Chrome或整个系统均被禁止。规则未修改，受拒连接未通过替代代理、身份或其他通道重试。

剩余外部输入：操作者核对当前任务网络权限；提供遮蔽token的扩展详情与状态页，确认0.4.19加载和Bridge连接；受控云端页面/DNS接口。输入齐备前，不以离线回归或CI替代真实活页、Bridge回传、图片送达。

证据追加目录：`cdp-four-current`、`zoomeye-url-diagnosis`、`egress-live-diagnosis`、`release-c2acd9d`，均位于上述工作区外审查目录。

## 追加：用户 Chrome 实际 Bridge 复验（本节更新上文待确认项）

用户截图确认已加载 0.4.19；本地隔离服务启动后自动配对，Bridge 在线、回调 HTTP 200。未修改原配置；测试实例禁用调度与通知。

| 引擎 | 扩展真实采集数量 | 截图/落盘 | 结论 |
|---|---:|---|---|
| DayDayMap | 10 | PNG + 10 条落盘 | 采集截图链路通过 |
| FOFA | 10 | PNG + 合并 API 后 20 条落盘 | 扩展非空通过，不把 API 数量算作扩展数量 |
| Hunter | 9 | PNG + 合并 API 后 19 条落盘 | 非空通过；页面显示10行，截断CDN地址行及完整字段仍需核验 |
| ZoomEye | 10 | PNG + 10 条落盘 | .org 活页非空通过 |
| Censys | 100 | PNG + 100 条落盘 | 非空且端点无重复；页面数量与 page_size=10 并不等价，完整分页语义未验收 |
| Quake | 0 | 登录页 PNG，API另返回10条 | 扩展业务未通过，不能用API结果替代 |
| Shodan | 0 | Just a moment... 挑战页 PNG | 扩展业务未通过 |

本轮原生 CDP 七引擎全部运行：FOFA 导航超时，其余为 ERR_TUNNEL_CONNECTION_FAILED，未进入结果提取。独立 headless Chrome 与用户已登录 Chrome 是不同会话；未绕过 guarded 出口、替换网络身份或修改防火墙。此前 DayDayMap CDP 成功仍只代表此前运行。本轮不宣称 CDP 全绿。

本地待发布修复：Web 响应保留服务层 API 错误，避免 success 与历史 partial 不一致；扩展 0.4.20 增加专用登录标题、挑战页识别，并通过现有失败回调报告 login_required / browser_challenge。新增回归先失败后通过，扩展共42项无跳过。0.4.20 仍需用户重新加载后活页确认，不把源码版本等同于正在运行的版本。

证据目录：工作区外 unimap-review-20260906/bridge-live-server、cdp-all-current、round51。通知未发送，云端配置未动。全引擎完整验收尚未完成。

修复后服务实时复跑DayDayMap：扩展仍回传10条及PNG，历史新增10条；接口与历史均为partial，并如实保留DayDayMap API凭据错误。健康检查200、Bridge在线、队列归零、通知记录0。该结果证明服务端状态修复，未证明0.4.20已在浏览器重新加载。

## 最新追加：登录后复测与域名卡片

Quake在用户登录后回传3条，Shodan回传10条，均有结果页PNG及落盘，原登录/挑战结论仅适用于前次运行。Quake活页随后确认10个卡片标题为域名，原IPv4解析遗漏；CDP与扩展已修复，源码版本0.4.21，44项Node回归通过。当前Bridge再测命中10行却回传0条，尚未确认加载修复，不能宣称活页修复完成。服务层新增有行无资产诊断，避免API的10条结果掩盖采集问题。

火绒日志已证实all.exe一次TCP连接被拦截。固定程序经用户放行后、用户退出火绒后各复跑一次：DayDayMap均10条通过；Hunter/Quake登录页、Shodan/Censys挑战、FOFA空响应、ZoomEye空白/0条仍未通过。默认Chrome根目录中的Profile 1不是独立CDP目录；新增Windows启动前预检。交互CDP登录窗口启动仍受工具策略限制，不改变防护或通过其他通道绕过。

### 服务与传输层后续核验

后续Quake任务在SQLite中保存10条浏览器来源域名资产，IP均为空；历史为partial。HTTP客户端在约60秒收到连接关闭，因此不把该次判作HTTP完整交付通过。服务端保留API连接权限拒绝诊断，健康检查200，Bridge队列清空。

查询handler的逐请求写截止时间修复已通过真实本地HTTP+SQLite对照：对照组资产已存但客户端EOF，修复组完整收到HTTP 200/partial及相同资产，重复3次。该超时修复尚未部署至运行服务；不得以本地fixture通过宣称外部HTTP复验完成。
