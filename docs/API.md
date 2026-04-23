# UniMap API 文档

## 1. 概述

UniMap 提供了丰富的 API 接口，支持查询、截图、篡改检测等功能。本文档详细描述了所有 API 接口的使用方法和参数说明。

## 2. 基础信息

### 2.1 基础 URL

所有 API 接口的基础 URL 为：`http://localhost:8448`

### 2.2 请求格式

- 大多数 API 接口使用 `POST` 方法，需要在请求体中传递 JSON 格式的数据
- 部分查询接口使用 `GET` 方法，通过 URL 参数传递参数

### 2.3 响应格式

所有 API 接口返回 JSON 格式的数据，包含以下字段：

```json
{
  "success": true,  // 是否成功
  "message": "操作成功",  // 提示信息
  "data": {},  // 数据
  "error": "错误信息"  // 错误信息（仅当 success 为 false 时存在）
}
```

## 3. API 接口

### 3.1 查询相关接口

#### 3.1.1 健康检查

- **接口**: `GET /health`
- **功能**: 检查服务是否正常运行
- **参数**: 无
- **返回**: 
  ```json
  {
    "status": "ok",
    "timestamp": 1679800000
  }
  ```

#### 3.1.2 指标监控

- **接口**: `GET /metrics`
- **功能**: 获取服务指标
- **参数**: 无
- **返回**: Prometheus 格式的指标数据

#### 3.1.3 页面查询

- **接口**: `GET /query`
- **功能**: 页面查询接口（用于前端页面）
- **参数**: 
  - `q`: 查询语句
  - `e`: 引擎列表（逗号分隔）
  - `l`: 限制数量
  - `offset`: 偏移量
- **返回**: 查询结果页面

#### 3.1.4 API 查询

- **接口**: `POST /api/query`
- **功能**: API 查询接口
- **参数**: 
  ```json
  {
    "query": "country=\"CN\" && port=\"80\"",
    "engines": ["fofa", "hunter"],
    "limit": 100,
    "offset": 0,
    "timeout": 30
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "results": [...],
      "total": 100,
      "engines": ["fofa", "hunter"]
    }
  }
  ```

#### 3.1.5 查询状态

- **接口**: `GET /api/query/status`
- **功能**: 获取查询状态
- **参数**: 
  - `task_id`: 任务 ID
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "status": "completed",
      "progress": 100,
      "results": [...]
    }
  }
  ```

### 3.2 Cookie 管理接口

#### 3.2.1 保存 Cookie

- **接口**: `POST /api/cookies`
- **功能**: 保存 Cookie
- **参数**: 
  ```json
  {
    "engine": "fofa",
    "cookies": "cookie1=value1; cookie2=value2"
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "message": "Cookie 保存成功"
  }
  ```

#### 3.2.2 验证 Cookie

- **接口**: `POST /api/cookies/verify`
- **功能**: 验证 Cookie 是否有效
- **参数**: 
  ```json
  {
    "engine": "fofa",
    "cookies": "cookie1=value1; cookie2=value2"
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "valid": true,
      "message": "Cookie 有效"
    }
  }
  ```

#### 3.2.3 导入 Cookie

- **接口**: `POST /api/cookies/import`
- **功能**: 导入 Cookie JSON
- **参数**: 
  ```json
  {
    "cookies": {
      "fofa": "cookie1=value1; cookie2=value2",
      "hunter": "cookie1=value1; cookie2=value2"
    }
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "message": "Cookie 导入成功"
  }
  ```

### 3.3 CDP 接口

#### 3.3.1 CDP 状态

- **接口**: `GET /api/cdp/status`
- **功能**: 获取 CDP 状态
- **参数**: 无
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "status": "connected",
      "version": "110.0.5481.77"
    }
  }
  ```

#### 3.3.2 CDP 连接

- **接口**: `POST /api/cdp/connect`
- **功能**: 连接 CDP
- **参数**: 
  ```json
  {
    "address": "localhost:9222"
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "message": "CDP 连接成功"
  }
  ```

### 3.4 WebSocket 接口

#### 3.4.1 WebSocket

- **接口**: `GET /api/ws`
- **功能**: WebSocket 连接
- **参数**: 无
- **返回**: WebSocket 连接

### 3.5 截图接口

#### 3.5.1 单页截图

- **接口**: `POST /api/screenshot`
- **功能**: 对单个页面进行截图
- **参数**: 
  ```json
  {
    "url": "https://example.com",
    "width": 1920,
    "height": 1080,
    "timeout": 30
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "file_path": "./screenshots/example.com.png"
    }
  }
  ```

#### 3.5.2 搜索引擎截图

- **接口**: `GET /api/screenshot/search-engine`
- **功能**: 对搜索引擎结果进行截图
- **参数**: 
  - `query`: 搜索查询
  - `engine`: 搜索引擎
  - `page`: 页码
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "file_path": "./screenshots/search_fofa_example.png"
    }
  }
  ```

#### 3.5.3 目标截图

- **接口**: `POST /api/screenshot/target`
- **功能**: 对目标进行截图
- **参数**: 
  ```json
  {
    "target": {
      "ip": "192.168.1.1",
      "port": "80",
      "protocol": "http"
    },
    "width": 1920,
    "height": 1080,
    "timeout": 30
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "file_path": "./screenshots/192.168.1.1_80.png"
    }
  }
  ```

#### 3.5.4 批量截图

- **接口**: `POST /api/screenshot/batch`
- **功能**: 批量截图
- **参数**: 
  ```json
  {
    "targets": [
      {
        "ip": "192.168.1.1",
        "port": "80",
        "protocol": "http"
      },
      {
        "ip": "192.168.1.2",
        "port": "443",
        "protocol": "https"
      }
    ],
    "width": 1920,
    "height": 1080,
    "concurrency": 5,
    "timeout": 30
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "batch_id": "batch_1234567890"
    }
  }
  ```

#### 3.5.5 批量 URL 截图

- **接口**: `POST /api/screenshot/batch-urls`
- **功能**: 批量 URL 截图
- **参数**: 
  ```json
  {
    "urls": ["https://example.com", "https://google.com"],
    "batch_id": "batch_1234567890",
    "width": 1920,
    "height": 1080,
    "concurrency": 5,
    "timeout": 30
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "batch_id": "batch_1234567890",
      "total": 2,
      "success": 2,
      "failed": 0
    }
  }
  ```

#### 3.5.6 获取截图批次

- **接口**: `GET /api/screenshot/batches`
- **功能**: 获取截图批次列表
- **参数**: 无
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "batches": [
        {
          "id": "batch_1234567890",
          "total": 2,
          "success": 2,
          "failed": 0,
          "timestamp": 1679800000
        }
      ]
    }
  }
  ```

#### 3.5.7 获取截图批次文件

- **接口**: `GET /api/screenshot/batches/files`
- **功能**: 获取截图批次文件列表
- **参数**: 
  - `batch_id`: 批次 ID
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "files": [
        {
          "path": "./screenshots/batch_1234567890/example.com.png",
          "url": "https://example.com",
          "status": "success"
        }
      ]
    }
  }
  ```

#### 3.5.8 删除截图批次

- **接口**: `DELETE /api/screenshot/batches/delete`
- **功能**: 删除截图批次
- **参数**: 
  - `batch_id`: 批次 ID
- **返回**: 
  ```json
  {
    "success": true,
    "message": "批次删除成功"
  }
  ```

#### 3.5.9 删除截图文件

- **接口**: `DELETE /api/screenshot/file/delete`
- **功能**: 删除截图文件
- **参数**: 
  - `file_path`: 文件路径
- **返回**: 
  ```json
  {
    "success": true,
    "message": "文件删除成功"
  }
  ```

#### 3.5.10 获取截图文件

- **接口**: `GET /screenshots/`
- **功能**: 获取截图文件
- **参数**: 文件路径（作为 URL 路径的一部分）
- **返回**: 图片文件

### 3.6 导入接口

#### 3.6.1 导入 URL

- **接口**: `POST /api/import/urls`
- **功能**: 导入 URL 列表
- **参数**: 
  ```json
  {
    "urls": ["https://example.com", "https://google.com"]
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "total": 2,
      "valid": 2,
      "invalid": 0
    }
  }
  ```

#### 3.6.2 URL 可达性检测

- **接口**: `POST /api/url/reachability`
- **功能**: 检测 URL 可达性
- **参数**: 
  ```json
  {
    "urls": ["https://example.com", "https://google.com"],
    "concurrency": 5
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "data": {
      "summary": {
        "total": 2,
        "reachable": 2,
        "unreachable": 0,
        "invalid_format": 0
      },
      "results": [
        {
          "url": "https://example.com",
          "status": "reachable",
          "reason": "HTTP 200"
        }
      ]
    }
  }
  ```

### 3.7 篡改检测接口

#### 3.7.1 篡改检测

- **接口**: `POST /api/tamper/check`
- **功能**: 检测网站是否被篡改
- **参数**: 
  ```json
  {
    "urls": ["https://example.com", "https://google.com"],
    "concurrency": 5,
    "mode": "relaxed"
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "mode": "relaxed",
    "summary": {
      "total": 2,
      "tampered": 0,
      "safe": 2,
      "noBaseline": 0,
      "unreachable": 0,
      "failed": 0
    },
    "results": [
      {
        "url": "https://example.com",
        "current_hash": {
          "url": "https://example.com",
          "title": "Example Domain",
          "full_hash": "abcdef123456",
          "segment_hashes": [...],
          "timestamp": 1679800000
        },
        "baseline_hash": {
          "url": "https://example.com",
          "title": "Example Domain",
          "full_hash": "abcdef123456",
          "segment_hashes": [...],
          "timestamp": 1679700000
        },
        "tampered": false,
        "status": "normal",
        "timestamp": 1679800000
      }
    ]
  }
  ```

#### 3.7.2 设置基线

- **接口**: `POST /api/tamper/baseline`
- **功能**: 为网站设置基线
- **参数**: 
  ```json
  {
    "urls": ["https://example.com", "https://google.com"],
    "concurrency": 5
  }
  ```
- **返回**: 
  ```json
  {
    "success": true,
    "summary": {
      "total": 2,
      "saved": 2,
      "unreachable": 0,
      "failed": 0
    },
    "results": [
      {
        "url": "https://example.com",
        "title": "Example Domain",
        "full_hash": "abcdef123456",
        "segment_hashes": [...],
        "timestamp": 1679800000,
        "status": "success"
      }
    ]
  }
  ```

#### 3.7.3 获取基线列表

- **接口**: `GET /api/tamper/baseline/list`
- **功能**: 获取基线列表
- **参数**: 无
- **返回**: 
  ```json
  {
    "success": true,
    "urls": ["https://example.com", "https://google.com"],
    "count": 2
  }
  ```

#### 3.7.4 删除基线

- **接口**: `DELETE /api/tamper/baseline/delete`
- **功能**: 删除基线
- **参数**: 
  - `url`: URL
- **返回**: 
  ```json
  {
    "success": true,
    "message": "基线删除成功",
    "url": "https://example.com"
  }
  ```

#### 3.7.5 获取历史记录

- **接口**: `GET /api/tamper/history`
- **功能**: 获取检测历史记录
- **参数**: 
  - `limit`: 限制数量
  - `url`: URL 过滤
  - `type`: 类型过滤
  - `mode`: 模式过滤
  - `q`: 搜索关键词
  - `start_time`: 开始时间（时间戳）
  - `end_time`: 结束时间（时间戳）
- **返回**: 
  ```json
  {
    "success": true,
    "count": 10,
    "records": [
      {
        "id": "1234567890",
        "url": "https://example.com",
        "check_type": "normal",
        "detection_mode": "relaxed",
        "status": "normal",
        "tampered": false,
        "tampered_segments": [],
        "changes_count": 0,
        "timestamp": 1679800000,
        "baseline_timestamp": 1679700000,
        "current_full_hash": "abcdef123456",
        "baseline_full_hash": "abcdef123456"
      }
    ],
    "urls": ["https://example.com", "https://google.com"]
  }
  ```

#### 3.7.6 删除历史记录

- **接口**: `DELETE /api/tamper/history/delete`
- **功能**: 删除历史记录
- **参数**: 
  - `url`: URL
- **返回**: 
  ```json
  {
    "success": true,
    "url": "https://example.com"
  }
  ```

## 4. 错误码

| 错误码 | 描述 |
|--------|------|
| 400 | 请求参数错误 |
| 401 | 未授权 |
| 403 | 禁止访问 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |
| 503 | 服务不可用 |

## 5. 限流策略

部分 API 接口受到限流保护，具体包括：

- 查询相关接口
- 截图相关接口
- 导入相关接口
- 篡改检测相关接口

限流策略：
- 每个 IP 每分钟最多 60 个请求
- 每个 API 接口有独立的限流配置

## 6. 安全注意事项

1. **认证授权**：部分 API 接口需要认证，请确保在请求中包含正确的认证信息
2. **输入验证**：所有用户输入都会经过验证，请勿尝试注入恶意代码
3. **HTTPS**：建议在生产环境中使用 HTTPS 协议
4. **CORS**：API 接口支持 CORS，但仅允许指定的域名访问

## 7. 示例代码

### 7.1 使用 cURL 调用 API

```bash
# 调用篡改检测接口
curl -X POST http://localhost:8448/api/tamper/check \
  -H "Content-Type: application/json" \
  -d '{"urls": ["https://example.com"], "concurrency": 5, "mode": "relaxed"}'

# 调用设置基线接口
curl -X POST http://localhost:8448/api/tamper/baseline \
  -H "Content-Type: application/json" \
  -d '{"urls": ["https://example.com"], "concurrency": 5}'

# 获取历史记录（带时间范围）
curl "http://localhost:8448/api/tamper/history?limit=100&start_time=1679700000&end_time=1679800000"

# 获取基线列表
curl http://localhost:8448/api/tamper/baseline/list

# 删除基线
curl -X DELETE "http://localhost:8448/api/tamper/baseline/delete?url=https://example.com"

# 删除历史记录
curl -X DELETE "http://localhost:8448/api/tamper/history/delete?url=https://example.com"
```

### 7.2 使用 Python 调用 API

```python
import requests
import json

# 调用篡改检测接口
url = "http://localhost:8448/api/tamper/check"
data = {
    "urls": ["https://example.com"],
    "concurrency": 5,
    "mode": "relaxed"
}
response = requests.post(url, json=data)
print(response.json())

# 调用设置基线接口
url = "http://localhost:8448/api/tamper/baseline"
data = {
    "urls": ["https://example.com"],
    "concurrency": 5
}
response = requests.post(url, json=data)
print(response.json())

# 获取历史记录（带时间范围）
url = "http://localhost:8448/api/tamper/history"
params = {
    "limit": 100,
    "start_time": 1679700000,
    "end_time": 1679800000
}
response = requests.get(url, params=params)
print(response.json())

# 获取基线列表
url = "http://localhost:8448/api/tamper/baseline/list"
response = requests.get(url)
print(response.json())

# 删除基线
url = "http://localhost:8448/api/tamper/baseline/delete"
params = {
    "url": "https://example.com"
}
response = requests.delete(url, params=params)
print(response.json())

# 删除历史记录
url = "http://localhost:8448/api/tamper/history/delete"
params = {
    "url": "https://example.com"
}
response = requests.delete(url, params=params)
print(response.json())
```

### 7.3 使用 JavaScript 调用 API

```javascript
// 调用篡改检测接口
fetch('http://localhost:8448/api/tamper/check', {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json'
    },
    body: JSON.stringify({
        urls: ['https://example.com'],
        concurrency: 5,
        mode: 'relaxed'
    })
})
.then(response => response.json())
.then(data => console.log(data));

// 调用设置基线接口
fetch('http://localhost:8448/api/tamper/baseline', {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json'
    },
    body: JSON.stringify({
        urls: ['https://example.com'],
        concurrency: 5
    })
})
.then(response => response.json())
.then(data => console.log(data));

// 获取历史记录（带时间范围）
const params = new URLSearchParams({
    limit: 100,
    start_time: 1679700000,
    end_time: 1679800000
});
fetch(`http://localhost:8448/api/tamper/history?${params.toString()}`)
.then(response => response.json())
.then(data => console.log(data));

// 获取基线列表
fetch('http://localhost:8448/api/tamper/baseline/list')
.then(response => response.json())
.then(data => console.log(data));

// 删除基线
fetch('http://localhost:8448/api/tamper/baseline/delete?url=https://example.com', {
    method: 'DELETE'
})
.then(response => response.json())
.then(data => console.log(data));

// 删除历史记录
fetch('http://localhost:8448/api/tamper/history/delete?url=https://example.com', {
    method: 'DELETE'
})
.then(response => response.json())
.then(data => console.log(data));
```

## 8. 总结

本文档详细描述了 UniMap 的 API 接口，包括查询、截图、篡改检测等功能。使用这些 API 接口，您可以：

1. 执行网络空间资产查询
2. 对网站进行截图
3. 检测网站是否被篡改
4. 管理检测历史记录
5. 导出检测结果

如果您有任何疑问或建议，请随时联系我们。