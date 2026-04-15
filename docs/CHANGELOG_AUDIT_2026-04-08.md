# Changelog 对账与综合评估

**日期**: 2026-04-08
**审计范围**: `docs_archive/CHANGELOG.md` 与当前仓库代码、测试、路由、服务层实现的对账
**审计结论**: 主业务功能大体成立，但 changelog 对部分能力的“已完成”表述偏乐观，当前仓库仍存在构建/测试失配与安全硬缺口

## 1. 审计方法

本次核查采用三步法：

1. 对照 `docs_archive/CHANGELOG.md` 中 2.0.1 至 2.1.5 的版本条目，提取关键功能声明。
2. 在代码中检查对应文件、服务层调用链、路由注册、测试覆盖与实际运行入口。
3. 运行全量测试 `go test ./...` 验证仓库是否达到 changelog 中隐含的“可用、已验证、已闭环”状态。

## 2. 总体结论

### 2.1 结论摘要

- 查询、截图、篡改检测、历史记录、桥接等主业务链路都已落地，说明仓库不是“只有文档没有实现”。
- 但缓存优化、工程稳定性和安全防护的完成度低于 changelog 的表述，至少有一部分“实现了模块”并不等于“真正进入主流程”。
- 在审计时点，仓库并非全绿，`go test ./...` 当时已失败；后续修复后，当前状态已恢复通过。

### 2.2 评估等级

- 功能完整度: 中高
- 业务闭环度: 中等
- 工程健康度: 中低
- 安全完善度: 中低

## 3. 版本对账表

### 3.1 [2.1.5] 篡改检测系统优化

| changelog 声明 | 代码核查结果 | 判断 |
|---|---|---|
| 5 种检测模式、恶意内容阈值、强信号概念 | `internal/tamper/detector.go` 中确实存在多模式判定、`suspiciousFlags`、`hidden_iframe`、危险事件处理器等逻辑 | 基本属实 |
| Script 切片分析引擎 | `internal/tamper/analyzer/script_analyzer.go` 存在，且 `detector.go` 对脚本片段有哈希/检测逻辑 | 基本属实 |
| 黑帽 SEO 检测模块 | `internal/tamper/analyzer/seo_detector.go` 存在，且 `detector.go` 中有隐藏元素、隐藏 iframe、重定向等模式 | 基本属实 |
| 恶意代码识别算法 | `internal/tamper/analyzer/malicious_detector.go` 存在 | 基本属实 |
| 规则数据库与白名单管理 | `internal/tamper/database/` 目录下的 schema/repository 文件存在 | 基本属实 |
| 编码解码、动态阈值、规则优先级 | `internal/tamper/decoder/decoder.go`、`internal/tamper/threshold/dynamic_threshold.go`、`internal/tamper/priority/rule_priority.go` 存在 | 基本属实 |

#### 3.1.1 需要注意的点

- 篡改检测确实已经从“单一哈希比对”升级到“分段 + 恶意内容 + 模式判定”的组合方案。
- 但 changelog 中的“准确率测试通过、误报率 0%”这类数值，本次未重新跑基准，不能直接视为当前状态。

### 3.2 [2.1.2] 数据库与内存优化

| changelog 声明 | 代码核查结果 | 判断 |
|---|---|---|
| Redis Keys 改 Scan、批量操作支持 | `internal/utils/cache.go` 和相关缓存层确实存在对应能力说明 | 基本属实 |
| Redis 连接池参数优化 | `internal/utils/resourcepool/http_pool.go`、`internal/utils/cache.go` 等有连接池配置逻辑 | 基本属实 |
| 对象池框架 | `internal/utils/objectpool/` 目录存在 | 基本属实 |
| 内存监控模块 | `internal/utils/memory/memory_monitor.go` 存在 | 基本属实 |

#### 3.2.1 需要注意的点

- 这些能力更多是“组件存在”，并不等于“被业务链路充分使用”。
- changelog 的表述偏“平台化完成”，但从主服务视角看，仍要再核实是否真正驱动查询/缓存/监控闭环。

### 3.3 [2.1.1] 日志系统优化

| changelog 声明 | 代码核查结果 | 判断 |
|---|---|---|
| 日志轮转、异步写入、动态级别 | `internal/logger/logger.go` 等相关能力在仓库中有实现痕迹，且项目历史文档也显示已接入 | 基本属实 |
| 结构化字段与性能日志 | 仓库中有相应日志封装和指标记录 | 基本属实 |

#### 3.3.1 需要注意的点

- 这部分没有暴露出明显的“文档说有、代码里完全没有”的反例。
- 主要问题不在日志模块是否存在，而在全仓库仍存在构建和测试不一致，导致日志与其他模块的“完成度”口径不统一。

### 3.4 [2.1.0] 性能优化

| changelog 声明 | 代码核查结果 | 判断 |
|---|---|---|
| LFU 缓存、动态 TTL、查询特征分析 | `internal/utils/cache_strategy.go` 中确实有 `DynamicCacheStrategy`、`ConfigBasedCacheStrategy`、`CacheStrategyManager` | 部分属实 |
| 动态工作池大小调整 | `internal/utils/workerpool/workerpool.go` 中存在动态并发与负载监控逻辑 | 基本属实 |
| HTTP 连接池管理与重试策略 | `internal/utils/http_client.go`、`internal/utils/resourcepool/http_pool.go` 存在相关实现 | 基本属实 |

#### 3.4.1 关键偏差

- `CacheStrategyManager` 的实现存在，但在主查询服务中未看到它进入实际查询链路。
- `UnifiedService` 目前主要使用统一 TTL 和引擎级 TTL 配置，见 [internal/service/unified_service.go](../internal/service/unified_service.go#L107)，没有发现策略管理器被真正调用。
- 这意味着 changelog 中“缓存策略优化已完成”的说法，至少在业务闭环层面是偏满的。

### 3.5 [2.0.1] 代码缺陷修复

| changelog 声明 | 代码核查结果 | 判断 |
|---|---|---|
| FOFA 错误处理与 URL 构建修复 | `internal/adapter/fofa.go` 与相关适配器已做安全构建 | 基本属实 |
| 编排器空指针检查 | `internal/adapter/orchestrator.go` 中已有更完整的 nil 防护 | 基本属实 |
| URL 构建安全修复 | 适配器层使用 `net/url` 的证据存在 | 基本属实 |

## 4. 业务链路对账

### 4.1 查询链路

#### 4.1.1 实际实现

- Web 路由已将查询入口接到 `handleQuery` / `handleAPIQuery`。
- `QueryAppService` 已接入 Web 层，且支持浏览器联动截图。
- `UnifiedService` 负责解析 UQL、调用编排器、执行搜索、合并结果、写入缓存。

#### 4.1.2 结论

- 查询业务链路是完整的，能形成“输入查询 -> 翻译 -> 搜索 -> 合并 -> 返回”的闭环。
- 但是缓存策略优化并没有完全进入这一闭环，因此查询功能是完整的，性能策略不是完全闭环的。

### 4.2 截图链路

#### 4.2.1 实际实现

- `/api/screenshot`、`/api/screenshot/search-engine`、`/api/screenshot/target`、`/api/screenshot/batch` 等路由都已注册。
- `web/screenshot_handlers.go` 已能完成 chromedp 截图、结果保存、批量截图等动作。
- 扩展桥接端点也已存在，包含 health、status、pair、token rotate、task next 等。

#### 4.2.2 风险与缺口

- 截图入口对目标 URL 仅做协议校验，没有做内网/回环地址限制，存在 SSRF 风险。
- 这类问题不是“功能缺失”，而是“功能开放面过大”。

### 4.3 篡改检测链路

#### 4.3.1 实际实现

- `TamperAppService` 已接入 Web 层，提供检测、设置基线、列出基线、删除基线、历史记录等能力。
- `web/tamper_handlers.go` 已将这些能力暴露为 HTTP 接口。
- `internal/tamper/detector.go` 已实现分段哈希、模式判定、恶意内容识别、基线比较、历史记录存储。

#### 4.3.2 结论

- 篡改检测是当前仓库里完成度最高的业务域之一。
- 需要保留的判断是：changelog 的“强准确率”声明未复测，应该视为历史数据而非当前保证。

### 4.4 监控链路

#### 4.4.1 实际实现

- 监控页面和导入、可达性、端口扫描等 handler 均已存在。
- 从代码结构看，监控能力并不是空壳。

#### 4.4.2 缺口

- 监控页面模板渲染没有显式处理错误返回值，存在静默失败风险。

## 5. 主要偏差点

### 5.1 缓存策略“已实现”不等于“已接入主链路”

- 证据：`internal/utils/cache_strategy.go` 中有完整的策略实现。
- 反证：`internal/service/unified_service.go` 只加载默认 TTL 和引擎 TTL，没有看见策略管理器进入查询流程。
- 影响：文档里“缓存策略优化已完成”在工程语义上偏乐观。

### 5.2 截图接口 SSRF 防护已补齐

- 证据：`web/screenshot_handlers.go` 已增加 `isPrivateOrInternalIP()`，并在多个截图入口拒绝私网、回环和链路本地地址。
- 影响：原先可被利用探测内网服务的风险已被显著收敛。
- 结论：此项已从“待修复”转为“已修复”，文档历史保留但不再作为当前遗留缺陷。

### 5.3 模板渲染错误未处理

- 证据：`web/monitor_handlers.go` 中 `ExecuteTemplate` 没有检查错误返回。
- 影响：模板异常时可能只表现为空白页或不完整页面，排障困难。

### 5.4 构建与测试状态已恢复一致

- 证据：当前执行 `go test ./...` 已通过。
- 主要变化：此前的测试编译错误与签名失配问题已修复，仓库已恢复全量测试可通过状态。
- 影响：仓库可按“已验证”状态继续推进文档收口与部署收口。

## 6. 综合评估

### 6.1 功能完整性

- 主功能完整度: 80% 左右
- 说明: 核心查询、截图、篡改检测、基础监控均已具备可用实现。

### 6.2 工程质量

- 工程健康度: 75% 左右
- 说明: 主测试链路已恢复，当前主要矛盾转为部署配置、少量模板错误处理和缓存策略接入口径。

### 6.3 安全性

- 安全完善度: 75% 左右
- 说明: 截图 SSRF 缺口已修复，但分布式默认 token 为空、部署暴露面过大等配置风险仍需治理。

### 6.4 文档可信度

- 文档可信度: 中等偏上
- 说明: changelog 并非虚构，很多功能确实存在；但对“已接入”“已验证”“性能数值”的表述需要进一步分层，不能一概等同于当前生产可用。

## 7. 优先级建议

### 7.1 P0

1. 统一容器端口与程序默认端口，避免 8080/8448 不一致导致的部署失联。
2. 修复当前模板渲染错误处理缺口，避免静默失败和排障困难。

### 7.2 P1

1. 将 `CacheStrategyManager` 真正接入 `UnifiedService` 查询主链路，或者在 changelog 中明确改写为“策略能力已实现，尚未默认启用”。
2. 收紧分布式默认配置，避免空 token 在公网部署时形成管理面暴露。
3. 统一文档中对“已实现”“已接入”“已验证”的口径，减少历史条目反复误判。

### 7.3 P2

1. 重新跑关键性能测试与准确率测试，更新 changelog 中的数值声明。
2. 将“模块存在”与“业务闭环完成”拆成不同的文档口径，减少歧义。

## 8. 参考文件

- [docs_archive/CHANGELOG.md](../docs_archive/CHANGELOG.md)
- [internal/service/unified_service.go](../internal/service/unified_service.go)
- [internal/utils/cache_strategy.go](../internal/utils/cache_strategy.go)
- [web/screenshot_handlers.go](../web/screenshot_handlers.go)
- [web/tamper_handlers.go](../web/tamper_handlers.go)
- [web/monitor_handlers.go](../web/monitor_handlers.go)
- [docs/CODE_REVIEW_FIXES_2026-04-03.md](CODE_REVIEW_FIXES_2026-04-03.md)

## 9. 结语

当前仓库属于“主功能可用、局部能力完整、工程收口不足”的状态。changelog 反映的方向大体正确，但需要把“模块实现”“主链路接入”“测试验证完成”“安全边界完成”四层状态分开写，否则很容易把部分完成误写成整体完成。

---

## 10. 验证情况（2026-04-10）

### 1. 构建与测试状态验证

| 项目 | 审计结论 | 验证结果 | 证据 |
|------|----------|----------|------|
| `go test ./...` | ✅ 通过 | ✅ 确认通过 | 当前全量测试已恢复正常 |
| 测试文件 | 15个 | ✅ 确认存在 | 覆盖部分模块，核心handler缺乏测试 |
| 编译状态 | ✅ 通过 | ✅ 确认通过 | 主程序可正常编译运行 |
| 运行状态 | ✅ 正常 | ✅ 确认正常 | Web服务可正常启动，路由注册正常 |

### 2. 安全风险验证

| 风险 | 审计结论 | 验证结果 | 证据 |
|------|----------|----------|------|
| 截图接口 SSRF | ✅ 已修复 | ✅ 确认修复 | `web/screenshot_handlers.go` 已增加内网/回环/链路本地地址限制 |
| 模板渲染错误未处理 | ⚠️ 中等风险 | ✅ 确认存在 | `web/monitor_handlers.go` 中 `ExecuteTemplate` 未检查错误返回 |

### 3. 功能实现验证

| 功能 | 审计结论 | 验证结果 | 证据 |
|------|----------|----------|------|
| 篡改检测系统 | ✅ 基本属实 | ✅ 验证通过 | 5种检测模式、多引擎检测、规则管理均已实现 |
| 缓存策略优化 | ⚠️ 部分属实 | ✅ 确认存在偏差 | `CacheStrategyManager` 仍未接入查询主链路 |
| 日志系统优化 | ✅ 基本属实 | ✅ 验证通过 | 动态级别、异步写入、结构化字段已实现 |
| 数据库与内存优化 | ✅ 基本属实 | ✅ 验证通过 | Redis优化、连接池、对象池、内存监控已实现 |

### 4. 业务链路验证

| 链路 | 审计结论 | 验证结果 | 说明 |
|------|----------|----------|------|
| 查询链路 | ✅ 完整 | ✅ 验证通过 | 输入查询 → 翻译 → 搜索 → 合并 → 返回 |
| 截图链路 | ✅ 完整 | ✅ 验证通过 | CDP截图、批量截图、结果保存已实现 |
| 篡改检测链路 | ✅ 完整 | ✅ 验证通过 | 检测、基线管理、历史记录已实现 |
| 监控链路 | ⚠️ 存在缺口 | ✅ 确认存在缺口 | 页面模板渲染仍有错误处理缺口 |

### 5. 验证结论

✅ **审计准确性**：大部分历史结论仍成立，但 SSRF 和测试状态已更新为已修复  
✅ **风险真实性**：当前仍然存在的风险已通过代码核查确认存在  
✅ **评估合理性**：综合评估等级（功能完整度80%、工程健康度75%、安全完善度75%、文档可信度中等偏上）更贴近当前状态  
✅ **建议有效性**：优先级建议仍有效，但 P0 现在应聚焦端口一致性和模板错误处理  

**验证人**：Vibe-Control Guardian  
**验证日期**：2026-04-13  
**验证方法**：代码核查 + 构建测试 + 运行验证