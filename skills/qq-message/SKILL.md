---
name: "QQ消息发送"
description: "QQ消息发送接口调用指南。当 AI 需要回复QQ消息、向QQ群或用户发送消息时触发。基于 OneBot 11 协议，通过 NapCat HTTP API 发送。"
---

# QQ消息发送 — 调用手册

你收到了来自QQ的消息，需要通过 NapCat（OneBot 11）HTTP API 回复。本手册提供完整的调用方式。

## 调用流程

```
1. 从【QQ消息】的「频道凭证」中获取 NapCat地址 和 Token
2. 根据消息类型（私聊/群聊）调用对应 API 发送消息
```

## 发送私聊消息

```bash
curl -s -X POST '<NAPCAT_URL>/send_private_msg' \
  -H 'Authorization: Bearer <TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{"user_id": <USER_ID>, "message": "你的回复内容"}'
```

## 发送群聊消息

```bash
curl -s -X POST '<NAPCAT_URL>/send_group_msg' \
  -H 'Authorization: Bearer <TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{"group_id": <GROUP_ID>, "message": "你的回复内容"}'
```

## 通用发送接口

```bash
# 私聊
curl -s -X POST '<NAPCAT_URL>/send_msg' \
  -H 'Authorization: Bearer <TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{"message_type": "private", "user_id": <USER_ID>, "message": "内容"}'

# 群聊
curl -s -X POST '<NAPCAT_URL>/send_msg' \
  -H 'Authorization: Bearer <TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{"message_type": "group", "group_id": <GROUP_ID>, "message": "内容"}'
```
## 完整示例：收到QQ消息后回复

当你收到如下格式的消息时：

```
【QQ消息】
类型: 私聊
发送者: 123456789
消息ID: 12345
内容: 用户说的话
---
频道凭证（用于回复）:
NapCat地址: http://127.0.0.1:3000
Token: your-token
```

回复步骤：

```bash
# 私聊回复（使用消息中的发送者ID）
curl -s -X POST 'http://127.0.0.1:3000/send_private_msg' \
  -H 'Authorization: Bearer your-token' \
  -H 'Content-Type: application/json' \
  -d '{"user_id": 123456789, "message": "你的回复内容"}'
```

群聊回复：

```bash
# 群聊回复（使用消息中的群号）
curl -s -X POST 'http://127.0.0.1:3000/send_group_msg' \
  -H 'Authorization: Bearer your-token' \
  -H 'Content-Type: application/json' \
  -d '{"group_id": 123456, "message": "你的回复内容"}'
```

## 注意事项

- `user_id` 和 `group_id` 是数字类型，不要加引号
- `message` 字段为字符串，直接写文本即可（也支持 CQ 码富文本）
- Token 为空时可省略 Authorization 头
- NapCat地址 和 Token 从【QQ消息】的「频道凭证」部分获取
- 如果 Token 鉴权失败，检查 NapCat 配置中的 token 是否一致
