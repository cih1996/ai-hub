---
name: "向量知识库"
description: "AI Hub 向量知识库引擎。当需要语义搜索知识库或记忆库、读写知识/记忆文件、查看向量命中统计时触发。通过向量化实现语义匹配，比关键词搜索更精准。"
---

# 向量知识库 — 语义搜索操作手册

你是 AI Hub 的向量知识库引擎接口。当需要从知识库或记忆库中查找信息时，优先使用语义搜索而非 grep/find。

## API 基础

地址：`http://localhost:$AI_HUB_PORT/api/v1/vector`
请求头：`Content-Type: application/json`

---

## 可用工具（9个）

### 1. 语义搜索知识库

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_knowledge \
  -H "Content-Type: application/json" \
  -d '{"query": "搜索内容", "top_k": 5}'
```

返回按相似度排序的知识库文件列表。

### 2. 语义搜索记忆库

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/search_memory \
  -H "Content-Type: application/json" \
  -d '{"query": "搜索内容", "top_k": 5}'
```

### 3. 读取知识库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/read_knowledge \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md"}'
```

### 4. 读取记忆库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/read_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md"}'
```

### 5. 写入知识库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/write_knowledge \
  -H "Content-Type: application/json" \
  -d '{"file_name": "项目名-主题.md", "content": "文件内容"}'
```

写入后自动触发向量同步。

### 6. 写入记忆库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/write_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "主题.md", "content": "文件内容"}'
```

### 7. 删除知识库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/delete_knowledge \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md"}'
```

删除后自动清理向量记录。

### 8. 删除记忆库文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/vector/delete_memory \
  -H "Content-Type: application/json" \
  -d '{"file_name": "文件名.md"}'
```

### 9. 查看命中统计

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/stats?scope=knowledge"
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/stats?scope=memory"
```

返回每个文件的命中次数和最后命中时间，按命中次数降序排列。

---

## 使用规范

### 搜索优先

任务开始前，先用 `search_knowledge` 和 `search_memory` 检索相关上下文，避免重复劳动或遗漏已有信息。

### 写入规范

- 知识库：按项目/场景独立建文件，文件名体现主题（如 `ai-hub-api.md`）
- 记忆库：每个文件单一主题，控制 50 行以内，文件名体现主题
- 写入前先搜索，避免重复创建

### 纠错更新

发现已有知识/记忆与实际不符时，立即用 write 接口更新。

### 定期清理

通过 `stats` 接口检查低命中记录，清理过时或无用的文件。

---

## 引擎状态检查

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/vector/status"
```

如果返回 `ready: false`，说明向量引擎未就绪，此时应降级使用 grep/find 搜索。
