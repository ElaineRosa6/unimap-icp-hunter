# UniMap 开发规范

## 1. 项目结构

### 1.1 目录结构

```text
cmd/                    # 应用入口
  unimap-cli/          # CLI 入口
  unimap-gui/          # GUI 入口
  unimap-web/          # Web 入口
internal/              # 内部包
  adapter/             # 引擎适配与编排
  core/unimap/         # UQL 解析与结果归并
  service/             # 统一服务层
  plugin/              # 插件与处理管道
  screenshot/          # 截图能力
  tamper/              # 网页篡改检测
  config/              # 配置管理
  exporter/            # 导出功能
  logger/              # 日志管理
  metrics/             # 指标监控
  model/               # 数据模型
  requestid/           # 请求ID管理
  util/                # 工具函数
  utils/               # 通用工具
web/                   # Web 相关代码
  server.go            # Web 服务与路由
  templates/           # 页面模板
  static/              # 前端静态资源
configs/               # 配置文件
```

### 1.2 包结构

- **cmd/**：应用入口，包含 CLI、GUI 和 Web 三种入口
- **internal/**：内部包，不对外暴露
- **web/**：Web 相关代码，包含服务、路由和前端资源
- **configs/**：配置文件，包含默认配置和示例配置

## 2. 代码规范

### 2.1 Go 代码规范

1. **代码风格**：
   - 遵循 Go 官方代码风格
   - 使用 `gofmt` 格式化代码
   - 使用 `go vet` 检查代码

2. **命名规范**：
   - 包名：小写，使用简短的单词
   - 函数名：驼峰命名法，首字母大写表示导出
   - 变量名：驼峰命名法，首字母小写表示非导出
   - 常量名：全大写，使用下划线分隔

3. **注释规范**：
   - 包注释：每个包应有简短的包级注释
   - 函数注释：每个导出函数应有注释
   - 关键代码注释：复杂逻辑应有注释

4. **错误处理**：
   - 使用 `errors` 包创建错误
   - 错误信息应清晰明了
   - 避免忽略错误

5. **并发处理**：
   - 使用 `sync` 包处理并发
   - 避免数据竞争
   - 使用 `context` 管理上下文

### 2.2 前端代码规范

1. **HTML 规范**：
   - 使用语义化标签
   - 缩进使用 4 个空格
   - 标签和属性使用小写

2. **CSS 规范**：
   - 使用类选择器
   - 命名使用 kebab-case
   - 样式应与系统整体风格保持一致

3. **JavaScript 规范**：
   - 使用 ES6+ 语法
   - 缩进使用 4 个空格
   - 变量声明使用 `const` 或 `let`
   - 函数使用箭头函数或函数声明

## 3. 开发流程

### 3.1 分支管理

- **main**：主分支，稳定版本
- **develop**：开发分支，集成新功能
- **feature/**：特性分支，开发新功能
- **bugfix/**：修复分支，修复 bug

### 3.2 提交规范

提交信息应包含：
- 类型：feat（新功能）、fix（修复）、docs（文档）、style（样式）、refactor（重构）、test（测试）、chore（构建）
- 范围：功能模块或文件
- 描述：简短的提交描述
- 详细信息：可选，详细的提交信息

示例：
```
feat(tamper): 添加历史记录导出功能

- 实现历史记录的JSON和Excel导出
- 添加时间范围选择功能
- 优化导出按钮样式
```

### 3.3 测试规范

- 单元测试：每个包应有相应的测试文件
- 集成测试：测试模块间的交互
- 端到端测试：测试完整的功能流程

## 4. 性能优化

### 4.1 Go 性能优化

1. **内存管理**：
   - 避免频繁分配内存
   - 使用对象池减少GC压力
   - 合理使用缓存（如5分钟内存缓存）

2. **并发优化**：
   - 使用 goroutines 并行处理
   - 使用 worker pool 控制并发数
   - 避免不必要的锁

3. **I/O 优化**：
   - 使用缓冲 I/O
   - 避免频繁文件操作
   - 使用批量操作

4. **篡改检测性能优化**：
   - 实现三种性能模式：Fast、Balanced、Comprehensive
   - 并行处理网页分段哈希计算
   - 正则表达式预编译优化
   - 内存缓存（5分钟有效期）

### 4.2 前端性能优化

1. **资源优化**：
   - 压缩 CSS 和 JavaScript
   - 优化图片资源
   - 使用 CDN 加速

2. **渲染优化**：
   - 减少 DOM 操作
   - 使用事件委托
   - 避免重排和重绘

3. **网络优化**：
   - 使用 HTTP/2
   - 减少 HTTP 请求
   - 使用缓存策略

4. **前端交互优化**：
   - 响应式布局设计
   - 时间范围选择器优化
   - 导出功能性能优化

## 5. 安全规范

### 5.1 Go 安全规范

1. **输入验证**：
   - 验证所有用户输入
   - 避免 SQL 注入
   - 避免 XSS 攻击

2. **认证授权**：
   - 使用安全的认证机制
   - 实现细粒度的授权
   - 保护敏感信息

3. **网络安全**：
   - 使用 HTTPS
   - 避免明文传输敏感数据
   - 实现 CORS 策略

### 5.2 前端安全规范

1. **输入验证**：
   - 前端验证用户输入
   - 避免执行恶意脚本
   - 保护用户数据

2. **API 调用**：
   - 使用安全的 API 调用
   - 避免暴露敏感信息
   - 实现 CSRF 保护

3. **Cookie 管理**：
   - 使用安全的 Cookie 设置
   - 避免存储敏感信息
   - 实现 Cookie 过期策略

## 6. 部署规范

### 6.1 构建流程

1. **构建命令**：
   - Web 服务：`go build -o output/unimap-web ./cmd/unimap-web`
   - CLI 工具：`go build -o output/unimap-cli ./cmd/unimap-cli`
   - GUI 工具：`go build -tags gui -o output/unimap-gui ./cmd/unimap-gui`

2. **构建环境**：
   - Go 1.24+
   - 依赖管理：`go mod tidy`

### 6.2 部署方式

1. **本地部署**：
   - 运行构建后的可执行文件
   - 配置 `configs/config.yaml`

2. **Docker 部署**：
   - 使用项目根目录的 `Dockerfile`
   - 运行 `docker-compose up -d`

3. **服务器部署**：
   - 复制构建后的文件到服务器
   - 使用 systemd 或 supervisor 管理服务

## 7. 开发工具

### 7.1 Go 工具

- **代码格式化**：`gofmt`
- **代码检查**：`go vet`
- **测试**：`go test`
- **依赖管理**：`go mod`

### 7.2 前端工具

- **HTML/CSS 编辑器**：VS Code, Sublime Text
- **JavaScript 调试**：Chrome DevTools
- **样式检查**：Stylelint

### 7.3 版本控制

- **Git**：版本控制
- **GitHub**：代码托管

## 8. 文档规范

### 8.1 文档结构

- **README.md**：项目概述
- **QUICKSTART.md**：快速启动指南
- **USAGE.md**：使用说明
- **DEVELOPMENT_GUIDE.md**：开发规范
- **API.md**：API 文档
- **TAMPER_DETECTION_OPTIMIZATION.md**：篡改检测优化文档

### 8.2 文档格式

- 使用 Markdown 格式
- 清晰的标题层级
- 代码块使用 ``` 包围
- 表格使用 Markdown 表格语法

## 9. 示例代码

### 9.1 Go 代码示例

```go
// 创建篡改检测实例
detector := tamper.NewDetector(tamper.DetectorConfig{
    BaseDir:         "./hash_store",
    DetectionMode:   tamper.DetectionModeRelaxed,
    PerformanceMode: tamper.PerformanceModeBalanced,
})

// 检测网站是否被篡改
result, err := detector.CheckTampering(ctx, "https://example.com")
if err != nil {
    log.Printf("检测失败: %v", err)
    return
}

// 批量检测多个网站
urls := []string{"https://example.com", "https://google.com"}
results, err := detector.BatchCheckTampering(ctx, urls, 5)
if err != nil {
    log.Printf("批量检测失败: %v", err)
    return
}

// 设置基线
baselineResults, err := detector.BatchSetBaseline(ctx, urls, 5)
if err != nil {
    log.Printf("设置基线失败: %v", err)
    return
}

// 加载检测记录
records, err := detector.LoadCheckRecords("https://example.com", 100)
if err != nil {
    log.Printf("加载记录失败: %v", err)
    return
}

// 获取检测统计信息
stats, err := detector.GetCheckStats("https://example.com")
if err != nil {
    log.Printf("获取统计信息失败: %v", err)
    return
}
```

### 9.2 前端代码示例

```javascript
// 导出历史记录
document.getElementById('btn-export-json').addEventListener('click', function() {
    const activeTab = document.querySelector('.tab.active').dataset.tab;
    if (activeTab === 'history') {
        if (historyRecords.length === 0) {
            alert('暂无历史记录可导出');
            return;
        }
        // 构建导出数据
        const exportData = historyRecords.map(record => {
            return {
                URL: record.url || '',
                Reachable: '是',
                Screenshot: '',
                Tampered: record.tampered ? '是' : '否',
                BaselineTime: record.baseline_timestamp ? formatTimestamp(record.baseline_timestamp) : '',
                CheckTime: record.timestamp ? formatTimestamp(record.timestamp) : ''
            };
        });
        // 创建下载链接
        const jsonString = JSON.stringify(exportData, null, 2);
        const blob = new Blob([jsonString], { type: 'application/json;charset=utf-8;' });
        const link = document.createElement('a');
        const url = URL.createObjectURL(blob);
        const fileName = activeTab === 'history' ? `tamper_history_${new Date().toISOString().slice(0, 10)}.json` : `tamper_detection_${new Date().toISOString().slice(0, 10)}.json`;
        link.setAttribute('href', url);
        link.setAttribute('download', fileName);
        link.style.visibility = 'hidden';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
    }
});

// 导出为Excel（CSV格式）
document.getElementById('btn-export-excel').addEventListener('click', function() {
    const activeTab = document.querySelector('.tab.active').dataset.tab;
    let exportData = [];

    if (activeTab === 'history') {
        // 导出历史记录
        if (historyRecords.length === 0) {
            alert('暂无历史记录可导出');
            return;
        }
        exportData = historyRecords;
    } else {
        // 导出当前检测结果
        if (currentTamperResults.length === 0) {
            alert('暂无检测结果可导出');
            return;
        }
        exportData = currentTamperResults;
    }

    // 构建CSV内容
    let csvContent = 'URL,可达性,网页截图,是否被篡改,基线时间,检测时间\n';

    exportData.forEach(item => {
        let url, reachable, screenshot, tampered, baselineTime, checkTime;

        if (activeTab === 'history') {
            // 处理历史记录
            url = item.url || '';
            reachable = '是'; // 历史记录默认视为可达
            screenshot = '';
            tampered = item.tampered ? '是' : '否';
            baselineTime = item.baseline_timestamp ? formatTimestamp(item.baseline_timestamp) : '';
            checkTime = item.timestamp ? formatTimestamp(item.timestamp) : '';
        } else {
            // 处理当前检测结果
            url = item.url || '';
            reachable = (item.status !== 'unreachable' && item.status !== 'invalid_format') ? '是' : '否';
            screenshot = '';
            tampered = item.tampered ? '是' : '否';
            baselineTime = item.baseline_hash?.timestamp ? formatTimestamp(item.baseline_hash.timestamp) : '';
            checkTime = item.timestamp ? formatTimestamp(item.timestamp) : '';
        }

        // 转义CSV特殊字符
        const escapeCSV = (value) => {
            if (value.includes(',') || value.includes('"') || value.includes('\n')) {
                return '"' + value.replace(/"/g, '""') + '"';
            }
            return value;
        };

        csvContent += `${escapeCSV(url)},${escapeCSV(reachable)},${escapeCSV(screenshot)},${escapeCSV(tampered)},${escapeCSV(baselineTime)},${escapeCSV(checkTime)}\n`;
    });

    // 创建下载链接
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    const url = URL.createObjectURL(blob);
    const fileName = activeTab === 'history' ? `tamper_history_${new Date().toISOString().slice(0, 10)}.csv` : `tamper_detection_${new Date().toISOString().slice(0, 10)}.csv`;
    link.setAttribute('href', url);
    link.setAttribute('download', fileName);
    link.style.visibility = 'hidden';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
});

// 时间范围过滤功能
function getFilteredHistoryRecords() {
    const urlFilter = historyUrlFilter.value;
    const typeFilter = historyTypeFilter.value;
    const modeFilter = historyModeFilter.value;
    const keyword = historyQueryInput.value.trim().toLowerCase();

    // 获取时间范围
    let startTime = null;
    let endTime = null;

    if (historyStartTimeInput.value) {
        startTime = new Date(historyStartTimeInput.value).getTime() / 1000;
    }

    if (historyEndTimeInput.value) {
        endTime = new Date(historyEndTimeInput.value).getTime() / 1000;
    }

    // 检查时间范围有效性
    if (startTime && endTime && startTime > endTime) {
        alert('开始时间不能晚于结束时间');
        return [];
    }

    return historyRecords.filter(record => {
        if (urlFilter && record.url !== urlFilter) {
            return false;
        }
        if (typeFilter) {
            const checkType = String(record.check_type || '').toLowerCase();
            const status = String(record.status || '').toLowerCase();
            if (checkType !== typeFilter && status !== typeFilter) {
                return false;
            }
        }
        if (modeFilter) {
            const mode = String(record.detection_mode || '').toLowerCase();
            if (mode !== modeFilter) {
                return false;
            }
        }
        if (keyword) {
            const haystack = [
                record.url,
                record.check_type,
                record.status,
                record.detection_mode,
                (record.tampered_segments || []).join(',')
            ].join(' ').toLowerCase();
            if (!haystack.includes(keyword)) {
                return false;
            }
        }
        // 时间范围过滤
        if (startTime && record.timestamp < startTime) {
            return false;
        }
        if (endTime && record.timestamp > endTime) {
            return false;
        }
        return true;
    });
}
```

## 10. 总结

本开发规范旨在确保 UniMap 项目的代码质量、性能和安全性，同时提供清晰的开发流程和文档规范。遵循这些规范将有助于提高开发效率，减少错误，确保项目的可维护性和可扩展性。

如有任何疑问或建议，请随时提出，以便不断完善本规范。