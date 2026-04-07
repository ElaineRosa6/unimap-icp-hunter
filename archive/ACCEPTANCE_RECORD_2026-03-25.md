# 修复项验收记录（2026-03-25）

## 验收范围
- 依据修复清单：[REPAIR_CHECKLIST_2026-03-17.md](REPAIR_CHECKLIST_2026-03-17.md)
- 依据能力对齐计划：[FEATURE_PARITY_PLAN_2026-03-23.md](FEATURE_PARITY_PLAN_2026-03-23.md)
- 验收方式：代码证据核对 + 自动化测试与构建验证

## 自动化验证结果
- 2026-03-25 执行 go test ./...：通过
- 2026-03-25 执行 go build ./...：通过

## 逐项验收（通过 / 证据 / 风险）

1. URL 可达性探测接口与监控前置过滤
- 结果：通过
- 证据：
  - 路由注册 [web/router.go](web/router.go#L73)
  - 处理器实现 [web/monitor_handlers.go](web/monitor_handlers.go#L80)
  - 前端过滤与提示 [web/templates/monitor.html](web/templates/monitor.html#L674)
  - 前端不可达统计更新 [web/templates/monitor.html](web/templates/monitor.html#L696)
- 残余风险：对特殊网络环境（代理、私有证书、企业内网 DNS）的可达性分类，仍建议做现场联调。

2. 监控统计语义修正（格式合法、可达、不可达）
- 结果：通过
- 证据：
  - 格式合法计数 [web/templates/monitor.html](web/templates/monitor.html#L398)
  - 可达计数 [web/templates/monitor.html](web/templates/monitor.html#L406)
  - 不可达计数 [web/templates/monitor.html](web/templates/monitor.html#L410)
- 残余风险：仅覆盖页面统计逻辑，未包含多语言文案一致性验收。

3. 篡改检测状态分层与 summary 返回
- 结果：通过
- 证据：
  - 篡改检测处理器 [web/tamper_handlers.go](web/tamper_handlers.go#L47)
  - summary 返回 [web/tamper_handlers.go](web/tamper_handlers.go#L80)
  - no_baseline 状态 [web/tamper_handlers.go](web/tamper_handlers.go#L228)
- 残余风险：对大批量 URL 并发下状态分布准确性，建议补充压力测试。

4. Tamper Baseline Delete 方法契约修复（DELETE + query url）
- 结果：通过
- 证据：
  - 路由方法为 DELETE [web/router.go](web/router.go#L79)
  - 方法契约测试 [web/tamper_handlers_test.go](web/tamper_handlers_test.go#L79)
  - query 参数删除测试 [web/tamper_handlers_test.go](web/tamper_handlers_test.go#L91)
- 残余风险：跨版本客户端若仍使用旧调用方式，可能收到 405，需要发布说明。

5. Tamper History Delete API
- 结果：通过
- 证据：
  - 路由注册 [web/router.go](web/router.go#L81)
  - 删除行为测试 [web/tamper_handlers_test.go](web/tamper_handlers_test.go#L16)
- 残余风险：删除操作为按 URL 全量删除，未提供软删除或回收站机制。

6. Screenshot 批次/文件管理 API（列表与删除）
- 结果：通过
- 证据：
  - 路由注册 [web/router.go](web/router.go#L65)
  - 路由注册 [web/router.go](web/router.go#L66)
  - 路由注册 [web/router.go](web/router.go#L67)
  - 路由注册 [web/router.go](web/router.go#L68)
  - 列表测试 [web/screenshot_handlers_test.go](web/screenshot_handlers_test.go#L44)
  - 删除安全测试 [web/screenshot_handlers_test.go](web/screenshot_handlers_test.go#L93)
- 残余风险：大目录场景下批次/文件列表的性能边界尚未量化。

7. Screenshot 路径安全（路径穿越拦截）
- 结果：通过
- 证据：
  - 规范化测试 [web/screenshot_handlers_test.go](web/screenshot_handlers_test.go#L20)
  - 路径穿越请求用例 [web/screenshot_handlers_test.go](web/screenshot_handlers_test.go#L105)
- 残余风险：仅覆盖服务器端校验，建议在前端也增加输入预校验提升可用性。

8. CLI API-first 子命令（query/tamper-check/screenshot-batch）
- 结果：通过
- 证据：
  - query 调用 API [cmd/unimap-cli/api_subcommands.go](cmd/unimap-cli/api_subcommands.go#L82)
  - tamper-check 子命令 [cmd/unimap-cli/api_subcommands.go](cmd/unimap-cli/api_subcommands.go#L109)
  - screenshot-batch 子命令 [cmd/unimap-cli/api_subcommands.go](cmd/unimap-cli/api_subcommands.go#L151)
- 残余风险：当前仍依赖 Web API 可用性，离线场景需要明确 fallback 或错误提示策略。

9. GUI API-first + 本地回退（监控/基线/历史/截图）
- 结果：通过
- 证据：
  - 篡改检测 API 调用 [cmd/unimap-gui/monitor_native.go](cmd/unimap-gui/monitor_native.go#L1107)
  - 基线设置 API 调用 [cmd/unimap-gui/monitor_native.go](cmd/unimap-gui/monitor_native.go#L1161)
  - 基线列表 API 调用 [cmd/unimap-gui/monitor_native.go](cmd/unimap-gui/monitor_native.go#L1214)
  - 历史删除 API 调用 [cmd/unimap-gui/monitor_native.go](cmd/unimap-gui/monitor_native.go#L1323)
  - 截图批次列表 API 调用 [cmd/unimap-gui/monitor_native.go](cmd/unimap-gui/monitor_native.go#L1359)
  - 批量截图 API 调用 [cmd/unimap-gui/monitor_native.go](cmd/unimap-gui/monitor_native.go#L1596)
- 残余风险：API 失败时回退链路覆盖较多分支，建议补充 GUI 集成测试覆盖“API 失败 -> 本地成功”。

10. GUI 入口与中文字体修复
- 结果：通过
- 证据：
  - URL 监控/历史记录/截图管理页签 [cmd/unimap-gui/main.go](cmd/unimap-gui/main.go#L85)
  - 字体环境变量 UNIMAP_GUI_FONT [cmd/unimap-gui/cjk_theme_gui.go](cmd/unimap-gui/cjk_theme_gui.go#L43)
- 残余风险：不同 Windows 版本字体安装差异可能导致个别机器仍需手动指定字体路径。

## 总体结论
- 当前修复项在“代码存在性 + 自动化验证”维度均满足通过条件。
- 未发现阻断发布的回归信号。
- 建议发布前补 1 轮端到端冒烟（Web 页面与 GUI 实机）以覆盖环境相关因素。