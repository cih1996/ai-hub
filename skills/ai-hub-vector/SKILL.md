---
name: "一号向量记忆库"
description: "AI Hub 向量记忆库引擎。记忆库（memory/）的唯一操作入口。当需要语义搜索、读写、删除记忆文件、查看向量命中统计时触发。通过向量化实现语义匹配，比关键词搜索更精准。禁止通过「文件管理」Skill 操作记忆库。"
---

# 向量记忆库 — 记忆库唯一操作入口

你是 AI Hub 的向量记忆库引擎接口。记忆库的所有操作（搜索、读取、写入、删除）必须通过本 Skill 完成，禁止通过「文件管理」Skill 操作。

## API 基础

地址：`http://localhost:$AI_HUB_PORT/api/v1/vector`
请求头：`Content-Type: application/json`

---

## 三层 Scope 隔离架构

所有接口均支持 `scope` 字段，数据按三层隔离存储：

| 层级 | scope 格式 | 物理路径 | 可见性 |
|------|-----------|---------|--------|
| 全局 | `memory` | `~/.ai-hub/memory/` | 所有会话可读 |
| 团队 | `<团队名>/memory` | `~/.ai-hub/teams/<团队名>/memory/` | 同团队可读写 |
| 会话 | `<团队名>/sessions/<id>/memory` | `~/.ai-hub/teams/<团队名>/sessions/<id>/memory/` | 仅该会话可读写 |
| 团队规则 | `<团队名>/rules` | `~/.ai-hub/teams/<团队名>/rules/` | 仅 list/read |

## 自动 Scope（session_id 推断，强制隔离）

**推荐用法：** 所有接口传 `session_id`，后端自动路由到正确层级：

- **写入**（未传 scope）：默认写入**会话级** scope（`<团队名>/sessions/<id>/memory`）
- **写入**（传 scope=`<团队名>/memory`）：写入团队级（允许，需属于该团队）
- **写入**（传 scope=`memory`）：写入全局（允许）
- **写入**（传 scope 指向其他团队）：**403 拒绝**
- **搜索**（未传 scope）：三层合并，会话级 → 团队级 → 全局，去重按优先级排序
- **读取/删除**（未传 scope）：自动指向会话级
- 若会话无团队 → 只操作全局库

**环境变量**：`$AI_HUB_SESSION_ID` 由 AI Hub 自动注入到 Claude 子进程，CLI 自动读取，无需手动传参。

---

## 可用工具（9个）

### 1. 语义搜索记忆库

```bash
# 推荐：传 session_id 自动三层合并搜索
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_memory \
  -H "Content-Type: application/json" \
  -d '{"query": "搜索内容", "top_k": 5, "session_id": '"$AI_HUB_SESSION_ID"'}'

# 显式指定 scope（覆盖自动推断）
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_memory \
  -H "Content-Type: application/json" \
  -d '{"query": "搜索内容", "top_k": 5, "scope": "AI Hub 维护团队/memory"}'
```

支持可选 `tags` 参数按标签过滤：
```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_memory \
  -H "Content-Type: application/json" \
  -d '{"query": "部署流程", "top_k": 5, "session_id": '"$AI_HUB_SESSION_ID"', "tags": ["deploy"]}'
```

返回按 **自身 > 会话级 > 团队级 > 全局** 优先级排序（同级按相似度）的文件列表：

| 字段 | 说明 |
|------|------|
| `id` | 文件名 |
| `similarity` | 相似度 0-1 |
| `document` | 向量化内容（文件名+前200字） |
| `metadata` | 元信息（scope、file_path、source_session_id、tags 等） |
| `type` | `"memory"` |
| `source_session_id` | 写入该文件的会话 ID（0 表示未知） |

> 排序逻辑：self(0) > session(1) > team(2) > global(3)；传 `session_id` 才能激活三层智能排序。

### 2. 读取记忆库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/read_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "session_id": '"$AI_HUB_SESSION_ID"'}'
```

### 3. 写入记忆库文件

```bash
# 推荐：传 session_id，默认写入会话级记忆库
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/write_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "主题.md", "content": "文件内容", "session_id": '"$AI_HUB_SESSION_ID"'}'

# 显式写入团队级（需属于该团队）
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/write_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "主题.md", "content": "文件内容", "scope": "AI Hub 维护团队/memory", "session_id": '"$AI_HUB_SESSION_ID"'}'

# 写入全局
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/write_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "通用主题.md", "content": "文件内容", "scope": "memory"}'
```

写入后自动触发向量同步。支持 `extra_metadata` 传入 tags 等结构化字段：

```bash
# 带 tags 的写入
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/write_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "主题.md", "content": "文件内容", "session_id": '"$AI_HUB_SESSION_ID"', "extra_metadata": {"tags": ["deploy","sop"]}}'
```

### 4. 删除记忆库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/delete_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md", "session_id": '"$AI_HUB_SESSION_ID"'}'
```

删除后自动清理向量记录。

### 5. 列出文件（富文本版，推荐）

**`GET /api/v1/vector/list_files`** — 返回文件名 + 预览 + 类型 + 来源 + origin 层级标注：

```bash
# 推荐：列出当前会话所有可见文件（三层合并）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?session_id=$AI_HUB_SESSION_ID"

# 只列会话级文件
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?session_id=$AI_HUB_SESSION_ID&level=session"

# 只列团队级文件
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?session_id=$AI_HUB_SESSION_ID&level=team"

# 只列全局文件
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?session_id=$AI_HUB_SESSION_ID&level=global"

# 按 tag 过滤
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list_files?session_id=$AI_HUB_SESSION_ID&tag=deploy"
```

请求参数：

| 参数 | 类型 | 说明 |
|------|------|------|
| `session_id` | int | 当前会话 ID（推断团队归属+排序） |
| `level` | string | `session` / `team` / `global` / `all`（默认 all） |
| `scope` | string | 可选，显式指定 scope，覆盖自动推断 |
| `list_global` | bool | 为 true 时额外列出全局库（默认 false） |
| `type` | string | `memory`（默认） |
| `tag` | string | 可选，按 tag 过滤（metadata 中包含该 tag 的文件） |

返回示例：
```json
{
  "total": 2,
  "files": [
    {"file_name":"我的笔记.md","preview":"# 笔记...","type":"memory","source_session_id":21,"updated_at":"2026-03-06T00:00:00+08:00","scope":"AI Hub 维护团队/sessions/21/memory","origin":"session"},
    {"file_name":"通用记忆.md","preview":"# 通用...","type":"memory","source_session_id":0,"updated_at":"2026-03-01T12:00:00+08:00","scope":"memory","origin":"global"}
  ]
}
```

### 5.1 列出文件（简版，文件名列表）

列出指定 scope 目录下所有 `.md` 文件名（纯文件系统，无需向量引擎就绪）：

```bash
# 团队记忆库文件列表
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list?scope=AI%20Hub%20维护团队/memory"

# 全局记忆库文件列表
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/list?scope=memory"
```

返回：`["文件A.md", "文件B.md"]`

### 5.2 列出记忆库文件（按团队，文件名列表）

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

### 6. 读取任意 scope 文件（通用）

指定 scope 和文件名即可读取：

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/read \
  -H "Content-Type: application/json" \
  -d '{"scope": "AI Hub 维护团队/memory", "file_name": "项目概览.md"}'
```

### 7. 查看命中统计

```bash
# 全局
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/stats?scope=memory"

# 团队（URL 编码团队名中的空格）
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/stats?scope=AI%20Hub%20维护团队/memory"
```

返回每个文件的命中次数和最后命中时间，按命中次数降序排列。

---

## 使用规范

### 搜索优先（强制）

任务开始前，必须先用 `search_memory` 检索相关上下文，避免重复劳动或遗漏已有信息。

**团队会话推荐用法：** 只需传 `session_id`，系统自动搜索三层（会话级 → 团队级 → 全局）并合并返回：

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_memory \
  -H "Content-Type: application/json" \
  -d '{"query": "纠错 <任务类型>", "top_k": 5, "session_id": '"$AI_HUB_SESSION_ID"'}'
```

### 写入规范

- **记忆库**：每个文件单一主题，控制 50 行以内，文件名体现主题
- 写入前先搜索，避免重复创建；发现内容过时立即用 write 接口更新
- **团队会话传 `session_id` 即可自动写入会话级**，无需手动指定 scope
- 需要写入团队级共享记忆时，显式传 `scope=<团队名>/memory`
- 需要写入全局通用记忆时，显式传 `scope=memory`

#### 主文件机制（强制）

同一主题只能维护一个主文件（canonical file）：

1. 写入前必须先 `search_memory` 检索同主题候选文件。
2. 命中候选时，优先选择已有主文件并执行更新，不新建平行文件。
3. 仅在"确实无同主题文件"时允许新建。
4. 发现同主题多文件并存时，必须执行收敛：
- 选定一个主文件承载"当前有效状态"。
- 将其他文件内容合并后删除或标记废弃。
- 收敛完成前不得继续新增同主题文件。

#### 内容写法规范（强制）

1. 正文写"当前有效状态"，禁止把"本次新增/增强版/本次修改/新版"作为正文主体。
2. 历史变更统一写在同文件末尾 `变更记录`，不要拆成多个迭代文件。
3. 文件名保持稳定，不使用 `xxx-增强版.md`、`xxx-v2.md` 这类时态命名。
4. 禁止过程复盘叙事进入正文：不写"此前问题经过/为什么当时失败/本次如何修复"，只写可执行结论、参数与边界。

### 可用环境变量

| 变量 | 说明 |
|------|------|
| `$AI_HUB_SESSION_ID` | 当前会话 ID（所有会话均注入） |
| `$AI_HUB_GROUP_NAME` | 当前会话所属团队名（无团队时为空） |
| `$AI_HUB_PORT` | AI Hub 服务端口 |

### 纠错更新

发现已有记忆与实际不符时，立即用 `write_memory` 更新，并在记忆中记录修正原因。

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
- 读写删除（`read_memory` / `write_memory` 等）仍可正常使用，不依赖向量引擎
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
> - 全局 memory：`~/.ai-hub/memory/<file_name>`
> - 团队 memory：`~/.ai-hub/teams/<团队名>/memory/<file_name>`
> - 会话 memory：`~/.ai-hub/teams/<团队名>/sessions/<id>/memory/<file_name>`

---

## CLI 命令（ai-hub）

所有命令统一使用 `--level` 参数指定三层 scope，环境变量 `AI_HUB_SESSION_ID`、`AI_HUB_GROUP_NAME`、`AI_HUB_PORT` 自动继承。

### list — 列出记忆文件

```bash
ai-hub list --level session
ai-hub list --level team
ai-hub list --level global
```

输出格式：文件名 + 100字预览 + 创建/更新时间。

### search — 语义搜索

```bash
ai-hub search "关键词" --level session
ai-hub search "部署流程" --level team --top 5
```

默认 top_k=10，输出格式同 list + 相似度分数。

### read — 读取文件

```bash
ai-hub read "文件名.md" --level session
```

### write — 写入文件

```bash
ai-hub write "文件名.md" --level session --content "# 内容"
echo "内容" | ai-hub write "文件名.md" --level team
```

### edit — 编辑文件（查找替换）

```bash
ai-hub edit "文件名.md" --level session --old "旧文本" --new "新文本"
```

先读取文件，查找替换后写回，输出 diff。

### delete — 删除文件

```bash
ai-hub delete "文件名.md" --level session --force
```

`--force` 跳过确认提示。

### --level 参数说明

| level | scope 解析 | 所需环境变量 |
|-------|-----------|-------------|
| `session` | `<团队名>/sessions/<id>/memory` | `AI_HUB_GROUP_NAME` + `AI_HUB_SESSION_ID` |
| `team` | `<团队名>/memory` | `AI_HUB_GROUP_NAME` |
| `global` | `memory` | 无 |
