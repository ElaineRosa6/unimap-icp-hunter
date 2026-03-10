# 代码功能实现检查与修复 Spec

## Why
通过代码审查发现多个功能实现存在逻辑错误、边界条件处理不当、性能问题等，需要修复以确保系统稳定性和正确性。

## What Changes
- 修复 orchestrator.go 缓存键设计问题
- 修复 merger.go 对象池清理问题
- 修复 unified_service.go 导出器遍历逻辑
- 修复 parser.go UTF-8遍历问题
- 优化锁粒度和错误处理

## Impact
- Affected code: internal/adapter/orchestrator.go, internal/core/unimap/merger.go, internal/service/unified_service.go, internal/core/unimap/parser.go
- Affected features: 缓存系统、结果合并、导出功能、UQL解析

## ADDED Requirements
### Requirement: 缓存键包含页码
The system SHALL include page number in cache key for paginated queries.

#### Scenario: 分页查询缓存
- **WHEN** 执行分页查询
- **THEN** 缓存键应包含页码参数

### Requirement: 对象池对象清理
The system SHALL clean objects retrieved from object pool before use.

#### Scenario: 合并结果
- **WHEN** 从对象池获取资产对象
- **THEN** 应清理对象避免脏数据

## MODIFIED Requirements
### Requirement: 导出器遍历逻辑
**Current**: 遍历所有导出器，找到匹配后不停止
**Modified**: 找到匹配的导出器后立即返回

### Requirement: UTF-8字符串遍历
**Current**: 使用 `rune(query[i])` 遍历
**Modified**: 使用 `for _, ch := range query` 正确遍历UTF-8

## REMOVED Requirements
None
