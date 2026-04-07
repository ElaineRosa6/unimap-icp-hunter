# 问题核实记录（2026-03-17）

## 核查范围
- Web 网页监控（URL 有效性判定、基线与篡改检测联动）
- GUI 与 Web 功能同步性
- GUI 中文显示（乱码）

## 结论总览
1. 已核实：Web 监控页面的“有效 URL”统计仅做格式校验，不做存活性校验，无法区分真实可访问与不可访问目标。
2. 已核实：当目标不可达时，基线可出现 0 成功，后续篡改检测会进入“检测失败”分支，体验上表现为“基线 0 + 篡改报错”。
3. 已核实：GUI 与 Web 功能未同步，GUI 当前只覆盖查询/配置/导出，缺少 Web 侧监控与截图等能力。
4. 已核实：GUI 存在中文显示风险，主题层对等宽字体未应用 CJK 字体，含中文且启用 Monospace 的文本可能出现乱码/方块字。

---

## 问题 1：Web 监控无法区分“格式有效”与“真实存活”URL

### 证据
- 前端有效 URL 判定仅使用正则匹配：
  - [web/templates/monitor.html](web/templates/monitor.html#L250)
- 有效 URL 列表直接来自上述正则过滤：
  - [web/templates/monitor.html](web/templates/monitor.html#L286)
- 正则通过后只做协议补全，不做连通性探测：
  - [web/templates/monitor.html](web/templates/monitor.html#L295)
- 后端篡改检测直接进入 chromedp 导航与取页流程，失败后记为 error：
  - [internal/tamper/detector.go](internal/tamper/detector.go#L197)
  - [internal/tamper/detector.go](internal/tamper/detector.go#L590)

### 影响
- 统计面板中的“有效 URL”本质是“格式合法 URL”，不代表网站存活。
- 大量不可达 URL 仍会被送入基线/篡改流程，导致失败率高、用户感知与统计名词不一致。

### 复现路径
1. 在监控页输入格式合法但不可达的 URL（例如不存在域名或被防火墙拦截地址）。
2. 观察“有效 URL”计数仍增加。
3. 执行篡改检测，结果出现检测失败（error 状态），而不是“无效/不存活 URL”分类。

---

## 问题 1-1（关联）：基线成功数为 0 后，篡改检测呈现报错

### 证据
- 设置基线调用：
  - [web/templates/monitor.html](web/templates/monitor.html#L654)
- 基线结果中，页面加载失败会写入 status=error:
  - [internal/tamper/detector.go](internal/tamper/detector.go#L633)
- 篡改检测中，页面加载失败同样写入 current_hash.status=error:
  - [internal/tamper/detector.go](internal/tamper/detector.go#L590)
- 前端渲染将该状态直接标记为“检测失败”：
  - [web/templates/monitor.html](web/templates/monitor.html#L717)

### 影响
- 当 URL 全部不可达时：基线“成功数=0”，后续篡改检测全部走失败分支。
- 用户会感知为“篡改检测报错”，而不是“目标不可访问/无基线可比对”。

---

## 问题 2：GUI 与 Web 功能未同步

### 证据（Web 侧存在）
- Web 暴露监控、篡改、批量截图相关路由：
  - [web/server.go](web/server.go#L310)
  - [web/server.go](web/server.go#L312)
  - [web/server.go](web/server.go#L313)
- 首页入口包含“网页监控”：
  - [web/templates/index.html](web/templates/index.html#L109)

### 证据（GUI 侧缺失）
- GUI 主界面按钮仅包含：查询、清空、导出、配置、帮助：
  - [cmd/unimap-gui/main.go](cmd/unimap-gui/main.go#L75)
  - [cmd/unimap-gui/main.go](cmd/unimap-gui/main.go#L260)
  - [cmd/unimap-gui/main.go](cmd/unimap-gui/main.go#L339)
- GUI 内未发现监控页、篡改检测、批量截图、配额页、CDP 连接、WebSocket 异步查询等对应入口与流程。

### 影响
- 两端能力集合不一致，用户在 GUI 中无法完成 Web 已支持的监控与防篡改场景。

---

## 问题 3：GUI 中文显示存在乱码风险

### 证据
- CJK 主题中，若文本样式为 Monospace，会回退默认字体，不使用 CJK 字体：
  - [cmd/unimap-gui/cjk_theme_gui.go](cmd/unimap-gui/cjk_theme_gui.go#L26)
- GUI 顶部标题显式设置了 Monospace，同时包含中文：
  - [cmd/unimap-gui/main.go](cmd/unimap-gui/main.go#L83)
- 这会导致在默认等宽字体不含中文字形时出现乱码/方块。

### 影响
- 在部分 Windows 环境下，虽然已加载 CJK 常规字体，但 Monospace 文本仍可能异常显示。

---

## 建议优先级
- P1：问题 1（有效 URL 与存活性判定脱节）
- P1：问题 1-1（基线 0 与篡改失败链路提示不清）
- P2：问题 2（GUI/Web 能力对齐）
- P2：问题 3（GUI 字体策略修正，覆盖 Monospace）

## 备注
- 本次为代码核查与逻辑核实，未对页面进行完整人工点击回归。
- 当前环境下 GUI 构建命令验证通过：go test -tags gui ./cmd/unimap-gui
