# Tasks

- [x] Task 1: 修复 orchestrator.go 缓存键设计问题
  - [x] SubTask 1.1: 修改 PaginatedSearchTask 缓存键包含页码
  - [x] SubTask 1.2: 添加类型断言检查避免 panic
  - [x] SubTask 1.3: 修复 Normalize 错误处理

- [x] Task 2: 修复 merger.go 对象池清理问题
  - [x] SubTask 2.1: 清理从对象池获取的资产对象
  - [x] SubTask 2.2: 修复 sourceStats 复制逻辑
  - [x] SubTask 2.3: 优化锁粒度使用 RLock

- [x] Task 3: 修复 unified_service.go 导出器遍历逻辑
  - [x] SubTask 3.1: 修复 Export 方法找到匹配后不停止的问题
  - [x] SubTask 3.2: 统一缓存处理逻辑

- [x] Task 4: 修复 parser.go UTF-8遍历问题
  - [x] SubTask 4.1: 修改 tokenize 使用 rune 切片遍历
  - [x] SubTask 4.2: 优化多字符操作符处理

- [x] Task 5: 运行测试验证修复
  - [x] SubTask 5.1: 运行所有单元测试
  - [x] SubTask 5.2: 验证功能正常

# Task Dependencies
- Task 5 depends on Task 1, Task 2, Task 3, Task 4
