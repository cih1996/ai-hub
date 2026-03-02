---
name: "一号笔记管理"
description: "AI Hub 笔记管理接口。当需要读写笔记（notes/）文件时触发。禁止直接 Edit/Write ~/.ai-hub/notes/ 下的文件，必须通过此 API 操作（Read/Grep 可直接使用）。操作规则请使用「规则管理」Skill，操作知识库和记忆库请使用「向量知识库」Skill。"
---

# 笔记管理 — notes/ 操作手册

## 核心原则

**禁止直接用 Edit/Write 工具操作 `~/.ai-hub/notes/` 下的文件（Read/Grep 可直接使用）。**

原因：`~/.ai-hub/` 下的文件由 AI Hub 管理，直接修改可能丢失。必须通过以下 API 操作。

## 记录治理（强制）

1. 先查后写：写 notes 前先 `GET /api/v1/files?scope=notes` 并检查是否已有同主题文件。
2. 一主题一主文件：同主题优先更新已有文件，不重复新建。
3. 禁止时态化命名：不要创建 `xxx-新增.md`、`xxx-增强版.md`、`xxx-本次修改.md`。
4. 正文写“当前有效状态”，历史变化追加到同文件 `变更记录` 区。
5. 禁止在正文记录“之前经过/失败原因叙事/本次修复过程”，只保留最终可用方案与注意事项。

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
