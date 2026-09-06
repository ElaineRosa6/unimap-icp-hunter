// Render only allowlisted diagnostics, never raw errors or credentials.
export function renderBridgeStatus(element, version, state = {}, now = Date.now()) {
  const poll = Number(state.last_poll_at);
  const age = now - poll;
  let label = "尚无成功轮询记录；请确认本地服务已启动。";
  if (Number.isFinite(poll) && poll > 0) {
    label = age >= 0 && age <= 30000
      ? "最近轮询成功（30秒内）；不代表引擎业务验收通过。"
      : "状态已过期；可能正在执行任务、后台休眠或连接中断，请刷新核对。";
  }
  if (Number(state.last_error_at) >= poll && poll > 0) {
    label = "最近一次操作失败；请核对本地服务与配对配置。";
  }
  element.textContent = `运行版本：${version}\n${label}`;
  if (Number.isFinite(poll) && poll > 0 && !Number.isNaN(new Date(poll).getTime())) {
    element.textContent += `\n最后成功轮询：${new Date(poll).toLocaleString()}`;
  }
}
