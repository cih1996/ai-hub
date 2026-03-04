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

## 可用工具（13个）

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

返回按 **自身 > 团队 > 全局** 优先级排序（同级按相似度）的文件列表：

| 字段 | 说明 |
|------|------|
| `id` | 文件名 |
| `similarity` | 相似度 0-1 |
| `document` | 向量化内容（文件名+前200字） |
| `metadata` | 元信息（scope、file_path、source_session_id、updated_at 等） |
| `type` | `"knowledge"` 或 `"memory"` |
| `source_session_id` | 写入该文件的会话 ID（0 表示未知） |

> 排序逻辑：source_session_id == 当前 session_id（自身） > 同团队 > 全局；传 `session_id` 才能激活智能排序。

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

### 9. 列出文件（富文本版，推荐）

**`GET /api/v1/vector/list_files`** — 返回文件名 + 前100字预览 + 类型 + 来源会话 + 更新时间，按 自身 > 团队 > 全局 排序：

```bash
# 推荐：传 session_id，自动列出当前会话所属团队的知识库+记忆库
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?session_id=$AI_HUB_SESSION_ID"

# 仅列出记忆库
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?session_id=$AI_HUB_SESSION_ID&type=memory"

# 额外包含全局库
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?session_id=$AI_HUB_SESSION_ID&list_global=true"

# 显式指定 scope
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?scope=AI%20Hub%20维护团队/knowledge"
```

请求参数：

| 参数 | 类型 | 说明 |
|------|------|------|
| `session_id` | int | 当前会话 ID（推断团队归属+排序） |
| `scope` | string | 可选，显式指定 scope，覆盖自动推断 |
| `list_global` | bool | 为 true 时额外列出全局库（默认 false） |
| `type` | string | `memory` / `knowledge` / `all`（默认 all） |

返回示例：
```json
{
  "total": 3,
  "files": [
    {"file_name":"项目概览.md","preview":"# 项目...","type":"knowledge","source_session_id":25,"updated_at":"2026-03-03T20:00:00+08:00","scope":"AI Hub 维护团队/knowledge"},
    {"file_name":"纠错-向量搜索.md","preview":"问题描述...","type":"memory","source_session_id":0,"updated_at":"2026-03-01T12:00:00+08:00","scope":"AI Hub 维护团队/memory"}
  ]
}
```

### 9.1 列出文件（简版，文件名列表）

列出指定 scope 目录下所有 `.md` 文件名（纯文件系统，无需向量引擎就绪）：

```bash
# 团队知识库文件列表
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list?scope=AI%20Hub%20维护团队/knowledge"

# 全局知识库文件列表
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list?scope=knowledge"
```

返回：`["文件A.md", "文件B.md"]`

### 9.2 列出知识库文件（按团队，文件名列表）

```bash
# 推荐：按当前会话自动定位团队知识库（无团队则回退全局 knowledge）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_knowledge?session_id=$AI_HUB_SESSION_ID"

# 显式指定 scope（覆盖自动推断）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_knowledge?scope=AI%20Hub%20维护团队/knowledge"
```

返回：
```json
{"scope":"AI Hub 维护团队/knowledge","files":["A.md","B.md"]}
```

### 9.3 列出记忆库文件（按团队，文件名列表）

```bash
# 推荐：按当前会话自动定位团队记忆库（无团队则回退全局 memory）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_memory?session_id=$AI_HUB_SESSION_ID"

# 显式指定 scope（覆盖自动推断）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_memory?scope=AI%20Hub%20维护团队/memory"
```

返回：
```json
{"scope":"AI Hub 维护团队/memory","files":["X.md","Y.md"]}
```

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

#### 主文件机制（强制）

同一主题只能维护一个主文件（canonical file）：

1. 写入前必须先 `search_*` 检索同主题候选文件。
2. 命中候选时，优先选择已有主文件并执行更新，不新建平行文件。
3. 仅在“确实无同主题文件”时允许新建。
4. 发现同主题多文件并存时，必须执行收敛：
- 选定一个主文件承载“当前有效状态”。
- 将其他文件内容合并后删除或标记废弃。
- 收敛完成前不得继续新增同主题文件。

#### 内容写法规范（强制）

1. 正文写“当前有效状态”，禁止把“本次新增/增强版/本次修改/新版”作为正文主体。
2. 历史变更统一写在同文件末尾 `变更记录`，不要拆成多个迭代文件。
3. 文件名保持稳定，不使用 `xxx-增强版.md`、`xxx-v2.md` 这类时态命名。
4. 禁止过程复盘叙事进入正文：不写“此前问题经过/为什么当时失败/本次如何修复”，只写可执行结论、参数与边界。

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
# 查询向量引擎状态（是否就绪）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/status"

# 轻量健康探针（只返回 ok/error，适合快速确认）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/health"

# 重启向量引擎（引擎异常时使用）
curl -X POST "http://localhost:$AI_HUB_PORT/api/v1/vector/restart"
```

如果返回 `ready: false`，说明向量引擎未就绪，此时：
- **搜索功能不可用**，降级用 `GET /api/v1/vector/list` 列出文件后逐一用 `POST /api/v1/vector/read` 读取
- 读写删除（`read_knowledge` / `write_knowledge` 等）仍可正常使用，不依赖向量引擎
- 可尝试 `POST /vector/restart` 重启引擎后再重试

---

## 操作分类规范（必须 API vs 可直接操作）

| 操作类型 | 方式 | 原因 |
|----------|------|------|
| 语义搜索（search） | **必须 API** | 需传 session_id，后端据此推断团队归属、智能排序 |
| 列出文件（list_files） | **必须 API** | 需感知 session_id / 团队 / 时间，自动排序 |
| 读取文件内容（read file） | 直接 Read 即可 | 只需知道文件路径，无需后端逻辑 |
| 编辑/修改内容 | 直接 Edit 即可 | 同上；编辑后向量引擎会自动通过 watcher 同步 |
| 写入新文件（create） | **必须 API** | 需绑定 source_session_id、触发向量同步 |
| 删除文件 | **必须 API** | 需同步清理向量记录，仅删文件会导致向量库孤记录 |

> **路径规则**：
> - 全局 knowledge：`~/.ai-hub/knowledge/<file_name>`
> - 全局 memory：`~/.ai-hub/memory/<file_name>`
> - 团队 knowledge：`~/.ai-hub/teams/<团队名>/knowledge/<file_name>`
> - 团队 memory：`~/.ai-hub/teams/<团队名>/memory/<file_name>`
