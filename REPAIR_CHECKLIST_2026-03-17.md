# 修复清单（2026-03-17）

基于问题核实记录 [ISSUE_VERIFICATION_2026-03-17.md](ISSUE_VERIFICATION_2026-03-17.md) 形成本清单。

## 目标
- 修正 Web 监控中“有效 URL”定义与实际可达性不一致问题。
- 消除“基线成功为 0 时仍进入篡改检测报错”的误导性流程。
- 推进 GUI 与 Web 功能对齐。
- 修复 GUI 中文显示乱码风险。

## P1：Web 监控可达性与检测流程

### 1. 修正“有效 URL”定义
1. 将监控页“有效 URL”改名为“格式合法 URL”，避免误导。
2. 新增 URL 探活接口（建议 `HEAD` 优先，失败回退 `GET`，并发可控、超时可配）。
3. 在监控页提交篡改检测/设置基线/批量截图前，先探活并分类：`reachable`、`unreachable`、`invalid_format`。
4. 统计项调整为：总数、格式合法、可达、不可达。

实现位置建议：
- 前端判定逻辑：[web/templates/monitor.html](web/templates/monitor.html#L250)
- 监控页动作入口：[web/templates/monitor.html](web/templates/monitor.html#L620)
- 服务端新增探活 API：建议新增于 [web/server.go](web/server.go)

验收标准：
- 不可访问 URL 不再计入“可达”。
- 检测前可明确看到不可达数量及失败原因（DNS/超时/TLS/连接拒绝）。

### 2. 修正“基线 0 + 篡改报错”流程
1. 基线接口返回结构化汇总：`total`、`saved`、`unreachable`、`failed`。
2. 当 `saved=0` 时，前端阻止直接进入篡改检测，提示“无可比对基线”。
3. 篡改检测结果增加状态分层：`no_baseline`、`unreachable`、`tampered`、`normal`。
4. 前端渲染中将“系统错误”和“目标不可达/无基线”分开展示。

实现位置建议：
- 基线接口：[web/server.go](web/server.go#L2467)
- 篡改检测接口：[web/server.go](web/server.go#L2422)
- 批处理状态生成：[internal/tamper/detector.go](internal/tamper/detector.go#L569)
- 前端渲染逻辑：[web/templates/monitor.html](web/templates/monitor.html#L699)

验收标准：
- 基线成功为 0 时，前端不再展示“篡改报错”，而是明确提示“无可比对基线/目标不可达”。

### 3. 提升监控结果可读性
1. 结果卡拆分三层状态：探活状态、基线状态、篡改状态。
2. 对失败目标提供一键重试探活。
3. 对首次检测与无基线场景分别提供明确文案。

实现位置建议：
- 结果卡渲染：[web/templates/monitor.html](web/templates/monitor.html#L699)

验收标准：
- 用户可快速判断是“页面被改”还是“不可访问/无基线”。

## P2：GUI 与 Web 功能对齐

### 4. 分阶段功能同步
1. 阶段 A（快速交付）：在 GUI 增加“网页监控”“配额”“截图管理”入口，跳转 Web 页面。
2. 阶段 B（完整对齐）：GUI 内原生实现 URL 监控、基线管理、篡改检测、批量截图、历史记录。

实现位置建议：
- GUI 主界面：[cmd/unimap-gui/main.go](cmd/unimap-gui/main.go#L75)
- Web 监控入口：[web/templates/index.html](web/templates/index.html#L109)
- Web 路由能力：[web/server.go](web/server.go#L310)

验收标准：
- GUI 至少具备与 Web 等价的监控能力入口（阶段 A）。
- 阶段 B 后 GUI 可独立完成监控全流程。

## P2：GUI 乱码修复

### 5. 字体策略修正
1. 调整 GUI 主题字体策略：`Monospace` 文本也支持 CJK 回退。
2. 移除中文标题上的 `Monospace` 样式，避免落回不含中文字形字体。
3. 在文档中补充字体排查与 `UNIMAP_GUI_FONT` 配置示例。

实现位置建议：
- 主题字体逻辑：[cmd/unimap-gui/cjk_theme_gui.go](cmd/unimap-gui/cjk_theme_gui.go#L26)
- 标题样式：[cmd/unimap-gui/main.go](cmd/unimap-gui/main.go#L83)
- 构建说明：[GUI_BUILD.md](GUI_BUILD.md)

验收标准：
- Windows 默认环境与手动设置字体两种场景下，中文均正常显示。

## 回归测试清单

### A. Web 监控
1. 全可达 URL 集合：基线成功率应接近 100%。
2. 全不可达 URL 集合：应标记为不可达，不进入篡改误报。
3. 混合集合：可达与不可达分类准确。

### B. 基线与篡改
1. 首次检测：显示首次检测状态并可设置基线。
2. 基线存在且页面未变：返回 `normal`。
3. 基线存在且页面变更：返回 `tampered` 且分段差异可见。

### C. GUI
1. 中文文本显示（标题、帮助、配置弹窗、结果表格）。
2. 功能入口一致性（阶段 A/B 对应能力）。

## 建议实施顺序
1. 先完成 P1（可达性 + 基线/篡改状态语义修正）。
2. 再完成 GUI 乱码修复（低风险高收益）。
3. 最后推进 GUI/Web 功能对齐（分阶段交付）。

## 进度记录（截至 2026-03-17）

### 已完成（P1 第一批）
1. 已新增 URL 可达性检测接口，并接入监控流程前置探测。
2. 监控页统计语义已调整为：总数、格式合法、格式非法、可达、不可达。
3. 基线设置与篡改检测接口已返回结构化汇总信息。
4. 基线列表返回值已修复为真实 URL（不再是存储文件名）。
5. 篡改结果前端文案已区分“目标不可达”“无基线”“已篡改/安全”。
6. 已完成编译与测试验证：go test ./... 通过。

对应变更位置：
- 可达性接口与汇总返回：[web/server.go](web/server.go#L312)
- 可达性实现：[web/server.go](web/server.go#L1798)
- 篡改检测 summary：[web/server.go](web/server.go#L2627)
- 基线设置 summary：[web/server.go](web/server.go#L2705)
- 基线列表 URL 修复：[internal/tamper/detector.go](internal/tamper/detector.go#L155)
- 监控页流程与状态渲染：[web/templates/monitor.html](web/templates/monitor.html#L338)

### 已完成（P1 第二批）
1. 结果卡已拆分三段状态：探活状态、基线状态、篡改状态。
2. 已增加失败目标一键重试探活入口。
3. 检测失败原因已结构化分类展示（DNS/超时/TLS/连接拒绝/HTTP 状态）。

对应变更位置：
- 探活错误分类与 reason_type 返回：[web/server.go](web/server.go#L1822)
- 篡改结果状态分层字段：[internal/tamper/detector.go](internal/tamper/detector.go#L59)
- 监控页三段状态与重试探活：[web/templates/monitor.html](web/templates/monitor.html#L823)

### 已完成（P2 第一批）
1. GUI 乱码修复：主题字体策略已支持 Monospace + CJK 回退。
2. GUI/Web 功能对齐（阶段 A）：已在 GUI 增加“网页监控”“配额”“截图管理”入口（跳转 Web）。
3. 已补充 GUI 字体排查与 `UNIMAP_GUI_FONT` 配置示例。

对应变更位置：
- GUI 字体回退策略：[cmd/unimap-gui/cjk_theme_gui.go](cmd/unimap-gui/cjk_theme_gui.go#L26)
- GUI 入口与标题样式：[cmd/unimap-gui/main.go](cmd/unimap-gui/main.go#L75)
- 构建文档补充：[GUI_BUILD.md](GUI_BUILD.md#L58)

### 后续阶段
1. P2-功能对齐（阶段 B）：GUI 内原生实现 URL 监控、基线管理、篡改检测、批量截图、历史记录。
