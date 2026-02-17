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

- Node.js 18+（Claude Code CLI 运行时依赖）
- npm

### 直接部署（推荐）

从 [Releases](https://github.com/cih1996/ai-hub/releases) 下载对应平台的二进制文件：

| 平台 | 文件 |
|------|------|
| macOS (Apple Silicon) | `ai-hub-darwin-arm64` |
| Linux (x86_64) | `ai-hub-linux-amd64` |
| Windows (x86_64) | `ai-hub-windows-amd64.exe` |

```bash
# macOS / Linux
chmod +x ai-hub-*
./ai-hub-darwin-arm64   # macOS
./ai-hub-linux-amd64    # Linux

# Windows
ai-hub-windows-amd64.exe
```

打开 `http://localhost:8080`

### 从源码编译

需要额外安装：Go 1.21+、Node.js 18+

```bash
git clone https://github.com/cih1996/ai-hub.git
cd ai-hub
cd web && npm install && cd ..
make          # 编译当前平台
make release  # 交叉编译所有平台（macOS/Linux/Windows）
```

编译产物在 `dist/` 目录下。

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
  "content": "你好",
  "work_dir": "/path/to/project"
}
```

- `session_id = 0`：自动创建新会话
- `session_id > 0`：在已有会话中发送
- `work_dir`：可选，工作目录路径。CLI 将在此目录下运行，空值则使用用户 home 目录

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
| GET | `/sessions` | 获取会话列表（含 `streaming` 状态、`work_dir` 字段） |
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

### 项目级规则管理

操作指定工作目录下 `{work_dir}/.claude/` 的规则文件，不走模板系统。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/project-rules?work_dir=xxx` | 列出 CLAUDE.md + rules/*.md |
| GET | `/project-rules/content?work_dir=xxx&path=xxx` | 读取规则文件内容 |
| PUT | `/project-rules/content` | 写入规则文件（body: `work_dir`, `path`, `content`） |

### 定时触发器

系统每分钟检查触发器，到时间自动向指定会话发送指令。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/triggers` | 获取所有触发器（可选 `?session_id=N` 过滤） |
| POST | `/triggers` | 创建触发器（必填：`session_id`、`content`、`trigger_time`） |
| PUT | `/triggers/:id` | 部分更新触发器（只传需要改的字段，不会清空未传字段） |
| DELETE | `/triggers/:id` | 删除触发器 |

`trigger_time` 支持三种格式：
- `"2026-02-17 10:30:00"` — 精确日期时间，只触发一次
- `"10:30:00"` — 每天固定时间
- `"1h30m"` — 固定间隔

`max_fires`: `-1` 表示无限触发，`0` 或不传时默认为 `1`。

PUT 接口支持 partial update，可更新的字段：`content`、`trigger_time`、`max_fires`、`enabled`。未传的字段保持原值不变。

会话列表接口返回 `has_triggers` 字段标识该会话是否关联了触发器。

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
