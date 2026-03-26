# UniMap 项目现状总结

更新日期：2026-03-23

## 1. 项目定位

UniMap 是一个多引擎网络资产查询与网页监控工具，提供三类入口：

- Web：cmd/unimap-web
- CLI：cmd/unimap-cli
- GUI：cmd/unimap-gui

当前主线能力包括：

- UQL 统一查询语法
- 多引擎适配与编排（FOFA、Hunter、ZoomEye、Quake、Shodan）
- 查询结果归并与导出（CSV/JSON/Excel）
- 截图能力与 CDP 登录态复用
- 网页篡改检测（基线、检查、历史）

## 2. 当前架构

核心分层如下：

- 入口层：cmd/*
- 服务层：internal/service/unified_service.go
- 引擎层：internal/adapter/* + orchestrator
- 插件层：internal/plugin/*
- 能力层：internal/screenshot/*、internal/tamper/*
- 表现层：web/server.go + web/templates + web/static

说明：当前是单仓多入口架构，不是严格微服务拆分。

## 3. 工程现状

基于 2026-03-23 的本地检查：

- go test ./...：通过
- go vet ./...：通过
- Go 源文件数：59
- 单元测试文件数：4
- web/server.go 行数：510（已完成多轮 handler 拆分）

## 4. 已确认优化方向

高优先级：

1. 拆分 web/server.go，降低单文件复杂度（已完成主要拆分）
2. 校准文档与实现一致性（README、QUICKSTART、USAGE）（已完成首轮）
3. 补齐基础单测（parser/orchestrator/tamper）（已完成首轮）

中优先级：

1. 缓存配置统一化（避免 service 层硬编码）（进行中）
2. 并发模型统一化（tamper 批处理接入 workerpool）（已完成）
3. API 分层清晰化（handler 与业务逻辑进一步解耦）（进行中）

低优先级：

1. 观测性增强（指标、request_id）
2. 热路径性能优化（regex 复用、对象分配优化）
3. 文档自动化校验脚本

## 5. 已有复核文档

详细复核见：PROJECT_FULL_REVIEW_2026-03-20.md

该文档包含：

- 架构全景
- 优化点分级与路线图
- 文档体系问题与重构建议

## 6. 下一步建议

建议按以下顺序执行：

1. 文档统一（已完成首轮）
2. 基础测试骨架（已完成首轮）
3. Web handler 拆分（已完成 tamper/screenshot）
4. 缓存与并发配置统一（已完成首轮）
5. 继续推进 query/tamper/screenshot 的 application service 下沉
6. 完成 request_id 链路与文档自动校验脚本

---

本文件用于描述项目“当前状态”，不再使用“100% 完成度”这类不可验证口径。
