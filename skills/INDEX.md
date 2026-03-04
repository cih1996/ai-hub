# AI Hub Skill 索引

本文档是所有 Skill 的导航索引，帮助 AI 快速定位应该使用哪个 Skill。

## 核心原则

**Skill 优先**：凡是 Skill 能完成的操作，必须通过 Skill 执行，严禁绕过 Skill 直接用底层工具。

---

## 系统管理类

### 一号智能体（ai-hub-agent）
**触发场景**：需要创建/调度会话、多角色协作、任务编排时
**核心能力**：
- 创建新会话并派发任务
- 向已有会话发送消息
- 查询会话状态和结果
- 团队协作调度（异步回调）
- 临时任务调度（可轮询）

**关键接口**：
- `POST /api/v1/chat/send` — 创建会话或发送消息
- `GET /api/v1/sessions/{id}` — 查询会话状态
- `GET /api/v1/sessions/{id}/messages` — 读取会话消息

---

### 一号规则管理（ai-hub-rules）
**触发场景**：需要查看或修改全局规则、团队规则、会话规则时
**核心能力**：
- 读写会话级规则（角色定位和行为约束）
- 读写全局规则（所有会话共享）
- 读写团队规则（同团队会话共享）

**关键接口**：
- `GET /api/v1/session-rules/{id}` — 读取会话规则
- `PUT /api/v1/session-rules/{id}` — 修改会话规则
- `GET /api/v1/files/content?scope=rules&path=CLAUDE.md` — 读取全局规则
- `PUT /api/v1/files/content` — 修改全局规则或团队规则

---

### 一号系统感知（ai-hub-system）
**触发场景**：版本检查、日志排障、系统诊断时
**核心能力**：
- 版本检查和更新提醒
- 日志查看和问题定位
- 会话/向量/认证/代理问题诊断

**关键接口**：
- `GET /api/v1/version` — 查看版本
- 日志文件：`~/.ai-hub/logs/ai-hub.log`

---

## 数据管理类

### 一号向量知识库（ai-hub-vector）
**触发场景**：需要语义搜索、读写知识库/记忆库时
**核心能力**：
- 语义搜索知识库和记忆库
- 读写删除知识/记忆文件
- 自动推断团队 scope（传 session_id）
- 命中统计和文件列表

**关键接口**：
- `POST /api/v1/vector/search_knowledge` — 搜索知识库
- `POST /api/v1/vector/search_memory` — 搜索记忆库
- `POST /api/v1/vector/write_knowledge` — 写入知识库
- `POST /api/v1/vector/write_memory` — 写入记忆库
- `GET /api/v1/vector/list_files` — 列出文件（富文本版）

**重要原则**：
- 知识库和记忆库的唯一操作入口，禁止通过「文件管理」Skill 操作
- 写入前必须先搜索，避免重复
- 推荐传 `session_id` 自动推断团队 scope

---

### 一号笔记管理（ai-hub-files）
**触发场景**：需要读写笔记（notes/）文件时
**核心能力**：
- 列出笔记文件
- 读写删除笔记

**关键接口**：
- `GET /api/v1/files?scope=notes` — 列出笔记
- `GET /api/v1/files/content?scope=notes&path=notes/xxx.md` — 读取笔记
- `PUT /api/v1/files/content` — 写入/更新笔记

**重要原则**：
- 禁止直接 Edit/Write `~/.ai-hub/notes/` 下的文件，必须通过 API 操作
- Read/Grep 可直接使用

---

## 自动化类

### 一号定时器（ai-hub-timer）
**触发场景**：需要定时执行任务、定期检查、周期性提醒时
**核心能力**：
- 创建定时触发器（精确时间/每日固定时间/固定间隔）
- 查看/修改/删除触发器
- 触发器到时自动向指定会话发送指令

**关键接口**：
- `POST /api/v1/triggers` — 创建触发器
- `GET /api/v1/triggers?session_id={id}` — 查看触发器
- `PUT /api/v1/triggers/{id}` — 修改触发器
- `DELETE /api/v1/triggers/{id}` — 删除触发器

**重要原则**：
- content 必须自包含，写清楚完整的执行指令
- 用绝对路径，明确文件或目录
- 写清楚异常处理

---

### 一号脚本引擎（ai-hub-scripts）
**触发场景**：需要执行多步重复操作（≥3步）、浏览器自动化、批量 API 调用时
**核心能力**：
- 脚本批量化执行
- 浏览器自动化（Chrome MCP）
- 参数化脚本复用
- 失败修复重跑

**脚本仓库**：`~/.ai-hub/scripts/`
**索引文件**：`~/.ai-hub/scripts/INDEX.md`

**重要原则**：
- ≥3 步的重复性操作必须脚本化
- 先查索引复用已有脚本，无则新建

---

## 需求孵化类

### 一号导航员（ai-hub-navigator）
**触发场景**：用户提到"我需要你引导我打造一个智能体"时
**核心能力**：
- 多轮对话引导用户从模糊想法到明确需求
- 验证关键材料
- 生成项目规则与记忆
- 构建多智能体协作网络

---

## 第三方平台类

### 飞书应用部署（feishu-deploy）
**触发场景**：需要在飞书开放平台创建应用、配置机器人、设置事件订阅时
**核心能力**：
- 通过 Chrome MCP 操作浏览器完成全流程
- 创建应用、配置机器人、开通权限、发布版本

---

### 飞书消息发送（feishu-message）
**触发场景**：需要回复飞书消息、向飞书群或用户发送消息时
**核心能力**：
- 获取 token
- 发送消息（文本/卡片）

---

### QQ 频道部署（qq-deploy）
**触发场景**：需要安装 NapCat、配置 QQ 机器人、对接 AI Hub 时
**核心能力**：
- 引导用户完成安装、登录、WebUI 配置
- 创建频道

---

### QQ 消息发送（qq-message）
**触发场景**：需要回复 QQ 消息、向 QQ 群或用户发送消息时
**核心能力**：
- 基于 OneBot 11 协议
- 通过 NapCat HTTP API 发送消息

---

## 使用建议

1. **任务开始前先查本索引**，确认是否有对应 Skill
2. **有 Skill 必须先用 Skill**，不要绕过 Skill 直接用底层工具
3. **不确定时先 Read Skill 手册**，禁止凭记忆猜测 API
4. **发现 Skill 不足时**，优先补充/修订 Skill，再继续任务
