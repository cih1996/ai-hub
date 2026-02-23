---
name: "一号规则管理"
description: "AI Hub 规则管理接口。当需要查看或修改全局规则、项目级规则、会话级规则时触发。涵盖三个层级：全局规则（所有会话生效）、项目级规则（特定项目生效）、会话级规则（当前会话的角色定位和行为约束）。"
---

# 规则管理 — 三级规则操作手册

AI Hub 的规则体系分三个层级，优先级从高到低：会话级 > 项目级 > 全局。

## API 基础

地址：`http://localhost:$AI_HUB_PORT`
请求头：`Content-Type: application/json`
当前会话ID：环境变量 `$AI_HUB_SESSION_ID`

---

## 一、会话级规则（Session Rules）

每个会话独立的角色定位和行为约束，Claude 进程启动时自动注入。

存储位置：`~/.ai-hub/session-rules/{session_id}.md`

**生效机制：** 会话规则通过 `--system-prompt` 在进程启动时注入。修改规则后，需要关闭该会话的 Claude 进程，下次发消息时进程自动重启并加载新规则。

### 读取自己的会话规则

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/session-rules/$AI_HUB_SESSION_ID"
```

### 修改自己的会话规则

```bash
# 第一步：写入新规则
curl -X PUT "http://localhost:$AI_HUB_PORT/api/v1/session-rules/$AI_HUB_SESSION_ID" \
  -H "Content-Type: application/json" \
  -d '{"content": "你是售后客服，职责是..."}'

# 第二步：关闭该会话的进程（下次发消息时自动重启并加载新规则）
# 如果是修改自己的规则，进程会在当前对话结束后自然空闲超时回收
# 如果是修改其他会话的规则，可以通过 DELETE 会话进程来强制重启
```

### 修改其他会话的规则并使其生效

```bash
# 写入规则
curl -X PUT "http://localhost:$AI_HUB_PORT/api/v1/session-rules/23" \
  -H "Content-Type: application/json" \
  -d '{"content": "新的角色规则..."}'

# 查看该会话进程状态
curl "http://localhost:$AI_HUB_PORT/api/v1/sessions/23"
# 如果 process_alive=true，进程还在用旧规则，下次重启时才会加载新规则
```

### 删除会话规则

```bash
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/session-rules/$AI_HUB_SESSION_ID"
```

### 读取其他会话的规则

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/session-rules/23"
```

---

## 二、全局规则（Global Rules）

所有会话都会加载的规则，包括主规则 CLAUDE.md 和子规则 rules/*.md。

**重要：全局规则使用模板机制，支持 `{{VAR}}` 占位符，通过 --system-prompt 注入。必须通过 API 操作，禁止直接编辑 ~/.ai-hub/rules/ 下的文件。**

### 读取全局主规则（模板源文件）

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/files/content?scope=rules&path=CLAUDE.md"
```

### 修改全局主规则

```bash
curl -X PUT http://localhost:$AI_HUB_PORT/api/v1/files/content \
  -H "Content-Type: application/json" \
  -d '{"scope": "rules", "path": "CLAUDE.md", "content": "规则内容，可使用 {{HOME_DIR}} 等变量"}'
```

### 列出所有子规则

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/files?scope=rules"
```

### 创建子规则

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/files \
  -H "Content-Type: application/json" \
  -d '{"scope": "rules", "path": "rule-描述.md", "content": "子规则内容"}'
```

### 删除子规则

```bash
curl -X DELETE "http://localhost:$AI_HUB_PORT/api/v1/files?scope=rules&path=rule-描述.md"
```

### 查看可用模板变量

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/files/variables"
```

---

## 三、项目级规则（Project Rules）

特定项目目录下的规则，仅在该项目工作目录的会话中生效。不走模板渲染。

### 列出项目规则

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/project-rules?work_dir=/path/to/project"
```

### 读取项目规则

```bash
curl "http://localhost:$AI_HUB_PORT/api/v1/project-rules/content?work_dir=/path/to/project&path=CLAUDE.md"
```

### 修改项目规则

```bash
curl -X PUT http://localhost:$AI_HUB_PORT/api/v1/project-rules/content \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project", "path": "CLAUDE.md", "content": "项目规则内容"}'
```

注意：项目级规则也可以直接用 Read/Edit 工具操作，因为不涉及模板渲染。

---

## 使用原则

1. **会话级规则**：定义"我是谁"（角色、职责、行为边界），每个会话独立
2. **项目级规则**：定义"这个项目怎么做"（团队分工、编译流程、Git 规范），项目内共享
3. **全局规则**：定义"通用行为准则"（记忆管理、知识库规范、纠错流程），所有会话共享
4. **修改自己的会话规则**：当发现自己的角色定位需要调整时，可以主动修改
5. **禁止越权**：不要修改其他会话的规则，除非用户明确要求
