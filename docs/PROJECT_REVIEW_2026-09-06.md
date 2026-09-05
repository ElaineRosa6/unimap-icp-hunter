# 项目检查与优化（2026-09-06，持续进行）

## 范围与基线

- 基线提交：`d20323f`，不是旧状态页记录的 `e31e03d`。
- 本轮覆盖历史库数据完整性和 CI 发布依赖；这不是整个项目已无问题的结论。
- 基线 CI：[33855935603](https://github.com/ElaineRosa6/unimap/actions/runs/33855935603) 成功。该纯文档提交未触发按路径过滤的 bridge-smoke。
- 不更动运行数据库、配置凭据、云端任务开关；不自动清理历史遗留数据。

## 已复现与本地修复

### P1：删除操作历史遗留结果明细

`operation_results` 定义了 `ON DELETE CASCADE`，但 `NewDatabase` 没有为连接启用外键。新回归测试在原代码上证实：单条删除、按类型清空、全部清空后明细仍存在；连接池的三个独立连接和数据库重开后均接受不存在父记录的明细。

修复：DSN 添加 `_foreign_keys=on`，使每个新建连接都执行外键约束，而不是仅在一个连接上执行 PRAGMA。现有数据库表结构保持不变。

回归覆盖：上述三种删除方式、保留其他类型的记录、连接池和重开后的外键开关、拒绝新孤儿数据。历史库与配置包竞态测试已通过。

已有孤儿行不会因为启用外键自动消失。上线前可在备份副本执行以下只读查询评估遗留数量：

```sql
SELECT COUNT(*) AS orphan_results
FROM operation_results AS r
WHERE NOT EXISTS (SELECT 1 FROM operation_history AS h WHERE h.id = r.history_id);
```

清理遗留数据应单独确认并备份；删除行也不等同于数据库文件立即缩小。

### P2：镜像发布没有等待所有验证门槛

Docker job 原先只依赖 test/lint/security，浏览器 E2E 或扩展脚本检查失败仍可能发布镜像。现补上 `headless-browser` 和 `extension-scripts` 依赖，并添加解析 YAML 的回归测试。

### P2：多平台覆盖率附件使用同一名称

矩阵中两个系统都上传名为 `coverage` 的附件，存在同名冲突风险。改为 `coverage-${{ matrix.os }}`；测试约束附件名称包含矩阵维度。[官方说明](https://github.com/actions/upload-artifact#not-uploading-to-the-same-artifact)。不把这一潜在风险描述为基线 CI 实际失败。

## 后续检查队列

### 第二轮：手工保存历史的原子性

故障注入在第二条结果插入时触发 SQLite ABORT。旧 HTTP handler 返回 500，但留下 1 条主记录；重试成功后主记录变成 2 条。改用现有 `CreateHistoryWithResults`，失败时整次事务回滚；成功重试只有 1 条历史、2 条明细。补测空结果、普通结果和默认 1000 条截断上限。接口字段不变，不扩展为成功请求幂等协议。

### 第三轮：历史查询中途失败被当作成功

在 `54366b8` 基线上使用标准库 SQL 测试驱动，先返回一条可正常 Scan 的记录，再返回指定错误。旧 `ListHistory`（含类型过滤分支）和 `GetResults` 均返回部分数据及 nil 错误。现于循环结束后检查 `rows.Err()`，包装原始错误并丢弃不完整结果；`errors.Is` 可追溯原因。正常 EOF 的完整读取行为保持不变，并有对照测试。本轮不变更 SQL、表结构或运行数据库。

后续检查：
### 第四轮：截图文件管理的目录边界

在临时 fixture 中创建指向截图根目录外的批次目录链接（绝对、相对两种）。旧 `ListBatchFiles` 列出外部文件并调用预览回调；旧 `DeleteFile` 删除外部 fixture 文件。该问题要求截图树内存在可利用的目录链接，不代表已发现远程创建链接入口或生产入侵。

改为 `os.Root` 的目录句柄相对操作：根目录内打开批次、读取列表、删除单文件、递归删除批次。避免仅 `Abs/Rel` 校验后再使用普通绝对路径的检查/使用间隙。列表及文件计数仅展示普通文件，不把链接或特殊文件作为截图。配置的根目录本身仍是受信任的管理员配置；文件管理边界不等同于挂载隔离或截图下载 handler 的完整审计。

回归实际创建符号链接并执行（本机无跳过）：目录外列举/删除被拒，外部文件原样保留；批次内嵌外部链接的递归删除保留链接目标；普通文件列举、预览回调、计数及删除正常。原先整批删除测试已经保护链接目标，未把它误记为已复现的破坏行为。

后续检查：
- 查询/调度取消、资源释放、配置只读提示和性能热点继续审查。
- 浏览器与生产验收维持既有边界，不用本地单测替代真实 CollectAndCapture 验收。
- 已提交 `54366b8` 的 [CI 33980447373](https://github.com/ElaineRosa6/unimap/actions/runs/33980447373)：测试、lint、安全、构建、浏览器和扩展检查成功，Docker 在 `go mod download` 阶段因 `goproxy.cn` 返回 504 失败；bridge-smoke 成功。待处理构建依赖下载稳定性。第三、四轮本地修改尚未提交，不属于该次远端验证范围。


### 第五轮：Docker 依赖代理 504 不回退

远端 `54366b8` 的 Docker 日志确认 `goproxy.cn` 对 `github.com/fyne-io/glfw-js` 的模块请求返回 504。原 `GOPROXY=https://goproxy.cn,direct` 使用逗号，仅在 404/410 时尝试下一项；原注释所说的官方代理回退也未实际配置。

默认改为 `https://proxy.golang.org|https://goproxy.cn|direct`，通过 `ARG GOPROXY` 传给构建阶段环境，允许针对网络覆盖。竖线在代理错误时尝试下一项，不把所有代理失败伪装为成功。不改 `go.sum`，不关闭生产校验和数据库，不改变 APK 源。

本地回归使用两个 loopback HTTP 模块代理、隔离的空模块缓存和真实 `go mod download`。旧分隔符在 504 场景失败且第二代理请求次数为 0；新分隔符成功下载 fixture 模块。正常主代理、404 回退、全部代理失败也有对照。测试中的 GOSUMDB=off 仅用于无公共校验记录的本地合成模块。

本机未安装 Docker CLI，此处不宣称已验证完整镜像构建；本轮修改尚未提交，远端镜像构建及发布仍需验证。


### 第六轮：取消调度器后仍等待并启动重试

`executeTaskWithRetry` 原先使用不可取消的 `time.Sleep`，循环也未检查调度器上下文。回归证实：执行前已取消时仍调用 handler 两次；第一次失败后在退避中取消，仍等待两秒并再次调用 handler。

改用监听 `s.ctx.Done()` 的计时器等待，在启动尝试前和失败后检查取消。取消记录沿用 `failed` 状态，错误为 `context canceled`，并写入完成时间/耗时；`RetryCount` 及重试指标只在实际准备启动重试时增加。取消前已开始且忽略上下文的 handler 仍需自身配合退出，此修复不宣称强制终止任意 handler。

定向回归覆盖执行前取消、退避中取消（500ms 内结束、不再启动下一次 handler），以及正常两秒退避后成功重试。超时分类与成功记录问题见第七轮；通知重试的取消传播仍待检查。


### 第七轮：重试执行记录与真实结果不一致

复现三种不一致：两次立即返回普通错误的尝试，仅因中间两秒退避就被记录为 timeout；重试成功后 Error 仍保留首次失败；handler 等到单次上下文过期后返回 nil，仍被记录为 success。

修复：按 `errors.Is(err, context.DeadlineExceeded)` 判断超时，超时描述使用该次尝试耗时，整体 DurationMs 仍包含退避和全部尝试。`runTaskHandler` 返回前检查单次上下文，过期结果不作为成功发布。重试成功清空 Error。不改变超时配置、重试次数或现有状态枚举；不承诺强制打断忽略上下文的 handler，也不撤销它已经完成的外部副作用。

回归覆盖普通错误跨退避仍为 failed、成功清空旧错误、过期的 nil 返回被记为 timeout、正常协作式超时；并同时复跑第六轮取消与正常重试测试。


### 第八轮：渠道隐藏超时原因导致重复通知重试

现有策略明确要求通知超时不重试，因为消息可能已经送达。旧逻辑只判断渠道返回错误是否包装 `context.DeadlineExceeded`；fixture 渠道等待发送上下文过期后返回普通 transport error，旧代码发送两次，未遵守此策略。

在主动 cancel 前读取发送上下文错误，发送失败时用 `errors.Join` 同时保留渠道错误和超时原因，现有超时分支因此终止重试。渠道明确返回 nil 的成功确认仍按成功处理；普通暂态错误保留重试。fixture 全部本地执行，无真实消息发送。

退出约定：本轮不将通知上下文强行改为调度器上下文。当前 Stop 阻止新通知并等待已经开始的通知 goroutine；立即取消与限时排空属于不同交付策略，未凭后台上下文这一点认定为缺陷。后续如优化退出延迟，应先明确通知丢失容忍度与持久化重试策略。


### 第九轮：查询上下文丢失值与取消原因

原 `withoutDeadline` 在父上下文有 deadline 时从 Background 新建子上下文并启动转发 goroutine。回归证实上下文值丢失、自定义取消原因变成普通 canceled、父级 deadline 到期也被改成 canceled。转发 goroutine 在查询提前完成后仍需等待父上下文结束。

改成嵌入父 Context、仅覆盖 Deadline 的轻量包装：Value、Done、Err 及 Cause 保持父级语义，不新建等待 goroutine。保留已有 nil 输入、无 deadline 时原样返回和手动取消传播测试，并补测值、自定义 cause 和已过期父上下文。

同时修正误导注释：隐藏 Deadline 不等于忽略父级 Done；旧实现本来也会在父级超时后取消查询。本轮不让查询脱离调用方生命周期。浏览器与 API 若需真正不同的截止时间，应从共同的调用方上下文分别派生，而不是屏蔽整个请求取消；该工作流预算设计尚需另行核对。


### 第十轮：HTTP 默认浏览器动作与预算不一致

同步服务将空动作默认为 collect_and_capture，但 HTTP handler 在默认值生效前已按空字符串选择 60 秒预算。HTTP→服务→截图 router→fixture provider 回归实际读取浏览器上下文，空/空白动作仅剩约 60 秒，显式 collect_and_capture 为 150 秒。

在 HTTP handler 创建上下文前解析同步工作流默认值，空/空白动作现在使用 150 秒；显式 collect 仍是 60 秒。测试只读取上下文并立即返回，不启动浏览器、不访问外部网站。保留全局等待函数与 WebSocket 的 legacy collect 默认值，不将同步入口的默认值强加给另一入口。

并行 API 与浏览器预算完全独立的问题仍需按调用方上下文分别派生设计，本轮不将修复默认预算误报为已经解决该问题。


### 第十一轮：配置读取竞态与可变引用外泄

保存失败不发布、Web 并发写入序列化已有测试保护，本轮未重复修改该路径。新回归在 SetConfig 与配置/缓存/Redis getter 并发时触发 race；另证实 SetConfig 保留调用方指针、GetEngineConfig 返回内部字段地址、GetAllEngineCacheConfigs 返回内部 map，调用方可绕过发布流程改变内存配置。

SetConfig 克隆输入；引擎 getter 返回配置快照中的引擎；缓存 map 复制后返回。IsValid、缓存与 Redis getter 使用读锁，TTL/容量读取通过同一把锁内的 helper 获取引擎值和全局默认，避免嵌套 RWMutex 读锁与并发写锁形成死锁。保留 nil/default 和现有保存协议。标量缓存/Redis 热路径仅加锁，不序列化整个配置。

回归覆盖输入指针、返回 map、返回引擎指针的隔离，以及全部相关 getter 与配置替换并发。此结论不等同于所有配置存储调用的事务序列化保证，直接并发 SaveConfig 的磁盘/内存一致性仍应单独核对。

本轮发生在 integration 整体验收包之后；该包记录的是前 18 文件状态，恢复当前状态时须先应用第十一轮回滚，再按需使用 integration 回滚。


### 第十二轮：配置快照热路径的 YAML 往返开销

Web currentConfig 使用 Manager.GetConfig，而后者每次 YAML marshal/unmarshal 整份配置，且 YAML 路径会改变 nil/empty 表示。现直接复用既有 Config.Clone，删除无其他调用的 copyConfig。读取锁与独立快照语义保持不变。

新增全字段填充测试：Config 的每个字段都赋非零值，再比较输入与快照，可发现今后新增字段漏入 Clone；同时验证零值结构、之前的引用隔离及并发 getter 测试。

同机、同一合成配置、非 race 微基准，200ms × 3 次：旧中位数 1,360,016 ns/op、824,586 B/op、5,725 allocs/op；新中位数 2,946 ns/op、5,602 B/op、33 allocs/op。只表示配置快照操作的性能，不外推整站吞吐或请求延迟。完整 benchmark 输出与摘要保存在 round12 验证目录。

当前变更晚于 integration 包；该包不是包含第十一、十二轮的新整合包。


### 第十三轮：WebSocket 汇合时覆盖已完成的浏览器结果

浏览器结果通道发送一次后关闭；原 select 保留该分支，在等待 API 的期间反复读到关闭通道零值，覆盖 browserOutcome。回归走 executeWSQueryAsync、真实服务编排和本地 provider/adapter fixture：浏览器先返回 1 条资产，API 后结束，原 query_complete 丢失全部浏览器资产；修复后保留资产和 BrowserCollectedData。该 fixture 的 API 等待受 5 秒父上下文限制，不作为成功 API 查询或真实网站验收。

收到一次结果后将相应通道置 nil，从 select 移除；浏览器完成后停用计时器，浏览器等待超时后也移除结果与计时器分支。避免关闭通道的持续就绪空转及零值覆盖；未更改查询字段、浏览器执行动作或超时预算。定向回归重复 3 次通过，并保留 nil 查询服务错误测试。


### 第十四轮：工作池退出为查询额外增加约 5 秒

追查第十三轮 fixture 的 5 秒耗时，发现 SearchEnginesWithContext 每次查询创建工作池并 defer Stop，而负载监控协程为避免抢占 worker 缩减信号，只每 5 秒 ticker 检查 running。Stop 等待监控协程，导致空闲工作池也拖延查询返回。两种构造器的基线测试均测得约 5.000 秒。

新增仅用于整个工作池退出的 monitorStopCh，Stop 关闭该信号，监控 select 立即退出；保留 exitCh 专用于 worker 退休/退出。固定、动态工作池分别重复 10 次验证 Stop 在 1 秒阈值内完成、结果通道关闭、重复 Stop 正常。未改变任务执行或强制中断语义，不把这项修复等同于解决所有 Submit/Stop 并发问题。

同一 WebSocket browser-first 回归从每次约 5.00 秒降为约 0.10 秒，重复 3 次通过；其 100ms 延迟由 fixture 显式设置。此前的慢返回已定位为真实工作池退出开销，而非只归因于测试夹具或外部网络。全量竞态测试通过。


### 第十五轮：工作池超时停止后发送错误导致 panic

受控任务先阻塞，StopWithTimeout(10ms) 返回后再释放任务并返回错误；基线实际发生 `panic: send on closed channel`。原因是 Stop 超时仍关闭 results，而 worker 尚未结束。

把 results 的关闭移到 wg.Wait 完成之后，由最后的生产者退出保证关闭时机。worker 发布错误同时监听停止信号，退出期间不依赖调用方排空错误队列。已缓冲的错误仍可读取；停止过程中尾部错误允许不再投递。StopWithTimeout 到期仍返回，但 Results 只有在生产者全部退出后才关闭，调用方不应把停止超时当成任务强制终止。

晚到错误、满错误队列退出、上轮低延迟退出回归重复 10 次通过。此处仍不宣称 Submit 与 Stop、并发扩缩容及已停止池重启的所有组合已通过审计。


### 第十六轮：满队列 Submit 与 Stop 并发崩溃

固定与动态工作池分别用 testing/synctest 确认：一个任务占住 worker，填满队列，再让一个 Submit 阻塞；StopWithTimeout 关闭 tasks 时，原实现使该 Submit panic（send on closed channel）。测试仅在夹具 goroutine 捕获 panic 以记录失败，生产代码不使用 recover 掩盖错误。

增加提交读写锁：Submit 在读锁内检查运行状态，并同时等待入队或停止信号；Stop 先广播停止，唤醒满队列提交者，再取写锁关闭任务通道，避免先取写锁造成死锁。停止期间的提交仍为 best-effort、没有返回值；此修改未将 Stop 定义为排空所有队列任务，也未承诺强制终止正在执行的 Task。

两种构造器的阻塞提交回归重复 20 次；同时复验超时停止后的晚到错误、满错误队列和低延迟退出。Start/Stop 与扩缩容之间的生命周期审计仍待继续，不把该回归扩大为所有工作池并发组合已验证。


### 第十七轮：工作池生命周期与扩缩容互斥

基线再次复现：扩到 4 个 worker 后 Stop，再 SetConcurrency(1) 会向已关闭的 exitCh 发送并 panic。另一个 synctest 回归证实，Start 前 SetConcurrency(4) 已经偷偷启动 3 个 worker，能从内部队列执行任务。

Start、Stop、自动与手动扩缩容共用生命周期锁，保证新增 worker 的 WaitGroup.Add 完成在关闭/等待之前，且退出信号关闭与缩容发送互斥。停止开始后标记终态，Start/SetConcurrency 不再复活已关闭的通道；当前 6 个生产构造点均为每次操作新建工作池，未发现复用已停止池的生产路径。Pool 注释明确单次使用，下一轮操作新建实例；未提供新的重启 API。Start 前 SetConcurrency 只配置初始并发数，由 Start 统一创建 worker；未启动时 Stop 仍保持原有 no-op 行为。

回归包括停止后缩容、已停止池重复启动、停止超时但任务仍执行时的启动/扩容、启动前配置，以及 50 组同时 Start/Stop/Submit/扩缩容（半数从未启动状态开始）。定向套件重复 20 次，共 1000 组并发生命周期场景，连同此前提交与错误发布回归通过。重复 Stop 仍保持原有快速返回语义，等待实际 worker 完成使用 Wait/Results；未增加强制终止 Task 的承诺。


### 第十八轮：推送后 CI 暴露测试与 lint 差异

GitHub 连接恢复，已推送 5fbcfcb2e180deeb130e615056bb9d2d83eb5266，并以 ls-remote 核对 master 同 SHA。该提交不含第十六、十七轮尚未提交的工作池改动。

远端 ci run 33985155270 最终 failure：Ubuntu 测试的 TestDockerProxyFallback 三个成功下载子用例在 TempDir 清理时因模块缓存只读而 permission denied；历史回归夹具的三处同结构字面量触发 gosimple/S1016。Ubuntu lint failure，macOS 两项后续任务为 cancelled；两平台构建、无界面浏览器、扩展脚本和漏洞扫描 success，Docker skipped。独立 bridge-smoke run 33985155280 success。远端原始状态和失败日志保存在 round18/ci.json、ci-failed.log；这不是全绿记录，也尚未验证 Docker 镜像构建。

修复：历史夹具三个同结构转换使用显式类型转换；Go proxy 测试子进程 GOFLAGS=-modcacherw，仅让独立临时模块缓存可写，生产 Docker 的代理与校验策略不变。Windows 上 golangci-lint v1.64.8 复现 3 处 S1016，修改后完整 lint 通过；副本无 Git 元数据，因此该副本 lint 显式 GOFLAGS=-buildvcs=false，不关闭任何 lint 规则。定向历史/proxy 测试重复 5 次通过，并执行全量竞态测试。

本机 WSL Ubuntu 当前无 Go，Windows 验证不代替 Linux 清理复验。上述改动还需提交后的远端运行证明 Linux/macOS、lint 与 Docker 门禁全部通过；持续保留未完成状态，未重跑同一失败提交来掩盖问题。


### 第十九轮：镜像成功提示改为运行态验收

364a19d90ba7bd0f83c4416450e3fb4877efbd79 已与远端 master 对齐，ci run 33985769035 的 10 个 job 和 bridge-smoke run 33985769157 均 success。镜像 digest 为 sha256:88e575851f4ede34647768ba20a96afa2c1da940e338b2a4943b88ee4b36bc94；原始状态、Docker 日志与哈希核验见本地 release-364a19d 验证目录。

审查发现原 Verify image 只有两条 echo，不能证明镜像可运行。新增 scripts/ci_image_smoke.py，CI 从 build-push 输出读取 digest，而不是从可变 latest/master 标签取镜像。容器 network=none，无发布端口与宿主挂载；由入口脚本在内部 runtime-config 路径初始化配置。90 秒内检查 /health/ready 的 ok 与构建提交 SHA，检查实际 UID 非 root、配置 mode=600，随后发送正常停止并确认退出码 0。finally 只清理本次创建的容器。

新增 Go 回归拒绝仅 echo、缺少 digest 或提交号绑定的工作流；Python 单元夹具覆盖成功、提前退出、超时、坏 JSON、错版本、root、配置权限、异常退出和非 digest 引用。夹具不等于真实 Docker 执行：本机无 Docker，实际镜像启动由提交后的 CI 执行，验收状态需以对应新 run 为准。

这个步骤位于现有镜像推送之后，失败会使 CI 失败，但不撤回已发布的标签。它不等于生产发布审批、真实测绘引擎访问验收或容器内浏览器截图测试；本轮没有更改生产实例、凭据或定时任务。


### 第二十轮：截图预览通过符号链接读取根目录外文件

前一轮 e19c6d9 的 10 项 CI 与 Bridge smoke 全通过；新增镜像运行态验收返回 readiness=ok、shutdown=ok，并已核对构建提交。验收证据见 release-e19c6d9。

本轮审查 GET /screenshots/...：原逻辑只有 filepath.Rel 的词法边界校验，随后 os.Stat/os.Open/ServeFile 都跟随符号链接。以临时目录中的公开夹具复现，文件链接与目录链接的绝对/相对四种形式均返回根目录外文件的完整标记，HTTP 200；具有 .png 后缀的目录还返回重定向。请求使用原有受信任 Origin，并非证明未认证攻击者有创建链接能力；漏洞前提是截图树中存在指向外部的链接。

改为 os.OpenRoot(baseDir) 后 root.Open(cleanRelPath)，仅接受普通文件。内容魔数读取、响应体和 Range 使用同一打开句柄，ServeContent 不再按路径重新打开；保留原有 Origin/Referer、扩展名、nosniff、private 缓存策略和历史 JPEG 内容识别行为。

四种越界链接和伪装成图片名的目录修复后均 404；普通 PNG、根目录内的相对链接、Range=206、If-Modified-Since=304、历史 JPEG 与原有错误路径重复 10 次通过。API 文档同步边界约束。全程只读自建夹具，未读取真实目录外文件或修改运行截图。


### 第二十一轮：不完整备份误报成功并淘汰完整恢复点

1439b27 的 Ubuntu/macOS 截图回归、10 项 CI、Bridge 和容器运行态验收均通过，证据见 release-1439b27。Go_project 下已无 unimap-v2-codex；未删除其余 v2 工作区或验收材料。

当前 Web 没有自动恢复接口，恢复通过受控部署流程进行。本轮转查备份创建：一个有效源加一个缺失源，Backup 原先忽略 sourceErrors、返回成功；目录内文件读取失败时虽然返回部分失败错误，却已发布不完整归档并执行保留清理。MaxBackups=1 的两种回归均证实最后一份完整归档被删除。HTTP buildBackupSources 还会提前过滤掉显式缺失来源，因此原请求返回 201 并替换完整备份。

现在来源收集或 addFileToTar 任一失败立即返回 nil result 与错误，临时文件由已有清理逻辑移除；不发布新归档、不执行旧备份清理。所有文件和归档尾部成功写入、同步、关闭、发布后才允许保留清理。HTTP 保留全部显式来源交由备份包检查，同时支持其原本声明支持的单个文件来源；默认发现目录行为不变。

包级缺失源/不可读条目、HTTP 缺失源、显式单文件、正常备份和同秒唯一归档回归重复 10 次通过。失败回归逐字节核对旧备份未变，输出目录仅保留原归档，无残留临时文件。未操作真实备份、恢复目标或生产数据。归档命名冲突、源符号链接范围与活跃 SQLite 快照一致性不由本轮测试证明，仍需独立核对。


### 第二十二轮：备份来源链接越界与 tar 路径兼容

58a9721 的全部远端门禁和容器运行态检查通过，Ubuntu/macOS 已逐项确认失败备份保留完整恢复点，证据见 release-58a9721。

本轮以临时目录复现：来源目录内的绝对/相对文件符号链接会被 os.Stat/os.Open 跟随，来源外的标记内容进入 alias.txt；因归档被视为成功，MaxBackups=1 又会淘汰原完整备份。另一个实际归档解析回归确认 Windows 条目名为 nested\data.txt，而非跨平台 tar 所需的 nested/data.txt。

addFileToTar 先校验相对路径为本地路径，再以 os.OpenRoot(baseDir) 和 root.Open 打开文件，从同一句柄获取元数据并复制内容，不再在检查后重新按路径打开。只接受普通文件，删除越界路径降级为 basename 的逻辑；tar Header.Name 统一 filepath.ToSlash。来源根目录本身仍是用户配置的信任边界；这不是对整个来源树的原子快照。

越界链接现使整次备份失败，不生成新归档并保留旧恢复点；根内相对链接仍可备份，嵌套文件使用斜杠路径。新回归及原有正常备份、失败保留、同秒唯一文件回归重复 10 次通过。全部读取仅使用自建临时夹具，没有读取真实来源目录之外的数据；活跃 SQLite 一致性仍单独待查。


### 第二十三轮：SQLite 一致性快照基础组件（尚未接入备份入口）

sqlite-snapshot-audit 用真实 Backup 和临时 WAL 数据库两次复现：原库 1 条已提交记录，裸 db 文件备份返回成功，恢复后 0 条；VACUUM INTO 对照组恢复 1 条。该审计脚本已保存并可重放，未修改实际数据库。

本轮增加 SnapshotSQLite(ctx, existingDB, newDestination)：通过现有 *sql.DB 的连接和 go-sqlite3 Online Backup API 分页复制，不重新打开来源路径。目标文件独占创建，已有文件不覆盖；取消、连接池耗尽、关闭来源、SQLite 忙锁都会返回错误并清理本次目标。完成后 quick_check、关闭目标数据库并同步文件。目标目录由调用方持有，调用方需提供有界 context；不是整个来源目录或多库联合事务快照。

组件回归验证 WAL 中的已提交记录及显式 rowid=42 经“快照→现有 Backup→解包→重新查询”完整保留；源记录未变，含空格/#/% 的目标路径按 URI 转义。取消、池等待超时、关闭来源、防覆盖、实际 EXCLUSIVE 锁等待与失败目录清理通过，套件重复 10 次。无 CGO 构建保留同名入口并明确报错、不创建文件。

注意：本轮仅交付组件，生产 Backup、Web、BackupRunner 尚未使用它，先前裸文件丢 WAL 问题仍未修复。已定位六处应用 SQLite 打开路径（history、auth、batchdb、ICP、tamper 规则库及 check_records）；接下来需要接通连接生命周期、数据库来源识别/sidecar 处理和调用方 context，再以两个实际入口和并发事务恢复测试验收。不能把本轮单组件通过写成生产备份一致性已完成。


### 第二十四轮：备份入口取消传播与旧恢复点保留

上一轮实际入口审计已经确认，显式 SQLite 文件的 Web/Runner 备份会成功但漏掉 WAL 中的提交；整目录无并发写对照和预先生成快照对照恢复成功。本轮不重复该审计，也不把辅助组件当作生产修复。

接入前的取消回归进一步复现：已取消的 Web 请求仍返回 201；已取消/已超时的 BackupRunner 仍返回成功。当 max_backups=1，三种情况均发布新归档并删除旧恢复点。原始失败输出保存于 round24/baseline.log。

新增 BackupContext，Web 传入 r.Context()，BackupRunner 保留 Execute 的 ctx。来源遍历检查取消，tar 复制通过仅实现 Read 的 contextReader 避免 io.Copy 的文件快速路径绕开检查，在读前和读后检查 context；发布前再次检查。保持 Backup(cfg) 的背景 context 兼容入口、原有 os.Root 读取边界以及成功发布后保留清理顺序。取消不是文件系统系统调用的硬中断；若与最终 rename 竞争并在发布后才到达，则已发布归档保持成功，不执行补偿删除。

两个实际入口的取消/超时回归验证返回失败且旧归档字节不变、目录中没有新增或临时归档；底层验证启动前取消不创建目录、tar header 后取消不复制内容、读取中取消不继续读取以及正常归档完整恢复。目标套件开启竞态检测重复 10 次通过。

仍未完成：六处数据库连接生命周期、来源身份映射、SQLite 快照替换与 sidecar 处理。本轮修复取消缺陷，是该接入的先决步骤，不是 WAL 一致性修复完成。


### 第二十五轮：可注入 SQLite 快照的归档流程（连接绑定待接入）

在 5d9ad44 的原始 BackupContext 上重新执行裸文件恢复基线，已提交 WAL 标记仍恢复为 0 条，测试按正确性要求退出 1。随后实现 BackupConfig.SQLiteSnapshotter：通过来源根目录内的同一个文件句柄读取 SQLite 头和元数据，回调根据文件身份匹配已有连接，使用 SnapshotSQLite 生成独立快照；归档保留原相对路径，不使用临时文件名。启用回调后，SQLite 来源未绑定、快照失败、取消或无效输出均返回错误，不回退裸文件。

只有成功快照的主库，其精确对应且主库身份再次匹配的 -wal/-shm 才从本次归档排除；其他后缀文件、未选择主库时单独选择的 WAL 保留。临时目录由本次备份独占创建，成功和失败均清理；回调元数据来自 os.Root 约束下的源句柄，不把来源 pathname 交给回调重新打开。

真实恢复测试覆盖单文件、嵌套目录、两个数据库同名但不同目录、无关后缀文件、仅选择 sidecar、未知连接、回调部分写入后失败、取消、目录/非 SQLite 输出以及旧恢复点字节保持和临时目录清理。并发事务测试在持续两行事务更新中连续生成三次归档，恢复后的两行总和始终为 100，已提交 rowid=42 标记保留；整组开启竞态检测重复 10 次。

边界：这是单库 SQLite 快照，不是多库联合事务或目录原子快照。提供回调的连接所有者负责匹配真实文件身份、连接生命周期和有界 context；新快照组件仍未在 Web/BackupRunner 配置回调。nil 回调保留旧行为，生产 WAL 缺失问题尚未解决，下一步必须绑定全部六类数据库来源并验证两个真实入口，不能据本轮归档组件通过宣称生产完成。

补充边界回归在初版中发现：独立数据库名为 main.sqlite-wal 时会被误过滤；已改为成功快照的数据库优先保留。回归要求两个数据库都存在且各自恢复出标记记录。另外确认越界链接不会触发快照回调，旧恢复点保持不变。
