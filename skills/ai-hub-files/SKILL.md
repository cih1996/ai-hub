---
name: "一号笔记管理"
description: "AI Hub 笔记管理接口。当需要读写笔记（notes/）文件时触发。禁止直接 Edit/Write ~/.ai-hub/notes/ 下的文件，必须通过此 API 操作（Read/Grep 可直接使用）。操作规则请使用「规则管理」Skill，操作知识库和记忆库请使用「向量知识库」Skill。"
---

# 笔记管理 — notes/ 操作手册

## 核心原则

**禁止直接用 Edit/Write 工具操作 `~/.ai-hub/notes/` 下的文件（Read/Grep 可直接使用）。**

原因：`~/.ai-hub/` 下的文件由 AI Hub 管理，直接修改可能丢失。必须通过以下 API 操作。

## API 基础

地址：`http://localhost:$AI_HUB_PORT/api/v1/files`
请求头：`Content-Type: application/json`
scope 固定为：`notes`

---

## 可用接口

### 1. 列出笔记

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=notes"
```

### 2. 读取笔记内容

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/files/content?scope=notes&path=notes/文件名.md"
```

### 3. 写入/更新笔记

```bash
curl -X PUT http://localhost:$AI_HUB_PORT/api/v1/files/content \
  -H "Content-Type: application/json" \
  -d '{"scope": "notes", "path": "notes/文件名.md", "content": "笔记内容"}'
```

### 4. 创建新笔记

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/files \
  -H "Content-Type: application/json" \
  -d '{"scope": "notes", "path": "notes/新文件.md", "content": "笔记内容"}'
```

### 5. 删除笔记

```bash
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/files?scope=notes&path=notes/要删除的笔记.md"
```
