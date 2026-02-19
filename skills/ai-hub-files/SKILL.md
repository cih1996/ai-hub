---
name: "一号文件管理"
description: "AI Hub 文件管理接口。当需要读写全局规则（CLAUDE.md）、子规则（rules/）、笔记（notes/）文件时触发。必须通过此接口操作，禁止直接 Read/Edit 这些文件，否则模板渲染会覆盖修改。知识库和记忆库请使用「向量知识库」Skill。"
---

# 文件管理 — 规则/笔记操作手册

## 核心原则

**禁止直接用 Read/Edit/Write 工具操作 `~/.claude/` 下的文件。**

原因：`~/.claude/` 下的文件是模板渲染产物，每次对话开始时会被 `~/.ai-hub/templates/` 的模板重新渲染覆盖。直接修改会丢失。

必须通过以下 API 操作，API 会自动双写：模板源文件 + 渲染产物。

## API 基础

地址：`http://localhost:$AI_HUB_PORT/api/v1/files`
请求头：`Content-Type: application/json`

## scope 说明

| scope | 对应目录 | 说明 |
|-------|---------|------|
| rules | CLAUDE.md + rules/*.md | 全局规则（CLAUDE.md 是主规则） |
| notes | notes/*.md | 笔记 |

---

## 可用接口

### 1. 列出文件

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=rules"
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=notes"
```

### 2. 读取文件内容

```bash
# 读取全局规则模板
curl "http://localhost:$AI_HUB_PORT/api/v1/files/content?scope=rules&path=CLAUDE.md"

# 读取子规则
curl "http://localhost:$AI_HUB_PORT/api/v1/files/content?scope=rules&path=rules/规则名.md"

# 读取笔记
curl "http://localhost:$AI_HUB_PORT/api/v1/files/content?scope=notes&path=notes/文件名.md"
```

### 3. 写入/更新文件

```bash
curl -X PUT http://localhost:$AI_HUB_PORT/api/v1/files/content \
  -H "Content-Type: application/json" \
  -d '{"scope": "rules", "path": "CLAUDE.md", "content": "新的规则内容"}'
```

scope 可选：rules、notes
path 示例：CLAUDE.md、rules/子规则.md、notes/文件.md

### 4. 创建新文件

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/files \
  -H "Content-Type: application/json" \
  -d '{"scope": "notes", "path": "notes/新文件.md", "content": "文件内容"}'
```

### 5. 删除文件

```bash
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/files?scope=rules&path=rules/要删除的规则.md"
```

### 6. 查看可用模板变量

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/files/variables"
```

返回所有可用的 `{{VAR}}` 占位符及当前值。写入全局规则时可使用这些变量，渲染时自动替换。

### 7. 恢复默认规则

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/files/default?path=CLAUDE.md"
```

返回系统内置的默认 CLAUDE.md 模板内容。

---

## 模板变量

在全局规则（CLAUDE.md）中可使用以下占位符，渲染时自动替换为实际值：

| 变量 | 说明 |
|------|------|
| `{{HOME_DIR}}` | 用户主目录 |
| `{{CLAUDE_DIR}}` | ~/.claude 目录 |
| `{{MEMORY_DIR}}` | 记忆库目录 |
| `{{KNOWLEDGE_DIR}}` | 知识库目录 |
| `{{RULES_DIR}}` | 规则目录 |
| `{{OS}}` | 操作系统 |
| `{{PORT}}` | 服务端口 |
| `{{DATE}}` | 当前日期 |
| `{{TIME_BEIJING}}` | 北京时间 |

---

## 项目级规则（独立接口）

项目级规则不走模板渲染，使用独立 API：

```bash
# 列出项目规则
curl "http://localhost:$AI_HUB_PORT/api/v1/project-rules?work_dir=/path/to/project"

# 读取项目规则
curl "http://localhost:$AI_HUB_PORT/api/v1/project-rules/content?work_dir=/path/to/project&path=CLAUDE.md"

# 写入项目规则
curl -X PUT http://localhost:$AI_HUB_PORT/api/v1/project-rules/content \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project", "path": "CLAUDE.md", "content": "项目规则内容"}'
```

项目级规则可以直接 Read/Edit，因为不涉及模板渲染。
