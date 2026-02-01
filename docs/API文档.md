# UniMap + ICP-Hunter API 文档

## 概述

本文档描述了 UniMap + ICP-Hunter 系统的 Web API 接口。这些接口用于与系统进行交互，执行查询、扫描、获取统计信息等操作。

## 基础信息

- **Base URL**: `http://localhost:8080`
- **认证方式**: Bearer Token (可选)
- **数据格式**: JSON
- **响应格式**: JSON

## 认证

### 无认证模式 (默认)

```bash
curl http://localhost:8080/api/v1/stats/daily
```

### Bearer Token 认证

```bash
curl -H "Authorization: Bearer YOUR_API_TOKEN" \
     http://localhost:8080/api/v1/stats/daily
```

**配置方法**:
在 `configs/config.yaml` 中添加:

```yaml
server:
  api_key: "YOUR_API_TOKEN"
```

## 通用响应格式

### 成功响应

```json
{
  "status": "success",
  "data": {
    // 响应数据
  },
  "message": "操作成功",
  "timestamp": "2026-01-14T10:30:00Z"
}
```

### 错误响应

```json
{
  "status": "error",
  "code": "ERROR_CODE",
  "message": "错误描述",
  "timestamp": "2026-01-14T10:30:00Z"
}
```

### 错误代码

| 代码 | 说明 | HTTP 状态码 |
|------|------|-------------|
| `UQL_PARSE_ERROR` | UQL 语法错误 | 400 |
| `ENGINE_ERROR` | 引擎调用失败 | 500 |
| `DB_ERROR` | 数据库错误 | 500 |
| `AUTH_ERROR` | 认证失败 | 401 |
| `VALIDATION_ERROR` | 参数验证失败 | 400 |
| `NOT_FOUND` | 资源不存在 | 404 |
| `RATE_LIMIT` | 请求频率限制 | 429 |

---

## UniMap 相关接口

### 1. 查询资产

**端点**: `POST /api/v1/unimap/query`

**描述**: 使用 UQL 查询语言搜索网络资产

**请求体**:

```json
{
  "query": "country=\"CN\" && port=\"80\" && protocol=\"http\"",
  "engines": ["fofa", "hunter"],
  "limit": 100,
  "timeout": 30
}
```

**参数说明**:

| 参数 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| query | string | 是 | - | UQL 查询语句 |
| engines | array | 否 | 所有启用引擎 | 引擎列表 |
| limit | number | 否 | 100 | 最大结果数 |
| timeout | number | 否 | 30 | 超时时间(秒) |

**响应示例**:

```json
{
  "status": "success",
  "data": {
    "total": 100,
    "engines": {
      "fofa": 50,
      "hunter": 50
    },
    "results": [
      {
        "ip": "192.168.1.1",
        "port": 80,
        "protocol": "http",
        "url": "http://192.168.1.1",
        "title": "Example Site",
        "country": "CN",
        "region": "beijing",
        "city": "beijing",
        "asn": "AS4837",
        "org": "China Unicom",
        "isp": "China Unicom",
        "source": "fofa"
      }
    ]
  }
}
```

**错误示例**:

```json
{
  "status": "error",
  "code": "UQL_PARSE_ERROR",
  "message": "Invalid UQL syntax at position 15",
  "timestamp": "2026-01-14T10:30:00Z"
}
```

### 2. 验证 UQL

**端点**: `POST /api/v1/unimap/validate`

**描述**: 验证 UQL 语法是否正确

**请求体**:

```json
{
  "query": "country=\"CN\" && port=\"80\""
}
```

**响应示例**:

```json
{
  "status": "success",
  "data": {
    "valid": true,
    "message": "Valid UQL syntax"
  }
}
```

### 3. 列出支持的引擎

**端点**: `GET /api/v1/unimap/engines`

**描述**: 获取所有支持的搜索引擎及其状态

**响应示例**:

```json
{
  "status": "success",
  "data": {
    "engines": [
      {
        "name": "fofa",
        "enabled": true,
        "description": "FOFA Pro"
      },
      {
        "name": "hunter",
        "enabled": true,
        "description": "Hunter"
      },
      {
        "name": "zoomeye",
        "enabled": false,
        "description": "ZoomEye"
      },
      {
        "name": "quake",
        "enabled": false,
        "description": "Quake"
      }
    ]
  }
}
```

---

## ICP 检测相关接口

### 4. 获取统计信息

**端点**: `GET /api/v1/stats/daily`

**描述**: 获取每日统计信息

**查询参数**:

| 参数 | 类型 | 说明 | 示例 |
|------|------|------|------|
| days | number | 最近天数 | 7 |
| from | string | 开始日期 (YYYY-MM-DD) | 2026-01-01 |
| to | string | 结束日期 (YYYY-MM-DD) | 2026-01-14 |
| format | string | 输出格式 (table/json) | json |

**示例 1 - 最近 7 天**:

```bash
curl http://localhost:8080/api/v1/stats/daily?days=7
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "summary": {
      "total_days": 7,
      "total_scanned": 8500,
      "total_registered": 6200,
      "total_unregistered": 1500,
      "total_uncertain": 800,
      "avg_unregistered_rate": 17.6
    },
    "daily": [
      {
        "date": "2026-01-07",
        "total_scanned": 1200,
        "registered": 890,
        "unregistered": 210,
        "uncertain": 100,
        "unregistered_rate": 17.5
      },
      {
        "date": "2026-01-08",
        "total_scanned": 1150,
        "registered": 850,
        "unregistered": 200,
        "uncertain": 100,
        "unregistered_rate": 17.4
      }
    ],
    "trend": {
      "total_scanned": 15.2,
      "unregistered": 8.5
    }
  }
}
```

**示例 2 - 指定日期范围**:

```bash
curl "http://localhost:8080/api/v1/stats/daily?from=2026-01-01&to=2026-01-14"
```

### 5. 获取未备案列表

**端点**: `GET /api/v1/unregistered`

**描述**: 获取未备案资产列表

**查询参数**:

| 参数 | 类型 | 说明 | 示例 |
|------|------|------|------|
| days | number | 最近天数 | 7 |
| from | string | 开始日期 | 2026-01-01 |
| to | string | 结束日期 | 2026-01-14 |
| limit | number | 每页数量 | 50 |
| offset | number | 偏移量 | 0 |
| format | string | 输出格式 (json/csv) | json |

**示例**:

```bash
curl "http://localhost:8080/api/v1/unregistered?days=7&limit=50&offset=0"
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "total": 1500,
    "limit": 50,
    "offset": 0,
    "items": [
      {
        "id": 1,
        "ip": "192.168.1.1",
        "port": 80,
        "url": "http://192.168.1.1",
        "title": "未备案网站",
        "region": "beijing",
        "country_code": "CN",
        "check_time": "2026-01-14T10:30:00Z",
        "icp_code": null,
        "screenshot_path": "http://minio:9000/icp-screenshots/1_20260114.png"
      }
    ]
  }
}
```

### 6. 获取单个资产详情

**端点**: `GET /api/v1/assets/:id`

**描述**: 获取指定资产的详细信息

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | number | 资产 ID |

**示例**:

```bash
curl http://localhost:8080/api/v1/assets/1
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "asset": {
      "id": 1,
      "ip": "192.168.1.1",
      "port": 80,
      "protocol": "http",
      "url": "http://192.168.1.1",
      "title": "Example Site",
      "body_snippet": "<html>...",
      "server": "nginx",
      "headers": {"Server": "nginx"},
      "status_code": 200,
      "country_code": "CN",
      "region": "beijing",
      "city": "beijing",
      "asn": "AS4837",
      "org": "China Unicom",
      "isp": "China Unicom",
      "sources": ["fofa", "hunter"],
      "first_seen_at": "2026-01-10T08:00:00Z",
      "last_seen_at": "2026-01-14T10:30:00Z",
      "created_at": "2026-01-10T08:00:00Z"
    },
    "checks": [
      {
        "id": 1,
        "check_time": "2026-01-14T10:30:00Z",
        "http_status_code": 200,
        "title": "Example Site",
        "icp_code": "京ICP备2021000000号",
        "is_registered": 1,
        "match_method": "regex",
        "screenshot_path": "http://minio:9000/icp-screenshots/1_20260114.png"
      }
    ]
  }
}
```

### 7. 获取 ICP 检查详情

**端点**: `GET /api/v1/checks/:id`

**描述**: 获取指定 ICP 检查的详细信息

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | number | ICP 检查 ID |

**示例**:

```bash
curl http://localhost:8080/api/v1/checks/1
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "id": 1,
    "asset_id": 1,
    "check_time": "2026-01-14T10:30:00Z",
    "url": "http://192.168.1.1",
    "http_status_code": 200,
    "title": "Example Site",
    "icp_code": "京ICP备2021000000号",
    "is_registered": 1,
    "match_method": "regex",
    "html_hash": "a1b2c3d4e5f6",
    "screenshot_path": "http://minio:9000/icp-screenshots/1_20260114.png",
    "error_message": null,
    "tags": ["verified", "https"]
  }
}
```

---

## 扫描任务相关接口

### 8. 开始扫描任务

**端点**: `POST /api/v1/scan/start`

**描述**: 手动触发一个新的扫描任务

**请求体**:

```json
{
  "policy": "http_80",
  "priority": "normal",
  "description": "每日例行扫描"
}
```

**参数说明**:

| 参数 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| policy | string | 是 | - | 策略名称 |
| priority | string | 否 | normal | 优先级 (low/normal/high) |
| description | string | 否 | - | 任务描述 |

**响应**:

```json
{
  "status": "success",
  "data": {
    "task_id": 12345,
    "status": "pending",
    "message": "Task created successfully"
  }
}
```

### 9. 获取任务状态

**端点**: `GET /api/v1/tasks/status`

**描述**: 获取所有任务的当前状态

**响应**:

```json
{
  "status": "success",
  "data": {
    "total": 10,
    "running": 2,
    "pending": 3,
    "completed": 4,
    "failed": 1,
    "tasks": [
      {
        "id": 12345,
        "policy_id": 1,
        "status": "running",
        "start_time": "2026-01-14T10:00:00Z",
        "total_candidates": 1000,
        "total_probed": 500,
        "total_unregistered": 50
      }
    ]
  }
}
```

### 10. 获取单个任务详情

**端点**: `GET /api/v1/tasks/:id`

**描述**: 获取指定任务的详细信息

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | number | 任务 ID |

**示例**:

```bash
curl http://localhost:8080/api/v1/tasks/12345
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "task": {
      "id": 12345,
      "policy_id": 1,
      "status": "completed",
      "start_time": "2026-01-14T10:00:00Z",
      "end_time": "2026-01-14T10:15:00Z",
      "total_candidates": 1000,
      "total_probed": 950,
      "total_unregistered": 85,
      "stats_summary": {
        "fofa": {"success": 500, "failed": 0},
        "hunter": {"success": 450, "failed": 0}
      },
      "error_message": null
    }
  }
}
```

### 11. 取消任务

**端点**: `POST /api/v1/tasks/:id/cancel`

**描述**: 取消正在运行的任务

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | number | 任务 ID |

**响应**:

```json
{
  "status": "success",
  "data": {
    "task_id": 12345,
    "status": "cancelled",
    "message": "Task cancelled successfully"
  }
}
```

---

## 白名单相关接口

### 12. 列出白名单

**端点**: `GET /api/v1/whitelist`

**描述**: 获取所有白名单项

**查询参数**:

| 参数 | 类型 | 说明 | 示例 |
|------|------|------|------|
| type | string | 类型筛选 | domain |
| search | string | 搜索关键词 | gov |
| limit | number | 每页数量 | 50 |
| offset | number | 偏移量 | 0 |

**示例**:

```bash
curl "http://localhost:8080/api/v1/whitelist?type=domain&limit=20"
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "total": 10,
    "items": [
      {
        "id": 1,
        "type": "domain",
        "value": "gov.cn",
        "reason": "政府域名",
        "creator": "system",
        "created_at": "2026-01-14T08:00:00Z"
      }
    ]
  }
}
```

### 13. 添加白名单

**端点**: `POST /api/v1/whitelist`

**描述**: 添加新的白名单项

**请求体**:

```json
{
  "type": "domain",
  "value": "example.com",
  "reason": "测试域名",
  "creator": "admin"
}
```

**参数说明**:

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| type | string | 是 | domain/domain_exact/ip/cidr/url |
| value | string | 是 | 白名单值 |
| reason | string | 否 | 原因 |
| creator | string | 否 | 创建者 |

**响应**:

```json
{
  "status": "success",
  "data": {
    "id": 11,
    "message": "Whitelist added successfully"
  }
}
```

### 14. 删除白名单

**端点**: `DELETE /api/v1/whitelist/:id`

**描述**: 删除白名单项

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | number | 白名单 ID |

**示例**:

```bash
curl -X DELETE http://localhost:8080/api/v1/whitelist/11
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "id": 11,
    "message": "Whitelist deleted successfully"
  }
}
```

### 15. 批量导入白名单

**端点**: `POST /api/v1/whitelist/batch`

**描述**: 批量导入白名单

**请求体**:

```json
{
  "items": [
    {
      "type": "domain",
      "value": "example1.com",
      "reason": "测试1"
    },
    {
      "type": "domain",
      "value": "example2.com",
      "reason": "测试2"
    }
  ]
}
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "total": 2,
    "success": 2,
    "failed": 0,
    "message": "Batch import completed"
  }
}
```

---

## 策略相关接口

### 16. 列出策略

**端点**: `GET /api/v1/policies`

**描述**: 获取所有扫描策略

**响应**:

```json
{
  "status": "success",
  "data": {
    "policies": [
      {
        "id": 1,
        "name": "http_80",
        "uql": "country=\"CN\" && port=\"80\" && protocol=\"http\"",
        "engines": ["fofa", "hunter"],
        "page_size": 100,
        "max_records": 5000,
        "ports": [80],
        "enabled": true,
        "description": "HTTP 80端口扫描"
      }
    ]
  }
}
```

### 17. 创建策略

**端点**: `POST /api/v1/policies`

**描述**: 创建新的扫描策略

**请求体**:

```json
{
  "name": "custom_scan",
  "uql": "country=\"CN\" && port=\"8080\"",
  "engines": ["fofa", "hunter"],
  "page_size": 50,
  "max_records": 2000,
  "ports": [8080],
  "enabled": true,
  "description": "自定义8080端口扫描"
}
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "id": 3,
    "message": "Policy created successfully"
  }
}
```

### 18. 更新策略

**端点**: `PUT /api/v1/policies/:id`

**描述**: 更新指定策略

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | number | 策略 ID |

**请求体**: 同创建策略

**响应**:

```json
{
  "status": "success",
  "data": {
    "id": 3,
    "message": "Policy updated successfully"
  }
}
```

### 19. 删除策略

**端点**: `DELETE /api/v1/policies/:id`

**描述**: 删除指定策略

**响应**:

```json
{
  "status": "success",
  "data": {
    "id": 3,
    "message": "Policy deleted successfully"
  }
}
```

---

## 报告相关接口

### 20. 生成报告

**端点**: `POST /api/v1/report/generate`

**描述**: 生成统计报告

**请求体**:

```json
{
  "type": "daily",
  "date": "2026-01-14",
  "format": "html"
}
```

**参数说明**:

| 参数 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| type | string | 是 | - | daily/weekly/monthly/custom |
| date | string | 否 | 今天 | 日期 (YYYY-MM-DD) |
| from | string | 否 | - | 开始日期 (custom 类型) |
| to | string | 否 | - | 结束日期 (custom 类型) |
| format | string | 否 | json | json/html |

**响应**:

```json
{
  "status": "success",
  "data": {
    "report_id": "20260114_daily",
    "type": "daily",
    "date": "2026-01-14",
    "summary": {
      "total_scanned": 1234,
      "registered": 890,
      "unregistered": 234,
      "uncertain": 110
    },
    "download_url": "http://localhost:8080/reports/20260114_daily.html"
  }
}
```

### 21. 获取报告

**端点**: `GET /api/v1/report/:id`

**描述**: 获取已生成的报告

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| id | string | 报告 ID |

**示例**:

```bash
curl http://localhost:8080/api/v1/report/20260114_daily
```

### 22. 导出数据

**端点**: `GET /api/v1/export/:type`

**描述**: 导出数据为 CSV/JSON

**路径参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| type | string | unregistered/assets/checks |

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| format | string | csv/json |
| from | string | 开始日期 |
| to | string | 结束日期 |

**示例**:

```bash
curl "http://localhost:8080/api/v1/export/unregistered?format=csv&from=2026-01-01&to=2026-01-14"
```

---

## 通知相关接口

### 23. 测试通知

**端点**: `POST /api/v1/notification/test`

**描述**: 发送测试通知

**请求体**:

```json
{
  "channels": ["email", "webhook"],
  "message": "这是一条测试通知"
}
```

**响应**:

```json
{
  "status": "success",
  "data": {
    "email": {
      "status": "success",
      "message": "Email sent successfully"
    },
    "webhook": {
      "status": "success",
      "message": "Webhook sent successfully"
    }
  }
}
```

### 24. 获取通知历史

**端点**: `GET /api/v1/notification/history`

**描述**: 获取最近的通知记录

**查询参数**:

| 参数 | 类型 | 说明 |
|------|------|------|
| limit | number | 数量 (默认 20) |
| type | string | 类型筛选 (email/webhook) |

**响应**:

```json
{
  "status": "success",
  "data": {
    "total": 5,
    "items": [
      {
        "id": 1,
        "type": "email",
        "subject": "每日报告 - 2026-01-14",
        "status": "sent",
        "created_at": "2026-01-14T10:00:00Z"
      }
    ]
  }
}
```

---

## 系统相关接口

### 25. 健康检查

**端点**: `GET /api/v1/health`

**描述**: 检查系统健康状态

**响应**:

```json
{
  "status": "success",
  "data": {
    "timestamp": "2026-01-14T10:30:00Z",
    "uptime": "2h 30m",
    "services": {
      "database": "connected",
      "redis": "connected",
      "storage": "connected"
    },
    "stats": {
      "total_assets": 15000,
      "total_checks": 50000,
      "queue_length": 5
    }
  }
}
```

### 26. 系统信息

**端点**: `GET /api/v1/system/info`

**描述**: 获取系统信息

**响应**:

```json
{
  "status": "success",
  "data": {
    "version": "1.0.0",
    "environment": "production",
    "database": {
      "host": "mysql",
      "port": 3306,
      "name": "unimap_icp_hunter"
    },
    "redis": {
      "host": "redis",
      "port": 6379
    },
    "engines": {
      "fofa": {"enabled": true, "api_key_configured": true},
      "hunter": {"enabled": true, "api_key_configured": true}
    }
  }
}
```

### 27. 清理缓存

**端点**: `POST /api/v1/system/clear-cache`

**描述**: 清理系统缓存

**响应**:

```json
{
  "status": "success",
  "data": {
    "message": "Cache cleared successfully",
    "cleared_keys": 150
  }
}
```

---

## 错误处理示例

### 400 Bad Request

```json
{
  "status": "error",
  "code": "VALIDATION_ERROR",
  "message": "Query parameter 'days' must be a positive integer",
  "timestamp": "2026-01-14T10:30:00Z"
}
```

### 401 Unauthorized

```json
{
  "status": "error",
  "code": "AUTH_ERROR",
  "message": "Missing or invalid API token",
  "timestamp": "2026-01-14T10:30:00Z"
}
```

### 404 Not Found

```json
{
  "status": "error",
  "code": "NOT_FOUND",
  "message": "Task with ID 99999 not found",
  "timestamp": "2026-01-14T10:30:00Z"
}
```

### 429 Rate Limit

```json
{
  "status": "error",
  "code": "RATE_LIMIT",
  "message": "Too many requests, please try again later",
  "timestamp": "2026-01-14T10:30:00Z",
  "retry_after": 60
}
```

### 500 Internal Server Error

```json
{
  "status": "error",
  "code": "DB_ERROR",
  "message": "Database connection failed",
  "timestamp": "2026-01-14T10:30:00Z"
}
```

---

## 使用示例

### 完整工作流程示例

```bash
# 1. 验证 UQL
curl -X POST http://localhost:8080/api/v1/unimap/validate \
  -H "Content-Type: application/json" \
  -d '{"query": "country=\"CN\" && port=\"80\""}'

# 2. 查询资产
curl -X POST http://localhost:8080/api/v1/unimap/query \
  -H "Content-Type: application/json" \
  -d '{
    "query": "country=\"CN\" && port=\"80\"",
    "engines": ["fofa"],
    "limit": 50
  }'

# 3. 开始扫描
curl -X POST http://localhost:8080/api/v1/scan/start \
  -H "Content-Type: application/json" \
  -d '{"policy": "http_80"}'

# 4. 查看任务状态
curl http://localhost:8080/api/v1/tasks/status

# 5. 获取统计
curl http://localhost:8080/api/v1/stats/daily?days=7

# 6. 查看未备案列表
curl http://localhost:8080/api/v1/unregistered?days=7&limit=50

# 7. 生成报告
curl -X POST http://localhost:8080/api/v1/report/generate \
  -H "Content-Type: application/json" \
  -d '{"type": "daily", "date": "2026-01-14", "format": "html"}'
```

### Python 示例

```python
import requests
import json

BASE_URL = "http://localhost:8080/api/v1"

# 查询资产
def query_assets(query, engines=["fofa"], limit=100):
    response = requests.post(
        f"{BASE_URL}/unimap/query",
        json={
            "query": query,
            "engines": engines,
            "limit": limit
        }
    )
    return response.json()

# 获取统计
def get_stats(days=7):
    response = requests.get(
        f"{BASE_URL}/stats/daily",
        params={"days": days}
    )
    return response.json()

# 使用示例
result = query_assets('country="CN" && port="80"')
print(json.dumps(result, indent=2))

stats = get_stats(7)
print(json.dumps(stats, indent=2))
```

### JavaScript 示例

```javascript
const BASE_URL = "http://localhost:8080/api/v1";

// 查询资产
async function queryAssets(query, engines = ["fofa"], limit = 100) {
  const response = await fetch(`${BASE_URL}/unimap/query`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, engines, limit })
  });
  return response.json();
}

// 获取统计
async function getStats(days = 7) {
  const response = await fetch(`${BASE_URL}/stats/daily?days=${days}`);
  return response.json();
}

// 使用示例
queryAssets('country="CN" && port="80"').then(console.log);
getStats(7).then(console.log);
```

---

## 附录

### A. UQL 字段映射表

| UQL 字段 | FOFA 字段 | Hunter 字段 | ZoomEye 字段 | Quake 字段 |
|----------|-----------|-------------|--------------|------------|
| ip | ip | ip | ip | ip |
| port | port | port | port | port |
| protocol | protocol | protocol | service.name | service.name |
| country | country | country | country | country |
| region | region | region | - | - |
| city | city | city | - | - |
| title | title | title | title | title |
| body | body | body | - | - |
| server | server | server | http.server | http.server |
| host | domain | domain | - | - |
| url | url | url | - | - |
| asn | asn | asn | - | - |
| org | org | org | - | - |
| isp | isp | isp | - | - |

### B. 状态码映射

| 状态 | 数值 | 说明 |
|------|------|------|
| 已备案 | 1 | 成功匹配 ICP 备案号 |
| 未备案 | 0 | 明确检测到无备案 |
| 不确定 | 2 | 无法确定备案状态 |

### C. 优先级规则

结果合并时的优先级:
1. **字段优先级**: IP > Port > URL > Title > 其他
2. **引擎优先级**: FOFA > Hunter > ZoomEye > Quake
3. **时间优先级**: 最新结果覆盖旧结果

### D. 限流策略

| 引擎 | 限制 | 说明 |
|------|------|------|
| FOFA | 100 次/分钟 | 需要企业版才能提高 |
| Hunter | 100 次/分钟 | 需要企业版才能提高 |
| ZoomEye | 10000 次/天 | 需要 VIP |
| Quake | 10000 次/天 | 需要 VIP |

---

## 技术支持

如有问题或建议，请通过以下方式联系：

- **项目地址**: GitHub Repository
- **问题反馈**: Issue Tracker
- **文档更新**: 本文件

**版本**: v1.0.0
**最后更新**: 2026-01-14
**维护者**: Claude Code