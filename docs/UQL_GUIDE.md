# UniMap 统一查询语言 (UQL) 用户手册

UniMap 使用一套统一的查询语言 (UQL, Unified Query Language) 来屏蔽不同网络空间搜索引擎（FOFA, Hunter, ZoomEye, Quake）之间的语法差异。您只需编写一次 UQL 查询，系统会自动将其“翻译”为各引擎能理解的原生语法并并行执行。

## 1. 基础语法

UQL 的基本格式为 `key="value"`。

### 示例
```bash
# 查询使用了 nginx 的服务
app="nginx"

# 查询位于中国的服务
country="CN"

# 组合查询
app="nginx" && country="CN"
```

---

## 2. 支持的字段 (Fields)

以下字段在所有支持的引擎中均已通用化，您无需关心具体引擎的底层字段名。

| UQL 字段名 | 含义 | FOFA 映射 | Hunter 映射 | ZoomEye 映射 | Quake 映射 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **ip** | IP 地址 | ip | ip | ip | ip |
| **port** | 端口号 | port | port | port | port |
| **protocol** | 协议名称 | protocol | protocol | service | service |
| **app** | 应用/产品名称 | app | app.name | app | app |
| **title** | 网页标题 | title | web.title | title | title |
| **body** | 网页正文/响应体 | body | web.body | site | response |
| **header** | HTTP 响应头 | header | header | headers | headers |
| **server** | 服务器软件 (banner) | server | header_server | app | app |
| **status_code** | HTTP 状态码 | status_code | web.status_code | site | status_code |
| **domain** | 域名 | domain | domain | hostname | domain |
| **country** | 国家代码 (CN, US...) | country | ip.country | country | country |
| **region** | 省份/地区 | region | ip.province | subdivisions | province |
| **city** | 城市 | city | ip.city | city | city |
| **org** | 组织/机构 | org | ip.org | org | org |
| **isp** | 运营商 | isp | ip.isp | isp | isp |
| **os** | 操作系统 | os | os | os | os |
| **asn** | 自治系统号 | asn | ip.asn | asn | asn |

---

## 3. 操作符 (Operators)

### 3.1 比较操作符

| 操作符 | 说明 | 示例 |
| :--- | :--- | :--- |
| `=` 或 `==` | 等于/包含 (视引擎默认行为) | `port="80"` |
| `!=` 或 `<>` | 不等于/排除 | `country!="CN"` |
| `IN` | 包含于集合 (等同于多个 OR) | `port IN ["80", "8080", "443"]` |

### 3.2 逻辑操作符

| 操作符 | 说明 | 示例 |
| :--- | :--- | :--- |
| `AND` 或 `&&` | 逻辑与 (同时满足) | `title="login" && country="CN"` |
| `OR` 或 `||` | 逻辑或 (满足其一) | `app="redis" || port="6379"` |
| `()` | 优先级分组 | `(app="redis" || app="mysql") && country="CN"` |

> **注意**: 如果不使用括号，`AND` 的优先级默认通常高于 `OR`，但建议始终使用括号来明确复杂的逻辑关系。

---

## 4. 典型查询示例

### 查找 Web 服务
```bash
# 查找标题包含 "后台管理" 且状态码为 200 的网站
title="后台管理" && status_code="200"
```

### 查找特定漏洞资产
```bash
# 查找 WebLogic 服务 (常用端口)
app="WebLogic" OR port IN ["7001", "7002"]
```

### 查找特定区域的数据库
```bash
# 查找北京地区的 MySQL 数据库
protocol="mysql" && city="Beijing"
```

### 排除干扰项
```bash
# 查找 Nginx 服务，但不要显示 404 页面，且必须有标题
app="nginx" && status_code!="404" && title!=""
```

## 5. 常见问题 (FAQ)

**Q: 为什么有些字段查询没有结果？**
A: 不同引擎的数据覆盖度不同。例如，Hunter 对国内非 Web 资产覆盖较少，ZoomEye 对海外资产覆盖较好。建议勾选所有引擎以获取最全结果。

**Q: `country` 字段应该填什么？**
A: 请使用 ISO 3166-1 alpha-2 国家代码，例如：
- 中国: `CN`
- 美国: `US`
- 日本: `JP`
- 韩国: `KR`

**Q: 如何进行精确匹配？**
A: 目前 UQL 的 `=`通常映射为各引擎的默认搜索（通常是模糊或分词搜索）。如果引擎本身支持精确匹配语法（如 `ip="1.1.1.1"`），UQL 会直接传递。对于字符串字段（如 title），通常是包含匹配。
