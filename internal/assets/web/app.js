const $ = (selector) => document.querySelector(selector);
const state = {
  groups: [], nodes: [], group: "", query: "",
  token: localStorage.getItem("tz-admin-token") || "",
};

async function api(path, options = {}) {
  const response = await fetch(`/api/${path}`, {
    ...options,
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${state.token}`, ...options.headers },
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

function uptime(value) {
  if (!value) return "—";
  const days = Math.floor(value / 86400), hours = Math.floor((value % 86400) / 3600), minutes = Math.floor((value % 3600) / 60);
  return days ? `${days}天 ${hours}时` : `${hours}时 ${minutes}分`;
}

const percent = (used, total) => total ? Math.min(100, used / total * 100) : 0;
const online = (node) => Date.now() - new Date(node.lastSeen).getTime() < 15000;
const escapeHTML = (value) => String(value ?? "").replace(/[&<>'"]/g, (char) => ({
  "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;",
})[char]);

function toast(message) {
  const element = $("#toast"); element.textContent = message; element.classList.add("show");
  setTimeout(() => element.classList.remove("show"), 2200);
}

async function load() {
  try {
    const dashboard = await api("dashboard");
    state.groups = dashboard.groups; state.nodes = dashboard.nodes;
    $("#syncStatus").classList.remove("error"); $("#syncStatus").lastChild.textContent = " 已同步";
    render();
  } catch (error) { if (error.status !== 401) toast(error.message); }
}

function render() {
  const active = state.nodes.filter(online);
  const uploadSpeed = active.reduce((sum, node) => sum + node.uploadSpeed, 0);
  const downloadSpeed = active.reduce((sum, node) => sum + node.downloadSpeed, 0);
  const totalUpload = state.nodes.reduce((sum, node) => sum + node.totalUpload, 0);
  const totalDownload = state.nodes.reduce((sum, node) => sum + node.totalDownload, 0);
  $("#sumUpSpeed").textContent = bytes(uploadSpeed, true); $("#sumDownSpeed").textContent = bytes(downloadSpeed, true);
  $("#sumTraffic").textContent = bytes(totalUpload + totalDownload);
  $("#sumTrafficSub").textContent = `上传 ${bytes(totalUpload)} · 下载 ${bytes(totalDownload)}`;
  $("#overviewMeta").textContent = `${active.length} 台在线 · ${state.nodes.length - active.length} 台离线 · 每 3 秒刷新`;
  renderGroups(); renderNodes();
}

function renderGroups() {
  const counts = {}; state.nodes.forEach((node) => { counts[node.groupId] = (counts[node.groupId] || 0) + 1; });
  $("#groupList").innerHTML = `<button class="group ${state.group === "" ? "active" : ""}" data-group=""><span><i class="dot all"></i>全部服务器</span><b>${state.nodes.length}</b></button>` + state.groups.map((group) => `<div class="group-row"><button class="group ${state.group === group.id ? "active" : ""}" data-group="${group.id}"><span><i class="dot online"></i>${escapeHTML(group.name)}</span><b>${counts[group.id] || 0}</b></button><button class="group-edit" data-group-edit="${group.id}" title="编辑分组">✎</button><button class="group-delete" data-group-delete="${group.id}" title="删除分组">×</button></div>`).join("");
  document.querySelectorAll(".group").forEach((button) => { button.onclick = () => { state.group = button.dataset.group; render(); }; });
  document.querySelectorAll("[data-group-edit]").forEach((button) => { button.onclick = () => openEntityForm("group", state.groups.find((group) => group.id === button.dataset.groupEdit)); });
  document.querySelectorAll("[data-group-delete]").forEach((button) => { button.onclick = () => deleteGroup(button.dataset.groupDelete); });
}

function renderNodes() {
  const nodes = state.nodes.filter((node) => (!state.group || node.groupId === state.group) && (!state.query || `${node.name} ${node.ip}`.toLowerCase().includes(state.query)));
  const group = state.groups.find((item) => item.id === state.group);
  $("#listTitle").textContent = group ? group.name : "全部服务器"; $("#listMeta").textContent = `${nodes.length} 台服务器`;
  $("#emptyState").hidden = nodes.length > 0;
  $("#nodeRows").innerHTML = nodes.map((node) => {
    const isOnline = online(node), memoryPercent = percent(node.memoryUsed, node.memoryTotal), diskPercent = percent(node.diskUsed, node.diskTotal);
    return `<tr><td><div class="server-name"><i class="status-dot ${isOnline ? "" : "offline"}"></i><div><strong>${escapeHTML(node.name)}</strong><small>${escapeHTML(node.ip || "等待首次上报")} · ${isOnline ? "在线" : "离线"}</small></div></div></td><td>${uptime(node.uptime)}</td><td><div class="metric"><span class="value">${isOnline ? `${node.cpu.toFixed(1)}%` : "—"}</span><div class="bar"><i style="width:${node.cpu || 0}%"></i></div></div></td><td><div class="metric"><span class="value">${bytes(node.memoryUsed)} / ${bytes(node.memoryTotal)}</span><div class="bar blue"><i style="width:${memoryPercent}%"></i></div></div></td><td><div class="metric"><span class="value">${bytes(node.diskUsed)} / ${bytes(node.diskTotal)}</span><div class="bar violet"><i style="width:${diskPercent}%"></i></div></div></td><td class="speed up">↑ ${isOnline ? bytes(node.uploadSpeed, true) : "—"}</td><td class="speed down">↓ ${isOnline ? bytes(node.downloadSpeed, true) : "—"}</td><td>${bytes(node.totalUpload)}</td><td>${bytes(node.totalDownload)}</td><td><div class="row-actions"><button class="action" title="升级探针" data-upgrade="${node.id}">↻</button><button class="action" title="编辑" data-edit="${node.id}">✎</button><button class="action delete" title="删除" data-delete="${node.id}">×</button></div></td></tr>`;
  }).join("");
  document.querySelectorAll("[data-edit]").forEach((button) => { button.onclick = () => editNode(button.dataset.edit); });
  document.querySelectorAll("[data-delete]").forEach((button) => { button.onclick = () => deleteNode(button.dataset.delete); });
  document.querySelectorAll("[data-upgrade]").forEach((button) => { button.onclick = () => upgradeNode(button.dataset.upgrade); });
}

function openEntityForm(kind, item = null) {
  const isNode = kind === "node";
  $("#modalKicker").textContent = isNode ? "服务器" : "分组";
  $("#modalTitle").textContent = item ? `编辑${isNode ? "服务器" : "分组"}` : "添加分组";
  $("#formFields").innerHTML = isNode
    ? `<div class="field"><label>服务器名称</label><input name="name" required maxlength="60" value="${escapeHTML(item.name)}"></div><div class="two-fields"><div class="field"><label>所属分组</label><select name="groupId"><option value="">未分组</option>${state.groups.map((group) => `<option value="${group.id}" ${item.groupId === group.id ? "selected" : ""}>${escapeHTML(group.name)}</option>`).join("")}</select></div><div class="field"><label>排序</label><input name="sort" type="number" value="${item.sort}"></div></div>`
    : `<div class="field"><label>分组名称</label><input name="name" required maxlength="40" value="${escapeHTML(item?.name || "")}"></div><div class="field"><label>排序</label><input name="sort" type="number" value="${item?.sort ?? state.groups.length}"></div>`;
  $("#entityForm").onsubmit = async (event) => {
    event.preventDefault(); const body = Object.fromEntries(new FormData(event.target)); body.sort = Number(body.sort) || 0;
    const path = item ? `${isNode ? "nodes" : "groups"}/${item.id}` : "groups";
    try { await api(path, { method: item ? "PUT" : "POST", body: JSON.stringify(body) }); $("#formDialog").close(); await load(); toast("已保存"); }
    catch (error) { if (error.status !== 401) toast(error.message); }
  };
  $("#formDialog").showModal(); setTimeout(() => $("#formFields input")?.focus(), 50);
}

const editNode = (id) => openEntityForm("node", state.nodes.find((node) => node.id === id));
async function deleteNode(id) {
  const node = state.nodes.find((item) => item.id === id);
  if (!confirm(`确定删除“${node.name}”？如果探针仍在运行，节点会自动重新加入。`)) return;
  try { await api(`nodes/${id}`, { method: "DELETE" }); await load(); toast("服务器已删除"); }
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
  $("#formFields").innerHTML = `<div class="field"><label>管理令牌</label><input name="token" type="password" required autocomplete="current-password"><small style="display:block;color:${message ? "var(--danger)" : "var(--muted)"};margin-top:8px">${message || "与面板启动时的 TZ_ADMIN_TOKEN 保持一致"}</small></div>`;
  $("#entityForm").onsubmit = (event) => { event.preventDefault(); state.token = new FormData(event.target).get("token").trim(); localStorage.setItem("tz-admin-token", state.token); $("#formDialog").close(); load(); };
  $("#formDialog").showModal(); setTimeout(() => $("#formFields input")?.focus(), 50);
}

async function openInstallCommand() {
  try {
    const config = await api("install");
    $("#installCommand").textContent = `curl -fsSL ${location.origin}/install.sh | bash -s -- --url ${location.origin} --token ${config.agentToken}`;
    $("#commandDialog").showModal();
  } catch (error) { if (error.status !== 401) toast(error.message); }
}

function registerWebMCP() {
  const context = document.modelContext; if (!context?.registerTool) return;
  const register = (tool) => Promise.resolve(context.registerTool(tool)).catch(() => {});
  register({ name: "get_server_overview", title: "获取服务器概览", description: "读取当前面板中的服务器、分组和在线状态。", inputSchema: { type: "object", properties: {}, additionalProperties: false }, annotations: { readOnlyHint: true, untrustedContentHint: false }, execute: async () => { const dashboard = await api("dashboard"); return { groups: dashboard.groups.map((group) => ({ id: group.id, name: group.name })), servers: dashboard.nodes.map((node) => ({ id: node.id, name: node.name, ip: node.ip, online: online(node), cpu: node.cpu })) }; } });
  register({ name: "get_agent_install_command", title: "获取探针安装命令", description: "获取可在所有 Linux 节点重复使用的通用探针安装命令。", inputSchema: { type: "object", properties: {}, additionalProperties: false }, annotations: { readOnlyHint: true, untrustedContentHint: false }, execute: async () => { const config = await api("install"); return { installCommand: `curl -fsSL ${location.origin}/install.sh | bash -s -- --url ${location.origin} --token ${config.agentToken}` }; } });
}

$("#installAgentBtn").onclick = openInstallCommand; $("#emptyInstallBtn").onclick = openInstallCommand;
$("#addGroupBtn").onclick = () => openEntityForm("group"); $("#tokenBtn").onclick = () => openToken();
$("#search").oninput = (event) => { state.query = event.target.value.trim().toLowerCase(); renderNodes(); };
$("#copyCommand").onclick = async () => { await navigator.clipboard.writeText($("#installCommand").textContent); toast("命令已复制"); };
$("#commandDone").onclick = () => $("#commandDialog").close();
if (state.token) load(); else openToken();
setInterval(() => { if (state.token) load(); }, 3000);
registerWebMCP();
