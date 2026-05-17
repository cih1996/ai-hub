---
name: "一号核心手册"
description: "系统核心操作手册。当需要操作记忆库、知识库、会话、规则、笔记、定时器、系统诊断、脚本引擎时触发。AI 应自行 Read 本手册获取完整指令。"
when_to_use: "当需要操作记忆库、知识库、会话、规则、笔记、定时器、系统诊断、脚本引擎时"
---

# 一号核心手册

> Skill 名称：一号核心手册（ai-hub-core）
> 触发条件：当需要操作记忆库、会话、规则、笔记、定时器、系统诊断、脚本引擎时触发。
> 工具数量：文件系统工具 + CLI 命令 + HTTP API

---

## §1 执行原则

### 1.1 Skill 优先

- Skill 是执行协议，不是参考文档。
- 记忆库和知识库优先通过文件系统工具（Read / Grep / Glob / Edit / Write）直接操作。
- 需要脱离本地文件系统访问时，使用 HTTP API。
- 发现"执行不流畅、重复步骤、规则不足"时，优先补充/修订 Skill。

### 1.2 环境变量

CLI 命令自动继承以下环境变量（由 AI Hub 进程注入）：

| 变量 | 说明 |
|------|------|
| `AI_HUB_SESSION_ID` | 当前会话 ID |
| `AI_HUB_GROUP_NAME` | 当前团队名 |
| `AI_HUB_PORT` | 服务端口（默认 9527） |

也可通过全局 flag 覆盖：`--session <id>` / `--group <name>` / `--port <port>`

### 1.3 三层架构

所有数据遵循三层隔离：

| 层级 | 作用域 | 说明 |
|------|--------|------|
| 会话级 | `<group>/sessions/<id>/memory` | 当前会话私有 |
| 团队级 | `<group>/memory` | 同团队共享 |
| 全局级 | `memory` | 所有会话可见 |

搜索时自动合并三层结果，优先级：会话 > 团队 > 全局。

### 1.4 记录治理

- 先搜索后写入，命中则更新，避免重复。
- 每个主题一个主文件，禁止按日期命名。
- 正文写当前状态，变更追加到「变更记录」章节。
- 禁止在正文写过程叙述。

### 1.5 诊断优先

- 遇到问题先诊断再修复，禁止盲目操作。
- 优先用 API 或文件工具查询，减少直接文件操作。
- 安全重启：kill → wait → verify。

### 1.6 调度安全

- 执行类调度必须带上下文头：`[group_name|scope|target|task_id]`
- 子会话回调必须带同一 `task_id`
- 未锁定 scope/target 的执行任务不得下发

---

## §2 记忆库

### 文件路径

记忆文件存储在 `~/.ai-hub/memory/` 下，三层隔离：

| 层级 | 路径 | 说明 |
|------|------|------|
| 全局 | `~/.ai-hub/memory/*.md` | 所有会话可见 |
| 团队 | `~/.ai-hub/teams/<group>/memory/*.md` | 同团队共享 |
| 会话 | `~/.ai-hub/teams/<group>/sessions/<id>/memory/*.md` | 当前会话私有 |

### AI 操作方式

- **读取**：直接用 Read 工具读取文件
- **搜索**：用 Grep 工具搜索关键词，或用 Glob 工具按文件名匹配
- **写入**：用 Write / Edit 工具直接写入
- **删除**：用 Bash 工具 `rm` 删除文件

三层数据独立存储，AI 按需检索对应层级的目录即可。

### 写入规则

- 先搜索后写入，命中则更新，避免重复。
- 每个主题一个主文件，禁止按日期命名。
- 正文写当前状态，变更追加到「变更记录」章节。
- 禁止在正文写过程叙述。

### HTTP API

当 ai-hub 未注册为系统 MCP 时，可通过 HTTP 接口访问记忆。两组接口：

**通用文件接口**（全局级，`scope=memory`）：

```bash
# 列出全局记忆文件
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=memory"

# 读取记忆文件
curl "http://localhost:$AI_HUB_PORT/api/v1/files/content?scope=memory&path=文件名.md"

# 写入记忆文件
curl -X PUT "http://localhost:$AI_HUB_PORT/api/v1/files/content" \
  -H "Content-Type: application/json" \
  -d '{"scope": "memory", "path": "文件名.md", "content": "内容"}'

# 创建记忆文件
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files" \
  -H "Content-Type: application/json" \
  -d '{"scope": "memory", "path": "文件名.md", "content": "内容"}'

# 删除记忆文件
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/files" \
  -H "Content-Type: application/json" \
  -d '{"scope": "memory", "path": "文件名.md"}'
```

**隔离记忆接口**（支持 session / team / global 三层）：

```bash
# 列出记忆文件（自动合并三层，按优先级排序）
curl "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/list?session_id=$AI_HUB_SESSION_ID&type=memory&level=all"

# 仅列出团队级记忆
curl "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/list?scope=memory/团队名&type=memory"

# 读取记忆文件（session_id 自动推断层级）
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/read" \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "session_id": '$AI_HUB_SESSION_ID'}'

# 显式指定 scope 读取
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/read" \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "scope": "memory/团队名"}'

# 写入记忆文件
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/write" \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "content": "内容", "session_id": '$AI_HUB_SESSION_ID'}'

# 删除记忆文件
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/delete" \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "session_id": '$AI_HUB_SESSION_ID'}'
```

> **scope 格式**：`memory`（全局）/ `memory/{团队名}`（团队级）/ `{团队名}/sessions/{id}/memory`（会话级）
> **level 参数**：`all`（合并三层）/ `session` / `team` / `global`

### 结构化记忆

结构化 mem 运行时已下线，仅保留历史 JSON Schema 参考。

---

## §2.1 知识库

### 文件路径

知识文件存储在 `~/.ai-hub/knowledge/` 下，按项目/主题组织子目录：

```
~/.ai-hub/knowledge/
├── project-a/
│   └── api-spec.md
├── project-b/
│   └── architecture.md
└── misc/
    └── notes.md
```

### AI 操作方式

与记忆库一致：通过文件系统工具直接读写，无需专用 CLI。

### 前端管理

知识库提供 Web 前端供人工查看和在线编辑，地址：
```
http://localhost:$AI_HUB_PORT/knowledge
```

### HTTP API

知识库通过通用文件接口访问，`scope=knowledge`：

```bash
# 列出知识文件
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=knowledge"

# 读取知识文件
curl "http://localhost:$AI_HUB_PORT/api/v1/files/content?scope=knowledge&path=project-a/api-spec.md"

# 写入知识文件
curl -X PUT "http://localhost:$AI_HUB_PORT/api/v1/files/content" \
  -H "Content-Type: application/json" \
  -d '{"scope": "knowledge", "path": "project-a/api-spec.md", "content": "内容"}'

# 创建知识文件
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files" \
  -H "Content-Type: application/json" \
  -d '{"scope": "knowledge", "path": "project-a/api-spec.md", "content": "内容"}'

# 删除知识文件
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/files" \
  -H "Content-Type: application/json" \
  -d '{"scope": "knowledge", "path": "project-a/api-spec.md"}'
```

---

## §3 会话管理

### CLI 命令

```bash
# 列出所有会话（含状态：idle/streaming/alive）
ai-hub sessions

# 查看会话详情
ai-hub sessions 25

# 查看会话最近消息（默认 20 条）
ai-hub sessions 25 messages
ai-hub sessions 25 messages --limit 50

# 发消息（session_id=0 创建新会话）
ai-hub send 25 "你好"
ai-hub send 0 "初始化" --group "团队A" --work-dir "/path/to/project"

# 转移会话到其他团队
ai-hub sessions 25 move --group "新团队"
```

### 调度模式

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| 串行 | 逐个发送，等回调再发下一个 | 有依赖的任务链 |
| 并行 | 同时发送多个，各自回调 | 独立子任务 |
| 主从 | 主会话分发，从会话回报 | 团队协作 |

### 回调协议

- 子会话完成后必须主动回报，包含：执行结果 + 关键变更 + 是否需后续操作
- 异步派发：发完消息即继续，禁止轮询等待

---

## §4 团队管理

### CLI 命令

```bash
# 列出所有团队
ai-hub groups

# 查看团队详情（含会话列表）
ai-hub groups "团队名"

# 创建团队
ai-hub groups create "团队名"
ai-hub groups create "团队名" --desc "团队描述"

# 删除团队（仅限空团队）
ai-hub groups delete "团队名"
```

### API 接口

```bash
# 列出团队
GET /api/v1/groups

# 创建团队
POST /api/v1/groups
{"name": "团队名", "description": "描述"}

# 删除团队
DELETE /api/v1/groups/:name

# 转移会话到其他团队
PUT /api/v1/sessions/:id/group
{"group_name": "新团队名"}
```

---

## §5 规则管理

### CLI 命令

```bash
# 读取当前会话规则（自动使用 AI_HUB_SESSION_ID）
ai-hub rules get

# 读取指定会话规则
ai-hub rules get 25

# 写入会话规则
ai-hub rules set 25 --content "你是技术维护工程师"

# 删除会话规则
ai-hub rules delete 25
```

### 三层规则体系

| 层级 | 路径 | 说明 |
|------|------|------|
| 全局 | `~/.ai-hub/rules/CLAUDE.md` | 模板文件，支持 `{{VAR}}` 占位符 |
| 团队 | `~/.ai-hub/teams/<group>/rules/*.md` | 团队私有，按目录文件读取 |
| 会话 | `~/.ai-hub/session-rules/{id}.md` | 每会话角色定义，优先级最高 |

规则在进程启动时注入，修改后需重启进程生效。

---

## §6 笔记管理

### CLI 命令

```bash
# 列出所有笔记
ai-hub notes list

# 读取笔记
ai-hub notes read todo.md

# 写入笔记
ai-hub notes write todo.md --content "# TODO\n- item 1"

# 删除笔记
ai-hub notes delete todo.md
```

笔记存储在 `~/.ai-hub/notes/`，AI 可通过 CLI 或文件系统工具操作。

---

## §7 定时器

### CLI 命令

```bash
# 列出所有定时器
ai-hub triggers list

# 按会话筛选
ai-hub triggers list --session 25

# 创建定时器（max-fires: -1=无限, 1=一次, N=N次）
ai-hub triggers create --session 25 --time "09:00:00" --content "早报" --max-fires -1

# 更新定时器
ai-hub triggers update 1 --content "新指令"
ai-hub triggers update 1 --time "10:00:00" --enabled false

# 删除定时器
ai-hub triggers delete 1
```

### 时间格式

| 格式 | 示例 | 说明 |
|------|------|------|
| 精确时间 | `2026-03-06 09:00:00` | 一次性触发 |
| 每日时间 | `09:00:00` | 每天触发 |
| 间隔 | `1h30m` | 周期触发 |

### 指令编写原则

- 指令必须自包含，不依赖上下文
- 包含完整路径和错误处理
- 所有时间使用 UTC+8

---

## §7.1 事件钩子

### 事件类型

| 事件 | 中文名 | 触发时机 | 支持的条件 |
|------|--------|----------|------------|
| `message.received` | 收到消息 | 任意会话收到用户消息 | `content_match` |
| `message.sent` | AI 回复完成 | AI 完成流式回复后 | `content_match` |
| `message.count` | 消息计数 | 与收到消息同时触发 | `count_gt` |
| `session.created` | 会话创建 | 新会话首次发消息 | 无 |
| `session.error` | 会话错误 | AI 流式输出报错 | `content_match` |
| `session.compressed` | 上下文压缩 | 自动压缩完成后 | 无 |

### 条件格式

- `content_match:关键词1|关键词2` — 内容命中任一关键词/正则即触发
- `count_gt:N` — 消息数超过 N 时触发
- 留空 = 无条件触发

### 模板变量（Payload 中使用）

- `{source_session_id}` — 触发源会话 ID
- `{event_type}` — 事件类型
- `{content}` — 消息内容或错误摘要（截断 500 字）
- `{message_count}` — 当前消息计数

### API

```bash
# 列出钩子
curl "http://localhost:$AI_HUB_PORT/api/v1/hooks"

# 按事件类型筛选
curl "http://localhost:$AI_HUB_PORT/api/v1/hooks?event=message.received"

# 创建钩子
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/hooks" \
  -H "Content-Type: application/json" \
  -d '{
    "event": "message.received",
    "condition": "content_match:紧急|urgent",
    "target_session": 5,
    "payload": "[紧急消息] 来自会话 {source_session_id}:\n{content}",
    "enabled": true
  }'

# 启停钩子
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/hooks/1/enable"
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/hooks/1/disable"
```

---

## §8 系统诊断

### CLI 命令

```bash
# 版本信息
ai-hub version

# 系统状态（服务 + 进程池）
ai-hub status
```

### 常见故障排查

| 症状 | 检查命令 | 处理 |
|------|----------|------|
| 记忆搜索无结果 | `ai-hub list --level <level>` | 改用文件列表和正文检查命中内容 |
| 会话无响应 | `ai-hub sessions <id>` | 检查进程状态 |
| 规则未生效 | `ai-hub rules get <id>` | 确认规则内容，重启进程 |
| 子agent模型报错 | 查看日志中 `[proxy]` 行的 model 字段 | 确认 ANTHROPIC_MODEL 环境变量是否正确传递 |

### 日志分析

```bash
# 查看最近日志
tail -50 ~/.ai-hub/logs/ai-hub.log

# 搜索错误
grep -i "error\|forbidden\|timeout" ~/.ai-hub/logs/ai-hub.log | tail -20

# 查看代理层请求（含 model 字段，用于排查子agent问题）
grep "\[proxy\]" ~/.ai-hub/logs/ai-hub.log | tail -20

# 查看钩子触发记录
grep "\[hooks\]" ~/.ai-hub/logs/ai-hub.log | tail -20
```

### 上下文详情（Raw Request 诊断）

右上角「上下文详情」可查看最后一次发给模型的完整请求数据：

- **Messages Tab**：结构化展示消息列表，支持 tool_use/tool_result 跳转
- **Raw Tab**：完整 JSON 请求体（含 model、messages、tools 等），显示 KB 大小
- **全量日志 Tab**：会话历史归档，支持搜索和导航

**持久化说明**：Raw 请求体已持久化到 `session_raw_requests.proxy_body` 列，重启后仍可查看。

```bash
# API 获取最后一次请求数据
curl "http://localhost:$AI_HUB_PORT/api/v1/sessions/$AI_HUB_SESSION_ID/last-request"

# 响应包含：system_prompt, query, anthropic_request（完整请求体）, context_count 等
```

---

## §9 静态资源挂载

### CLI 命令

```bash
# 挂载本地目录
ai-hub mount ~/Pictures --alias media
ai-hub mount /tmp/screenshots --alias shots

# 列出所有挂载
ai-hub mount list

# 移除挂载
ai-hub mount remove media
```

### 访问方式

挂载后，文件可通过 HTTP 访问：
```
http://localhost:<port>/static/<alias>/<filename>
```

### 在 AI 回复中使用

```html
<img src="http://localhost:9527/static/media/test.png">
<video src="http://localhost:9527/static/media/demo.mp4" controls></video>
```

### 使用场景

- AI 生成图片后展示给用户
- 截图分享
- 本地媒体文件预览

---

## §10 脚本引擎

### 规范

多步重复操作（≥3 步）必须脚本化，禁止逐步单条交互。

### 脚本仓库

```
~/.ai-hub/scripts/
├── INDEX.md          # 脚本索引（必须维护）
├── shell/            # 系统运维脚本
├── browser/          # Chrome MCP 自动化脚本
└── api/              # HTTP 批量请求脚本
```

### 执行流程

1. 查 INDEX.md 是否有可复用脚本
2. 有 → 传参执行
3. 无 → 新建脚本 → 执行 → 更新 INDEX.md

### 脚本规范

- 命名：`<动作>-<对象>.<扩展名>`（如 `upgrade-production.sh`）
- Shell：`set -euo pipefail`
- JS：try/catch
- Python：sys.exit()
- 禁止硬编码 URL/端口/ID，全部参数化
- 失败时修复脚本重跑，禁止回退到手动操作
