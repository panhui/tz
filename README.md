# TZ Linux 探针面板

一个轻量、自托管的 Linux 服务器监控面板。面板和探针均为单文件程序，无需外部数据库。

## 功能

- 服务器名称、IP、运行时间、CPU、内存、存储实时状态
- 实时上传/下载速度、今日上传/下载流量与累计上传/下载流量
- 全局流量汇总、在线/离线状态
- 服务器编辑、删除、自定义排序（数字越大越靠前）
- 自定义分组，同一服务器可同时属于多个分组
- 探针一键安装、面板内一键升级
- 所有节点共用一条安装命令，首次上报自动注册
- amd64、arm64、armv7 Linux 支持
- 管理接口令牌保护、探针独立随机令牌
- 登录后可修改管理令牌，修改结果持久保存
- 管理令牌只要求非空，不限制长度、复杂度，支持中文、空格和符号
- 分组编辑支持服务器多选和一键全选

## 部署面板

### 一键安装（无需 Docker）

使用 root 用户运行：

```bash
curl -fsSL https://raw.githubusercontent.com/panhui/tz/main/scripts/install-panel.sh | bash
```

普通用户可以运行：

```bash
curl -fsSL https://raw.githubusercontent.com/panhui/tz/main/scripts/install-panel.sh | sudo bash
```

安装完成后会显示访问地址和自动生成的管理令牌。请保存管理令牌，并在服务器防火墙中放行 TCP 876 端口。

需要重新查看当前管理令牌时，在面板服务器运行：

```bash
grep '^TZ_ADMIN_TOKEN=' /etc/tz-panel.env
```

令牌可能以双引号包裹保存，最外层引号不是令牌内容。升级会保留当前令牌。

### Docker Compose（推荐）

```bash
git clone https://github.com/panhui/tz.git
cd tz
# 请先修改 docker-compose.yml 中的 TZ_ADMIN_TOKEN
docker compose up -d
```

打开 `http://服务器IP:876`，输入你设置的管理令牌。

### 直接运行

从 Releases 下载对应架构的 `tz-panel-linux-*`，然后运行：

```bash
chmod +x tz-panel-linux-amd64
TZ_ADMIN_TOKEN='替换为高强度令牌' TZ_LISTEN=':876' TZ_DATA='./data/data.json' ./tz-panel-linux-amd64
```

生产环境建议在面板前配置 HTTPS 反向代理。

## 升级面板与修改端口

重新执行一键安装命令即可升级。新安装默认使用 **876**；已有安装保留 `/etc/tz-panel.env` 中的端口，避免现有探针断连。显式切换到 876：

```bash
curl -fsSL https://raw.githubusercontent.com/panhui/tz/main/scripts/install-panel.sh | bash -s -- --port 876
```

切换前请放行 TCP 876。切换后访问 `http://服务器IP:876`，在每台节点重新执行新面板中显示的安装命令，以更新上报地址；节点 ID 会保留。通过反向代理接入且外部地址不变时，只需更新代理的后端端口。

直接运行程序时，Linux 的低端口需要 root 权限或 `CAP_NET_BIND_SERVICE`；一键安装已为服务配置该权限，Docker Compose 已配置容器内低端口监听。

## 今日流量统计

列表显示每台服务器的「今日上传」「今日下载」。顶部五张卡片随分组切换：选择「全部服务器」时汇总全部节点，选择分组时只统计该分组。速度只统计在线节点，今日和累计流量包含该分组的离线节点，同一节点只计一次。按北京时间（UTC+8）每天 00:00 重新累计。

统计从升级后的首次上报建立基线开始，不把节点此前累计流量算入今日。面板保存计数，服务重启后继续累计；节点重启或网卡计数归零时按新计数继续累加。跨零点的上报间隔按时间比例分配流量，长时间断连跨日时属于估算；断连期间重启而丢失的计数无法恢复。现有探针无需升级即可提供今日流量统计。

## 安装探针

进入面板，点击“安装探针”，复制弹窗中的通用安装命令，在所有目标 Linux 服务器以 root 用户执行即可。每台服务器会生成自己的节点 ID，首次上报后自动出现在列表中，无需提前添加服务器。

## 升级探针

点击服务器行末尾的“↻”按钮。在线探针会在下一次心跳收到升级指令，从最新 GitHub Release 下载新版本并自动重启。

## 卸载探针

点击面板右上角“安装探针”，弹窗中同时提供安装与卸载命令。卸载完成后，可以在面板中删除对应的离线记录。

也可以直接点击服务器行的删除按钮，确认后移除面板记录，同时将卸载指令持久保存。在线探针在下一次上报时收到指令；离线探针在恢复连接后执行。指令未执行前不会重新注册到列表。

v0.7.0 起探针支持远程卸载：清理 `tz-agent` 服务、`/usr/local/bin/tz-agent` 和 `/etc/tz-agent.env`。旧版探针先接收升级指令，升级成功后再卸载，因此需要节点能够下载 GitHub Release。提示“已排队”不代表机器已完成卸载。卸载后重新安装会生成新的节点 ID。

## 配置

| 环境变量 | 用途 | 默认值 |
| --- | --- | --- |
| `TZ_ADMIN_TOKEN` | 面板管理令牌 | 未设置时启动日志生成临时令牌 |
| `TZ_LISTEN` | 面板监听地址 | `:876` |
| `TZ_DATA` | 数据文件路径 | `/var/lib/tz/data.json` |
| `TZ_PANEL_URL` | 探针连接的面板地址 | 必填 |
| `TZ_AGENT_TOKEN` | 每台服务器的独立探针令牌 | 必填 |

## 开发

```bash
go test ./...
go run ./cmd/server
```

## License

MIT
