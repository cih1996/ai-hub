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

### 关键目录

| 目录 | 说明 |
|------|------|
| main.go | 入口，路由注册，go:embed 安装逻辑 |
| claude/CLAUDE.md | 全局规则模板源文件（支持 {{VAR}} 占位符） |
| skills/ | 内置 Skill 源文件（go:embed，启动时覆盖安装到 ~/.ai-hub/skills/） |
| server/api/ | HTTP 接口层 |
| server/core/ | 核心逻辑层（进程池、模板渲染、向量引擎等） |
| server/store/ | SQLite 数据层 |
| server/model/ | 数据模型 |
| web/src/views/ | Vue 前端页面 |
| vector-engine/ | Python 向量引擎（go:embed 嵌入） |

不确定具体文件时，直接 `ls` 对应目录查看，严禁凭记忆猜测。

## 2. 内嵌资源与 Skill 同步规范

AI Hub 通过 go:embed 嵌入资源，启动时自动安装到 ~/.ai-hub/：
- `skills/` → ~/.ai-hub/skills/（覆盖安装）
- `claude/CLAUDE.md` → ~/.ai-hub/rules/CLAUDE.md（首次安装）
- `vector-engine/` → Python 向量引擎

**修改规则：必须改项目源文件，禁止直接改 ~/.ai-hub/ 下的文件，编译后重启生效。**

**接口与 Skill 同步（重要）：**
- 新增/修改/删除 HTTP 接口后，必须同步更新对应的 Skill 文档（skills/<skill名>/SKILL.md）
- 新增/修改前端页面或交互流程后，检查是否影响相关 Skill 的操作指引
- Skill 是用户和 AI 理解系统能力的唯一入口，接口变了 Skill 没更新 = 系统能力断层

## 3. 团队角色与通信

| 角色 | 会话ID | 职责 |
|------|--------|------|
| 售后客服 | 21 | 接收反馈，整理问题，跟踪进度，升级生产实例 |
| 测试工程师 | 23 | 复现 BUG，创建 Issue，验证修复，发布 Release |
| 技术维护 | 25 | 查看 Issue，定位修复，编译提交代码 |
| 运营专员 | 73 | 通过 Chrome MCP 操作第三方平台，记录流程到知识库 |

通信协议
- API：`POST http://localhost:8080/api/v1/chat/send`，body：`{"session_id": <ID>, "content": "【角色】内容"}`
- 闭环原则：收到任务后必须回复结果，包含执行结果 + 关键变更 + 是否需后续操作
- 异步派发：发完消息即继续，禁止轮询等待，子会话完成后主动回报

工作流：客服 → 测试（复现）→ Issue → 技术（修复）→ 测试（验证）→ 发布 → 客服（升级）

## 4. 开发规范

### 编译与测试
1. 代码修改后 `make` 编译，产物在 dist/
2. 停 8081：`lsof -ti:8081 | xargs kill -9 2>/dev/null`，`sleep 2`
3. 启 8081：`nohup dist/ai-hub-darwin-arm64 -port 8081 >> ~/.ai-hub/logs/ai-hub-8081.log 2>&1 &`
4. 验证：`sleep 5 && curl -s http://localhost:8081/api/v1/version`（失败查日志 `tail -50`）
5. 前端验证：通过 Chrome MCP 访问 http://localhost:8081
6. 严禁操作 8080 生产实例

### 升级生产（售后客服执行）
1. `git pull`
2. 先打 tag：`git tag v<版本号>`（必须在编译前打 tag，否则版本号不准确）
3. `make release`（或 `VERSION=v<版本号> make release` 显式指定）
4. 验证版本：`strings dist/ai-hub-darwin-arm64 | grep "v版本号"`
5. 杀旧进程：`kill -9 <PID>`，`sleep 2` 确认退出
6. 启新版本：`nohup dist/ai-hub-darwin-arm64 -port 8080 >> ~/.ai-hub/logs/ai-hub-8080.log 2>&1 &`
7. 验证：`sleep 4 && curl -s http://localhost:8080/api/v1/version`
8. 推送 tag：`git push origin v<版本号>`

### Git 规范
- 分支：feat/<issue>-<简述> 或 fix/<issue>-<简述>
- 提交：fix(scope): description (closes #issue)
- 发布流程：合并 main → 打 tag → 编译 release → 推送 tag → 创建 GitHub Release

### API 开发规范
- PUT update 接口必须先读数据库再 merge，禁止直接 bind 覆盖（防零值覆盖未传字段）

### 防回归原则（重要）
- 修复 BUG 时必须梳理受影响的上下游逻辑，禁止只盯当前问题而忽略已有流程
- 新增/修改路由、参数格式、数据结构时，保留旧接口兼容或明确标注 breaking change
- 技术维护提交修复前，需在 commit message 中说明「影响范围」和「兼容性处理」
- 测试工程师验证时，除验证修复本身外，必须回归测试相关联的功能路径

## 5. 测试规范

- 创建测试会话：`POST http://localhost:8081/api/v1/chat/send` + `session_id: 0`
- 必须通过 Chrome MCP 在 8081 上验证前端，不能仅做 API 测试
- 禁止跨端口（8081 问题不能跑 8080 验证）
- 注意上下文长度，单次验证完成后及时回复
- 发布：验证通过 → `make release` → `gh release create` → `git push`

## 6. 执行准则

- 任务前先读本文件 + `ls` 关键目录，确保认知最新
- 上下文压缩后，第一步重新读本文件恢复认知
- 不确定时先读代码，严禁凭记忆猜测
