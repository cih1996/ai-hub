# AI Hub

基于 Web 的多会话 AI 聊天平台。以 Claude Code CLI 作为核心 Agent 引擎，同时支持任意 OpenAI 兼容 API 供应商。

单文件部署，前端通过 Go `embed` 打包进二进制文件。

## 架构

```
Vue3 前端 <── WebSocket/REST ──> Go 后端 <── 子进程 ──> Claude Code CLI
                                    │
                                    └──> SQLite (~/.ai-hub/ai-hub.db)
```

- 发送消息通过 HTTP API，立即返回
- AI 处理结果通过 WebSocket 实时推送
- 多标签页/多客户端通过 WS 广播同步状态
- Claude Code CLI 模式内置会话管理和上下文压缩，无需传递历史消息
- OpenAI 兼容模式自动携带完整历史上下文

## 部署

### 环境要求

- Go 1.21+（编译）
- Node.js 18+（编译前端 + Claude Code CLI 运行时依赖）
- npm（安装 Claude Code CLI）

### 编译运行

```bash
# 克隆仓库
git clone https://github.com/cih1996/ai-hub.git
cd ai-hub

# 安装前端依赖并编译
cd web && npm install && cd ..

# 一键编译（前端 + Go 二进制）
./build.sh

# 运行
./ai-hub
```

打开 `http://localhost:8080`

### 启动参数

```bash
./ai-hub [选项]

选项:
  --port <端口>    服务端口，默认 8080
  --data <目录>    数据目录，默认 ~/.ai-hub
```

### 首次使用

1. 启动后打开浏览器访问 `http://localhost:8080`
2. 系统会自动检测 Node.js / npm / Claude Code CLI 是否已安装
3. 如果 Claude Code CLI 未安装，页面顶部会提示一键安装
4. 进入 Settings 页面添加 Provider（供应商配置）
5. 开始对话

### Provider 配置说明

- **Claude Code 模式**：Model ID 包含 `claude` 关键字时自动识别，通过 CLI 子进程调用
- **OpenAI 兼容模式**：其他 Model ID 走标准 OpenAI Chat Completions API，支持任意兼容供应商

## API 接口

Base URL: `http://localhost:8080/api/v1`

### 发送消息

```
POST /chat/send
```

发送消息并触发 AI 处理，立即返回。AI 处理结果通过 WebSocket 推送。

**请求体：**

```json
{
  "session_id": 0,
  "content": "你好"
}
```

- `session_id = 0`：自动创建新会话
- `session_id > 0`：在已有会话中发送

**响应：**

```json
{
  "session_id": 1,
  "status": "started"
}
```

**错误码：**

| 状态码 | 说明 |
|--------|------|
| 400 | 内容为空 / 未配置默认 Provider |
| 404 | 会话不存在 |
| 409 | 会话正在处理中 |

### 供应商管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/providers` | 获取所有供应商 |
| POST | `/providers` | 创建供应商 |
| PUT | `/providers/:id` | 更新供应商 |
| DELETE | `/providers/:id` | 删除供应商 |

### 会话管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/sessions` | 获取会话列表（含 `streaming` 状态字段） |
| POST | `/sessions` | 创建会话 |
| GET | `/sessions/:id` | 获取单个会话 |
| PUT | `/sessions/:id` | 更新会话 |
| DELETE | `/sessions/:id` | 删除会话及其消息 |
| GET | `/sessions/:id/messages` | 获取会话的所有消息 |

### 系统状态

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/status` | 获取依赖状态（Node/npm/Claude CLI） |
| POST | `/status/retry-install` | 重试安装 Claude Code CLI |

## WebSocket

连接地址：`ws://localhost:8080/ws/chat`

WebSocket 用于接收实时推送，不用于发送消息。

### 客户端 → 服务端

| type | 说明 |
|------|------|
| `subscribe` | 订阅某个会话的流式事件，`session_id` 必填 |
| `stop` | 中断当前订阅会话的 AI 处理 |

### 服务端 → 客户端（定向推送，仅订阅者）

| type | 说明 |
|------|------|
| `streaming_status` | 订阅时会话正在处理中的确认 |
| `thinking` | AI 思考过程（Claude 扩展思考） |
| `tool_start` | 工具调用开始，含 `tool_id`、`tool_name` |
| `tool_input` | 工具调用输入流，含 `tool_id` |
| `tool_result` | 工具调用完成，含 `tool_id` |
| `chunk` | AI 回复文本流 |
| `done` | 回复完成 |
| `error` | 错误信息 |

### 服务端 → 客户端（广播，所有连接）

| type | 说明 |
|------|------|
| `session_created` | 新会话创建，`content` 为会话 JSON |
| `session_update` | 会话状态变更，`content` 为 `streaming` 或 `idle` |

## 技术栈

- **后端**：Go、Gin、SQLite、gorilla/websocket
- **前端**：Vue 3、TypeScript、Vite、Pinia、vue-router
- **AI 引擎**：Claude Code CLI（自动安装）、OpenAI 兼容 API
