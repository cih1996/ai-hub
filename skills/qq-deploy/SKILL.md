---
name: "QQ频道部署"
description: "QQ Bot 全流程部署引导。当需要安装 NapCat、配置 QQ 机器人、对接 AI Hub 时触发。引导用户完成安装、登录、WebUI 配置和频道创建。"
---

# QQ Bot 部署 — 引导手册

你是 QQ Bot 的部署引导专家。通过对话引导用户完成 NapCat 安装、QQ 登录、网络配置和 AI Hub 频道对接。

## 前置检查

开始前确认以下信息：

1. **操作系统**：询问用户使用的系统（macOS / Linux / Windows / Docker）
2. **必要参数**（由调用方提供或询问用户）：
   - `session_id`：要绑定的 AI Hub 会话 ID
   - `webhook_url`（可选）：如果调用方已提供则直接使用，否则在「网络环境检测」步骤自动获取

## 安装 NapCat

根据用户操作系统引导安装：

### macOS

使用 NapCat-Mac-Installer：

```
安装地址：https://github.com/NapNeko/NapCat-Mac-Installer
```

告诉用户：「打开上面的链接，按 README 下载安装即可。安装完成后告诉我。」

### Linux

```bash
curl -o napcat.sh https://nclatest.znin.net/NapNeko/NapCat-Installer/main/script/install.sh && bash napcat.sh
```

告诉用户：「复制上面的命令到终端执行，按提示完成安装。」

### Windows

```
下载地址：https://github.com/NapNeko/NapCatQQ/releases
下载 NapCat.Shell.Windows.OneKey.zip，解压后运行 launcher.bat
```

### Docker

```
仓库地址：https://github.com/NapNeko/NapCat-Docker
按 README 的 docker-compose 示例启动即可。
```

## 启动与登录

告诉用户：

> 启动 NapCat 后，终端会显示一个二维码，用手机 QQ 扫码登录即可。登录成功后终端会提示连接成功。

等待用户确认登录成功后继续。

## 网络环境检测

> 如果调用方已提供 `webhook_url`，跳过本步骤，直接进入 WebUI 配置。

本步骤自动检测网络环境，确定 QQ webhook 回调地址。

### 1. 获取 AI Hub 端口

```bash
echo $AI_HUB_PORT
```
如果为空，默认使用 `8080`。

### 2. 获取公网 IP

```bash
curl -s --max-time 5 ifconfig.me
```

### 3. 检测公网可达性

```bash
curl -s --max-time 5 -o /dev/null -w "%{http_code}" http://<公网IP>:<端口>/api/v1/version
```

- 返回 `200` → 公网可达，使用 `http://<公网IP>:<端口>/api/v1/webhook/qq` 作为 webhook URL
- 其他结果 → 公网不可达，进入内网穿透引导

### 4. 内网穿透引导（公网不可达时）

告诉用户：

> 检测到你的网络无法从外部直接访问，需要做内网穿透。别担心，跟着下面的步骤操作就行。

**方案 A：Cloudflare Tunnel（推荐，免费无需注册）**

引导用户执行：
```bash
# 安装（macOS）
brew install cloudflared
# 安装（Linux）
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /usr/local/bin/cloudflared && chmod +x /usr/local/bin/cloudflared
```

```bash
# 启动穿透
cloudflared tunnel --url http://localhost:<端口>
```

告诉用户：「运行后会看到一行 `https://xxx.trycloudflare.com` 的地址，把它复制发给我就行。」

拿到地址后，webhook URL = `<用户提供的地址>/api/v1/webhook/qq`

**方案 B：用户自行提供**

如果用户已有公网域名或其他穿透服务，直接让用户提供可访问的 URL。

确定 webhook URL 后，继续 WebUI 配置。

## WebUI 配置

NapCat 启动后自带 WebUI 管理界面。

告诉用户：

> NapCat 启动日志中会显示 WebUI 地址和 token，类似：
> `WebUI is running at http://0.0.0.0:6099`
> `WebUI token: xxxx`
>
> 请打开 http://localhost:6099 ，用日志中的 token 登录。登录后告诉我。

等待用户确认登录 WebUI 后，引导配置两项网络设置：

### 1. HTTP 服务端（供 AI Hub 调用发消息）

告诉用户：

> 在 WebUI 的「网络配置」中，添加一个 **HTTP 服务端**：
> - 启用：开
> - 端口：`3055`（默认即可，也可自定义）
> - 设置一个 token（用于鉴权，随便写一个字符串）
> - 保存
>
> 记下这个端口和 token，等下要用。比如地址就是 `http://<NapCat所在IP>:3055`。

### 2. WebSocket 服务端（供 AI Hub 连接收消息）

告诉用户：

> 继续添加一个 **WebSocket 服务端**：
> - 启用：开
> - 端口：`3056`（或自定义）
> - token：跟 HTTP 服务端用同一个即可
> - 保存
>
> 记下这个端口，比如地址就是 `ws://<NapCat所在IP>:3056`。

### 3. HTTP 客户端（可选，本地部署时使用）

> 如果 NapCat 和 AI Hub 在同一台机器上，也可以添加一个 **HTTP 客户端**主动推送消息：
> - 启用：开
> - URL：`{webhook_url}`
> - 保存
>
> 远程部署时不需要配置此项，AI Hub 会通过 WebSocket 主动连接 NapCat 收消息。

## 在 AI Hub 创建频道

WebUI 配置完成后，引导用户在 AI Hub 创建 QQ 频道：

告诉用户：

> 现在打开 AI Hub 的「通讯频道」页面，点击「新建频道」：
> - 名称：随便起，比如「QQ Bot」
> - 平台：选择 `qq`
> - NapCat HTTP 地址：填 `http://<NapCat所在IP>:3055`（发消息用）
> - NapCat WebSocket 地址：填 `ws://<NapCat所在IP>:3056`（收消息用）
> - Token：填刚才在 WebUI 设置的 token
> - 绑定会话：选择要接收 QQ 消息的会话（ID: {session_id}）
> - 点击「创建」并启用频道
>
> 创建后 AI Hub 会自动通过 WebSocket 连接 NapCat 接收消息。

## 验证

引导用户验证整个链路：

1. 告诉用户：「现在用手机 QQ 给 Bot 发一条消息试试。」
2. 检查 AI Hub 绑定的会话是否收到了消息
3. 确认 AI 能通过 NapCat API 回复消息：

```bash
curl -s -X POST 'http://localhost:3000/send_private_msg' \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{"user_id": <发送者QQ号>, "message": "你好，AI Hub 已连接！"}'
```

如果验证失败，排查顺序：
- NapCat 是否在运行（进程是否存在）
- WebUI 中 HTTP 服务端和客户端是否都已启用
- Token 是否一致（AI Hub 频道配置 vs NapCat WebUI）
- Webhook URL 是否可达（curl 测试）

## 完成后输出

部署完成后，输出以下信息：

```
✅ QQ 频道部署完成
- QQ 号：<登录的QQ号>
- NapCat 地址：<napcat_url>
- Webhook 地址：<webhook_url>
- 绑定会话：#<session_id>
- 状态：已启用
```

将 NapCat 地址和 Token 回传给任务发起方。
