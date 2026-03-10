// 页面加载完成后执行
 document.addEventListener('DOMContentLoaded', function() {
	// 初始化WebSocket连接
	initWebSocket();
	
	// 初始化查询表单
	initQueryForm();
	
	// 初始化结果表格
	initResultsTable();
	
	// 初始化配额页面
	initQuotaPage();
});

// 初始化查询表单
function initQueryForm() {
	const form = document.querySelector('form[action="/query"]');
	if (!form) return;
	
	// 示例查询点击事件
	const examples = document.querySelectorAll('.example-item code');
	examples.forEach(example => {
		example.addEventListener('click', function() {
			const queryInput = document.getElementById('query');
			queryInput.value = this.textContent.trim();
			queryInput.focus();
		});
	});
	
	// 工具栏按钮事件
	const toolbarBtns = document.querySelectorAll('.toolbar-btn');
	toolbarBtns.forEach(btn => {
		btn.addEventListener('click', function() {
			const action = this.getAttribute('data-action');
			handleToolbarAction(action);
		});
	});
	
	// 保存查询按钮
	const saveQueryBtn = document.getElementById('btn-save-query');
	if (saveQueryBtn) {
		saveQueryBtn.addEventListener('click', function() {
			saveQuery();
		});
	}

	const saveCookiesBtn = document.getElementById('btn-save-cookies');
	if (saveCookiesBtn) {
		saveCookiesBtn.addEventListener('click', function() {
			saveCookies(saveCookiesBtn);
		});
	}

	const clearCookiesBtn = document.getElementById('btn-clear-cookies');
	if (clearCookiesBtn) {
		clearCookiesBtn.addEventListener('click', function() {
			clearCookies(clearCookiesBtn);
		});
	}

	const verifyCookiesBtn = document.getElementById('btn-verify-cookies');
	if (verifyCookiesBtn) {
		verifyCookiesBtn.addEventListener('click', function() {
			verifyCookies(verifyCookiesBtn);
		});
	}

	const importCookieBtn = document.getElementById('btn-import-cookie-json');
	if (importCookieBtn) {
		importCookieBtn.addEventListener('click', function() {
			importCookieJSON(importCookieBtn);
		});
	}

	initCookieStatus();
	initCDPControls();
	
	// 表单提交事件
	form.addEventListener('submit', function(e) {
		e.preventDefault(); // 阻止默认提交
		
		const query = document.getElementById('query').value;
		if (!query.trim()) {
			alert('请输入查询语句');
			return;
		}
		
		// 保存到查询历史
		saveQueryToHistory(query);
		
		// 显示加载状态
		const submitBtn = form.querySelector('button[type="submit"]');
		const originalText = submitBtn.textContent;
		submitBtn.textContent = '查询中...';
		submitBtn.disabled = true;
		submitBtn.classList.add('loading');
		
		// 获取选中的引擎
		const engines = [];
		const engineInputs = document.querySelectorAll('input[name="engines"]:checked');
		engineInputs.forEach(input => {
			engines.push(input.value);
		});
		
		if (engines.length === 0) {
			alert('请至少选择一个引擎');
			submitBtn.textContent = originalText;
			submitBtn.disabled = false;
			submitBtn.classList.remove('loading');
			return;
		}
		
		// 执行异步查询
	executeAsyncQuery(query, engines, submitBtn, originalText);
	});

	// 初始化引擎状态
	checkEngineStatus();
}

function initCDPControls() {
	const statusBadge = document.getElementById('cdp-status');
	const statusInfo = document.getElementById('cdp-status-info');
	const connectBtn = document.getElementById('btn-connect-cdp');

	if (!statusBadge && !statusInfo && !connectBtn) {
		return;
	}

	const refresh = function() {
		refreshCDPStatus(statusBadge, statusInfo);
	};

	refresh();
	setInterval(refresh, 15000);

	if (connectBtn) {
		connectBtn.addEventListener('click', function() {
			connectCDP(connectBtn, statusBadge, statusInfo);
		});
	}
}

function refreshCDPStatus(statusBadge, statusInfo) {
	fetch('/api/cdp/status')
		.then(resp => resp.json())
		.then(data => {
			const online = data && data.online;
			const url = data && data.url ? data.url : '';
			updateCDPBadge(statusBadge, online);
			if (statusInfo) {
				if (online) {
					statusInfo.textContent = url ? `在线: ${url}` : '在线';
				} else if (data && data.error) {
					statusInfo.textContent = data.error;
				} else {
					statusInfo.textContent = '未连接';
				}
			}
		})
		.catch(err => {
			console.error('CDP status error:', err);
			updateCDPBadge(statusBadge, false);
			if (statusInfo) {
				statusInfo.textContent = '检测失败';
			}
		});
}

function connectCDP(button, statusBadge, statusInfo) {
	const originalText = button.textContent;
	button.textContent = '连接中...';
	button.disabled = true;

	fetch('/api/cdp/connect', {
		method: 'POST'
	})
		.then(resp => resp.json())
		.then(data => {
			const online = data && data.online;
			const url = data && data.url ? data.url : '';
			updateCDPBadge(statusBadge, online);
			if (statusInfo) {
				if (online) {
					statusInfo.textContent = url ? `在线: ${url}` : '在线';
				} else if (data && data.error) {
					statusInfo.textContent = data.error;
				} else {
					statusInfo.textContent = '连接失败';
				}
			}
		})
		.catch(err => {
			console.error('CDP connect error:', err);
			updateCDPBadge(statusBadge, false);
			if (statusInfo) {
				statusInfo.textContent = '连接失败';
			}
		})
		.finally(() => {
			button.textContent = originalText;
			button.disabled = false;
		});
}

function updateCDPBadge(badge, online) {
	if (!badge) return;
	badge.textContent = online ? '在线' : '未连接';
	badge.classList.toggle('cookie-status--on', online);
	badge.classList.toggle('cookie-status--off', !online);
}

function importCookieJSON(button) {
	const engine = document.getElementById('cookie-json-engine').value;
	const jsonText = document.getElementById('cookie-json').value;
	if (!jsonText.trim()) {
		alert('请粘贴 Cookie JSON');
		return;
	}

	const formData = new FormData();
	formData.append('engine', engine);
	formData.append('cookie_json', jsonText);

	const originalText = button.textContent;
	button.textContent = '导入中...';
	button.disabled = true;

	fetch('/api/cookies/import', {
		method: 'POST',
		body: formData
	})
		.then(resp => resp.json())
		.then(data => {
			if (data && data.success) {
				const inputId = `cookie-${engine}`;
				const input = document.getElementById(inputId);
				if (input && data.cookieHeader) {
					input.value = data.cookieHeader;
				}
				initCookieStatus();
				alert('Cookie JSON 导入成功');
			} else {
				alert(data && data.error ? data.error : '导入失败');
			}
		})
		.catch(err => {
			console.error('Import cookie json error:', err);
			alert('导入失败');
		})
		.finally(() => {
			button.textContent = originalText;
			button.disabled = false;
		});
}

function verifyCookies(button) {
	const query = document.getElementById('query').value;
	if (!query.trim()) {
		alert('请先输入查询语句');
		return;
	}

	const formData = new FormData();
	formData.append('query', query);

	const engineInputs = document.querySelectorAll('input[name="engines"]:checked');
	engineInputs.forEach(input => {
		formData.append('engines', input.value);
	});

	const fofa = document.getElementById('cookie-fofa');
	const hunter = document.getElementById('cookie-hunter');
	const zoomeye = document.getElementById('cookie-zoomeye');
	const quake = document.getElementById('cookie-quake');
	if (fofa && fofa.value) formData.append('cookie_fofa', fofa.value);
	if (hunter && hunter.value) formData.append('cookie_hunter', hunter.value);
	if (zoomeye && zoomeye.value) formData.append('cookie_zoomeye', zoomeye.value);
	if (quake && quake.value) formData.append('cookie_quake', quake.value);

	const resultBox = document.getElementById('cookie-verify-result');
	if (resultBox) {
		resultBox.textContent = '正在验证 Cookie，请稍候...';
	}

	const originalText = button.textContent;
	button.textContent = '验证中...';
	button.disabled = true;

	fetch('/api/cookies/verify', {
		method: 'POST',
		body: formData
	})
		.then(resp => resp.json())
		.then(data => {
			if (!resultBox) return;
			if (data && data.results) {
				const items = [];
				Object.keys(data.results).forEach(engine => {
					const item = data.results[engine];
					const ok = item && item.ok;
					const hint = item && item.hint ? item.hint : '';
					const title = item && item.title ? item.title : '';
					const status = ok ? '<span class="ok">正常</span>' : '<span class="fail">异常</span>';
					items.push(`<div>${engine}: ${status}${title ? ' - ' + title : ''}${hint ? ' (' + hint + ')' : ''}</div>`);
				});
				resultBox.innerHTML = items.join('');
			} else if (data && data.error) {
				resultBox.textContent = data.error;
			} else {
				resultBox.textContent = '验证失败，请稍后重试。';
			}
		})
		.catch(err => {
			console.error('Verify cookies error:', err);
			if (resultBox) {
				resultBox.textContent = '验证失败，请稍后重试。';
			}
		})
		.finally(() => {
			button.textContent = originalText;
			button.disabled = false;
		});
}

function initCookieStatus() {
	const map = [
		{ input: 'cookie-fofa', badge: 'cookie-status-fofa' },
		{ input: 'cookie-hunter', badge: 'cookie-status-hunter' },
		{ input: 'cookie-zoomeye', badge: 'cookie-status-zoomeye' },
		{ input: 'cookie-quake', badge: 'cookie-status-quake' }
	];

	map.forEach(item => {
		const input = document.getElementById(item.input);
		const badge = document.getElementById(item.badge);
		if (!input || !badge) return;
		input.addEventListener('input', function() {
			updateCookieBadge(badge, input.value);
		});
		updateCookieBadge(badge, input.value);
	});
}

function updateCookieBadge(badge, value) {
	const hasValue = value && value.trim().length > 0;
	badge.textContent = hasValue ? '已配置' : '未配置';
	badge.classList.toggle('cookie-status--on', hasValue);
	badge.classList.toggle('cookie-status--off', !hasValue);
}

function clearCookies(button) {
	if (!confirm('确定要清空所有引擎 Cookie 吗？')) {
		return;
	}

	const fofa = document.getElementById('cookie-fofa');
	const hunter = document.getElementById('cookie-hunter');
	const zoomeye = document.getElementById('cookie-zoomeye');
	const quake = document.getElementById('cookie-quake');
	if (fofa) fofa.value = '';
	if (hunter) hunter.value = '';
	if (zoomeye) zoomeye.value = '';
	if (quake) quake.value = '';

	const formData = new FormData();
	formData.append('clear_cookies', 'true');

	const originalText = button.textContent;
	button.textContent = '清空中...';
	button.disabled = true;

	fetch('/api/cookies', {
		method: 'POST',
		body: formData
	})
		.then(resp => resp.json())
		.then(data => {
			if (data && data.success) {
				initCookieStatus();
				alert('Cookie 已清空');
			} else {
				alert('Cookie 清空失败');
			}
		})
		.catch(err => {
			console.error('Clear cookies error:', err);
			alert('Cookie 清空失败');
		})
		.finally(() => {
			button.textContent = originalText;
			button.disabled = false;
		});
}

function saveCookies(button) {
	const fofa = document.getElementById('cookie-fofa');
	const hunter = document.getElementById('cookie-hunter');
	const zoomeye = document.getElementById('cookie-zoomeye');
	const quake = document.getElementById('cookie-quake');

	const formData = new FormData();
	if (fofa && fofa.value) {
		formData.append('cookie_fofa', fofa.value);
	}
	if (hunter && hunter.value) {
		formData.append('cookie_hunter', hunter.value);
	}
	if (zoomeye && zoomeye.value) {
		formData.append('cookie_zoomeye', zoomeye.value);
	}
	if (quake && quake.value) {
		formData.append('cookie_quake', quake.value);
	}

	if ([...formData.keys()].length === 0) {
		alert('请先填写至少一个 Cookie');
		return;
	}

	const originalText = button.textContent;
	button.textContent = '保存中...';
	button.disabled = true;

	fetch('/api/cookies', {
		method: 'POST',
		body: formData
	})
		.then(resp => resp.json())
		.then(data => {
			if (data && data.success) {
				initCookieStatus();
				alert('Cookie 已保存到配置文件');
			} else {
				alert('Cookie 保存失败');
			}
		})
		.catch(err => {
			console.error('Save cookies error:', err);
			alert('Cookie 保存失败');
		})
		.finally(() => {
			button.textContent = originalText;
			button.disabled = false;
		});
}

// WebSocket连接管理
let wsConnection = null;
let wsConnected = false;
let currentQueryID = null;

// 初始化WebSocket连接
function initWebSocket() {
	// 关闭现有连接
	if (wsConnection) {
		wsConnection.close();
	}

	// 创建新连接
	const wsProtocol = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
	const wsUrl = wsProtocol + window.location.host + '/api/ws';
	wsConnection = new WebSocket(wsUrl);

	wsConnection.onopen = function() {
		console.log('WebSocket connected');
		wsConnected = true;
		// 发送ping消息保持连接
		startPingInterval();
	};

	wsConnection.onmessage = function(event) {
		const message = JSON.parse(event.data);
		handleWebSocketMessage(message);
	};

	wsConnection.onclose = function() {
		console.log('WebSocket disconnected');
		wsConnected = false;
		// 尝试重连
		setTimeout(initWebSocket, 5000);
	};

	wsConnection.onerror = function(error) {
		console.error('WebSocket error:', error);
	};
}

// 发送ping消息保持连接
function startPingInterval() {
	setInterval(() => {
		if (wsConnected && wsConnection.readyState === WebSocket.OPEN) {
			wsConnection.send(JSON.stringify({ type: 'ping' }));
		}
	}, 30000);
}

// 处理WebSocket消息
function handleWebSocketMessage(message) {
	switch (message.type) {
		case 'pong':
			// 心跳响应，无需处理
			break;
		case 'query_start':
			handleQueryStart(message);
			break;
		case 'progress_update':
			handleProgressUpdate(message);
			break;
		case 'query_complete':
			handleQueryComplete(message);
			break;
	}
}

// 处理查询开始
function handleQueryStart(message) {
	currentQueryID = message.query_id;
	const status = message.status;

	// 更新结果页面
	const resultsContent = document.getElementById('results-content');
	if (resultsContent) {
		resultsContent.innerHTML = `
			<div class="query-status">
				<h3>查询状态</h3>
				<p>查询ID: ${status.ID}</p>
				<p>状态: ${status.Status}</p>
				<p>进度: <span id="progress-bar">0%</span></p>
				<div class="progress-container">
					<div id="progress-fill" class="progress-fill" style="width: 0%"></div>
				</div>
				<p>开始时间: ${new Date(status.StartTime).toLocaleString()}</p>
			</div>
		`;
	}
}

// 处理进度更新
function handleProgressUpdate(message) {
	const progress = message.progress;
	const progressBar = document.getElementById('progress-bar');
	const progressFill = document.getElementById('progress-fill');

	if (progressBar) {
		progressBar.textContent = `${progress.toFixed(1)}%`;
	}

	if (progressFill) {
		progressFill.style.width = `${progress}%`;
	}
}

// 处理查询完成
function handleQueryComplete(message) {
	const results = message.results;

	// 恢复按钮状态
	const submitBtn = document.querySelector('button[type="submit"]');
	if (submitBtn) {
		submitBtn.textContent = '执行查询';
		submitBtn.disabled = false;
		submitBtn.classList.remove('loading');
	}

	// 显示结果
	if (results.error) {
		showResultsError(results.error);
	} else {
		showResults(results);
	}
}

// 执行异步查询（WebSocket版本）
function executeAsyncQuery(query, engines, submitBtn, originalText) {
	// 创建结果页面
	const resultsPage = document.createElement('div');
	resultsPage.className = 'results-page';
	resultsPage.innerHTML = `
		<div class="results-header">
			<h2>查询结果</h2>
			<p>查询语句: <code>${query}</code></p>
			<p>使用引擎: ${engines.join(', ')}</p>
			<div class="loading-indicator">
				<div class="spinner"></div>
				<p>正在查询...请稍候</p>
			</div>
		</div>
		<div id="results-content" class="results-content">
			<!-- 结果将在这里动态加载 -->
		</div>
	`;
	
	// 替换当前页面内容
	const main = document.querySelector('main');
	main.innerHTML = '';
	main.appendChild(resultsPage);

	// 检查WebSocket连接
	if (!wsConnected || wsConnection.readyState !== WebSocket.OPEN) {
		// WebSocket未连接，使用传统API
		useFallbackAPI(query, engines, submitBtn, originalText);
		return;
	}

	// 使用WebSocket执行查询
	wsConnection.send(JSON.stringify({
		type: 'query',
		query: query,
		engines: engines,
		page_size: 50
	}));
}

// 传统API回退方案
function useFallbackAPI(query, engines, submitBtn, originalText) {
	// 发送API请求
	fetch('/api/query', {
		method: 'POST',
		headers: {
			'Content-Type': 'application/x-www-form-urlencoded',
		},
		body: new URLSearchParams({
			'query': query,
			'engines': engines.join(','),
			'page_size': '50',
		}),
	})
	.then(response => response.json())
	.then(data => {
		// 恢复按钮状态
		if (submitBtn) {
			submitBtn.textContent = originalText;
			submitBtn.disabled = false;
			submitBtn.classList.remove('loading');
		}
		
		// 显示结果
		if (data.error) {
			showResultsError(data.error);
		} else {
			showResults(data);
		}
	})
	.catch(error => {
		// 恢复按钮状态
		if (submitBtn) {
			submitBtn.textContent = originalText;
			submitBtn.disabled = false;
			submitBtn.classList.remove('loading');
		}
		
		// 显示错误
		showResultsError('查询失败: ' + error.message);
	});
}

// 显示查询错误
function showResultsError(error) {
	const resultsContent = document.getElementById('results-content');
	if (resultsContent) {
		resultsContent.innerHTML = `
			<div class="error-message">
				<h3>查询错误</h3>
				<p>${error}</p>
				<button type="button" class="btn btn-primary" onclick="window.location.href='/'">返回首页</button>
			</div>
		`;
	}
}

// 显示查询结果
function showResults(data) {
	const resultsContent = document.getElementById('results-content');
	if (resultsContent) {
		// Normalize field names (WS vs HTTP) and asset shapes
		const assets = (data && (data.assets || data.Assets)) || [];
		const totalCount = (data && (data.totalCount ?? data.TotalCount)) ?? (Array.isArray(assets) ? assets.length : 0);
		const engineStats = data && (data.engineStats || data.EngineStats);
		const errors = (data && (data.errors || data.Errors)) || [];

		function pick(obj, ...keys) {
			if (!obj) return '';
			for (const key of keys) {
				if (obj[key] !== undefined && obj[key] !== null) return obj[key];
			}
			return '';
		}

		// 构建结果HTML
		let html = `
			<div class="results-info">
				<p>总结果数: ${totalCount}</p>
			</div>
		`;
		
		// 显示错误信息
		if (errors && errors.length > 0) {
			html += `
				<div class="errors">
					<h3>错误信息</h3>
					<ul>
						${errors.map(err => `<li>${err}</li>`).join('')}
					</ul>
				</div>
			`;
		}
		
		// 显示引擎统计
		if (engineStats) {
			html += `
				<div class="engine-stats">
					<h3>引擎统计</h3>
					<div class="stats-grid">
						${Object.entries(engineStats).map(([engine, count]) => `
							<div class="stat-item">
								<span class="engine-name">${engine}</span>
								<span class="count">${count}</span>
							</div>
						`).join('')}
					</div>
				</div>
			`;
		}
		
		// 显示结果表格
		if (Array.isArray(assets) && assets.length > 0) {
			html += `
				<div class="filter-bar" style="margin: 15px 0; padding: 10px; background: #f8f9fa; border-radius: 4px; display: flex; gap: 10px; flex-wrap: wrap; align-items: center;">
					<strong>结果筛选:</strong>
					<input type="text" id="filter-ip" placeholder="IP" class="form-control" style="width: 120px; padding: 5px;">
					<input type="text" id="filter-port" placeholder="端口" class="form-control" style="width: 80px; padding: 5px;">
					<input type="text" id="filter-protocol" placeholder="协议" class="form-control" style="width: 80px; padding: 5px;">
					<input type="text" id="filter-source" placeholder="来源" class="form-control" style="width: 100px; padding: 5px;">
					<button id="btn-apply-filter" class="btn btn-sm btn-primary">筛选</button>
					<button id="btn-reset-filter" class="btn btn-sm btn-secondary">重置</button>
					<span style="font-size: 0.9em; margin-left: auto;">
						显示: <span id="displayed-count">${assets.length}</span> / <span id="total-count">${assets.length}</span>
					</span>
				</div>

				<div class="results-table-container">
					<table class="results-table">
						<thead>
							<tr>
								<th>IP</th>
								<th>端口</th>
								<th>协议</th>
								<th>主机</th>
								<th>标题</th>
								<th>服务器</th>
								<th>状态码</th>
								<th>来源</th>
								<th style="min-width: 200px;">操作</th>
							</tr>
						</thead>
						<tbody id="results-body">
							${assets.map(asset => `
								<tr>
									<td>${pick(asset, 'ip', 'IP')}</td>
									<td>${pick(asset, 'port', 'Port')}</td>
									<td>${pick(asset, 'protocol', 'Protocol')}</td>
									<td>${pick(asset, 'host', 'Host')}</td>
									<td>${pick(asset, 'title', 'Title')}</td>
									<td>${pick(asset, 'server', 'Server')}</td>
									<td>${pick(asset, 'status_code', 'statusCode', 'StatusCode')}</td>
									<td>${pick(asset, 'source', 'Source')}</td>
									<td>
										<button type="button" class="btn btn-sm btn-info btn-detail" data-ip="${pick(asset, 'ip', 'IP')}" data-port="${pick(asset, 'port', 'Port')}">详情</button>
										<button type="button" class="btn btn-sm btn-success btn-copy" data-ip="${pick(asset, 'ip', 'IP')}">复制IP</button>
										<a href="${getEngineLink(pick(asset, 'source', 'Source'), pick(asset, 'ip', 'IP'))}" target="_blank" class="btn btn-sm btn-primary" style="text-decoration:none; color:white;">
											跳转
										</a>
										<button type="button" class="btn btn-sm btn-warning btn-screenshot" data-url="${pick(asset, 'url', 'URL')}" data-ip="${pick(asset, 'ip', 'IP')}" data-port="${pick(asset, 'port', 'Port')}" data-protocol="${pick(asset, 'protocol', 'Protocol')}">
											截图
										</button>
									</td>
								</tr>
							`).join('')}
						</tbody>
					</table>
				</div>
			`;
		} else {
			html += `
				<div class="no-results">
					<h3>未找到结果</h3>
					<p>当前查询条件下没有找到任何资产。</p>
				</div>
			`;
		}
		
		// 添加截图操作栏
		html += `
			<div class="screenshot-actions" style="margin: 15px 0; padding: 15px; background: #f0f8ff; border-radius: 4px; border: 1px solid #b0d4f1;">
				<h4 style="margin-bottom: 10px;">📸 截图功能</h4>
				<div style="display: flex; gap: 10px; flex-wrap: wrap; align-items: center;">
					<button type="button" id="btn-screenshot-all" class="btn btn-primary" onclick="captureAllScreenshots()">
						批量截图所有结果
					</button>
					<button type="button" id="btn-screenshot-search-engines" class="btn btn-info" onclick="captureSearchEngineScreenshots()">
						截图搜索引擎结果页
					</button>
					<span id="screenshot-status" style="margin-left: 10px; color: #666;"></span>
				</div>
				<div id="screenshot-progress" style="margin-top: 10px; display: none;">
					<div class="progress-container" style="width: 100%; height: 20px; background: #e9ecef; border-radius: 10px; overflow: hidden;">
						<div id="screenshot-progress-bar" style="width: 0%; height: 100%; background: #007bff; transition: width 0.3s;"></div>
					</div>
					<p id="screenshot-progress-text" style="margin-top: 5px; font-size: 0.9em; color: #666;"></p>
				</div>
			</div>
		`;

		// 添加返回按钮
		html += `
			<div class="results-footer">
				<button type="button" class="btn btn-secondary" onclick="window.location.href='/'">返回首页</button>
			</div>
		`;
		
		// 更新结果内容
		resultsContent.innerHTML = html;
		
		// 保存当前查询数据供截图使用
		window.currentQueryData = {
			query: data.query || '',
			engines: data.engines || [],
			assets: assets,
			queryID: 'query_' + Date.now()
		};
		
		// 初始化结果表格功能
		initResultsTable();
	}
}


// 处理工具栏操作
function handleToolbarAction(action) {
	const queryInput = document.getElementById('query');
	
	switch (action) {
		case 'clear':
			queryInput.value = '';
			queryInput.focus();
			break;
		case 'format':
			formatQuery(queryInput);
			break;
		case 'history':
			openQueryHistory();
			break;
	}
}

// 格式化查询语句
function formatQuery(input) {
	const query = input.value;
	if (!query.trim()) return;
	
	// 简单的格式化逻辑
	let formatted = query
		.replace(/&&/g, ' && ')  // 在&&前后添加空格
		.replace(/\|\|/g, ' || ')  // 在||前后添加空格
		.replace(/!=/g, ' != ')  // 在!=前后添加空格
		.replace(/=~/g, ' =~ ')  // 在=~前后添加空格
		.replace(/=/g, ' = ')  // 在=前后添加空格
		.replace(/\s+/g, ' ')  // 多个空格替换为单个空格
		.trim();
	
	input.value = formatted;
}

// 打开查询历史
function openQueryHistory() {
	const modal = document.getElementById('query-history');
	const historyList = document.getElementById('history-list');
	
	// 清空历史记录列表
	historyList.innerHTML = '';
	
	// 从本地存储获取历史记录
	const history = getQueryHistory();
	
	if (history.length === 0) {
		historyList.innerHTML = '<li class="no-history">无查询历史</li>';
	} else {
		history.forEach(item => {
			const li = document.createElement('li');
			li.innerHTML = `
				<code>${item.query}</code>
				<small>${new Date(item.timestamp).toLocaleString()}</small>
			`;
			li.addEventListener('click', function() {
				const queryInput = document.getElementById('query');
				queryInput.value = item.query;
				queryInput.focus();
				closeQueryHistory();
			});
			historyList.appendChild(li);
		});
	}
	
	// 显示模态框
	modal.style.display = 'block';
	
	// 关闭按钮事件
	const closeBtns = modal.querySelectorAll('.close-btn');
	closeBtns.forEach(btn => {
		btn.addEventListener('click', closeQueryHistory);
	});
	
	// 清空历史按钮
	const clearBtn = document.getElementById('btn-clear-history');
	if (clearBtn) {
		clearBtn.addEventListener('click', function() {
			if (confirm('确定要清空所有查询历史吗？')) {
				clearQueryHistory();
				historyList.innerHTML = '<li class="no-history">无查询历史</li>';
			}
		});
	}
}

// 关闭查询历史
function closeQueryHistory() {
	const modal = document.getElementById('query-history');
	modal.style.display = 'none';
}

// 保存查询到历史记录
function saveQueryToHistory(query) {
	if (!query.trim()) return;
	
	const history = getQueryHistory();
	
	// 检查是否已存在相同查询
	const existingIndex = history.findIndex(item => item.query === query);
	if (existingIndex !== -1) {
		// 移除旧记录
		history.splice(existingIndex, 1);
	}
	
	// 添加新记录到开头
	history.unshift({
		query: query,
		timestamp: Date.now()
	});
	
	// 限制历史记录数量
	const maxHistory = 20;
	if (history.length > maxHistory) {
		history.splice(maxHistory);
	}
	
	// 保存到本地存储
	localStorage.setItem('queryHistory', JSON.stringify(history));
}

// 获取查询历史
function getQueryHistory() {
	try {
		const history = localStorage.getItem('queryHistory');
		return history ? JSON.parse(history) : [];
	} catch (e) {
		console.error('获取查询历史失败:', e);
		return [];
	}
}

// 清空查询历史
function clearQueryHistory() {
	localStorage.removeItem('queryHistory');
}

// 保存查询
function saveQuery() {
	const queryInput = document.getElementById('query');
	const query = queryInput.value;
	
	if (!query.trim()) {
		alert('请输入查询语句');
		return;
	}
	
	// 保存到本地存储
	const savedQueries = getSavedQueries();
	const queryName = prompt('请输入查询名称:');
	
	if (queryName) {
		savedQueries.push({
			name: queryName,
			query: query,
			timestamp: Date.now()
		});
		
		localStorage.setItem('savedQueries', JSON.stringify(savedQueries));
		showMessage('查询保存成功', 'success');
	}
}

// 获取保存的查询
function getSavedQueries() {
	try {
		const saved = localStorage.getItem('savedQueries');
		return saved ? JSON.parse(saved) : [];
	} catch (e) {
		console.error('获取保存的查询失败:', e);
		return [];
	}
}

// 检查引擎状态
function checkEngineStatus() {
	// 这里可以添加实际的引擎状态检查逻辑
	// 例如：通过API检查引擎是否可用，API密钥是否有效等
	
	// 模拟引擎状态检查
	const engines = ['fofa', 'hunter', 'zoomeye', 'quake'];
	engines.forEach(engine => {
		const statusElement = document.getElementById(`status-${engine}`);
		if (statusElement) {
			// 随机模拟状态（实际应用中应该通过API检查）
			const isAvailable = Math.random() > 0.1; // 90%的概率可用
			if (isAvailable) {
				statusElement.textContent = '✓';
				statusElement.style.color = '#27ae60';
			} else {
				statusElement.textContent = '✗';
				statusElement.style.color = '#e74c3c';
			}
		}
	});
}

// 初始化结果表格
function initResultsTable() {
	const table = document.querySelector('.results-table');
	if (!table) return;
	
	// 表格排序功能
	const headers = table.querySelectorAll('th');
	headers.forEach(header => {
		header.addEventListener('click', function() {
			const column = this.cellIndex;
			const rows = Array.from(table.querySelectorAll('tbody tr'));
			
			// 排序方向
			const isAscending = this.classList.contains('sort-asc');
			this.classList.toggle('sort-asc', !isAscending);
			this.classList.toggle('sort-desc', isAscending);
			
			// 排序
			rows.sort((a, b) => {
				const aValue = a.cells[column].textContent.trim();
				const bValue = b.cells[column].textContent.trim();
				
				// 数字比较
				if (!isNaN(aValue) && !isNaN(bValue)) {
					return isAscending ? parseFloat(aValue) - parseFloat(bValue) : parseFloat(bValue) - parseFloat(aValue);
				}
				
				// 字符串比较
				return isAscending ? aValue.localeCompare(bValue) : bValue.localeCompare(aValue);
			});
			
			// 重新插入行
			const tbody = table.querySelector('tbody');
			tbody.innerHTML = '';
			rows.forEach(row => tbody.appendChild(row));
		});
	});
	
	// 初始化筛选功能
	initFilterOptions();
	
	// 初始化分页功能
	initPagination();
	
	// 初始化资产详情功能
	initAssetDetail();
	
	// 初始化导出功能
	initExportButtons();
}

// 初始化导出按钮
function initExportButtons() {
	const csvBtn = document.getElementById('btn-export-csv');
	const jsonBtn = document.getElementById('btn-export-json');
	const excelBtn = document.getElementById('btn-export-excel');
	
	if (csvBtn) {
		csvBtn.addEventListener('click', function() {
			exportToCSV();
		});
	}
	
	if (jsonBtn) {
		jsonBtn.addEventListener('click', function() {
			exportToJSON();
		});
	}
	
	if (excelBtn) {
		excelBtn.addEventListener('click', function() {
			exportToExcel();
		});
	}
}

// 导出为CSV
function exportToCSV() {
	const table = document.querySelector('.results-table');
	if (!table) return;
	
	// 获取表头
	const headers = table.querySelectorAll('thead th');
	headerNames = Array.from(headers).map(header => header.textContent.trim());
	
	// 获取显示的行
	const rows = table.querySelectorAll('tbody tr');
	displayedRows = Array.from(rows).filter(row => row.style.display !== 'none');
	
	if (displayedRows.length === 0) {
		showMessage('没有可导出的结果', 'warning');
		return;
	}
	
	// 构建CSV内容
	let csvContent = headerNames.join(',') + '\n';
	
	displayedRows.forEach(row => {
		const cells = row.querySelectorAll('td');
		const rowData = Array.from(cells).map(cell => {
			const value = cell.textContent.trim();
			// 处理包含逗号的内容
			if (value.includes(',')) {
				return `"${value.replace(/"/g, '""')}"`;
			}
			return value;
		});
		csvContent += rowData.join(',') + '\n';
	});
	
	// 创建下载链接
	const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
	const link = document.createElement('a');
	const url = URL.createObjectURL(blob);
	link.setAttribute('href', url);
	link.setAttribute('download', `unimap_results_${new Date().toISOString().slice(0, 10)}.csv`);
	link.style.visibility = 'hidden';
	document.body.appendChild(link);
	link.click();
	document.body.removeChild(link);
	
	showMessage('CSV导出成功', 'success');
}

// 导出为JSON
function exportToJSON() {
	const table = document.querySelector('.results-table');
	if (!table) return;
	
	// 获取表头
	const headers = table.querySelectorAll('thead th');
	headerNames = Array.from(headers).map(header => header.textContent.trim());
	
	// 获取显示的行
	const rows = table.querySelectorAll('tbody tr');
	displayedRows = Array.from(rows).filter(row => row.style.display !== 'none');
	
	if (displayedRows.length === 0) {
		showMessage('没有可导出的结果', 'warning');
		return;
	}
	
	// 构建JSON数据
	const jsonData = displayedRows.map(row => {
		const cells = row.querySelectorAll('td');
		const rowObject = {};
		Array.from(cells).forEach((cell, index) => {
			if (index < headerNames.length) {
				rowObject[headerNames[index]] = cell.textContent.trim();
			}
		});
		return rowObject;
	});
	
	// 构建完整的JSON对象
	const exportData = {
		timestamp: new Date().toISOString(),
		resultCount: jsonData.length,
		data: jsonData
	};
	
	// 创建下载链接
	const jsonString = JSON.stringify(exportData, null, 2);
	const blob = new Blob([jsonString], { type: 'application/json;charset=utf-8;' });
	const link = document.createElement('a');
	const url = URL.createObjectURL(blob);
	link.setAttribute('href', url);
	link.setAttribute('download', `unimap_results_${new Date().toISOString().slice(0, 10)}.json`);
	link.style.visibility = 'hidden';
	document.body.appendChild(link);
	link.click();
	document.body.removeChild(link);
	
	showMessage('JSON导出成功', 'success');
}

// 导出为Excel（实际上是CSV）
function exportToExcel() {
	// 显示提示信息
	showMessage('Excel导出功能使用CSV格式，正在导出...', 'info');
	
	// 延迟执行CSV导出
	setTimeout(() => {
		exportToCSV();
	}, 500);
}

// 初始化筛选选项
function initFilterOptions() {
	const applyFilterBtn = document.getElementById('btn-apply-filter');
	const resetFilterBtn = document.getElementById('btn-reset-filter');
	
	if (applyFilterBtn) {
		applyFilterBtn.addEventListener('click', applyFilters);
	}
	
	if (resetFilterBtn) {
		resetFilterBtn.addEventListener('click', resetFilters);
	}
}

// 应用筛选
function applyFilters() {
	const ipFilter = document.getElementById('filter-ip').value.toLowerCase();
	const portFilter = document.getElementById('filter-port').value.toLowerCase();
	const protocolFilter = document.getElementById('filter-protocol').value;
	const sourceFilter = document.getElementById('filter-source').value;
	
	const rows = document.querySelectorAll('#results-body tr');
	let displayedCount = 0;
	
	rows.forEach(row => {
		const cells = row.cells;
		const ip = cells[0].textContent.toLowerCase();
		const port = cells[1].textContent.toLowerCase();
		const protocol = cells[2].textContent.toLowerCase();
		const source = cells[7].textContent.toLowerCase();
		
		// 检查筛选条件
		const ipMatch = !ipFilter || ip.includes(ipFilter);
		const portMatch = !portFilter || port.includes(portFilter);
		const protocolMatch = !protocolFilter || protocol.includes(protocolFilter);
		const sourceMatch = !sourceFilter || source.includes(sourceFilter);
		
		if (ipMatch && portMatch && protocolMatch && sourceMatch) {
			row.style.display = '';
			displayedCount++;
		} else {
			row.style.display = 'none';
		}
	});
	
	// 更新显示计数
	const displayedCountElement = document.getElementById('displayed-count');
	if (displayedCountElement) {
		displayedCountElement.textContent = displayedCount;
	}
}

// 重置筛选
function resetFilters() {
	document.getElementById('filter-ip').value = '';
	document.getElementById('filter-port').value = '';
	document.getElementById('filter-protocol').value = '';
	document.getElementById('filter-source').value = '';
	
	// 显示所有行
	const rows = document.querySelectorAll('#results-body tr');
	rows.forEach(row => {
		row.style.display = '';
	});
	
	// 更新显示计数
	const displayedCountElement = document.getElementById('displayed-count');
	const totalCountElement = document.getElementById('total-count');
	if (displayedCountElement && totalCountElement) {
		displayedCountElement.textContent = totalCountElement.textContent;
	}
}

// 初始化分页功能
function initPagination() {
	const prevBtn = document.getElementById('btn-prev-page');
	const nextBtn = document.getElementById('btn-next-page');
	const pageSizeSelect = document.getElementById('page-size');
	
	if (prevBtn) {
		prevBtn.addEventListener('click', function() {
			// 这里可以添加实际的分页逻辑
			// 例如：通过API获取上一页数据
			showMessage('上一页功能开发中', 'info');
		});
	}
	
	if (nextBtn) {
		nextBtn.addEventListener('click', function() {
			// 这里可以添加实际的分页逻辑
			// 例如：通过API获取下一页数据
			showMessage('下一页功能开发中', 'info');
		});
	}
	
	if (pageSizeSelect) {
		pageSizeSelect.addEventListener('change', function() {
			// 这里可以添加实际的分页逻辑
			// 例如：通过API获取指定页大小的数据
			showMessage('每页显示条数功能开发中', 'info');
		});
	}
}

// 初始化资产详情功能
function initAssetDetail() {
	// 详情按钮 - 使用特定类名选择
	const detailBtns = document.querySelectorAll('.btn-detail');
	detailBtns.forEach(btn => {
		btn.addEventListener('click', function() {
			const ip = this.getAttribute('data-ip');
			const port = this.getAttribute('data-port');
			showAssetDetail(ip, port);
		});
	});
	
	// 复制IP按钮 - 使用特定类名选择
	const copyIpBtns = document.querySelectorAll('.btn-copy');
	copyIpBtns.forEach(btn => {
		btn.addEventListener('click', function() {
			const ip = this.getAttribute('data-ip');
			if (!ip) return;
			
			copyToClipboard(ip)
				.then(() => {
					showMessage('IP地址已复制到剪贴板', 'success');
				})
				.catch(err => {
					console.error('复制失败:', err);
					// Fallback for older browsers or http context
					fallbackCopy(ip);
				});
		});
	});
	
	// 截图按钮
	const screenshotBtns = document.querySelectorAll('.btn-screenshot');
	screenshotBtns.forEach(btn => {
		btn.addEventListener('click', function() {
			const url = this.getAttribute('data-url');
			const ip = this.getAttribute('data-ip');
			const port = this.getAttribute('data-port');
			const proto = this.getAttribute('data-protocol');
			viewScreenshot(url, ip, port, proto);
		});
	});
}

function fallbackCopy(text) {
	const textArea = document.createElement("textarea");
	textArea.value = text;
	document.body.appendChild(textArea);
	textArea.focus();
	textArea.select();
	try {
		const successful = document.execCommand('copy');
		if (successful) {
			showMessage('IP地址已复制到剪贴板', 'success');
		} else {
			showMessage('复制失败，请手动复制', 'error');
		}
	} catch (err) {
		showMessage('复制失败', 'error');
	}
	document.body.removeChild(textArea);
}

// 显示资产详情
function showAssetDetail(ip, port) {
	const modal = document.getElementById('asset-detail');
	const content = document.getElementById('asset-detail-content');
	
	// 显示加载状态
	content.innerHTML = '<div class="loading">加载中...</div>';
	modal.style.display = 'block';
	
	// 模拟资产详情数据
	setTimeout(() => {
		content.innerHTML = `
			<div class="asset-detail-item">
				<span class="asset-detail-label">IP地址：</span>
				<span class="asset-detail-value"><code>${ip}</code></span>
			</div>
			<div class="asset-detail-item">
				<span class="asset-detail-label">端口：</span>
				<span class="asset-detail-value">${port}</span>
			</div>
			<div class="asset-detail-item">
				<span class="asset-detail-label">地理位置：</span>
				<span class="asset-detail-value">中国 北京</span>
			</div>
			<div class="asset-detail-item">
				<span class="asset-detail-label">运营商：</span>
				<span class="asset-detail-value">中国联通</span>
			</div>
			<div class="asset-detail-item">
				<span class="asset-detail-label">ASN：</span>
				<span class="asset-detail-value">AS4808</span>
			</div>
			<div class="asset-detail-item">
				<span class="asset-detail-label">风险等级：</span>
				<span class="asset-detail-value">低</span>
			</div>
		`;
	}, 500);
	
	// 关闭按钮事件
	const closeBtns = modal.querySelectorAll('.close-btn');
	closeBtns.forEach(btn => {
		btn.addEventListener('click', function() {
			modal.style.display = 'none';
		});
	});
	
	// 点击模态框外部关闭
	window.addEventListener('click', function(e) {
		if (e.target === modal) {
			modal.style.display = 'none';
		}
	});
}

// 初始化配额页面
function initQuotaPage() {
	const quotaGrid = document.querySelector('.quota-grid');
	if (!quotaGrid) return;

	// Set per-engine status based on rendered content
	quotaGrid.querySelectorAll('.quota-item').forEach(item => {
		const status = item.querySelector('.quota-status');
		const errText = (item.querySelector('.quota-error')?.textContent || '').trim();
		const hasDetails = !!item.querySelector('.quota-details');
		if (!status) return;
		if (errText) {
			status.textContent = '异常';
			status.classList.add('error');
		} else if (hasDetails) {
			status.textContent = '正常';
		} else {
			status.textContent = '未知';
			status.classList.add('warning');
		}
	});
	
	// 刷新配额按钮
	const refreshBtn = document.getElementById('btn-refresh-quota');
	if (refreshBtn) {
		refreshBtn.addEventListener('click', function() {
			// 显示加载状态
			const originalText = this.textContent;
			this.textContent = '刷新中...';
			this.disabled = true;
			
			// 模拟刷新
			setTimeout(() => {
				// 重新加载页面
				location.reload();
			}, 1000);
		});
	}
	
	// 导出配额按钮
	const exportBtn = document.getElementById('btn-export-quota');
	if (exportBtn) {
		exportBtn.addEventListener('click', function() {
			exportQuota();
		});
	}
	
	// 配额设置按钮
	const settingsBtn = document.getElementById('btn-quota-settings');
	if (settingsBtn) {
		settingsBtn.addEventListener('click', function() {
			openQuotaSettings();
		});
	}
	
	// 初始化配额概览
	initQuotaOverview();
	
	// 初始化配额趋势图
	initQuotaTrend();
	
	// 初始化配额预警
	initQuotaAlert();
}

// 初始化配额概览
function initQuotaOverview() {
	const quotaItems = document.querySelectorAll('.quota-item');
	let totalRemaining = 0;
	let totalUsed = 0;
	let totalQuota = 0;

	const parseNumber = (text) => {
		if (!text) return 0;
		const cleaned = String(text).replace(/,/g, '').match(/\d+(?:\.\d+)?/);
		return cleaned ? parseFloat(cleaned[0]) : 0;
	};

	quotaItems.forEach(item => {
		let remaining = 0;
		let used = 0;
		let quota = 0;

		const rows = item.querySelectorAll('.quota-row');
		rows.forEach(row => {
			const label = (row.querySelector('.label')?.textContent || '').trim();
			const value = (row.querySelector('.value')?.textContent || '').trim();
			if (label.includes('剩余配额')) remaining = parseNumber(value);
			else if (label.includes('已用配额')) used = parseNumber(value);
			else if (label.includes('总配额')) quota = parseNumber(value);
		});

		totalRemaining += remaining;
		totalUsed += used;
		totalQuota += quota;
	});
	
	// 更新概览数据
	const totalRemainingElement = document.getElementById('total-remaining');
	const totalUsedElement = document.getElementById('total-used');
	const totalQuotaElement = document.getElementById('total-quota');
	const totalUsageRateElement = document.getElementById('total-usage-rate');
	
	if (totalRemainingElement) totalRemainingElement.textContent = Math.round(totalRemaining);
	if (totalUsedElement) totalUsedElement.textContent = Math.round(totalUsed);
	if (totalQuotaElement) totalQuotaElement.textContent = Math.round(totalQuota);
	if (totalUsageRateElement && totalQuota > 0) {
		totalUsageRateElement.textContent = `${((totalUsed / totalQuota) * 100).toFixed(1)}%`;
	}
}

// 初始化配额趋势图
function initQuotaTrend() {
	const chartContainer = document.getElementById('quota-trend-chart');
	if (!chartContainer) return;

	chartContainer.innerHTML = '<div class="chart-placeholder">配额趋势图暂未实现（当前页面展示的是实时配额数据）</div>';
}

// 初始化配额预警
function initQuotaAlert() {
	const alertCheckboxes = document.querySelectorAll('input[name="quota-alert"]');
	alertCheckboxes.forEach(checkbox => {
		checkbox.addEventListener('change', function() {
			const thresholdInput = this.parentElement.parentElement.querySelector('input[name="alert-threshold"]');
			if (thresholdInput) {
				thresholdInput.disabled = !this.checked;
			}
		});
	});
	
	// 检查使用率，更新状态
	const quotaItems = document.querySelectorAll('.quota-item');
	quotaItems.forEach(item => {
		const usageRateElement = item.querySelector('.quota-row:nth-child(4) .value');
		const statusElement = item.querySelector('.quota-status');
		const progressElement = item.querySelector('.quota-progress');
		
		if (usageRateElement && statusElement && progressElement) {
			const usageRateText = usageRateElement.textContent;
			const usageRate = parseInt(usageRateText);
			
			if (usageRate >= 90) {
				statusElement.textContent = '紧急';
				statusElement.className = 'quota-status error';
				progressElement.className = 'quota-progress error';
			} else if (usageRate >= 70) {
				statusElement.textContent = '警告';
				statusElement.className = 'quota-status warning';
				progressElement.className = 'quota-progress warning';
			}
		}
	});
}

// 导出配额
function exportQuota() {
	// 模拟导出功能
	showMessage('配额数据导出中...', 'info');
	
	setTimeout(() => {
		// 创建CSV内容
		let csvContent = "引擎,剩余配额,已用配额,总配额,使用率,过期时间\n";
		
		const quotaItems = document.querySelectorAll('.quota-item');
		quotaItems.forEach(item => {
			const engineName = item.querySelector('h3').textContent;
			const rows = item.querySelectorAll('.quota-row');
			let remaining = '';
			let used = '';
			let total = '';
			let usage = '';
			let expiry = '';
			
			rows.forEach(row => {
				const label = row.querySelector('.label').textContent;
				const value = row.querySelector('.value').textContent;
				
				if (label.includes('剩余配额')) {
					remaining = value;
				} else if (label.includes('已用配额')) {
					used = value;
				} else if (label.includes('总配额')) {
					total = value;
				} else if (label.includes('使用率')) {
					usage = value;
				} else if (label.includes('过期时间')) {
					expiry = value;
				}
			});
			
			csvContent += `${engineName},${remaining},${used},${total},${usage},${expiry}\n`;
		});
		
		// 创建下载链接
		const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
		const link = document.createElement('a');
		const url = URL.createObjectURL(blob);
		link.setAttribute('href', url);
		link.setAttribute('download', `quota_${new Date().toISOString().slice(0, 10)}.csv`);
		link.style.visibility = 'hidden';
		document.body.appendChild(link);
		link.click();
		document.body.removeChild(link);
		
		showMessage('配额数据导出成功', 'success');
	}, 1000);
}

// 打开配额设置
function openQuotaSettings() {
	const modal = document.getElementById('quota-settings-modal');
	if (!modal) return;
	
	modal.style.display = 'block';
	
	// 关闭按钮事件
	const closeBtns = modal.querySelectorAll('.close-btn');
	closeBtns.forEach(btn => {
		btn.addEventListener('click', function() {
			modal.style.display = 'none';
		});
	});
	
	// 保存设置按钮
	const saveBtn = document.getElementById('btn-save-settings');
	if (saveBtn) {
		saveBtn.addEventListener('click', function() {
			// 模拟保存设置
			showMessage('设置保存成功', 'success');
			modal.style.display = 'none';
		});
	}
	
	// 点击模态框外部关闭
	window.addEventListener('click', function(e) {
		if (e.target === modal) {
			modal.style.display = 'none';
		}
	});
}

// 工具函数：格式化数字
function formatNumber(num) {
	return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

// 工具函数：格式化时间
function formatDate(dateString) {
	const date = new Date(dateString);
	return date.toLocaleString();
}

// 工具函数：复制到剪贴板
function copyToClipboard(text) {
	if (navigator.clipboard && window.isSecureContext) {
		return navigator.clipboard.writeText(text);
	} else {
		const textArea = document.createElement('textarea');
		textArea.value = text;
		textArea.style.position = 'fixed';
		textArea.style.left = '-999999px';
		textArea.style.top = '-999999px';
		document.body.appendChild(textArea);
		textArea.focus();
		textArea.select();
		const result = document.execCommand('copy');
		document.body.removeChild(textArea);
		return result ? Promise.resolve() : Promise.reject(new Error('复制失败'));
	}
}

// 添加复制按钮到代码块
function addCopyButtons() {
	const codeBlocks = document.querySelectorAll('code');
	codeBlocks.forEach(codeBlock => {
		const copyBtn = document.createElement('button');
		copyBtn.textContent = '复制';
		copyBtn.className = 'copy-btn';
		copyBtn.style.position = 'absolute';
		copyBtn.style.top = '0.5rem';
		copyBtn.style.right = '0.5rem';
		copyBtn.style.padding = '0.2rem 0.5rem';
		copyBtn.style.fontSize = '0.8rem';
		copyBtn.style.backgroundColor = 'rgba(255,255,255,0.8)';
		copyBtn.style.border = '1px solid #ddd';
		copyBtn.style.borderRadius = '3px';
		copyBtn.style.cursor = 'pointer';
		
		const parent = codeBlock.parentElement;
		parent.style.position = 'relative';
		parent.appendChild(copyBtn);
		
		copyBtn.addEventListener('click', function() {
			copyToClipboard(codeBlock.textContent.trim())
				.then(() => {
					this.textContent = '已复制';
					setTimeout(() => {
						this.textContent = '复制';
					}, 2000);
				})
				.catch(err => {
					console.error('复制失败:', err);
					this.textContent = '复制失败';
					setTimeout(() => {
						this.textContent = '复制';
					}, 2000);
				});
		});
	});
}

// 平滑滚动到指定元素
function scrollToElement(elementId) {
	const element = document.getElementById(elementId);
	if (element) {
		element.scrollIntoView({ behavior: 'smooth' });
	}
}

// 显示消息提示
function showMessage(message, type = 'info') {
	const messageDiv = document.createElement('div');
	messageDiv.className = `message message-${type}`;
	messageDiv.textContent = message;
	messageDiv.style.position = 'fixed';
	messageDiv.style.top = '20px';
	messageDiv.style.right = '20px';
	messageDiv.style.padding = '1rem';
	messageDiv.style.borderRadius = '4px';
	messageDiv.style.zIndex = '1000';
	messageDiv.style.boxShadow = '0 2px 8px rgba(0,0,0,0.2)';
	messageDiv.style.transition = 'all 0.3s ease';
	
	// 设置消息类型样式
	switch (type) {
		case 'success':
			messageDiv.style.backgroundColor = '#d4edda';
			messageDiv.style.color = '#155724';
			messageDiv.style.border = '1px solid #c3e6cb';
			break;
		case 'error':
			messageDiv.style.backgroundColor = '#f8d7da';
			messageDiv.style.color = '#721c24';
			messageDiv.style.border = '1px solid #f5c6cb';
			break;
		case 'warning':
			messageDiv.style.backgroundColor = '#fff3cd';
			messageDiv.style.color = '#856404';
			messageDiv.style.border = '1px solid #ffeeba';
			break;
		default:
			messageDiv.style.backgroundColor = '#d1ecf1';
			messageDiv.style.color = '#0c5460';
			messageDiv.style.border = '1px solid #bee5eb';
	}
	
	document.body.appendChild(messageDiv);
	
	// 2秒后自动消失
	setTimeout(() => {
		messageDiv.style.opacity = '0';
		messageDiv.style.transform = 'translateX(100%)';
		setTimeout(() => {
			document.body.removeChild(messageDiv);
		}, 300);
	}, 3000);
}

// 获取引擎跳转链接
function getEngineLink(source, ip) {
	if (!ip) return '#';
	const query = `ip="${ip}"`;
	// Base64 encode
	let b64 = "";
	try {
		b64 = btoa(query);
	} catch (e) {
		console.error("Base64 encode failed", e);
		return "#";
	}
	
	switch(source ? source.toLowerCase() : '') {
		case 'fofa': return `https://fofa.info/result?qbase64=${b64}`;
		case 'hunter': return `https://hunter.qianxin.com/list?searchValue=${b64}`;
		case 'quake': return `https://quake.360.cn/quake/#/searchResult?searchVal=${encodeURIComponent(query)}`;
		case 'zoomeye': return `https://www.zoomeye.org/searchResult?q=${encodeURIComponent('ip:"'+ip+'"')}`;
		default: return '#';
	}
}

// 查看截图
function viewScreenshot(url, ip, port, protocol) {
	let target = url;
	if (!target || target === "undefined") {
		if (ip) {
			target = `${(protocol || "http").toLowerCase()}://${ip}:${port || 80}`;
		} else {
			alert('无法获取目标URL');
			return;
		}
	}
	
	// 创建或获取模态框
	let modal = document.getElementById('screenshot-modal');
	if (!modal) {
		modal = document.createElement('div');
		modal.id = 'screenshot-modal';
		modal.className = 'modal';
		modal.innerHTML = `
			<div class="modal-content" style="max-width:900px; width:90%;">
				<div class="modal-header">
					<h3 id="screenshot-title">目标截图</h3>
					<button type="button" class="close-btn">&times;</button>
				</div>
				<div class="modal-body" style="text-align:center; min-height:300px; display:flex; justify-content:center; align-items:center; flex-direction: column;">
					<p>正在截图，请稍候...</p>
				</div>
				<div class="modal-footer">
					<button type="button" class="btn btn-secondary close-btn">关闭</button>
					<a href="#" target="_blank" id="open-link-btn" class="btn btn-primary">访问目标</a>
				</div>
			</div>
		`;
		document.body.appendChild(modal);
		
		// 绑定关闭事件
		modal.onclick = function(e) {
			if (e.target === modal) modal.style.display = 'none';
		};
		modal.querySelectorAll('.close-btn').forEach(btn => {
			btn.onclick = function() { modal.style.display = 'none'; };
		});
	}
	
	const title = modal.querySelector('#screenshot-title');
	title.textContent = "目标截图: " + target;

	const body = modal.querySelector('.modal-body');
	const linkBtn = modal.querySelector('#open-link-btn');
	
	body.innerHTML = '<div class="spinner"></div><p style="margin-top:10px;">正在截取页面，可能需要几秒钟(视目标响应速度)...</p>';
	linkBtn.href = target;
	modal.style.display = 'block';
	
	// 请求截图
	const apiUrl = `/api/screenshot?url=${encodeURIComponent(target)}`;
	const img = new Image();
	img.onload = function() {
		body.innerHTML = '';
		img.style.maxWidth = '100%';
		img.style.maxHeight = '600px';
		img.style.border = '1px solid #ddd';
		img.style.boxShadow = '0 0 10px rgba(0,0,0,0.1)';
		body.appendChild(img);
	};
	img.onerror = function() {
		body.innerHTML = `
			<div style="color:#721c24; background:#f8d7da; padding:20px; border-radius:5px;">
				<h4>截图失败</h4>
				<p>目标可能无法访问或响应超时。</p>
				<p>URL: ${target}</p>
			</div>
		`;
	};
	img.src = apiUrl;
}

// 截图搜索引擎结果页面
function captureSearchEngineScreenshots() {
	if (!window.currentQueryData) {
		showMessage('没有可用的查询数据', 'warning');
		return;
	}

	const { query, engines, queryID } = window.currentQueryData;
	if (!engines || engines.length === 0) {
		showMessage('没有可用的搜索引擎', 'warning');
		return;
	}

	const statusEl = document.getElementById('screenshot-status');
	const progressEl = document.getElementById('screenshot-progress');
	const progressBar = document.getElementById('screenshot-progress-bar');
	const progressText = document.getElementById('screenshot-progress-text');

	statusEl.textContent = '正在截图搜索引擎结果页...';
	progressEl.style.display = 'block';

	let completed = 0;
	const total = engines.length;

	engines.forEach((engine, index) => {
		setTimeout(() => {
			fetch(`/api/screenshot/search-engine?engine=${encodeURIComponent(engine)}&query=${encodeURIComponent(query)}&query_id=${queryID}`)
				.then(response => response.json())
				.then(data => {
					completed++;
					const percent = (completed / total) * 100;
					progressBar.style.width = percent + '%';
					progressText.textContent = `已完成 ${completed}/${total}: ${engine}`;

					if (completed === total) {
						statusEl.textContent = '搜索引擎结果页截图完成!';
						showMessage('搜索引擎结果页截图完成!', 'success');
					}
				})
				.catch(err => {
					completed++;
					logger.error(`截图 ${engine} 失败:`, err);
					if (completed === total) {
						statusEl.textContent = '截图完成(部分失败)';
					}
				});
		}, index * 2000); // 每个引擎间隔2秒，避免并发过高
	});
}

// 批量截图所有目标
function captureAllScreenshots() {
	if (!window.currentQueryData) {
		showMessage('没有可用的查询数据', 'warning');
		return;
	}

	const { assets, queryID } = window.currentQueryData;
	if (!assets || assets.length === 0) {
		showMessage('没有可截图的目标', 'warning');
		return;
	}

	// 先截图搜索引擎结果页
	captureSearchEngineScreenshots();

	// 然后批量截图目标
	const statusEl = document.getElementById('screenshot-status');
	const progressEl = document.getElementById('screenshot-progress');
	const progressBar = document.getElementById('screenshot-progress-bar');
	const progressText = document.getElementById('screenshot-progress-text');

	statusEl.textContent = '正在批量截图目标网站...';
	progressEl.style.display = 'block';

	// 准备批量截图请求
	const targets = assets.map(asset => ({
		url: asset.url || asset.URL || '',
		ip: asset.ip || asset.IP || '',
		port: String(asset.port || asset.Port || ''),
		protocol: asset.protocol || asset.Protocol || 'http'
	})).filter(t => t.ip); // 只保留有IP的目标

	// 分批处理，每批5个
	const batchSize = 5;
	let completed = 0;
	const total = targets.length;

	function processBatch(startIndex) {
		if (startIndex >= total) {
			statusEl.textContent = '所有截图完成!';
			showMessage(`批量截图完成! 共 ${total} 个目标`, 'success');
			return;
		}

		const batch = targets.slice(startIndex, startIndex + batchSize);
		const promises = batch.map(target => 
			fetch(`/api/screenshot/target?url=${encodeURIComponent(target.url)}&ip=${encodeURIComponent(target.ip)}&port=${encodeURIComponent(target.port)}&protocol=${encodeURIComponent(target.protocol)}&query_id=${queryID}`)
				.then(response => response.json())
				.catch(err => ({ error: err.message }))
		);

		Promise.all(promises).then(results => {
			completed += batch.length;
			const percent = (completed / total) * 100;
			progressBar.style.width = percent + '%';
			progressText.textContent = `已截图目标: ${completed}/${total}`;

			// 继续下一批
			setTimeout(() => processBatch(startIndex + batchSize), 1000);
		});
	}

	// 等待搜索引擎截图完成后再开始目标截图
	setTimeout(() => processBatch(0), engines.length * 2000 + 1000);
}
