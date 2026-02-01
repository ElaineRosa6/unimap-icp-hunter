# UniMap + ICP-Hunter

基于多引擎的统一网络资产测绘与未备案网站自动化检测系统

## 项目简介

**UniMap + ICP-Hunter** 是一个集成了多引擎网络资产测绘和自动化ICP备案检测的综合系统。系统采用 Go 语言开发，支持 FOFA、Hunter、ZoomEye、Quake 等多个搜索引擎，通过统一查询语言 (UQL) 进行资产发现，并自动检测目标网站的 ICP 备案状态。

### 核心功能

- 🔍 **多引擎统一查询**: 支持 FOFA、Hunter、ZoomEye、Quake 等引擎的统一查询接口
- 📝 **UQL 语言**: 自定义统一查询语言，支持复杂的查询逻辑
- 🤖 **自动化检测**: 每日自动扫描，检测未备案网站
- 📊 **智能分析**: 基于正则表达式的 ICP 备案号识别
- 📸 **截图存档**: 支持网站截图保存到 MinIO/S3
- 📧 **通知系统**: 邮件、Webhook (钉钉/企业微信/飞书) 通知
- 📈 **统计报表**: 每日统计、未备案列表、历史趋势
- 🐳 **容器化部署**: Docker Compose 一键部署
- ⚙️ **CLI 工具**: 完整的命令行工具支持

## 技术栈

- **语言**: Go 1.21+
- **Web 框架**: Gin
- **CLI 框架**: Cobra + Viper
- **数据库**: MySQL 8.0 (GORM)
- **缓存/队列**: Redis 7 (Stream)
- **对象存储**: MinIO / S3
- **日志**: Zap
- **监控**: Prometheus + Grafana
- **容器化**: Docker + Docker Compose

## 项目结构

```
unimap-icp-hunter/
├── cmd/
│   ├── unimap/          # UniMap CLI 入口
│   └── icp-hunter/      # ICP-Hunter CLI 入口
├── internal/
│   ├── model/           # 数据模型
│   │   ├── asset.go     # 资产和ICP检查模型
│   │   ├── redis.go     # Redis Stream 消息
│   │   └── unimap.go    # UQL AST 和接口定义
│   ├── core/
│   │   └── unimap/      # UQL 解析器和结果合并器
│   │       ├── parser.go
│   │       └── merger.go
│   ├── adapter/         # 引擎适配器
│   │   ├── adapter.go   # 接口定义
│   │   ├── fofa.go      # FOFA 适配器
│   │   ├── hunter.go    # Hunter 适配器
│   │   └── orchestrator.go  # 多引擎编排
│   ├── service/         # 业务服务
│   │   ├── worker/      # 工作节点服务
│   │   │   ├── probe.go     # HTTP 探测
│   │   │   └── analyzer.go  # ICP 分析器
│   │   ├── scheduler/   # 调度器服务
│   │   │   ├── scheduler.go
│   │   │   └── policy.go
│   │   ├── notification/ # 通知服务
│   │   │   └── notifier.go
│   │   ├── icp_service.go      # ICP 核心服务
│   │   └── unimap_service.go   # UniMap 核心服务
│   └── repository/      # 数据访问层
│       └── repository.go
├── pkg/
│   └── utils/           # 工具包
│       ├── redis.go     # Redis 工具
│       └── http.go      # HTTP 工具
├── configs/             # 配置文件
│   ├── config.yaml      # 主配置
│   ├── config.example.yaml  # 配置模板
│   └── redis.conf       # Redis 配置
├── scripts/             # 脚本
│   └── init.sql         # 数据库初始化
├── Dockerfile.unimap    # UniMap API Dockerfile
├── Dockerfile.icp       # ICP-Hunter Dockerfile
├── docker-compose.yml   # 编排配置
├── go.mod
├── go.sum
└── README.md
```

## 快速开始

### 前置要求

- Docker 20.10+
- Docker Compose 2.0+
- 环境变量配置 (API Keys)

### 1. 环境配置

创建 `.env` 文件：

```bash
# FOFA API 配置
FOFA_API_KEY=your_fofa_api_key
FOFA_EMAIL=your_fofa_email

# Hunter API 配置
HUNTER_API_KEY=your_hunter_api_key

# 邮件通知配置 (可选)
EMAIL_USERNAME=your_email@domain.com
EMAIL_PASSWORD=your_email_password
EMAIL_SMTP_HOST=smtp.domain.com
EMAIL_SMTP_PORT=587

# Webhook 配置 (可选)
WEBHOOK_URL=https://oapi.dingtalk.com/robot/send?access_token=xxx
```

### 2. 启动服务

```bash
# 构建并启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f icp-scheduler
docker-compose logs -f icp-worker-1
docker-compose logs -f unimap-api
```

### 3. 数据库初始化

数据库会自动通过 `scripts/init.sql` 初始化，包含：
- 所有数据表结构
- 默认扫描策略
- 敏感域名白名单
- 统计视图

### 4. 使用 CLI 工具

#### UniMap 查询工具

```bash
# 进入 UniMap 容器
docker exec -it unimap-api sh

# 查询资产
./unimap-api query 'country="CN" && port="80" && protocol="http"' --engines fofa,hunter --limit 100

# 验证 UQL 语法
./unimap-api validate 'country="CN" && port="80"'

# 列出支持的引擎
./unimap-api list-engines
```

#### ICP-Hunter 工具

```bash
# 进入 ICP-Hunter 容器
docker exec -it icp-scheduler sh

# 运行完整扫描流程
./icp-hunter run --policy http_80

# 手动触发扫描任务
./icp-hunter scan --policy https_443

# 启动调度器 (Cron 模式)
./icp-hunter scheduler

# 启动工作节点
./icp-hunter worker

# 查看统计
./icp-hunter stats --days 7

# 生成报告
./icp-hunter report --type daily --date 2026-01-14

# 管理白名单
./icp-hunter whitelist add --type domain --value "example.com" --reason "测试域名"
./icp-hunter whitelist list
./icp-hunter whitelist remove --id 1
```

### 5. Web API (可选扩展)

如果需要 Web API 接口，可以使用以下端点：

```bash
# 查询资产 (POST /api/v1/unimap/query)
curl -X POST http://localhost:8080/api/v1/unimap/query \
  -H "Content-Type: application/json" \
  -d '{"query": "country=\"CN\" && port=\"80\"", "engines": ["fofa", "hunter"], "limit": 100}'

# 获取统计 (GET /api/v1/stats/daily)
curl http://localhost:8080/api/v1/stats/daily?days=7

# 获取未备案列表 (GET /api/v1/unregistered)
curl http://localhost:8080/api/v1/unregistered?limit=50
```

## UQL 语法说明

UniMap 统一查询语言 (UQL) 支持以下语法：

### 基本操作符

```
# 等于
country="CN"
port="80"

# 不等于
country!="US"

# 包含
title~="管理后台"

# 正则匹配
body~="icp.*\\d+"

# 范围查询
port IN ["80", "443", "8080"]

# 逻辑组合
country="CN" && port="80" && protocol="http"
country="CN" || country="HK"
!(country="US")
```

### 支持的字段

| 字段 | 说明 | 示例 |
|------|------|------|
| ip | IP地址 | `ip="192.168.1.1"` |
| port | 端口 | `port="80"` |
| protocol | 协议 | `protocol="http"` |
| country | 国家代码 | `country="CN"` |
| region | 地区 | `region="beijing"` |
| city | 城市 | `city="shanghai"` |
| title | 网页标题 | `title~="登录"` |
| body | 网页内容 | `body~="ICP"` |
| server | Server头 | `server="nginx"` |
| host | 域名 | `host="example.com"` |
| url | 完整URL | `url~="https://"` |
| asn | ASN编号 | `asn="AS12345"` |
| org | 组织 | `org="Tencent"` |
| isp | 运营商 | `isp="China Telecom"` |

## ICP 检测模式

系统使用 30+ 正则表达式模式检测 ICP 备案信息：

### 支持的格式

```
京ICP备2021000000号
粤ICP备2021000000号-1
沪ICP备2021000000号-2A
浙公网安备 33010002000001号
公安网备 11010502000001号
京ICP备2021000000号-123
```

### 检测结果

- **已备案** (is_registered=1): 成功匹配到 ICP 备案号
- **未备案** (is_registered=0): 明确检测到无备案信息
- **不确定** (is_registered=2): 无法确定备案状态

## 配置说明

### 主要配置项

```yaml
# 数据库配置
database:
  host: mysql
  port: 3306
  username: unimap_user
  password: unimap_password
  name: unimap_icp_hunter

# Redis 配置
redis:
  host: redis
  port: 6379
  db: 0

# 存储配置
storage:
  type: minio  # minio 或 s3
  endpoint: http://minio:9000
  access_key: minioadmin
  secret_key: minioadmin
  bucket: icp-screenshots

# 引擎配置
engines:
  fofa:
    enabled: true
    api_key: ${FOFA_API_KEY}
    email: ${FOFA_EMAIL}
    timeout: 30
  hunter:
    enabled: true
    api_key: ${HUNTER_API_KEY}
    timeout: 30

# ICP 检测配置
icp:
  patterns:
    - "京ICP备[\\d]+号"
    - "粤ICP备[\\d]+号-[\\d]+"
    # ... 更多模式
  whitelist:
    - "gov.cn"
    - "edu.cn"
    - "mil.cn"
  screenshot: true
  timeout: 10

# 调度器配置
scheduler:
  cron: "0 2 * * *"  # 每天凌晨2点执行
  policies:
    - http_80
    - https_443
    - common_alt_ports

# 通知配置
notification:
  email:
    enabled: false
    smtp_host: smtp.example.com
    smtp_port: 587
    username: ${EMAIL_USERNAME}
    password: ${EMAIL_PASSWORD}
    to: admin@example.com
  webhook:
    enabled: false
    url: ${WEBHOOK_URL}
    type: dingtalk  # dingtalk, wechat, lark
```

## 高级功能

### 1. 分布式任务队列

使用 Redis Stream 实现分布式任务队列：

```go
// 调度器推送任务
scheduler.PushTask(task)

// 工作节点消费任务
worker.ConsumeTask()
```

支持多个 Worker 并行处理，自动负载均衡。

### 2. 智能去重

结果合并器支持：
- 基于 IP:Port 的去重
- URL 规范化
- 引擎优先级 (FOFA > Hunter > ZoomEye > Quake)
- 字段补全和更新

### 3. 策略管理

支持多种扫描策略：
- **区域策略**: 按国家/地区扫描
- **端口策略**: 常见 Web 端口扫描
- **优先级策略**: 高价值目标优先

### 4. 监控和告警

集成 Prometheus 指标：
- 任务执行数量
- 检测成功率
- 扫描耗时
- 队列长度

Grafana 仪表盘预设：
- 每日扫描统计
- 未备案趋势
- 引擎成功率对比

## 运维指南

### 日常运维

```bash
# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f [service]

# 重启服务
docker-compose restart [service]

# 备份数据库
docker exec unimap-mysql mysqldump -u root -p unimap_icp_hunter > backup.sql

# 恢复数据库
docker exec -i unimap-mysql mysql -u root -p unimap_icp_hunter < backup.sql
```

### 性能调优

1. **数据库优化**
   - 定期执行 `OPTIMIZE TABLE`
   - 添加必要索引
   - 调整 InnoDB 缓冲池

2. **Redis 优化**
   - 调整 `maxmemory-policy`
   - 监控内存使用
   - 合理设置过期时间

3. **Worker 扩展**
   - 根据负载增加 Worker 数量
   - 调整并发参数
   - 监控队列长度

### 故障排查

```bash
# 数据库连接问题
docker logs unimap-mysql
docker exec -it unimap-mysql mysql -u unimap_user -p

# Redis 连接问题
docker logs unimap-redis
redis-cli -h redis ping

# Worker 无响应
docker logs icp-worker-1
docker exec -it icp-worker-1 ps aux

# 查看任务队列
redis-cli -h redis XLEN icp:queue:probe
```

## 安全注意事项

1. **API Keys 管理**
   - 使用环境变量或密钥管理服务
   - 定期轮换 API Keys
   - 不要提交到版本控制

2. **数据库安全**
   - 修改默认密码
   - 限制访问 IP
   - 启用 SSL 连接

3. **Webhook 安全**
   - 验证签名
   - 限制接收 IP
   - 使用 HTTPS

4. **白名单管理**
   - 添加政府、教育等敏感域名
   - 定期审查白名单
   - 记录变更原因

## 开发指南

### 添加新引擎

1. 在 `internal/adapter/` 创建新适配器
2. 实现 `EngineAdapter` 接口
3. 在 `orchestrator.go` 注册引擎
4. 更新配置文件

### 自定义 ICP 模式

在 `configs/config.yaml` 的 `icp.patterns` 添加新正则：

```yaml
icp:
  patterns:
    - "你的正则表达式"
```

### 扩展通知渠道

在 `internal/service/notification/notifier.go` 添加新的通知方法：

```go
func (n *Notifier) SendToNewChannel(message string) error {
    // 实现新渠道
}
```

## 许可证

本项目仅供学习和研究使用，请遵守相关法律法规和服务条款。

## 贡献

欢迎提交 Issue 和 Pull Request。

## 相关文档

- [快速开始指南](QUICKSTART.md) - 快速部署和使用
- [部署指南](docs/部署指南.md) - 详细的部署说明
- [使用手册](docs/使用手册.md) - 功能使用说明
- [API文档](docs/API文档.md) - API 接口文档
- [性能优化指南](docs/性能优化指南.md) - 性能调优建议

## 更新日志

### v1.0.1 (2026-02-01)
- ✅ 修复编译错误和代码质量问题
- ✅ 优化 HTTP 客户端连接复用
- ✅ 添加单元测试框架
- ✅ 改进错误处理和日志记录
- ✅ 新增性能优化指南文档
- ✅ 代码格式化和标准化

### v1.0.0 (2026-01-14)
- ✅ 初始版本发布
- ✅ 支持 FOFA 和 Hunter 引擎
- ✅ 实现 UQL 查询语言
- ✅ 完整的 ICP 检测流程
- ✅ Docker 容器化部署
- ✅ CLI 工具支持
- ✅ 通知系统 (邮件/Webhook)
- ✅ 统计报表功能
- ✅ 白名单管理
- ✅ 分布式任务队列

---

**UniMap + ICP-Hunter** © 2026 | 架构设计与开发 by Claude Code