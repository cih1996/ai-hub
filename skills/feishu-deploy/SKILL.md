---
name: "飞书应用部署"
description: "飞书自建应用全自动部署。当需要在飞书开放平台创建应用、配置机器人、设置事件订阅、开通权限并发布版本时触发。通过 Chrome MCP 操作浏览器完成全流程。"
---

# 飞书自建应用 — 自动部署手册

你是飞书应用的自动部署专家。通过 Chrome MCP 操作浏览器，在飞书开放平台完成自建应用的创建和配置。

## 前置检查

开始前必须确认以下条件，缺一不可：

1. **Chrome 调试模式**：执行以下命令检查 Chrome DevTools 协议是否可用
   ```bash
   curl -s --max-time 3 http://localhost:9222/json/version
   ```
   - 返回 JSON（含 `webSocketDebuggerUrl`）→ Chrome 调试模式正常，继续
   - 连接失败或超时 → 自动启动 Chrome 调试模式：
     - macOS：
       ```bash
       /Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome --remote-debugging-port=9222 --user-data-dir="/tmp/chrome-debug" --no-first-run &
       ```
     - Windows：
       ```bash
       start chrome --remote-debugging-port=9222 --user-data-dir="%TEMP%\chrome-debug" --no-first-run
       ```
     - Linux：
       ```bash
       google-chrome --remote-debugging-port=9222 --user-data-dir="/tmp/chrome-debug" --no-first-run &
       ```
   - 启动后等待 3 秒，再次验证 `curl -s --max-time 3 http://localhost:9222/json/version`
   - 仍然失败 → **停止**，告诉用户：「Chrome 调试模式启动失败，请手动执行上述命令后重试。」

2. **浏览器登录状态**：用 `navigate_page` 打开 `https://open.larksuite.com/app?lang=zh-CN`，然后 `take_snapshot` 检查是否已登录
   - 如果看到登录页面或未登录提示，**立即停止**，告诉用户：「请先在浏览器中登录飞书开放平台（https://open.larksuite.com），登录完成后继续对话即可。」
   - 用户回复后重新检查登录状态，确认登录后再继续

3. **必要参数**（由调用方提供）：
   - `app_name`：应用名称
   - `app_desc`：应用描述
   - `webhook_url`（可选）：如果调用方提供了 webhook URL 则直接使用，否则在「网络环境检测」步骤自动获取

## 网络环境检测

> 如果调用方已提供 `webhook_url`，跳过本步骤，直接进入部署流程。

本步骤自动检测网络环境，确定飞书 webhook 回调地址。

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

- 返回 `200` → 公网可达，使用 `http://<公网IP>:<端口>/api/v1/webhook/feishu` 作为 webhook URL
- 其他结果 → 公网不可达，进入内网穿透引导

### 4. 内网穿透引导（公网不可达时）

告诉用户：

> 检测到你的网络无法从外部直接访问，需要做内网穿透。别担心，跟着下面的步骤操作就行，很简单。

**方案 A：Cloudflare Tunnel（推荐，免费无需注册）**

引导用户执行：
```bash
# 安装（macOS）
brew install cloudflared
# 安装（Linux）
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /usr/local/bin/cloudflared && chmod +x /usr/local/bin/cloudflared
```

```bash
# 启动穿透（一条命令搞定）
cloudflared tunnel --url http://localhost:<端口>
```

告诉用户：「运行后会看到一行 `https://xxx.trycloudflare.com` 的地址，把它复制发给我就行。」

拿到地址后，webhook URL = `<用户提供的地址>/api/v1/webhook/feishu`

**方案 B：用户自行提供**

如果用户已有公网域名、反向代理或其他穿透服务，直接让用户提供可访问的 URL。

确定 webhook URL 后，继续部署流程。

## 部署流程

### 第一步：创建应用

1. 打开 `https://open.larksuite.com/app?lang=zh-CN`
2. `take_snapshot` 找到「创建企业自建应用」按钮并 `click`
3. 在弹窗中填写：
   - 应用名称：`fill` 输入 `app_name`
   - 应用描述：`fill` 输入 `app_desc`
   - 图标：选择一个预设图标即可（蓝色背景 + 机器人图标）
4. 点击「创建」
5. `take_snapshot` 确认跳转到应用管理页面

### 第二步：获取凭证

1. 左侧菜单点击「凭证与基础信息」
2. `take_snapshot` 记录 App ID
3. 找到 App Secret 的查看按钮（眼睛图标），`click` 查看
4. **注意：旁边的刷新图标是重置按钮，绝对不要点！**
5. 记录 App ID 和 App Secret

### 第三步：添加机器人能力

1. 左侧菜单点击「添加应用能力」
2. 找到「机器人」卡片，点击「添加」
3. `take_snapshot` 确认添加成功（左侧菜单出现「机器人」选项）

### 第四步：配置事件订阅

1. 左侧菜单点击「事件与回调」
2. 点击「订阅方式」按钮展开配置面板
3. 在请求地址输入框中 `fill` 输入 `webhook_url`
4. 点击「保存」— 飞书会发送 challenge 验证，等待验证通过
5. `take_snapshot` 确认请求地址已保存
6. 点击「添加事件」按钮
7. 在弹窗中切换到「消息与群组」分类
8. 勾选「接收消息」（`im.message.receive_v1`）
9. 点击「确认添加」
10. 如果弹出权限推荐对话框，点击「确认开通权限」

**关键：必须先保存订阅方式，添加事件按钮才会启用。**

### 第五步：开通权限

1. 左侧菜单点击「权限管理」
2. 点击「开通权限」按钮
3. 切换到「消息与群组」分类
4. 滚动找到并勾选以下权限：
   - `获取与发送单聊、群组消息`（im:message）
   - `以应用的身份发消息`（im:message:send_as_bot）
   - `接收群聊中@机器人消息事件`（im:message.group_at_msg:readonly）
5. 点击「确认开通权限」
6. `take_snapshot` 确认所有权限显示「已开通」

### 第六步：创建版本并发布

1. 点击页面顶部「创建版本」按钮
2. 填写版本号：`1.0.0`
3. 填写更新说明：`初始版本，支持消息收发`
4. 设置可用范围为「全部成员」
5. 点击「保存」提交版本
6. `take_snapshot` 确认版本状态

**注意：如果提示需要管理员审核，告知调用方等待审核通过。**

## 操作规范

- **每一步操作前后都必须 `take_snapshot`**，确认页面状态再操作
- 遇到页面加载中（按钮 disabled），等待 2-3 秒后重新 `take_snapshot`
- 遇到弹窗遮挡，先关闭弹窗再继续
- 飞书页面元素 uid 会动态变化，每次操作前必须重新 `take_snapshot` 获取最新 uid
- 搜索功能偶尔报错，改用手动滚动分类查找

## 完成后输出

部署完成后，输出以下信息：

```
✅ 飞书应用部署完成
- 应用名称：{app_name}
- App ID：{app_id}
- App Secret：{app_secret}
- Webhook 地址：{webhook_url}
- 版本：1.0.0
- 状态：已发布 / 待审核
```

将 App ID 和 App Secret 回传给任务发起方。
