# 发布核验快照（2026-09-06）

> 本页记录已完成核验的代码版本，不是实时 HEAD、GitHub 分支或镜像 `latest` 查询。后续代码或文档提交需查看对应工作流；历史报告只证明其日期、提交与测试范围内的事实。

## 最新核验：代码发布与运行实例分列

- 已核验代码 `62302b23b6f1ee25628a665da0c273321250f065`：包含查询 HTTP 写截止时间、浏览器采集诊断、Quake 域名卡片、扩展状态入口/MV3滚动及调度关闭开关修复。
- [主 CI 34024377689](https://github.com/ElaineRosa6/unimap/actions/runs/34024377689)：11/11成功，含镜像构建推送；[Bridge smoke 34024377706](https://github.com/ElaineRosa6/unimap/actions/runs/34024377706)：2/2成功。日志对应此代码提交，不代表后续文档提交或云端运行版本。
- 扩展源码版本 **0.4.23**，本地49项扩展回归通过；尚未核验用户浏览器实际加载该版本。状态页提供版本和最近成功轮询，不显示token。
- 本机隔离服务仍是旧 `unimap-web-r54.exe`，健康版本 `dev (commit=unknown, built=unknown)`；最近 readiness **503**，截图检查 `mode=extension, cdp=true, ext=false`。CDP健康布尔值不是七引擎采集验收。
- 新版测试二进制已构建并校验，隔离配置副本明确关闭调度和通知；服务替换命令被工具策略拦截，**部署未执行**。原程序和原配置哈希未变；不要通过替代通道重试受限操作。
- 预检发现旧测试配置调度及通知总开关均为true，但已注册任务0、启用通知渠道0。旧“调度与通知总开关已关闭”的描述不适用于此次预检；没有据此观察到任务或通知被触发。
- 最新调度修复保留显式false，阻止cron/一次性/延迟任务自动注册，手动执行仍可用。旧版本已持久化的true不会自动反推成false；该设置需重启生效。

### 尚未完成的验收

1. 本机服务升级及运行配置确认；当前源码通过不等于部署通过。
2. 扩展实际加载版本、Bridge重新上线、新版本真实任务回传及字段/分页核验。
3. 七引擎独立CDP登录会话及最新提取脚本复验；保留既有分日期结果，不以CI headless fixture替代站点实测。
4. 受控云端DNS/页面变化、证据截图、图片送达及恢复后复验；云端截图启用状态未改动。

目录复核：Go_project下的unimap和unimap-v2是不同GitHub仓库；unimap-v2工作区干净，unimap-v2-codex目录不存在。artifacts和rollback-check目录未因名称相近被删除。本文不声明两仓库代码相同，也不把备份目录视作已确认废弃项。

以下为历史证据，保留原提交与当时范围。

## 后续已核验发布（追加快照）

- `c2acd9de71fc5a1fe36fd5191458e15cb29f1ca0`：[主CI 34013422414](https://github.com/ElaineRosa6/unimap/actions/runs/34013422414) 11/11通过；[bridge-smoke 34013422458](https://github.com/ElaineRosa6/unimap/actions/runs/34013422458) 通过。
- 两工作流均实际执行39项扩展回归、无失败或跳过，日志artifact已上传。Linux/macOS卡片测试明确PASS；Redis实例3轮通过。
- 镜像 `ghcr.io/elainerosa6/unimap@sha256:84a9345972b6ab788e34f2cb655eef931bdd5860b7347da05449b625d4e788ed` 的commit与该提交一致，readiness及shutdown为ok。这不是云端用户部署验收。
- Extension代码版本0.4.19。DayDayMap修复副本CDP实测10资产+PNG；扩展真实加载、活页和Bridge回传尚未验收。其他引擎及运行环境限制见[分路径复验](CDP_EXTENSION_RECHECK_2026-09-06.md)。
- 以下保留早先f6b3689核验；其中“本轮”只指该历史批次。不要据这些历史段落忽略上述新增证据。

## 早先已核验发布

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
