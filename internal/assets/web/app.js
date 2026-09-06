const $ = (selector) => document.querySelector(selector);
const state = {
  groups: [], nodes: [], group: "", query: "",
  token: localStorage.getItem("tz-admin-token") || "",
};

async function api(path, options = {}) {
  const response = await fetch(`/api/${path}`, {
    ...options,
    headers: { "Content-Type": "application/json", "X-TZ-Admin-Token": btoa(Array.from(new TextEncoder().encode(state.token), (byte) => String.fromCharCode(byte)).join("")), ...options.headers },
  });
  const body = await response.json().catch(() => ({}));
  if (response.status === 401) {
    state.token = "";
    localStorage.removeItem("tz-admin-token");
    $("#syncStatus").classList.add("error");
    $("#syncStatus").lastChild.textContent = " 需要管理令牌";
    if (!$("#formDialog").open) openToken("管理令牌错误，请重新填写");
    const error = new Error("管理令牌无效"); error.status = 401; throw error;
  }
  if (!response.ok) { const error = new Error(body.error || "操作失败"); error.status = response.status; throw error; }
  return body;
}

function bytes(value, speed = false) {
  if (value === undefined || value === null) return "—";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let unit = 0, size = value;
  while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1; }
  const digits = size >= 100 || unit === 0 ? 0 : size >= 10 ? 1 : 2;
  return `${size.toFixed(digits)} ${units[unit]}${speed ? "/s" : ""}`;
}

function megabits(value) {
  const mbps = value * 8 / 1000000;
  return `${mbps.toFixed(mbps >= 100 ? 0 : mbps >= 10 ? 1 : 2)} M/s`;
}

function speedHTML(value) {
  const [number, unit] = megabits(value).split(" ");
  return `<strong class="speed-number">${number}</strong> <span class="speed-unit">${unit}</span>`;
}

function uptime(value) {
  if (!value) return "—";
  const days = Math.floor(value / 86400), hours = Math.floor((value % 86400) / 3600), minutes = Math.floor((value % 3600) / 60);
  return days ? `${days}天 ${hours}时` : `${hours}时 ${minutes}分`;
}

const percent = (used, total) => total ? Math.min(100, used / total * 100) : 0;
const online = (node) => Date.now() - new Date(node.lastSeen).getTime() < 15000;
const groupIDs = (node) => node.groupIds || (node.groupId ? [node.groupId] : []);
const escapeHTML = (value) => String(value ?? "").replace(/[&<>'"]/g, (char) => ({
  "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;",
})[char]);

function toast(message) {
  const element = $("#toast"); element.textContent = message; element.classList.add("show");
  setTimeout(() => element.classList.remove("show"), 2200);
}

async function load() {
	if (state.loading) return;
	state.loading = true;
  try {
    const dashboard = await api("dashboard");
    state.groups = dashboard.groups; state.nodes = dashboard.nodes;
    $("#syncStatus").classList.remove("error"); $("#syncStatus").lastChild.textContent = " 已同步";
    render();
  } catch (error) { if (error.status !== 401) toast(error.message); }
  finally { state.loading = false; }
}

function render() {
  const scoped = state.nodes.filter((node) => !state.group || groupIDs(node).includes(state.group));
  const active = scoped.filter(online);
  const uploadSpeed = active.reduce((sum, node) => sum + node.uploadSpeed, 0);
  const downloadSpeed = active.reduce((sum, node) => sum + node.downloadSpeed, 0);
  const totalUpload = scoped.reduce((sum, node) => sum + (node.totalUpload || 0), 0);
  const totalDownload = scoped.reduce((sum, node) => sum + (node.totalDownload || 0), 0);
  $("#sumUpSpeed").textContent = megabits(uploadSpeed); $("#sumDownSpeed").textContent = megabits(downloadSpeed);
  $("#sumTrafficSub").textContent = `上传总流量 ${bytes(totalUpload)} · 下载总流量 ${bytes(totalDownload)}`;
  $("#sumTodayUpload").textContent = bytes(scoped.reduce((sum, node) => sum + (node.todayUpload || 0), 0));
  $("#sumTodayDownload").textContent = bytes(scoped.reduce((sum, node) => sum + (node.todayDownload || 0), 0));
  $("#sumYesterdayUpload").textContent = bytes(scoped.reduce((sum, node) => sum + (node.yesterdayUpload || 0), 0));
  $("#sumYesterdayDownload").textContent = bytes(scoped.reduce((sum, node) => sum + (node.yesterdayDownload || 0), 0));
  $("#onlineCount").textContent = active.length;
  $("#offlineCount").textContent = scoped.length - active.length;
  document.querySelectorAll(".daily-scope").forEach((element) => { element.textContent = state.group ? "北京时间 · 当前分组" : "北京时间 · 全部节点"; });
  renderGroups(); renderNodes();
}

function renderGroups() {
  const counts = {}; state.nodes.forEach((node) => { groupIDs(node).forEach((groupID) => { counts[groupID] = (counts[groupID] || 0) + 1; }); });
  $("#groupList").innerHTML = `<button class="group ${state.group === "" ? "active" : ""}" data-group=""><span><i class="dot all"></i>全部服务器</span><b>${state.nodes.length}</b></button>` + state.groups.map((group) => `<div class="group-row"><button class="group ${state.group === group.id ? "active" : ""}" data-group="${group.id}"><span><i class="dot online"></i>${escapeHTML(group.name)}</span><b>${counts[group.id] || 0}</b></button><button class="group-edit" data-group-edit="${group.id}" title="编辑分组">✎</button><button class="group-delete" data-group-delete="${group.id}" title="删除分组">×</button></div>`).join("");
  document.querySelectorAll(".group").forEach((button) => { button.onclick = () => { state.group = button.dataset.group; render(); }; });
  document.querySelectorAll("[data-group-edit]").forEach((button) => { button.onclick = () => openEntityForm("group", state.groups.find((group) => group.id === button.dataset.groupEdit)); });
  document.querySelectorAll("[data-group-delete]").forEach((button) => { button.onclick = () => deleteGroup(button.dataset.groupDelete); });
}

function renderNodes() {
  const nodes = state.nodes.filter((node) => (!state.group || groupIDs(node).includes(state.group)) && (!state.query || `${node.name} ${node.ip}`.toLowerCase().includes(state.query)));
  const group = state.groups.find((item) => item.id === state.group);
  $("#listTitle").textContent = group ? group.name : "全部服务器"; $("#listMeta").textContent = `${nodes.length} 台服务器`;
  $("#emptyState").hidden = nodes.length > 0;
  $("#nodeRows").innerHTML = nodes.map((node) => {
    const isOnline = online(node), memoryPercent = percent(node.memoryUsed, node.memoryTotal), diskPercent = percent(node.diskUsed, node.diskTotal);
    return `<tr><td class="sort-cell">${node.sort}</td><td><div class="server-name"><i class="status-dot ${isOnline ? "" : "offline"}"></i><button class="server-copy" data-copy-ip="${escapeHTML(node.ip)}" title="点击复制 IP"><strong>${escapeHTML(node.name)}</strong><small>${escapeHTML(node.ip || "等待首次上报")} · ${isOnline ? "在线" : "离线"}</small></button></div></td><td class="speed up">↑ ${isOnline ? speedHTML(node.uploadSpeed) : "—"}</td><td class="speed down">↓ ${isOnline ? speedHTML(node.downloadSpeed) : "—"}</td><td>${uptime(node.uptime)}</td><td><div class="metric"><span class="value">${isOnline ? `${node.cpu.toFixed(1)}%` : "—"}</span><progress class="bar" max="100" value="${isOnline ? Math.min(100, node.cpu) : 0}" aria-label="CPU 使用率"></progress></div></td><td><div class="metric"><span class="value">${isOnline ? `${memoryPercent.toFixed(1)}%` : "—"}</span><progress class="bar" max="100" value="${isOnline ? memoryPercent : 0}" aria-label="内存使用率"></progress></div></td><td><div class="metric"><span class="value">${isOnline ? `${diskPercent.toFixed(1)}%` : "—"}</span><progress class="bar" max="100" value="${isOnline ? diskPercent : 0}" aria-label="存储使用率"></progress></div></td><td>${bytes(node.todayUpload || 0)}</td><td>${bytes(node.todayDownload || 0)}</td><td>${bytes(node.totalUpload)}</td><td>${bytes(node.totalDownload)}</td><td><div class="row-actions"><button class="action" title="升级探针" data-upgrade="${node.id}">↻</button><button class="action" title="编辑" data-edit="${node.id}">✎</button><button class="action delete" title="删除" data-delete="${node.id}">×</button></div></td></tr>`;
  }).join("");
  document.querySelectorAll("[data-edit]").forEach((button) => { button.onclick = () => editNode(button.dataset.edit); });
  document.querySelectorAll("[data-delete]").forEach((button) => { button.onclick = () => deleteNode(button.dataset.delete); });
  document.querySelectorAll("[data-upgrade]").forEach((button) => { button.onclick = () => upgradeNode(button.dataset.upgrade); });
  document.querySelectorAll("[data-copy-ip]").forEach((button) => { button.onclick = () => copyText(button.dataset.copyIp).then(() => toast(`已复制 ${button.dataset.copyIp}`)).catch((error) => toast(error.message)); });
}

function openEntityForm(kind, item = null) {
  const isNode = kind === "node";
  const nodeGroups = isNode ? groupIDs(item) : [];
  const groupPicker = isNode ? `<div class="field"><label>所属分组</label><details class="server-picker"><summary><span id="groupSummary">已选择 ${nodeGroups.length} 个分组</span><i>⌄</i></summary><div class="server-options">${state.groups.map((group) => `<label class="server-option"><input class="group-choice" type="checkbox" value="${group.id}" ${nodeGroups.includes(group.id) ? "checked" : ""}><span>${escapeHTML(group.name)}</span></label>`).join("") || '<div class="no-options">暂无分组</div>'}</div></details></div>` : "";
  const selectedCount = !isNode && item ? state.nodes.filter((node) => groupIDs(node).includes(item.id)).length : 0;
  const serverPicker = !isNode && item ? `<div class="field"><label>分组服务器</label><details class="server-picker"><summary><span id="memberSummary">已选择 ${selectedCount} 台</span><i>⌄</i></summary><div class="server-options"><div class="picker-search"><span>⌕</span><input id="serverPickerSearch" type="search" placeholder="搜索名称或 IP" autocomplete="off"></div><label class="server-option select-all"><input type="checkbox" id="selectAllNodes" ${state.nodes.length > 0 && selectedCount === state.nodes.length ? "checked" : ""}><span>全选筛选结果</span><b id="visibleNodeCount">${state.nodes.length}</b></label>${state.nodes.map((node) => `<label class="server-option node-option"><input class="node-choice" type="checkbox" value="${node.id}" ${groupIDs(node).includes(item.id) ? "checked" : ""}><span>${escapeHTML(node.name)}<small>${escapeHTML(node.ip)}</small></span></label>`).join("") || '<div class="no-options">暂无服务器</div>'}</div></details><small class="field-help">这里只修改当前分组，不影响服务器所在的其他分组</small></div>` : "";
  $("#modalKicker").textContent = isNode ? "服务器" : "分组";
  $("#modalTitle").textContent = item ? `编辑${isNode ? "服务器" : "分组"}` : "添加分组";
  $("#formFields").innerHTML = isNode
    ? `<div class="field"><label>服务器名称</label><input name="name" required maxlength="60" value="${escapeHTML(item.name)}"></div><div class="field"><label>排序</label><input name="sort" type="number" value="${item.sort}"></div>${groupPicker}`
    : `<div class="field"><label>分组名称</label><input name="name" required maxlength="40" value="${escapeHTML(item?.name || "")}"></div><div class="field"><label>排序</label><input name="sort" type="number" value="${item?.sort ?? state.groups.length}"></div>${serverPicker}`;
  if (isNode) setupGroupPicker();
  if (!isNode && item) setupServerPicker();
  $("#entityForm").onsubmit = async (event) => {
    event.preventDefault(); const body = Object.fromEntries(new FormData(event.target)); body.sort = Number(body.sort) || 0;
    if (isNode) body.groupIds = [...document.querySelectorAll(".group-choice:checked")].map((checkbox) => checkbox.value);
    if (!isNode && item) body.nodeIds = [...document.querySelectorAll(".node-choice:checked")].map((checkbox) => checkbox.value);
    const path = item ? `${isNode ? "nodes" : "groups"}/${item.id}` : "groups";
    try { await api(path, { method: item ? "PUT" : "POST", body: JSON.stringify(body) }); $("#formDialog").close(); await load(); toast("已保存"); }
    catch (error) { if (error.status !== 401) toast(error.message); }
  };
  $("#formDialog").showModal(); setTimeout(() => $("#formFields input")?.focus(), 50);
}

function setupGroupPicker() {
  const choices = [...document.querySelectorAll(".group-choice")], summary = $("#groupSummary");
  const update = () => { summary.textContent = `已选择 ${choices.filter((checkbox) => checkbox.checked).length} 个分组`; };
  choices.forEach((checkbox) => { checkbox.onchange = update; });
  update();
}

function setupServerPicker() {
  const all = $("#selectAllNodes"), choices = [...document.querySelectorAll(".node-choice")], summary = $("#memberSummary"), search = $("#serverPickerSearch"), visibleCount = $("#visibleNodeCount");
  const visibleChoices = () => choices.filter((checkbox) => !checkbox.closest(".node-option").hidden);
  const update = () => {
    const count = choices.filter((checkbox) => checkbox.checked).length;
    const visible = visibleChoices(), visibleSelected = visible.filter((checkbox) => checkbox.checked).length;
    summary.textContent = `已选择 ${count} 台`;
    visibleCount.textContent = visible.length;
    all.checked = visible.length > 0 && visibleSelected === visible.length;
    all.indeterminate = visibleSelected > 0 && visibleSelected < visible.length;
  };
  search.oninput = () => {
    const query = search.value.trim().toLowerCase();
    choices.forEach((checkbox) => { checkbox.closest(".node-option").hidden = query !== "" && !checkbox.closest(".node-option").textContent.toLowerCase().includes(query); });
    update();
  };
  all.onchange = () => { visibleChoices().forEach((checkbox) => { checkbox.checked = all.checked; }); update(); };
  choices.forEach((checkbox) => { checkbox.onchange = update; });
  update();
}

const editNode = (id) => openEntityForm("node", state.nodes.find((node) => node.id === id));
async function deleteNode(id) {
  const node = state.nodes.find((item) => item.id === id);
  if (!confirm(`确定删除“${node.name}”并卸载这台服务器上的探针？离线节点将在重新上线后执行；旧版探针会先升级再卸载。`)) return;
  try { await api(`nodes/${id}`, { method: "DELETE" }); await load(); toast("服务器已移除，卸载指令已排队，等待节点执行"); }
  catch (error) { if (error.status !== 401) toast(error.message); }
}
async function upgradeNode(id) {
  const node = state.nodes.find((item) => item.id === id);
  if (!online(node)) { toast("服务器离线，暂时无法升级"); return; }
  if (!confirm(`向“${node.name}”发送探针升级指令？`)) return;
  try { await api(`nodes/${id}/upgrade`, { method: "POST" }); toast("升级指令已发送"); }
  catch (error) { if (error.status !== 401) toast(error.message); }
}
async function deleteGroup(id) {
  const group = state.groups.find((item) => item.id === id);
  if (!confirm(`确定删除分组“${group.name}”？组内服务器将变为未分组。`)) return;
  try { await api(`groups/${id}`, { method: "DELETE" }); if (state.group === id) state.group = ""; await load(); toast("分组已删除"); }
  catch (error) { if (error.status !== 401) toast(error.message); }
}

function openToken(message = "") {
  if ($("#formDialog").open) return;
  $("#modalKicker").textContent = "安全验证"; $("#modalTitle").textContent = "管理令牌";
  $("#formFields").innerHTML = `<div class="field"><label>管理令牌</label><input name="token" type="password" required autocomplete="current-password"><small class="field-help">${escapeHTML(message || "填写当前管理令牌")}</small></div>`;
  $("#entityForm").onsubmit = (event) => { event.preventDefault(); state.token = new FormData(event.target).get("token"); localStorage.setItem("tz-admin-token", state.token); $("#formDialog").close(); load(); };
  $("#formDialog").showModal(); setTimeout(() => $("#formFields input")?.focus(), 50);
}

function openChangeToken() {
  if ($("#formDialog").open) return;
  $("#modalKicker").textContent = "安全设置"; $("#modalTitle").textContent = "修改管理令牌";
  $("#formFields").innerHTML = `<div class="field"><label>新管理令牌</label><input name="token" type="password" required autocomplete="new-password"></div><div class="field"><label>确认新令牌</label><input name="confirmToken" type="password" required autocomplete="new-password"></div><small class="field-help">令牌不设长度和复杂度要求，填写非空内容即可。修改后其他浏览器需重新登录。</small>`;
  $("#entityForm").onsubmit = async (event) => {
    event.preventDefault(); const form = new FormData(event.target), token = form.get("token");
    if (token !== form.get("confirmToken")) { toast("两次输入的令牌不一致"); return; }
    try {
      await api("admin-token", { method: "PUT", body: JSON.stringify({ token }) });
      state.token = token; localStorage.setItem("tz-admin-token", token); $("#formDialog").close(); toast("管理令牌已修改");
    } catch (error) { if (error.status !== 401) toast(error.message); }
  };
  $("#formDialog").showModal(); setTimeout(() => $("#formFields input")?.focus(), 50);
}

async function copyText(text) {
  if (!text) throw new Error("没有可复制的内容");
  if (navigator.clipboard && window.isSecureContext) {
    try { await navigator.clipboard.writeText(text); return; } catch (_) { /* use the HTTP-compatible fallback */ }
  }
  const textarea = document.createElement("textarea"); textarea.value = text; textarea.style.position = "fixed"; textarea.style.opacity = "0";
  textarea.setAttribute("readonly", "");
  const container = document.querySelector("dialog[open]") || document.body;
  container.appendChild(textarea); textarea.focus(); textarea.select();
  const copied = document.execCommand("copy"); textarea.remove();
  if (!copied) throw new Error("复制失败，请长按或手动选择命令");
}

async function openInstallCommand() {
  try {
    const config = await api("install");
    $("#installCommand").textContent = `curl -fsSL ${location.origin}/install.sh | bash -s -- --url ${location.origin} --token ${config.agentToken}`;
    $("#uninstallCommand").textContent = `curl -fsSL ${location.origin}/uninstall.sh | bash`;
    $("#commandDialog").showModal();
  } catch (error) { if (error.status !== 401) toast(error.message); }
}

function registerWebMCP() {
  const context = document.modelContext; if (!context?.registerTool) return;
  const register = (tool) => Promise.resolve(context.registerTool(tool)).catch(() => {});
  register({ name: "get_server_overview", title: "获取服务器概览", description: "读取当前面板中的服务器、分组和在线状态。", inputSchema: { type: "object", properties: {}, additionalProperties: false }, annotations: { readOnlyHint: true, untrustedContentHint: false }, execute: async () => { const dashboard = await api("dashboard"); return { groups: dashboard.groups.map((group) => ({ id: group.id, name: group.name })), servers: dashboard.nodes.map((node) => ({ id: node.id, name: node.name, ip: node.ip, online: online(node), cpu: node.cpu })) }; } });
  register({ name: "get_agent_install_command", title: "获取探针命令", description: "获取可在所有 Linux 节点重复使用的探针安装和卸载命令。", inputSchema: { type: "object", properties: {}, additionalProperties: false }, annotations: { readOnlyHint: true, untrustedContentHint: false }, execute: async () => { const config = await api("install"); return { installCommand: `curl -fsSL ${location.origin}/install.sh | bash -s -- --url ${location.origin} --token ${config.agentToken}`, uninstallCommand: `curl -fsSL ${location.origin}/uninstall.sh | bash` }; } });
}

document.querySelectorAll("[data-close-form]").forEach((button) => { button.onclick = () => $("#formDialog").close(); });
$("#installAgentBtn").onclick = openInstallCommand; $("#emptyInstallBtn").onclick = openInstallCommand;
$("#addGroupBtn").onclick = () => openEntityForm("group"); $("#tokenBtn").onclick = () => state.token ? openChangeToken() : openToken();
$("#search").oninput = (event) => { state.query = event.target.value.trim().toLowerCase(); renderNodes(); };
$("#copyInstallCommand").onclick = () => copyText($("#installCommand").textContent).then(() => toast("安装命令已复制")).catch((error) => toast(error.message));
$("#copyUninstallCommand").onclick = () => copyText($("#uninstallCommand").textContent).then(() => toast("卸载命令已复制")).catch((error) => toast(error.message));
$("#commandDone").onclick = () => $("#commandDialog").close();
if (state.token) load(); else openToken();
setInterval(() => { if (state.token) load(); }, 3000);
registerWebMCP();
