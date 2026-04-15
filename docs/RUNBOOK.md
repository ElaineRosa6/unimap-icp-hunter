# UniMap 运维 Runbook

> **创建日期：** 2026-04-15
> **维护者：** UniMap Team
> **更新频率：** 每次架构变更后更新

---

## 1. Chrome 崩溃

### 症状
- 截图功能全部失败
- 日志中出现 `cdp` 连接错误或 Chrome 进程退出
- `/api/screenshot/*` 返回 500 错误

### 诊断步骤
1. 检查 Chrome 进程是否存在：
   ```bash
   ps aux | grep chrome
   # Windows: tasklist | findstr chrome
   ```
2. 查看系统日志中的 Chrome 崩溃信息：
   ```bash
   journalctl -u unimap-web --since "1 hour ago" | grep -i chrome
   ```
3. 检查 Chrome 数据目录是否损坏：
   ```bash
   ls -la /tmp/chrome-user-data/  # 或配置中的 userDataDir
   ```

### 恢复操作
1. **自动恢复**：UniMap 会自动尝试重新连接 CDP。如果配置的 `RemoteDebugURL` 可用，会自动重建连接。
2. **手动恢复**：
   ```bash
   # 杀掉残留的 Chrome 进程
   pkill -f "chrome.*remote-debugging"
   # 重启 UniMap
   systemctl restart unimap-web
   ```
3. **预防措施**：
   - 确保系统内存充足（Chrome 每个标签页约 100-300MB）
   - 在 Docker 中运行时设置 `--shm-size=2g`
   - 监控 `goroutine` 数量，异常增长可能表示资源泄漏

---

## 2. Bridge 断连

### 症状
- Extension 模式截图失败
- 日志中出现 `bridge websocket disconnected`
- `/api/screenshot/bridge/*` 返回连接错误

### 诊断步骤
1. 检查 Bridge WebSocket 连接状态：
   ```bash
   curl -s http://localhost:8448/api/health | jq '.bridge'
   ```
2. 检查 Bridge Token 是否有效：
   ```bash
   # 查看当前 token 状态
   journalctl -u unimap-web --since "30 min ago" | grep -i "bridge.*token"
   ```
3. 检查网络连通性：
   ```bash
   curl -v http://localhost:8448/api/screenshot/bridge/ping
   ```

### 恢复操作
1. **自动恢复**：Bridge 服务内置重试机制，通常会在 30s 内自动重连。
2. **令牌轮换**（如果 Token 过期）：
   ```bash
   # 重启 Bridge 服务以生成新 token
   curl -X POST http://localhost:8448/api/screenshot/bridge/restart
   ```
3. **手动恢复**：
   ```bash
   systemctl restart unimap-web
   ```
4. **预防措施**：
   - 定期检查 Bridge 日志中的 token 轮换频率
   - 如果频繁断连，检查网络稳定性和防火墙规则

---

## 3. Cookie 失效

### 症状
- 搜索引擎截图返回登录页面
- 登录状态检测显示 `not_logged_in`
- 截图显示"请登录后查看"等提示

### 诊断步骤
1. 逐引擎检测登录状态：
   ```bash
   curl -s http://localhost:8448/api/health | jq '.engines'
   ```
2. 查看 Cookie 存储目录：
   ```bash
   ls -la hash_store/
   cat hash_store/www_baidu_com.json | jq '.cookies | length'
   ```
3. 通过定时任务手动验证：
   - 访问 `/scheduler` 页面
   - 创建 `Cookie 验证` 类型任务立即执行

### 恢复操作
1. **重新导入 Cookie**：
   - 访问 Web 界面的 Cookie 管理页面
   - 从浏览器导出最新 Cookie 并导入
2. **API 方式**：
   ```bash
   curl -X POST http://localhost:8448/api/cookies/verify \
     -H 'Content-Type: application/json' \
     -d '{"engines": ["fofa", "hunter", "quake", "zoomeye"]}'
   ```
3. **预防措施**：
   - 创建定时任务定期验证 Cookie 有效性（建议每天一次）
   - 配置告警：当 Cookie 验证失败时发送通知

---

## 4. 节点失联

### 症状
- 分布式任务长时间处于 `PENDING` 或 `ASSIGNED` 状态
- 节点列表中显示节点为 `offline`
- 任务领取后无结果返回

### 诊断步骤
1. 检查节点健康状态：
   ```bash
   curl -s http://localhost:8448/api/distributed/nodes | jq '.[] | {id, online, last_heartbeat}'
   ```
2. 检查心跳超时配置：
   ```bash
   cat config.yaml | grep -A3 "distributed:"
   # 默认心跳超时: 30s
   ```
3. 检查网络连通性：
   ```bash
   # 从主节点 ping 工作节点
   ping <node-ip>
   # 检查端口连通性
   nc -zv <node-ip> <node-port>
   ```
4. 查看任务队列状态：
   ```bash
   curl -s http://localhost:8448/api/distributed/tasks | jq '{total, pending, claimed}'
   ```

### 恢复操作
1. **自动故障转移**：系统在检测到节点 offline 后，会自动将该节点领取的任务释放回 `PENDING` 状态，其他健康节点可领取。
2. **节点重启**：
   ```bash
   # 在工作节点上
   systemctl restart unimap-node
   ```
3. **手动重新分配**：
   ```bash
   # 强制将超时任务标记为 PENDING
   curl -X POST http://localhost:8448/api/distributed/tasks/reassign
   ```
4. **预防措施**：
   - 监控节点心跳间隔，设置告警阈值（超过 60s 未心跳）
   - 确保网络带宽和延迟满足要求
   - 配置至少 2 个工作节点实现高可用

---

## 5. 磁盘满

### 症状
- 截图保存失败，日志中出现 `no space left on device`
- 导出文件无法写入
- 系统整体响应变慢

### 诊断步骤
1. 检查磁盘使用情况：
   ```bash
   df -h
   du -sh ./screenshots/ ./data/ ./hash_store/ ./logs/ 2>/dev/null
   ```
2. 检查大文件：
   ```bash
   find ./screenshots/ -name "*.png" -mtime +30 | wc -l
   du -sh ./screenshots/*/ 2>/dev/null | sort -rh | head -20
   ```
3. 检查日志文件大小：
   ```bash
   du -sh *.log 2>/dev/null
   ```

### 恢复操作
1. **清理过期截图**（推荐）：
   - 访问 `/scheduler` 页面
   - 创建 `截图清理` 任务，设置 `max_age_days: 30`
   - 立即执行
2. **手动清理**：
   ```bash
   # 删除 30 天前的截图
   find ./screenshots/ -name "*.png" -mtime +30 -delete
   # 清理空的批次目录
   find ./screenshots/ -type d -empty -delete
   ```
3. **清理篡改检测记录**：
   - 创建 `篡改记录清理` 定时任务，设置 `max_age_days: 90`
4. **扩容**：
   ```bash
   # Docker 环境
   docker-compose down
   # 增加挂载的磁盘容量
   docker-compose up -d
   ```
5. **预防措施**：
   - 创建定时清理任务（截图 30 天，篡改记录 90 天）
   - 监控磁盘使用率，设置告警阈值 > 80%
   - 配置日志轮转（logrotate）

---

## 6. Redis 不可用

### 症状
- 缓存命中率骤降
- 日志中出现 `redis: connection refused` 或 `redis: timeout`
- 查询响应时间明显增加

### 诊断步骤
1. 检查 Redis 连接状态：
   ```bash
   redis-cli ping
   # 应返回 PONG
   ```
2. 检查 Redis 配置：
   ```bash
   cat config.yaml | grep -A5 "redis:"
   ```
3. 检查缓存降级状态：
   ```bash
   journalctl -u unimap-web --since "1 hour ago" | grep -i "redis\|cache\|fallback"
   ```

### 恢复操作
1. **自动降级**：系统检测到 Redis 不可用时，会自动降级到内存缓存（in-memory cache）。功能不受影响，但多实例间缓存不共享。
2. **重启 Redis**：
   ```bash
   systemctl restart redis
   # Docker 环境
   docker-compose restart redis
   ```
3. **验证恢复**：
   ```bash
   redis-cli ping  # 应返回 PONG
   # 观察日志中是否出现 cache fallback 停止
   journalctl -u unimap-web -f | grep cache
   ```
4. **预防措施**：
   - 在 Grafana 中监控缓存命中率，低于 50% 时告警
   - Redis 配置持久化（RDB 或 AOF）
   - 如果生产环境强依赖 Redis，考虑配置 Redis Sentinel 或 Cluster

---

## 附录：常用诊断命令

```bash
# 服务状态
systemctl status unimap-web

# 实时日志
journalctl -u unimap-web -f

# 健康检查
curl -s http://localhost:8448/api/health | jq

# 查看所有定时任务
curl -s http://localhost:8448/api/scheduler/tasks | jq

# 查看执行历史
curl -s 'http://localhost:8448/api/scheduler/history?limit=10' | jq

# Goroutine 数量（Go 运行时）
curl -s http://localhost:8448/debug/pprof/goroutine?debug=1 | head -5

# 内存使用
curl -s http://localhost:8448/debug/pprof/heap?debug=1 | head -20
```

## 附录：关键指标告警阈值

| 指标 | 警告阈值 | 严重阈值 | 检查频率 |
|------|---------|---------|---------|
| 查询 P95 延迟 | > 30s | > 60s | 1 min |
| 缓存命中率 | < 50% | < 20% | 5 min |
| 截图成功率 | < 90% | < 70% | 5 min |
| 节点在线率 | < 80% | < 50% | 1 min |
| Goroutine 数 | > 1000 | > 5000 | 5 min |
| 内存使用 | > 80% | > 95% | 1 min |
| 磁盘使用 | > 80% | > 90% | 15 min |
| Redis 连接 | 不可用 | 不可用 | 1 min |
