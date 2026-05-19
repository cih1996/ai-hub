# 一号核心手册 · HTTP

> 本文档记录 AI Hub 已注册的 HTTP 接口。
> 基础前缀统一为：`http://localhost:$AI_HUB_PORT/api/v1`
> 真实路由以 `main.go` 注册结果为准。
> 说明中若与某些源码注释冲突，以 `main.go` 为准，例如 `transfer` 实际注册的是 `/transfer/status/:id` 与 `/transfer/delete/:id`。

---

## 1. 总体原则

- 大多数 CLI 命令都直接映射到本文件中的接口。
- 记忆库、知识库、笔记、规则等文件型数据，能直接用文件系统工具时优先本地直读直写。
- 需要跨进程、跨机器或复用后端校验/状态管理时，再走 HTTP。

---

## 2. 与 CLI 的关系

最常用映射如下：

| CLI | HTTP |
|-----|------|
| `search/list/read/write/edit/delete` | `/files/scoped/*` |
| `sessions` / `send` | `/sessions*` / `/chat/send` |
| `errors` | `/sessions/:id/errors` / `/stats/errors` |
| `groups` | `/groups*` |
| `rules`（会话级） | `/session-rules/:id` |
| `triggers` | `/triggers*` |
| `hooks` | `/hooks*` |
| `services` | `/services*` |
| `skills` | `/skills*` |
| `schemas` | `/schemas*` |
| `status` / `version` | `/status` / `/version` |
| `reload` | `/reload/config` / `/reload/skills` |
| `mount` | `/mounts*` + `/static/...` |
| `transfer` | `/transfer/*` |
| `changelog` | `/changelog` / `/changelog/rollback` |

例外：

- `rules --level team|global` 直接读写文件，不走 HTTP
- `notes` 专用 CLI 已移除；笔记通过文件系统或通用 `/files` 接口访问
- `daemon` 只有探活/关停时会用到 `/version` 或 `/shutdown`

---

## 3. Files / Memory / Knowledge / Notes

### 3.1 通用文件接口

```bash
# 列出文件
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=memory"
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=knowledge"
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=notes"

# 读取文件
curl "http://localhost:$AI_HUB_PORT/api/v1/files/content?scope=knowledge&path=project-a/api-spec.md"

# 覆盖写入
curl -X PUT "http://localhost:$AI_HUB_PORT/api/v1/files/content" \
  -H "Content-Type: application/json" \
  -d '{"scope":"notes","path":"todo.md","content":"# TODO\n- item 1"}'

# 创建
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files" \
  -H "Content-Type: application/json" \
  -d '{"scope":"knowledge","path":"project-a/api-spec.md","content":"内容"}'

# 删除
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/files" \
  -H "Content-Type: application/json" \
  -d '{"scope":"notes","path":"todo.md"}'
```

相关接口：

- `GET /files`
- `GET /files/content`
- `PUT /files/content`
- `POST /files`
- `DELETE /files`
- `GET /files/variables`
- `GET /files/default`

### 3.2 记忆库三层接口

```bash
# 列表（按 scope 或 session 自动推断）
curl "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/list?scope=memory/团队名&type=memory"
curl "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/list?session_id=$AI_HUB_SESSION_ID&type=memory&level=all"

# 读取
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/read" \
  -H "Content-Type: application/json" \
  -d '{"file_name":"项目.md","session_id":25,"type":"memory"}'

# 写入
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/write" \
  -H "Content-Type: application/json" \
  -d '{"file_name":"项目.md","content":"正文","session_id":25,"type":"memory"}'

# 删除
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/files/scoped/delete" \
  -H "Content-Type: application/json" \
  -d '{"file_name":"项目.md","session_id":25,"type":"memory"}'
```

相关接口：

- `GET /files/scoped/list`
- `POST /files/scoped/read`
- `POST /files/scoped/write`
- `POST /files/scoped/delete`

说明：

- `scope` 支持 `memory` / `memory/<group>` / `<group>/sessions/<id>/memory`
- `type=memory` 是 memory CLI 使用方式

### 3.3 Project Rules

相关接口：

- `GET /project-rules`
- `GET /project-rules/content`
- `PUT /project-rules/content`

---

## 4. Sessions / Chat / Errors / Health

### 4.1 会话基础

相关接口：

- `GET /sessions`
- `POST /sessions`
- `GET /sessions/:id`
- `PUT /sessions/:id`
- `DELETE /sessions/:id`
- `PUT /sessions/:id/group`
- `PUT /sessions/:id/provider`

### 4.2 消息与日志

相关接口：

- `GET /sessions/:id/messages`
- `GET /sessions/:id/messages/:msg_id`
- `DELETE /sessions/:id/messages`
- `GET /sessions/:id/logs`
- `GET /sessions/:id/last-request`
- `GET /sessions/:id/context-usage`

### 4.3 发消息

CLI `ai-hub send` 实际命中：

```bash
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/chat/send" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": 25,
    "content": "你好"
  }'
```

新建会话时可用 `session_id=0`：

```bash
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/chat/send" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": 0,
    "content": "初始化",
    "group_name": "团队A",
    "work_dir": "/path/to/project"
  }'
```

### 4.4 错误与健康度

相关接口：

- `GET /sessions/:id/errors`
- `GET /stats/errors`
- `GET /sessions/:id/health`
- `PUT /sessions/:id/health`
- `POST /sessions/:id/reset`

示例：

```bash
# 错误列表
curl "http://localhost:$AI_HUB_PORT/api/v1/sessions/25/errors?level=error"

# 错误统计
curl "http://localhost:$AI_HUB_PORT/api/v1/stats/errors"

# 更新健康度
curl -X PUT "http://localhost:$AI_HUB_PORT/api/v1/sessions/25/health" \
  -H "Content-Type: application/json" \
  -d '{"health_score":"green"}'

# 重置会话
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/sessions/25/reset" \
  -H "Content-Type: application/json" \
  -d '{"confirm":true,"keep_last":20}'
```

---

## 5. Groups / Rules

### 5.1 Groups

相关接口：

- `GET /groups`
- `POST /groups`
- `GET /groups/:name`
- `PUT /groups/:name`
- `DELETE /groups/:name`

示例：

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/groups"

curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/groups" \
  -H "Content-Type: application/json" \
  -d '{"name":"团队名","description":"描述"}'
```

### 5.2 Session Rules

会话级规则相关接口：

- `GET /session-rules/:id`
- `PUT /session-rules/:id`
- `DELETE /session-rules/:id`

示例：

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/session-rules/25"

curl -X PUT "http://localhost:$AI_HUB_PORT/api/v1/session-rules/25" \
  -H "Content-Type: application/json" \
  -d '{"content":"你是技术维护工程师"}'

curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/session-rules/25"
```

---

## 6. Notes / Triggers / Hooks / Channels

### 6.1 Triggers

相关接口：

- `GET /triggers`
- `POST /triggers`
- `PUT /triggers/:id`
- `DELETE /triggers/:id`

示例：

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/triggers?session_id=25"

curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/triggers" \
  -H "Content-Type: application/json" \
  -d '{"session_id":25,"time_expr":"09:00:00","content":"早报","max_fires":-1}'
```

### 6.2 Hooks

相关接口：

- `GET /hooks`
- `GET /hooks/:id`
- `POST /hooks`
- `PUT /hooks/:id`
- `DELETE /hooks/:id`
- `POST /hooks/:id/enable`
- `POST /hooks/:id/disable`

事件类型：

- `message.received`
- `message.sent`
- `message.count`
- `session.created`
- `session.error`
- `session.compressed`

示例：

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/hooks?event=message.received"

curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/hooks" \
  -H "Content-Type: application/json" \
  -d '{
    "event":"message.received",
    "condition":"content_match:紧急|urgent",
    "target_session":5,
    "payload":"[紧急消息] 来自会话 {source_session_id}:\n{content}",
    "enabled":true
  }'
```

### 6.3 Channels 与 Webhook

相关接口：

- `GET /channels`
- `POST /channels`
- `PUT /channels/:id`
- `DELETE /channels/:id`
- `POST /webhook/feishu`
- `POST /webhook/qq`

---

## 7. Services

`services` 是旧手册漏掉的完整能力组，实际已经有完整 HTTP API。

相关接口：

- `GET /services`
- `POST /services`
- `GET /services/:id`
- `PUT /services/:id`
- `DELETE /services/:id`
- `POST /services/:id/start`
- `POST /services/:id/stop`
- `POST /services/:id/restart`
- `GET /services/:id/logs`

示例：

```bash
# 列表
curl "http://localhost:$AI_HUB_PORT/api/v1/services"

# 创建
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/services" \
  -H "Content-Type: application/json" \
  -d '{
    "name":"my-service",
    "command":"npm run dev",
    "work_dir":"/path/to/project",
    "port":3000,
    "auto_start":true
  }'

# 查看详情
curl "http://localhost:$AI_HUB_PORT/api/v1/services/1"

# 更新配置
curl -X PUT "http://localhost:$AI_HUB_PORT/api/v1/services/1" \
  -H "Content-Type: application/json" \
  -d '{
    "command":"npm run preview",
    "auto_start":false
  }'

# 启动 / 停止 / 重启
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/services/1/start"
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/services/1/stop"
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/services/1/restart"

# 日志
curl "http://localhost:$AI_HUB_PORT/api/v1/services/1/logs?lines=200"

# 删除
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/services/1"
```

返回字段常见包括：

- `id`
- `name`
- `command`
- `work_dir`
- `port`
- `log_path`
- `pid`
- `status`
- `auto_start`

---

## 8. Skills / MCP / Schemas

### 8.1 Skills

相关接口：

- `GET /skills`
- `GET /skills/:name`
- `POST /skills`
- `PUT /skills/:name`
- `DELETE /skills/:name`
- `POST /skills/toggle`
- `GET /skill-export/:name`
- `POST /skill-import/preview`
- `POST /skill-import`

### 8.2 MCP

相关接口：

- `GET /mcp`
- `POST /mcp/toggle`

### 8.3 Schemas

相关接口：

- `GET /schemas`
- `GET /schemas/:name`
- `POST /schemas`
- `PUT /schemas/:name`
- `DELETE /schemas/:name`

示例：

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/schemas"

curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/schemas" \
  -H "Content-Type: application/json" \
  -d '{
    "name":"my-schema",
    "definition":{"type":"object","properties":{"title":{"type":"string"}}}
  }'
```

说明：

- 结构化 `mem` 运行时虽已下线，但 schema 管理接口仍存在

---

## 9. Status / System / Reload / Version

相关接口：

- `GET /status`
- `POST /status/retry-install`
- `GET /version`
- `GET /system/init-status`
- `POST /system/install-dep`
- `POST /shutdown`
- `POST /reload/config`
- `POST /reload/skills`

示例：

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/status"
curl "http://localhost:$AI_HUB_PORT/api/v1/version"
curl "http://localhost:$AI_HUB_PORT/api/v1/system/init-status"

curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/reload/config"
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/reload/skills"
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/shutdown"
```

---

## 10. Mounts / Static / Transfer

### 10.1 Mounts

管理接口：

- `GET /mounts`
- `POST /mounts`
- `DELETE /mounts/:alias`

静态访问：

- `GET /static/:alias/*filepath`

示例：

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/mounts"

curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/mounts" \
  -H "Content-Type: application/json" \
  -d '{"alias":"media","local_path":"/Users/name/Pictures"}'
```

### 10.2 Transfer

实际注册接口：

- `POST /transfer/upload`
- `PUT /transfer/upload/:id/chunk`
- `POST /transfer/upload/:id/complete`
- `GET /transfer/status/:id`
- `GET /transfer/list`
- `GET /transfer/download/:id`
- `DELETE /transfer/delete/:id`

示例：

```bash
# 初始化上传
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/transfer/upload" \
  -H "Content-Type: application/json" \
  -d '{"filename":"data.zip","file_size":123456}'

# 查询状态
curl "http://localhost:$AI_HUB_PORT/api/v1/transfer/status/<id>"

# 列表
curl "http://localhost:$AI_HUB_PORT/api/v1/transfer/list"

# 下载
curl -O "http://localhost:$AI_HUB_PORT/api/v1/transfer/download/<id>"

# 删除
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/transfer/delete/<id>"
```

---

## 11. Export / Import / Changelog / Usage

相关接口：

- `GET /export/session/:id`
- `GET /export/team/:name`
- `POST /import`
- `GET /changelog`
- `POST /changelog/rollback`
- `GET /token-usage/message/:id`
- `GET /token-usage/session/:id`
- `GET /token-usage/system`
- `GET /token-usage/daily`
- `GET /token-usage/ranking`
- `GET /token-usage/hourly`

示例：

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/changelog?file_name=项目.md&scope=memory&limit=5"

curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/changelog/rollback" \
  -H "Content-Type: application/json" \
  -d '{"file_name":"项目.md","scope":"memory","version":3}'
```

---

## 12. Providers / Compression / Avatars / Proxy

相关接口：

- `GET /providers`
- `POST /providers`
- `PUT /providers/:id`
- `PUT /providers/:id/default`
- `DELETE /providers/:id`
- `GET /claude/auth-status`
- `GET /compression/settings`
- `PUT /compression/settings`
- `GET /avatars`
- `POST /avatars/upload`
- `ANY /proxy/s/:session_id/anthropic/*path`
- `ANY /proxy/anthropic/*path`

说明：

- 这些接口当前未在本技能的 CLI 中全面暴露，但 HTTP 已存在。
- 代理接口主要用于模型请求转发与精确 token 计量。
