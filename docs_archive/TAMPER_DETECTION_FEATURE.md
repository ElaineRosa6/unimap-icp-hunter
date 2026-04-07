# 网页分段Hash篡改检测功能

## 功能概述

该功能在批量截图的基础上，通过计算网页的分段hash来判断当前网页是否被篡改。支持对网页的不同部分（如head、body、scripts、styles等）进行独立的hash计算和比较。

## 核心模块

### 1. 篡改检测器 (internal/tamper/detector.go)

主要功能：
- **分段Hash计算**: 将网页分为多个语义段进行独立的hash计算
- **基线管理**: 支持保存、加载、删除基线hash
- **篡改检测**: 比较当前hash与基线hash，识别篡改的段落

### 2. 支持的分段类型

| 分段名称 | 描述 |
|---------|------|
| head | HTML head元素 |
| body | HTML body元素 |
| header | 页面header元素 |
| nav | 导航元素 |
| main | 主内容区域 |
| article | 文章内容 |
| section | 区块内容 |
| aside | 侧边栏 |
| footer | 页脚 |
| scripts | 所有脚本内容 |
| styles | 所有样式内容 |
| meta | 元数据标签 |
| links | 链接元素 |
| images | 图片元素 |
| forms | 表单元素 |
| full_content | 完整页面内容 |

### 3. Hash计算特点

- 使用SHA256算法
- 自动清理动态内容（如nonce、csrf token、data:image等）
- 对元素进行排序确保一致性
- 支持HTML5语义化标签

## API接口

### 1. 篡改检测接口

**POST** `/api/tamper/check`

请求体：
```json
{
    "urls": ["https://example.com", "https://example.org"],
    "concurrency": 5
}
```

响应：
```json
{
    "success": true,
    "results": [
        {
            "url": "https://example.com",
            "tampered": true,
            "tampered_segments": ["scripts", "footer"],
            "changes": [
                {
                    "segment": "scripts",
                    "old_hash": "abc123...",
                    "new_hash": "def456...",
                    "change_type": "modified",
                    "description": "Segment 'scripts' has been modified"
                }
            ],
            "current_hash": {
                "url": "https://example.com",
                "full_hash": "xyz789...",
                "segment_hashes": [...]
            },
            "baseline_hash": {...}
        }
    ]
}
```

### 2. 设置基线接口

**POST** `/api/tamper/baseline`

请求体：
```json
{
    "urls": ["https://example.com"],
    "concurrency": 5
}
```

### 3. 获取基线列表

**GET** `/api/tamper/baseline/list`

响应：
```json
{
    "success": true,
    "urls": ["https_example_com"],
    "count": 1
}
```

### 4. 删除基线

**POST** `/api/tamper/baseline/delete`

请求体：
```json
{
    "url": "https://example.com"
}
```

## 使用示例

### 1. 设置基线

```bash
curl -X POST http://localhost:8448/api/tamper/baseline \
  -H "Content-Type: application/json" \
  -d '{"urls": ["https://www.baidu.com"], "concurrency": 3}'
```

### 2. 检测篡改

```bash
curl -X POST http://localhost:8448/api/tamper/check \
  -H "Content-Type: application/json" \
  -d '{"urls": ["https://www.baidu.com"], "concurrency": 3}'
```

### 3. 查看基线列表

```bash
curl http://localhost:8448/api/tamper/baseline/list
```

## 数据存储

基线数据存储在 `./hash_store/` 目录下，每个URL对应一个JSON文件。

文件格式：
```json
{
    "url": "https://example.com",
    "title": "Example Domain",
    "full_hash": "abc123...",
    "segment_hashes": [
        {
            "name": "head",
            "hash": "def456...",
            "length": 1234,
            "elements": 1
        }
    ],
    "timestamp": 1234567890,
    "html_length": 5678,
    "status": "success"
}
```

## 篡改检测流程

1. **设置基线**: 首次访问目标URL，计算并保存各分段的hash作为基线
2. **定期检测**: 定期访问目标URL，计算当前hash
3. **比较分析**: 将当前hash与基线hash进行比较
4. **报告结果**: 返回篡改状态和具体篡改的分段信息

## 变更类型

| 类型 | 描述 |
|------|------|
| modified | 内容被修改 |
| structure_changed | 结构发生变化（元素数量改变） |
| added | 新增的分段 |
| removed | 被删除的分段 |

## 注意事项

1. 首次检测时如果没有基线，会返回 `tampered: false`，需要先设置基线
2. 动态内容（如时间戳、随机数）可能导致误报，系统会自动清理部分动态内容
3. 建议在网站稳定时设置基线，避免在更新期间设置
4. 并发数建议根据网络带宽和目标服务器承受能力设置

## 集成到批量截图

篡改检测功能已集成到批量截图功能中，可以在截图的同时进行篡改检测：

```go
// 带篡改检测的批量截图
results, err := manager.CaptureBatchURLsWithTamper(ctx, urls, batchID, concurrency, true, detector)
```

返回的结果中会包含 `TamperResult` 字段，包含篡改检测结果。
