# 0001 统一工具包目录结构

## 状态

已接受

## 背景

项目中存在多个工具包目录：
- `internal/utils/` - 包含缓存、重试等工具
- `internal/util/` - 包含 workerpool

这种不一致的命名和组织方式导致：
1. 代码导航困难
2. 导入路径混乱
3. 新开发者学习成本高

## 决策

统一工具包目录结构为：
- 使用 `internal/utils/` 作为统一的工具包目录
- 将所有工具包（包括 workerpool）移动到该目录下
- 按功能子目录组织：`utils/workerpool/`, `utils/cache/`, `utils/retry/` 等

## 影响

### 正面影响
- 统一的目录结构，便于导航和理解
- 一致的导入路径，减少混淆
- 更好的代码组织和可维护性

### 负面影响
- 需要更新所有相关文件的导入引用
- 可能影响现有代码的编译

## 实施步骤

1. 创建 `internal/utils/workerpool/` 目录
2. 将 `internal/util/workerpool/workerpool.go` 复制到新位置
3. 更新所有使用 `workerpool` 的文件导入路径
4. 删除旧的 `internal/util/` 目录
5. 运行编译验证

## 相关文件

- `internal/utils/workerpool/workerpool.go` - 移动后的工作池实现
- `internal/adapter/orchestrator.go` - 更新导入引用
- `internal/service/monitor_app_service.go` - 更新导入引用
- `internal/service/monitor_port_scan_service.go` - 更新导入引用
- `internal/tamper/detector.go` - 更新导入引用

## 验证

- 代码编译通过
- 所有测试通过
- 功能正常运行

## 结论

统一工具包目录结构是必要的架构改进，有助于提高代码质量和可维护性。