---
name: "一号向量知识库"
description: "AI Hub 向量知识库引擎。知识库（knowledge/）和记忆库（memory/）的唯一操作入口。当需要语义搜索、读写、删除知识/记忆文件、查看向量命中统计时触发。通过向量化实现语义匹配，比关键词搜索更精准。禁止通过「文件管理」Skill 操作知识库和记忆库。"
---

# 向量知识库 — 知识库与记忆库唯一操作入口

你是 AI Hub 的向量知识库引擎接口。知识库和记忆库的所有操作（搜索、读取、写入、删除）必须通过本 Skill 完成，禁止通过「文件管理」Skill 操作。

## API 基础

地址：`http://localhost:$AI_HUB_PORT/api/v1/vector`
请求头：`Content-Type: application/json`

---

## Scope 参数说明

所有接口均支持可选的 `scope` 字段，用于区分全局库与团队私有库：

| scope 值 | 说明 | 物理路径 |
|----------|------|---------|
| `knowledge`（默认） | 全局知识库 | `~/.ai-hub/knowledge/` |
| `memory`（默认） | 全局记忆库 | `~/.ai-hub/memory/` |
| `<团队名>/knowledge` | 团队私有知识库 | `~/.ai-hub/teams/<团队名>/knowledge/` |
| `<团队名>/memory` | 团队私有记忆库 | `~/.ai-hub/teams/<团队名>/memory/` |
| `<团队名>/rules` | 团队规则（仅 list/read） | `~/.ai-hub/teams/<团队名>/rules/` |

## 自动 Scope（session_id 推断）

**推荐用法：** 所有搜索/读写/删除接口均支持 `session_id` 字段。传入 `session_id` 后，后端自动推断团队 scope：

- 若该会话属于某团队（`group_name` 非空）且**未传 `scope`**：
  - **搜索接口**：先搜团队库，再搜全局库，合并返回（团队结果优先，按 file_name 去重）
  - **读写/删除接口**：自动指向团队库
- 若已传 `scope` → 按传入的搜（行为不变）
- 若会话无团队 → 只操作全局库（行为不变）

**环境变量**：`$AI_HUB_SESSION_ID` 由 AI Hub 自动注入，可直接使用。

---

## 可用工具（11个）

### 1. 语义搜索知识库

```bash
# 推荐：传 session_id 自动推断 scope（团队会话自动搜团队+全局）
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_knowledge \
  -H "Content-Type: application/json" \
  -d '{"query": "搜索内容", "top_k": 5, "session_id": '"$AI_HUB_SESSION_ID"'}'

# 显式指定 scope（覆盖自动推断）
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_knowledge \
  -H "Content-Type: application/json" \
  -d '{"query": "搜索内容", "top_k": 5, "scope": "AI Hub 维护团队/knowledge"}'
```

返回按相似度排序的文件列表（`results[].file_name` / `results[].score` / `results[].content`）。

### 2. 语义搜索记忆库

```bash
# 推荐：传 session_id 自动推断
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_memory \
  -H "Content-Type: application/json" \
  -d '{"query": "搜索内容", "top_k": 5, "session_id": '"$AI_HUB_SESSION_ID"'}'
```

### 3. 读取知识库文件

```bash
# 推荐：传 session_id
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/read_knowledge \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "session_id": '"$AI_HUB_SESSION_ID"'}'
```

### 4. 读取记忆库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/read_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "session_id": '"$AI_HUB_SESSION_ID"'}'
```

### 5. 写入知识库文件

```bash
# 推荐：传 session_id，团队会话自动写入团队知识库
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/write_knowledge \
  -H "Content-Type: application/json" \
  -d '{"file_name": "项目名-主题.md", "content": "文件内容", "session_id": '"$AI_HUB_SESSION_ID"'}'
```

写入后自动触发向量同步。

### 6. 写入记忆库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/write_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "主题.md", "content": "文件内容", "session_id": '"$AI_HUB_SESSION_ID"'}'
```

### 7. 删除知识库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/delete_knowledge \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "session_id": '"$AI_HUB_SESSION_ID"'}'
```

删除后自动清理向量记录。

### 8. 删除记忆库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/delete_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "session_id": '"$AI_HUB_SESSION_ID"'}'
```

### 9. 列出 scope 目录文件

列出指定 scope 目录下所有 `.md` 文件名（纯文件系统，无需向量引擎就绪）：

```bash
# 团队知识库文件列表
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list?scope=AI%20Hub%20维护团队/knowledge"

# 全局知识库文件列表
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list?scope=knowledge"
```

返回：`["文件A.md", "文件B.md"]`

### 10. 读取任意 scope 文件（通用）

不区分 knowledge/memory，指定 scope 和文件名即可读取：

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/read \
  -H "Content-Type: application/json" \
  -d '{"scope": "AI Hub 维护团队/knowledge", "file_name": "项目概览.md"}'
```

### 11. 查看命中统计

```bash
# 全局
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/stats?scope=knowledge"

# 团队（URL 编码团队名中的空格）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/stats?scope=AI%20Hub%20维护团队/knowledge"
```

返回每个文件的命中次数和最后命中时间，按命中次数降序排列。

---

## 使用规范

### 搜索优先（强制）

任务开始前，必须先用 `search_knowledge` 和 `search_memory` 检索相关上下文，避免重复劳动或遗漏已有信息。

**团队会话推荐用法：** 只需传 `session_id`，系统自动搜索团队库 + 全局库并合并返回：

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_memory \
  -H "Content-Type: application/json" \
  -d '{"query": "纠错 <任务类型>", "top_k": 5, "session_id": '"$AI_HUB_SESSION_ID"'}'
```

### 写入规范

- **知识库**：按项目/场景独立建文件，文件名体现主题（如 `ai-hub-vector-api.md`）
- **记忆库**：每个文件单一主题，控制 50 行以内，文件名体现主题
- 写入前先搜索，避免重复创建；发现内容过时立即用 write 接口更新
- **团队会话传 `session_id` 即可自动写入团队库**，无需手动指定 scope

### 可用环境变量

| 变量 | 说明 |
|------|------|
| `$AI_HUB_SESSION_ID` | 当前会话 ID（所有会话均注入） |
| `$AI_HUB_GROUP_NAME` | 当前会话所属团队名（无团队时为空） |
| `$AI_HUB_PORT` | AI Hub 服务端口 |

### 纠错更新

发现已有知识/记忆与实际不符时，立即用 `write_knowledge` / `write_memory` 更新，并在记忆中记录修正原因。

### 定期清理

通过 `stats` 接口检查低命中记录，清理过时或无用的文件。

---

## 引擎状态检查

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/status"
```

如果返回 `ready: false`，说明向量引擎未就绪，此时：
- **搜索功能不可用**，降级用 `GET /api/v1/vector/list` 列出文件后逐一用 `POST /api/v1/vector/read` 读取
- 读写删除（`read_knowledge` / `write_knowledge` 等）仍可正常使用，不依赖向量引擎
