---
name: "QQ频道部署"
description: "QQ Bot 全流程部署引导。当需要安装 NapCat、配置 QQ 机器人、对接 AI Hub 时触发。引导用户完成安装、登录、WebUI 配置和频道创建。"
---

# QQ Bot 部署 — 自动部署手册

你是 QQ Bot 的自动部署专家。全程自主执行，只在需要用户扫码登录时暂停。

## 核心原则

- **AI 自主执行所有命令**，不要让用户复制粘贴命令
- **唯一需要用户操作的环节**：手机 QQ 扫码登录
- 遇到下载失败自动切换备用源，3 次失败再求助用户
- 每步操作后验证结果，失败立即排查

## 前置检查

自动检测，不要问用户：

```bash
OS=$(uname -s)
ARCH=$(uname -m)
echo "系统: $OS, 架构: $ARCH"
```

必要参数（由调用方提供或询问用户）：
- `session_id`：要绑定的 AI Hub 会话 ID
- `webhook_url`（可选）：调用方已提供则直接使用

## 网络与代理检测

自动执行，不要让用户操作：

```bash
for port in 7890 7891 1080 1081 10808 10809; do
  if curl -s --max-time 2 --proxy http://127.0.0.1:$port https://www.google.com > /dev/null 2>&1; then
    echo "检测到代理: 127.0.0.1:$port"
    export http_proxy=http://127.0.0.1:$port
    export https_proxy=http://127.0.0.1:$port
    break
  fi
done
```

### 国内加速源

所有涉及 GitHub 下载的地方，按顺序尝试：
1. 首选：`https://ghfast.top/` 前缀
2. 备选：`https://gh-proxy.com/`、`https://mirror.ghproxy.com/`
3. 全部失败再直连

## 安装 NapCat

根据检测到的操作系统，**AI 自主执行安装命令**：

### macOS（Darwin）

```bash
# 检查是否已安装
if [ -d "/Applications/NapCat" ] || command -v napcat &>/dev/null; then
  echo "NapCat 已安装"
else
  # 通过 NapCat-Mac-Installer 安装
  # 1. 下载安装器
  cd /tmp
  curl -L -o NapCat-Mac-Installer.zip "https://ghfast.top/https://github.com/NapNeko/NapCat-Mac-Installer/releases/latest/download/NapCat-Mac-Installer.zip" || \
  curl -L -o NapCat-Mac-Installer.zip "https://gh-proxy.com/https://github.com/NapNeko/NapCat-Mac-Installer/releases/latest/download/NapCat-Mac-Installer.zip"
  # 2. 解压并执行安装
  unzip -o NapCat-Mac-Installer.zip
fi
```

macOS 安装器可能需要 GUI 交互，如果命令行安装失败，告诉用户：
「NapCat macOS 版需要通过安装器安装，请打开 https://ghfast.top/https://github.com/NapNeko/NapCat-Mac-Installer 下载安装。安装完成后告诉我。」

### Linux

AI 直接执行安装脚本：

```bash
# 国内加速版
curl -fsSL -o /tmp/napcat.sh https://ghfast.top/https://raw.githubusercontent.com/NapNeko/NapCat-Installer/main/script/install.sh && \
  bash /tmp/napcat.sh
```

如果加速源失败，自动回退：
```bash
curl -fsSL -o /tmp/napcat.sh https://nclatest.znin.net/NapNeko/NapCat-Installer/main/script/install.sh && \
  bash /tmp/napcat.sh
```

### Windows（MINGW/MSYS）

Windows 环境下 AI 自主下载：

```bash
# 获取最新版本号
LATEST=$(curl -sI https://github.com/NapNeko/NapCatQQ/releases/latest | grep -i location | grep -oP 'v[\d.]+')
# 下载
curl -L -o /tmp/NapCat.Shell.zip "https://ghfast.top/https://github.com/NapNeko/NapCatQQ/releases/download/${LATEST}/NapCat.Shell.zip"
```

下载完成后告诉用户：「NapCat 已下载到 /tmp/NapCat.Shell.zip，请解压后双击 launcher.bat 启动。」

> 如果用户主动提到想用 Docker，可以给链接 https://github.com/NapNeko/NapCat-Docker ，但不主动推荐。

## 启动与登录（唯一需要用户操作的步骤）

启动 NapCat 后，告诉用户：

> NapCat 已启动，终端会显示一个二维码。请用手机 QQ 扫码登录。登录成功后告诉我。

**这是整个流程中唯一需要用户手动操作的步骤。** 等待用户确认登录成功后继续。

## Webhook 网络环境检测

> 如果调用方已提供 `webhook_url`，跳过本步骤。

AI 自动检测网络环境：

```bash
# 1. 获取端口
PORT=${AI_HUB_PORT:-8080}
# 2. 获取公网 IP
PUBLIC_IP=$(curl -s --max-time 5 ifconfig.me)
# 3. 检测公网可达性
HTTP_CODE=$(curl -s --max-time 5 -o /dev/null -w "%{http_code}" http://${PUBLIC_IP}:${PORT}/api/v1/version)
```

- `HTTP_CODE=200` → 使用 `http://${PUBLIC_IP}:${PORT}/api/v1/webhook/qq`
- 其他 → 公网不可达，引导内网穿透（这一步可能需要用户配合）

### 内网穿透（公网不可达时）

优先尝试自动安装 cloudflared：

```bash
# macOS
brew install cloudflared 2>/dev/null || \
  curl -L -o /usr/local/bin/cloudflared https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-amd64 && chmod +x /usr/local/bin/cloudflared
# Linux
curl -L -o /usr/local/bin/cloudflared https://ghfast.top/https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 && chmod +x /usr/local/bin/cloudflared
```

启动穿透后需要用户提供生成的地址：
「运行后会看到一行 `https://xxx.trycloudflare.com` 的地址，把它发给我。」

webhook URL = `<地址>/api/v1/webhook/qq`

## WebUI 配置

NapCat 启动后自带 WebUI（默认 http://localhost:6099）。

告诉用户：

> NapCat 启动日志中会显示 WebUI 地址和 token。请在浏览器打开 http://localhost:6099 ，用日志中的 token 登录后告诉我。

等待用户确认登录 WebUI 后，引导配置：

### 1. HTTP 服务端（供 AI Hub 调用发消息）

> 在 WebUI「网络配置」中添加 **HTTP 服务端**：
> - 启用：开，端口：`3055`
> - 设置 token（如 `mytoken123`）
> - 保存

### 2. WebSocket 服务端（供 AI Hub 连接收消息）

> 继续添加 **WebSocket 服务端**：
> - 启用：开，端口：`3056`
> - token 与 HTTP 服务端相同
> - 保存

### 3. HTTP 客户端（可选，本地部署时使用）

> NapCat 和 AI Hub 同机时，可添加 **HTTP 客户端**：
> - URL：`{webhook_url}`
> - 远程部署不需要此项

## 在 AI Hub 创建频道

AI 通过 API 自动创建频道，不需要用户手动操作：

```bash
curl -s -X POST "http://localhost:${AI_HUB_PORT:-8080}/api/v1/channels" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "QQ Bot",
    "platform": "qq",
    "enabled": true,
    "session_id": <session_id>,
    "config": "{\"napcat_http_url\":\"http://<NapCat地址>:3055\",\"napcat_ws_url\":\"ws://<NapCat地址>:3056\",\"token\":\"<token>\"}"
  }'
```

创建后 AI Hub 会自动通过 WebSocket 连接 NapCat。

## 验证

AI 自动验证连接状态：

```bash
# 检查 AI Hub 日志中是否有 WS 连接成功记录
tail -20 ~/.ai-hub/logs/ai-hub.log | grep -i "qq-ws.*connected"
```

然后告诉用户：「请用手机 QQ 给 Bot 发一条消息测试。」

验证失败排查顺序：
1. NapCat 是否在运行
2. WebUI 中 HTTP/WS 服务端是否启用
3. Token 是否一致
4. 端口是否被防火墙拦截
5. AI Hub 日志：`tail -50 ~/.ai-hub/logs/ai-hub.log`

## 完成后输出

```
✅ QQ 频道部署完成
- QQ 号：<登录的QQ号>
- NapCat HTTP 地址：<napcat_http_url>
- NapCat WebSocket 地址：<napcat_ws_url>
- Token：<token>
- 绑定会话：#<session_id>
- 状态：已启用
```

将 NapCat HTTP 地址、WebSocket 地址和 Token 回传给任务发起方。
