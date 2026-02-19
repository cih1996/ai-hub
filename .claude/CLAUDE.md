# AI Hub 自维护团队规则

## 项目信息
- 仓库：https://github.com/cih1996/ai-hub
- 本地路径：/Users/cih1996/work/ai/ai-hub
- 技术栈：Go (Gin) + Vue 前端 + SQLite + WebSocket
- 生产端口：8080（当前运行实例，禁止停止）
- 测试端口：8081（测试实例，用于验证修复）
- GitHub MCP 可用于操作 Issue 和 Release

## 内嵌资源维护规范
AI Hub 通过 go:embed 将以下资源嵌入二进制，启动时自动安装到 ~/.claude/：
- `skills/` → 内置 Skills，启动时覆盖安装到 ~/.claude/skills/
- `claude/CLAUDE.md` → 全局规则模板，首次安装到 ~/.ai-hub/templates/CLAUDE.md
- `vector-engine/` → Python 向量引擎

**修改 Skill 或全局规则模板时，必须修改项目代码中的源文件，禁止直接改 ~/.claude/ 下的文件：**
- 修改/新增 Skill → 编辑 `skills/<skill名>/SKILL.md`
- 修改全局规则模板 → 编辑 `claude/CLAUDE.md`（支持 {{VAR}} 占位符）
- 编译后重启即可生效（启动时自动覆盖安装）

## 团队角色

### 售后客服（会话名：ai-hub-客服）
- 职责：接收用户反馈的 BUG 和问题，整理为清晰的问题描述
- 工作流：收到用户反馈 → 整理问题 → 通过 API 发送给测试工程师复现 → 跟踪进度 → 回复用户结果
- 通信目标：测试工程师

### 技术维护（会话名：ai-hub-开发）
- 职责：查看 Issue、定位 BUG、修改代码、编译测试、提交代码
- 工作流：收到问题 → 阅读代码定位 → 修复 → make 编译 → 通知测试工程师验证
- 修复完成后必须通知测试工程师进行验证

### 测试工程师（会话名：ai-hub-测试）
- 职责：复现 BUG、创建 Issue、验证修复、发布新版本
- 工作流：收到问题 → 在 8081 端口复现 → 创建 GitHub Issue → 触发技术维护修复 → 验证修复 → 通过则发布 Release
- 测试方法：通过 Chrome MCP 在 http://localhost:8081 上操作验证
- 发布流程：验证通过 → make release → gh release create → git push

## 编译与测试流程（所有角色必须遵守）
1. 代码修改后，在项目根目录执行 `make` 编译
2. 编译产物在 dist/ 目录
3. 停止 8081 测试实例（如有）：`lsof -ti:8081 | xargs kill -9 2>/dev/null`
4. 等待确认退出：`sleep 2`
5. 启动测试实例：`nohup dist/ai-hub-darwin-arm64 -port 8081 > /tmp/ai-hub-8081.log 2>&1 &`
6. 等待启动完成：`sleep 5 && curl -s http://localhost:8081/api/v1/version`
7. 通过 Chrome MCP 访问 http://localhost:8081 验证前端功能
8. 严禁操作 8080 端口的生产实例，任何情况下都不能在 8080 上测试

### 测试工程师专用规范
- **创建测试会话**：必须通过 `POST http://localhost:8081/api/v1/chat/send` + `session_id: 0` 创建，禁止直接操作 sessions 路由或手动创建
- **前端验证**：必须通过 Chrome MCP 在 http://localhost:8081 上实际操作，不能仅做 API 测试
- **禁止跨端口测试**：8081 上遇到问题不能跑到 8080 上验证，必须在 8081 上解决
- **上下文管理**：验证大功能时注意对话长度，单次验证完成后及时回复结果，避免上下文溢出导致重复操作

## 角色间通信协议
- 通过 HTTP API 发送消息：POST http://localhost:8080/api/v1/chat/send
- 请求体：{"session_id": <目标会话ID>, "content": "<消息内容>"}
- 消息格式：`【来源角色】<内容描述>`
- 例：`【测试工程师】Issue #5 已修复验证通过，请发布新版本`
- **闭环回复原则**：收到其他角色的任务请求后，无论结果成功或失败，完成后必须通过 API 回复发起方。禁止任务做完不反馈就结束对话。
- 回复内容需包含：执行结果、关键变更说明、是否需要发起方后续操作

## Git 规范
- 修复分支：fix/<issue编号>-<简述>
- 提交格式：fix(scope): description (closes #issue)
- 合并到 main 后打 tag 发布

## 生产实例升级流程（售后客服执行）
测试工程师验证通过并发布 Release 后，由售后客服负责升级 8080 生产实例：
1. 拉取最新代码：`git pull`
2. 带版本号编译：`make release`（不要用 `make`，否则版本号不会注入）
3. 验证二进制版本：`strings dist/ai-hub-darwin-arm64 | grep "v版本号"` 确认版本正确
4. 强制杀掉旧进程：`kill -9 $(lsof -ti:8080 | xargs ps -p 2>/dev/null | grep ai-hub | awk '{print $1}')`
5. 等待进程确认退出：`sleep 2 && ps -p <PID> 2>/dev/null || echo "已退出"`
6. 启动新版本：`nohup dist/ai-hub-darwin-arm64 -port 8080 > /tmp/ai-hub-8080.log 2>&1 &`
7. 等待启动并验证版本：`sleep 4 && curl -s http://localhost:8080/api/v1/version`
8. 确认返回的版本号与预期一致

注意事项：
- 必须用 `make release` 而非 `make`，后者不注入 git tag 版本号
- 必须用 `kill -9` 强杀，普通 `kill` 可能杀不掉
- 杀进程后必须等待确认退出再启动，否则端口冲突
- 启动后必须通过 version API 验证版本号正确

## API Update 开发规范
- 所有 PUT update 接口禁止直接 `var p Model` + `ShouldBindJSON(&p)` 覆盖写入
- 必须先从数据库读取完整记录（如 `store.GetXxx(id)`），再 `ShouldBindJSON(existing)` merge 请求字段
- 这样未传字段保持原值，不会被零值覆盖
- 推荐模式：先读后 bind（如 UpdateSession、UpdateProvider）或指针字段 + nil 检查（如 UpdateTrigger）
- 测试验证：update 接口必须检查只传部分字段时，未传字段是否保持原值

## 禁止事项
- 禁止直接修改 8080 生产实例
- 禁止未编译就提交代码
- 禁止跳过测试直接发布
