# UniMap + ICP-Hunter 快速开始指南

## 🚀 5分钟快速部署

### 前置条件

```bash
# 检查 Docker 是否安装
docker --version        # 需要 20.10+
docker-compose --version # 需要 2.0+
```

### 步骤 1: 准备配置

```bash
# 进入项目目录
cd unimap-icp-hunter

# 创建环境变量文件
cp .env.example .env

# 编辑配置，填入 API Keys
# 只需要填入以下三项:
# FOFA_API_KEY=your_key
# FOFA_EMAIL=your_email
# HUNTER_API_KEY=your_key
vim .env
```

### 步骤 2: 启动系统

```bash
# 一键启动所有服务
docker-compose up -d --build

# 查看启动状态
docker-compose ps

# 等待 1-3 分钟，直到所有服务显示 "running"
```

### 步骤 3: 验证部署

```bash
# 进入 ICP-Hunter 容器
docker exec -it icp-scheduler sh

# 查看帮助
./icp-hunter --help

# 查看统计 (应该显示空数据)
./icp-hunter stats --today

# 测试白名单
./icp-hunter whitelist list

# 退出容器
exit
```

### 步骤 4: 执行第一次扫描

```bash
# 进入调度器容器
docker exec -it icp-scheduler sh

# 手动触发扫描 (使用默认策略 http_80)
./icp-hunter scan --policy http_80

# 等待扫描完成 (约 1-5 分钟)

# 查看结果
./icp-hunter stats --today

# 查看未备案列表
./icp-hunter stats --today | grep "未备案"

# 退出容器
exit
```

### 步骤 5: 查看结果

```bash
# 进入数据库查看详细结果
docker exec -it unimap-mysql mysql -u unimap_user -p

# 输入密码: unimap_password

# 查看今日检测结果
SELECT
    a.ip,
    a.port,
    a.url,
    a.title,
    c.icp_code,
    c.is_registered,
    c.check_time
FROM assets a
JOIN icp_checks c ON a.id = c.asset_id
WHERE c.check_time >= CURDATE();

# 查看未备案列表
SELECT * FROM v_unregistered_assets;

# 退出
exit
```

---

## 📋 常用命令速查

### Docker 操作

```bash
# 启动系统
docker-compose up -d

# 停止系统
docker-compose stop

# 重启系统
docker-compose restart

# 查看日志
docker-compose logs -f icp-scheduler
docker-compose logs -f icp-worker-1

# 查看状态
docker-compose ps

# 进入容器
docker exec -it icp-scheduler sh
docker exec -it unimap-api sh
```

### UniMap CLI

```bash
# 查询资产
./unimap-api query 'country="CN" && port="80"' --limit 10

# 验证 UQL
./unimap-api validate 'country="CN" && port="80"'

# 列出引擎
./unimap-api list-engines
```

### ICP-Hunter CLI

```bash
# 运行完整流程
./icp-hunter run --policy http_80

# 手动扫描
./icp-hunter scan --policy http_80

# 启动调度器
./icp-hunter scheduler

# 启动工作节点
./icp-hunter worker

# 查看统计
./icp-hunter stats --days 7

# 生成报告
./icp-hunter report --type daily --date today

# 白名单管理
./icp-hunter whitelist list
./icp-hunter whitelist add --type domain --value "example.com" --reason "测试"
```

---

## 🎯 核心功能演示

### 场景 1: 查询并检测

```bash
# 1. 使用 UniMap 查询
docker exec -it unimap-api ./unimap-api query 'country="CN" && port="80"' --limit 10

# 2. 手动触发 ICP 检测
docker exec -it icp-scheduler ./icp-hunter scan --policy http_80

# 3. 查看检测结果
docker exec -it icp-scheduler ./icp-hunter stats --today
```

### 场景 2: 管理白名单

```bash
# 添加白名单
docker exec -it icp-scheduler ./icp-hunter whitelist add \
  --type domain --value "gov.cn" --reason "政府域名"

# 查看白名单
docker exec -it icp-scheduler ./icp-hunter whitelist list

# 删除白名单
docker exec -it icp-scheduler ./icp-hunter whitelist remove --id 1
```

### 场景 3: 生成报告

```bash
# 生成今日报告
docker exec -it icp-scheduler ./icp-hunter report --type daily --date today

# 查看报告文件 (如果保存为文件)
docker exec -it icp-scheduler ls -la /app/reports/
```

---

## 🔍 故障排查

### 问题: 服务启动失败

```bash
# 检查日志
docker-compose logs

# 常见原因:
# 1. 端口被占用: 修改 docker-compose.yml 中的端口
# 2. 缺少 API Keys: 检查 .env 文件
# 3. 磁盘空间不足: df -h 检查磁盘
```

### 问题: 扫描无结果

```bash
# 1. 检查 API Key 是否有效
echo $FOFA_API_KEY

# 2. 检查 Redis 队列
docker exec -it unimap-redis redis-cli XLEN icp:queue:probe

# 3. 查看 Worker 日志
docker-compose logs icp-worker-1 | grep ERROR
```

### 问题: 数据库连接失败

```bash
# 检查 MySQL 状态
docker ps | grep mysql

# 手动连接测试
docker exec -it unimap-mysql mysql -u unimap_user -p
```

---

## 📊 验证清单

部署完成后，请验证以下项目:

- [ ] Docker 容器全部运行 (docker-compose ps)
- [ ] 数据库表已创建 (SHOW TABLES)
- [ ] 默认策略已插入 (SELECT * FROM scan_policies)
- [ ] 白名单已配置 (SELECT * FROM whitelist)
- [ ] UniMap CLI 可用 (./unimap-api --help)
- [ ] ICP-Hunter CLI 可用 (./icp-hunter --help)
- [ ] 手动扫描可执行 (./icp-hunter scan --policy http_80)
- [ ] 统计功能正常 (./icp-hunter stats --today)

---

## ⚡ 快速测试

### 测试 1: 验证 UQL 解析

```bash
docker exec -it unimap-api ./unimap-api validate 'country="CN" && port="80"'
# 预期输出: ✓ Valid UQL syntax
```

### 测试 2: 查询测试

```bash
docker exec -it unimap-api ./unimap-api query 'country="CN" && port="80"' --limit 5
# 预期输出: 5 条资产记录
```

### 测试 3: 扫描测试

```bash
docker exec -it icp-scheduler ./icp-hunter scan --policy http_80
# 预期输出: Task created successfully
```

### 测试 4: 统计测试

```bash
docker exec -it icp-scheduler ./icp-hunter stats --today
# 预期输出: 今日扫描统计
```

---

## 🎓 下一步

完成快速部署后，建议:

1. **阅读完整文档**
   - 查看 `docs/部署指南.md` 了解详细配置
   - 查看 `docs/使用手册.md` 学习高级功能
   - 查看 `docs/API文档.md` 了解 API 接口

2. **配置定时任务**
   - 设置 Cron 每天自动扫描
   - 配置邮件/Webhook 通知

3. **优化配置**
   - 根据需求调整并发数
   - 添加自定义白名单
   - 创建自定义扫描策略

4. **监控系统**
   - 访问 Grafana 监控 (http://localhost:3000)
   - 查看 MinIO 控制台 (http://localhost:9001)
   - 监控 Redis 队列长度

---

## 💡 提示

### 性能优化

```bash
# 如果 API 调用受限，降低并发
# 编辑 configs/config.yaml
# max_concurrency: 10

# 如果扫描速度慢，增加 Worker
docker-compose up -d --scale icp-worker-1=3
```

### 数据管理

```bash
# 定期清理旧数据 (进入 MySQL)
DELETE FROM icp_checks WHERE check_time < DATE_SUB(NOW(), INTERVAL 90 DAY);
OPTIMIZE TABLE icp_checks;
```

### 安全建议

```bash
# 1. 修改默认密码
# 编辑 docker-compose.yml

# 2. 限制端口暴露
# 只暴露必要端口到公网

# 3. 保护 API Keys
# 不要提交 .env 文件到 Git
```

---

## 🆘 获取帮助

遇到问题时:

1. **查看日志**: `docker-compose logs -f [service]`
2. **检查文档**: `docs/部署指南.md` 故障排查章节
3. **验证配置**: `docker exec -it [container] env`
4. **测试连接**: `docker exec -it [container] ping [host]`

---

**版本**: v1.0.0
**更新时间**: 2026-01-14
**文档状态**: ✅ 完整