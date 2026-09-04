# TZ Linux 探针面板

一个轻量、自托管的 Linux 服务器监控面板。面板和探针均为单文件程序，无需外部数据库。

## 功能

- 服务器名称、IP、运行时间、CPU、内存、存储实时状态
- 实时上传/下载速度与累计上传/下载流量
- 全局流量汇总、在线/离线状态
- 服务器编辑、删除、自定义排序
- 自定义分组
- 探针一键安装、面板内一键升级
- amd64、arm64、armv7 Linux 支持
- 管理接口令牌保护、探针独立随机令牌

## 部署面板

### Docker Compose（推荐）

```bash
git clone https://github.com/panhui/tz.git
cd tz
# 请先修改 docker-compose.yml 中的 TZ_ADMIN_TOKEN
docker compose up -d
```

打开 `http://服务器IP:8080`，输入你设置的管理令牌。

### 直接运行

从 Releases 下载对应架构的 `tz-panel-linux-*`，然后运行：

```bash
chmod +x tz-panel-linux-amd64
TZ_ADMIN_TOKEN='替换为高强度令牌' TZ_LISTEN=':8080' TZ_DATA='./data/data.json' ./tz-panel-linux-amd64
```

生产环境建议在面板前配置 HTTPS 反向代理。

## 安装探针

进入面板，点击“添加服务器”。创建成功后，复制弹窗中的一键安装命令，在目标 Linux 服务器以 root 用户执行即可。

## 升级探针

点击服务器行末尾的“↻”按钮。在线探针会在下一次心跳收到升级指令，从最新 GitHub Release 下载新版本并自动重启。

## 配置

| 环境变量 | 用途 | 默认值 |
| --- | --- | --- |
| `TZ_ADMIN_TOKEN` | 面板管理令牌 | 未设置时启动日志生成临时令牌 |
| `TZ_LISTEN` | 面板监听地址 | `:8080` |
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

