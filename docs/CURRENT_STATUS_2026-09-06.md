# 发布核验快照（2026-09-06）

> 本页记录已完成核验的代码版本，不是实时 HEAD、GitHub 分支或镜像 `latest` 查询。后续代码或文档提交需查看对应工作流；历史报告只证明其日期、提交与测试范围内的事实。

## 已核验发布

- 发布分支：`master`；本次已核验代码：`f6b368927536a64c0f3b55e09dddea3b29aabadf`。Go 版本以 `go.mod` 的 `1.26.6` 为准。
- [CI 33996779465](https://github.com/ElaineRosa6/unimap/actions/runs/33996779465) 成功，10/10 jobs 通过。Linux/macOS 的备份保留隔离、旧版名称兼容和链接保护回归通过。
- 此次实际发布镜像 `ghcr.io/elainerosa6/unimap@sha256:58fa34a84306c441958ff65adaa0ab3aa8142ece5f90d19b68b42a832ca14148` 的冒烟记录：commit 为上述 SHA，readiness=`ok`，shutdown=`ok`。这是 CI 容器验证，不是云端用户环境的部署或恢复演练。
- `f6b3689` 未触发独立 Bridge 工作流。最近单独核验的 [Bridge 33994195171](https://github.com/ElaineRosa6/unimap/actions/runs/33994195171) 对应 `8ffb7b12342dcd8a493bc69d54b475f43a1ba2d8`，结果成功；不宣称是 `f6b3689` 上的同提交 Bridge 验收。

## 本轮项目审查成果

详细复现与回归范围见 [项目审查记录](PROJECT_REVIEW_2026-09-06.md)，操作契约见 [API](API.md) 与 [Runbook](RUNBOOK.md)。

- 历史写入事务、SQLite 连接外键配置与查询结果错误检查。
- 调度取消/超时传播、worker pool 生命周期与并发关闭修复。
- 配置副本隔离、查询预算和 WebSocket 通道关闭处理。
- 截图与备份来源的根目录边界及同句柄文件读取。
- 五类应用 SQLite 数据库的现有连接快照；归档路径冲突预检、输出目录排除及完整文件名保留隔离。
- 实际发布镜像启动、readiness 与退出的 CI 检查。

### 备份适用边界

Web/调度入口绑定五类应用数据库，未知 SQLite 来源失败关闭；底层兼容 API 未配置快照回调时仍保留原始文件备份语义。单库快照不是跨库事务，来源文件树也不是原子快照。完整文件名识别只划分保留清理的命名空间，不证明 gzip/tar 内容完整性；输出目录应专用。

## 仍需真实环境证据的工作

本轮未新增真实站点 CDP/Extension 或云端业务验收，不以 CI 通过替代这些工作：

- 调整 ExtractJS 后的 CollectAndCapture CDP 复跑及插件改后活页采集，按 [插件/CDP 记录](PLUGIN_CDP_STATUS_2026-08-21.md) 和 [剩余工作](REMAINING_WORK_2026-07-23.md) 验收。FOFA/Shodan/ZoomEye 等既有结论继续按原始日期和后端解释。
- 受控云端页面真实变化、证据截图、图片通知及重启恢复的端到端验收，见 [云端安全验收 Runbook](CLOUD_SECURITY_ACCEPTANCE_RUNBOOK_2026-07-29.md)。本轮未启用自动巡检证据截图。
- 原有凭据、任务启用与云端配置待办保持原状态；不因本轮代码修复修改用户环境。

## 历史与后续

- [2026-09-04 状态快照](CURRENT_STATUS_2026-09-04.md) 保留当日提交、分支差距与验收事实；其中“当前”仅指当日。
- 后续核验须记录具体提交、工作流及环境；此页不声明项目已无其他问题。
