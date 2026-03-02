---
name: "一号智能体"
description: "AI Hub 多会话调度 Skill（优化版）：通过 HTTP API 创建/调度会话、检查状态、汇总结果，并以 Skill-first 方式执行。"
---

# 一号智能体 Skill（v2）

本手册用于“多会话调度与编排”。目标是稳定、可回放、可扩展，不绑定特定业务场景。

## 0. 执行原则（先读）

1. Skill-first：已有 Skill 能做的动作，优先按 Skill 执行。
2. 先锁定上下文：`group_name/scope/target/task_id` 不完整时，先澄清再调度。
3. 先结论后证据：对用户先返回结果，再补关键过程。
4. 单会话串行：同一 `session_id` 不并发发消息。
5. 去特化：避免把固定业务词写死在全局流程里。

## 1. API 基础

- Base URL: `http://localhost:$AI_HUB_PORT/api/v1`
- Header: `Content-Type: application/json`
- 禁止启动新 ai-hub 实例；只通过当前服务 API 操作。

## 2. 标准流程（SOP）

1. 建立任务上下文：确定 `group_name/scope/target/task_id`。
2. 创建或选择会话：新任务用 `session_id=0`，续任务用已有 `session_id`。
3. 发送指令：内容必须包含目标、约束、交付物。
4. 等待完成：轮询会话 `streaming` 状态。
5. 读取结果：取最新 assistant 消息。
6. 汇总回传：给用户结论 + 证据。
7. 沉淀经验：必要时写入记忆/知识或提出 Skill 修订。

### 2.1 压缩后恢复（强制）

当会话刚执行过“压缩上下文/重置会话”，且要延续历史任务时，先读取后继续：

```bash
curl http://localhost:$AI_HUB_PORT/api/v1/sessions/$AI_HUB_SESSION_ID/messages
```

恢复步骤：
1. 提取最近目标、约束、未完成事项。
2. 输出三行恢复摘要（目标/上下文/下一步）。
3. 再进入正常执行流程。

禁令：
- 禁止假设 Claude CLI 内部上下文仍可用。
- 禁止调用不存在接口

## 3. 常用接口模板

### 3.1 创建会话并派发

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/chat/send \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": 0,
    "content": "<任务指令>",
    "work_dir": "<可选>",
    "group_name": "<建议填写>"
  }'
```

### 3.2 向已有会话发送

```bash
curl -X POST http://localhost:$AI_HUB_PORT/api/v1/chat/send \
  -H "Content-Type: application/json" \
  -d '{"session_id": 42, "content": "<后续指令>"}'
```

### 3.3 查询状态与结果

```bash
curl http://localhost:$AI_HUB_PORT/api/v1/sessions/42
curl http://localhost:$AI_HUB_PORT/api/v1/sessions/42/messages
```

### 3.4 会话规则读写

```bash
curl http://localhost:$AI_HUB_PORT/api/v1/session-rules/42
curl -X PUT http://localhost:$AI_HUB_PORT/api/v1/session-rules/42 \
  -H "Content-Type: application/json" \
  -d '{"content":"<角色规则>"}'
```

## 4. 调度模式

1. 串行：有依赖的任务按阶段推进。
2. 并行：互不依赖任务并发派发后统一汇总。
3. 主从：主会话编排，子会话执行并回调。

## 5. 调度防错（强制）

- 执行类消息必须携带：`[group_name|scope|target|task_id]`。
- 未锁定 `scope/target` 不下发。
- 子会话完成后必须回调上游并带 `task_id`。
- 若收到 409（会话忙），先等待再重发，不覆盖当前执行。

## 6. 错误处理

- 400：参数缺失或请求不合法，先修正参数。
- 404：会话不存在，重建会话并重发。
- 409：会话忙，等待完成后重试。
- 500：短重试；连续失败则切换新会话并保留证据。

## 7. Skill 演进机制（重点）

出现以下情况时，应优先升级 Skill 而非堆叠临时规则：
- 同类任务连续出现重复步骤。
- 易错点反复出现。
- 新 API/新流程已稳定可复用。

记录模板：问题、根因、改动点、适用边界、回滚方式。

## 8. 边界与禁令

- 不猜 API，不造字段，不省略关键上下文。
- 不在未知上下文下执行高风险动作。
- 不把团队/会话细则写回全局规则。
