# AI Hub 自维护团队规则

## 1. 项目概览

- 仓库：https://github.com/cih1996/ai-hub
- 本地路径：/Users/cih1996/work/ai/ai-hub
- 技术栈：Go (Gin) + Vue 前端 + SQLite + WebSocket
- 生产端口：8080（当前运行实例，禁止停止）
- 测试端口：8081（测试实例，用于验证修复）
- 日志路径：~/.ai-hub/logs/ai-hub.log（启动时自动清空）
- 数据目录：~/.ai-hub/（rules/、skills/、knowledge/、memory/、notes/）
- GitHub MCP 可用于操作 Issue 和 Release

### 目录结构

```
ai-hub/
├── main.go              # 入口，路由注册，go:embed 安装逻辑
├── Makefile             # make（开发编译）、make release（发布编译）
├── claude/CLAUDE.md     # 全局规则模板源文件（支持 {{VAR}} 占位符）
├── skills/              # 内置 Skill 源文件（go:embed 嵌入）
│   ├── ai-hub-agent/    # 多会话调度
│   ├── ai-hub-files/    # 笔记管理
│   ├── ai-hub-navigator/# 需求孵化
│   ├── ai-hub-rules/    # 规则管理
│   ├── ai-hub-system/   # 系统感知
│   ├── ai-hub-timer/    # 定时触发器
│   └── ai-hub-vector/   # 向量知识库
├── server/
│   ├── api/             # HTTP 接口层
│   │   ├── chat.go      # 会话消息（BuildSystemPrompt 注入）
│   │   ├── files.go     # 文件管理 API（rules/knowledge/memory/notes）
│   │   ├── session.go   # 会话 CRUD
│   │   ├── session_rules.go # 会话级规则
│   │   ├── skills.go    # Skills 列表
│   │   ├── vector.go    # 向量搜索 API
│   │   ├── trigger.go   # 定时触发器
│   │   ├── mcp.go       # MCP 配置
│   │   ├── provider.go  # Provider 管理
│   │   └── status.go    # 进程状态
│   ├── core/             # 核心逻辑层
│   │   ├── claude_pool.go   # Claude 进程池管理
│   │   ├── claude_code.go   # Claude CLI 调用
│   │   ├── template.go      # 规则模板渲染、BuildSystemPrompt()
│   │   ├── trigger.go       # 定时触发器调度
│   │   ├── vector.go        # 向量引擎管理
│   │   └── vector_watcher.go# 文件变更监听同步向量
│   ├── store/            # SQLite 数据层
│   ├── model/            # 数据模型
│   └── service/          # 业务服务层
├── vector-engine/        # Python 向量引擎（go:embed 嵌入）
├── web/src/views/        # Vue 前端页面
│   ├── ChatView.vue      # 对话页
│   ├── ManageView.vue    # 管理页（规则/知识库/记忆/笔记）
│   ├── SkillsView.vue    # 技能页
│   ├── McpView.vue       # MCP 配置页
│   ├── TriggersView.vue  # 定时器页
│   ├── SettingsView.vue  # 设置页
│   └── MainLayout.vue    # 主布局
└── .claude/CLAUDE.md     # 本文件（项目级规则）
```

## 2. 内嵌资源维护规范

AI Hub 通过 go:embed 将以下资源嵌入二进制，启动时自动安装到 ~/.ai-hub/：
- `skills/` → 启动时覆盖安装到 ~/.ai-hub/skills/
- `claude/CLAUDE.md` → 首次安装到 ~/.ai-hub/rules/CLAUDE.md
- `vector-engine/` → Python 向量引擎

**修改 Skill 或全局规则模板时，必须修改项目代码中的源文件，禁止直接改 ~/.ai-hub/ 下的文件：**
- 修改/新增 Skill → 编辑 `skills/<skill名>/SKILL.md`
- 修改全局规则模板 → 编辑 `claude/CLAUDE.md`（支持 {{VAR}} 占位符）
- 编译后重启即可生效（启动时自动覆盖安装）

## 3. 团队角色与通信协议

### 角色定义

| 角色 | 会话名 | 会话ID | 职责 |
|------|--------|--------|------|
| 售后客服 | ai-hub-客服 | 21 | 接收用户反馈，整理问题，跟踪进度，升级生产实例 |
| 测试工程师 | ai-hub-测试 | 23 | 复现 BUG，创建 Issue，验证修复，发布 Release |
| 技术维护 | ai-hub-开发 | 25 | 查看 Issue，定位修复 BUG，编译提交代码 |

### 通信协议
- API：`POST http://localhost:8080/api/v1/chat/send`
- 请求体：`{"session_id": <目标会话ID>, "content": "<消息内容>"}`
- 消息格式：`【来源角色】<内容描述>`
- **闭环原则**：收到任务后，无论成功或失败，完成后必须通过 API 回复发起方
- 回复需包含：执行结果、关键变更说明、是否需要后续操作

### 工作流
- 客服 → 测试工程师（复现）→ 创建 Issue → 技术维护（修复）→ 测试工程师（验证）→ 发布 → 客服（升级）

## 4. 开发规范

### 编译与测试流程（所有角色必须遵守）
1. 代码修改后，在项目根目录执行 `make` 编译
2. 编译产物在 dist/ 目录
3. 停止 8081 测试实例：`lsof -ti:8081 | xargs kill -9 2>/dev/null`
4. 等待确认退出：`sleep 2`
5. 启动测试实例：`nohup dist/ai-hub-darwin-arm64 -port 8081 > /dev/null 2>&1 &`
6. 等待启动完成：`sleep 5 && curl -s http://localhost:8081/api/v1/version`
7. 通过 Chrome MCP 访问 http://localhost:8081 验证前端功能
8. 严禁操作 8080 端口的生产实例

### Git 规范
- 功能分支：feat/<issue编号>-<简述>
- 修复分支：fix/<issue编号>-<简述>
- 提交格式：fix(scope): description (closes #issue)
- 合并到 main 后打 tag 发布

### API Update 开发规范
- 所有 PUT update 接口禁止直接 `var p Model` + `ShouldBindJSON(&p)` 覆盖写入
- 必须先从数据库读取完整记录，再 `ShouldBindJSON(existing)` merge 请求字段
- 未传字段保持原值，不会被零值覆盖
- 测试验证：update 接口必须检查只传部分字段时，未传字段是否保持原值

## 5. 测试规范（测试工程师专用）

- **创建测试会话**：必须通过 `POST http://localhost:8081/api/v1/chat/send` + `session_id: 0` 创建，禁止直接操作 sessions 路由
- **前端验证**：必须通过 Chrome MCP 在 http://localhost:8081 上实际操作，不能仅做 API 测试
- **禁止跨端口测试**：8081 上遇到问题不能跑到 8080 上验证
- **上下文管理**：验证大功能时注意对话长度，单次验证完成后及时回复结果，避免上下文溢出
- **发布流程**：验证通过 → `make release` → `gh release create` → `git push`

## 6. 升级流程（售后客服执行）

测试工程师验证通过并发布 Release 后，由售后客服升级 8080 生产实例：
1. 拉取最新代码：`git pull`
2. 带版本号编译：`make release`（不要用 `make`，否则版本号不会注入）
3. 验证二进制版本：`strings dist/ai-hub-darwin-arm64 | grep "v版本号"`
4. 强制杀掉旧进程：`kill -9 $(lsof -ti:8080 | xargs ps -p 2>/dev/null | grep ai-hub | awk '{print $1}')`
5. 等待确认退出：`sleep 2 && ps -p <PID> 2>/dev/null || echo "已退出"`
6. 启动新版本：`nohup dist/ai-hub-darwin-arm64 -port 8080 > /dev/null 2>&1 &`
7. 验证版本：`sleep 4 && curl -s http://localhost:8080/api/v1/version`

注意：必须用 `make release` 而非 `make`；必须用 `kill -9` 强杀；杀进程后必须等待确认退出再启动。

## 7. 禁止事项

- 禁止直接修改 8080 生产实例
- 禁止未编译就提交代码
- 禁止跳过测试直接发布
- 禁止直接改 ~/.ai-hub/ 下的内嵌资源文件（必须改项目源文件后编译）

## 8. 待解决问题

（当前无已知未修复问题）

## 9. 执行准则

- **任务前扫描**：所有角色在开始任务前，必须先读取本文件（项目规则）并扫描项目关键目录结构（`ls` main.go、server/api/、server/core/、skills/、web/src/views/），确保对项目的认知是最新的
- **上下文恢复**：上下文压缩后，第一步必须重新读取本文件，恢复项目认知
- **防止认知漂移**：不确定项目结构或接口时，先读代码再行动，严禁凭记忆猜测
