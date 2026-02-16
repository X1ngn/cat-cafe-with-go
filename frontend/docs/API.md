# API 接口文档

## 基础信息

- **Base URL**: `http://localhost:8080/api`
- **数据格式**: JSON
- **字符编码**: UTF-8

## 接口列表

### 1. 会话管理

#### 1.1 获取会话列表

```
GET /api/sessions
```

**响应示例**:
```json
[
  {
    "id": "sess_123456",
    "name": "关于猫猫的笑话",
    "summary": "用户：讲个关于橘猫的笑话...",
    "updatedAt": "2026-02-16T10:30:00Z",
    "messageCount": 15
  }
]
```

#### 1.2 创建新会话

```
POST /api/sessions
```

**响应示例**:
```json
{
  "id": "sess_789012",
  "name": "新对话",
  "summary": "",
  "updatedAt": "2026-02-16T11:00:00Z",
  "messageCount": 0
}
```

#### 1.3 获取会话详情

```
GET /api/sessions/:sessionId
```

**路径参数**:
- `sessionId`: 会话ID

**响应示例**:
```json
{
  "id": "sess_123456",
  "name": "关于猫猫的笑话",
  "summary": "用户：讲个关于橘猫的笑话...",
  "updatedAt": "2026-02-16T10:30:00Z",
  "messageCount": 15
}
```

#### 1.4 删除会话

```
DELETE /api/sessions/:sessionId
```

**路径参数**:
- `sessionId`: 会话ID

**响应**: 204 No Content

---

### 2. 消息管理

#### 2.1 获取会话消息列表

```
GET /api/sessions/:sessionId/messages
```

**路径参数**:
- `sessionId`: 会话ID

**查询参数**:
- `page`: 页码，默认 1
- `limit`: 每页数量，默认 50

**响应示例**:
```json
[
  {
    "id": "msg_001",
    "type": "cat",
    "content": "你好呀！我是三花猫花花，你的专属设计师。有什么可以帮你的吗？ᓚᘏᗢ",
    "sender": {
      "id": "cat_001",
      "name": "花花",
      "avatar": "",
      "color": "#ff9966"
    },
    "timestamp": "2026-02-16T10:00:00Z",
    "sessionId": "sess_123456"
  },
  {
    "id": "msg_002",
    "type": "user",
    "content": "你好！我想设计一个「猫猫咖啡屋」的官网。",
    "sender": {
      "id": "user_001",
      "name": "用户",
      "avatar": ""
    },
    "timestamp": "2026-02-16T10:01:00Z",
    "sessionId": "sess_123456"
  },
  {
    "id": "msg_003",
    "type": "system",
    "content": "薇薇 已加入对话",
    "timestamp": "2026-02-16T10:02:00Z",
    "sessionId": "sess_123456"
  }
]
```

#### 2.2 发送消息

```
POST /api/sessions/:sessionId/messages
```

**路径参数**:
- `sessionId`: 会话ID

**请求体**:
```json
{
  "content": "@花花 你好！我想设计一个「猫猫咖啡屋」的官网。",
  "mentionedCats": ["cat_001"]
}
```

**说明**:
- `content`: 消息内容，可以使用 `@猫猫名` 来提及特定的猫猫
- `mentionedCats`: 被提及的猫猫 ID 数组，系统会自动将任务分发给这些猫猫

**响应示例**:
```json
{
  "id": "msg_004",
  "type": "user",
  "content": "@花花 你好！我想设计一个「猫猫咖啡屋」的官网。",
  "sender": {
    "id": "user_001",
    "name": "用户",
    "avatar": ""
  },
  "timestamp": "2026-02-16T10:05:00Z",
  "sessionId": "sess_123456"
}
```

**工作流程**:
1. 用户发送消息后，系统会立即返回用户消息
2. 如果消息中提及了猫猫（通过 `mentionedCats`），系统会：
   - 添加系统消息："猫猫名 已加入对话"
   - 将任务发送给对应的 Agent 进行处理
3. Agent 处理完成后，会自动将回复添加到消息列表中
4. 前端可以通过轮询 `GET /api/sessions/:sessionId/messages` 来获取 Agent 的回复

**消息类型**:
- `user`: 用户消息（显示在右侧）
- `cat`: 猫猫回复消息（显示在左侧，带头像和颜色）
- `system`: 系统消息（居中显示）

#### 2.3 获取消息统计

```
GET /api/sessions/:sessionId/stats
```

**路径参数**:
- `sessionId`: 会话ID

**响应示例**:
```json
{
  "totalMessages": 1234,
  "catMessages": 868
}
```

---

### 3. 猫猫管理

#### 3.1 获取所有猫猫列表

```
GET /api/cats
```

**响应示例**:
```json
[
  {
    "id": "cat_001",
    "name": "花花",
    "avatar": "",
    "color": "#ff9966",
    "status": "idle"
  },
  {
    "id": "cat_002",
    "name": "薇薇",
    "avatar": "",
    "color": "#d9bf99",
    "status": "busy"
  },
  {
    "id": "cat_003",
    "name": "大橘",
    "avatar": "",
    "color": "#cccccc",
    "status": "idle"
  },
  {
    "id": "cat_004",
    "name": "布偶",
    "avatar": "",
    "color": "#cccccc",
    "status": "busy"
  }
]
```

#### 3.2 获取猫猫状态

```
GET /api/cats/:catId
```

**路径参数**:
- `catId`: 猫猫ID

**响应示例**:
```json
{
  "id": "cat_001",
  "name": "花花",
  "avatar": "",
  "color": "#ff9966",
  "status": "idle"
}
```

#### 3.3 获取可用的猫猫

```
GET /api/cats/available
```

**响应示例**:
```json
[
  {
    "id": "cat_001",
    "name": "花花",
    "avatar": "",
    "color": "#ff9966",
    "status": "idle"
  },
  {
    "id": "cat_003",
    "name": "大橘",
    "avatar": "",
    "color": "#cccccc",
    "status": "idle"
  }
]
```

---

### 4. 调用历史

#### 4.1 获取调用历史

```
GET /api/sessions/:sessionId/history
```

**路径参数**:
- `sessionId`: 会话ID

**响应示例**:
```json
[
  {
    "catId": "cat_001",
    "catName": "大橘",
    "sessionId": "sess_a1b2",
    "timestamp": "2026-02-16T09:00:00Z"
  },
  {
    "catId": "cat_002",
    "catName": "布偶",
    "sessionId": "sess_c3d4",
    "timestamp": "2026-02-16T08:30:00Z"
  }
]
```

---

## 数据模型

### Cat (猫猫)
```typescript
{
  id: string;           // 猫猫唯一标识
  name: string;         // 猫猫名称
  avatar: string;       // 头像URL
  color: string;        // 头像颜色（十六进制）
  status: 'idle' | 'busy' | 'offline';  // 状态
}
```

### Message (消息)
```typescript
{
  id: string;           // 消息唯一标识
  type: 'cat' | 'user' | 'system';  // 消息类型
  content: string;      // 消息内容
  sender?: {            // 发送者信息（系统消息无此字段）
    id: string;
    name: string;
    avatar: string;
    color?: string;     // 猫猫消息才有
  };
  timestamp: Date;      // 发送时间
  sessionId: string;    // 所属会话ID
}
```

### Session (会话)
```typescript
{
  id: string;           // 会话唯一标识
  name: string;         // 会话名称
  summary: string;      // 会话摘要
  updatedAt: Date;      // 最后更新时间
  messageCount: number; // 消息数量
}
```

### MessageStats (消息统计)
```typescript
{
  totalMessages: number;   // 总消息数
  catMessages: number;     // 猫猫消息数
}
```

### CallHistory (调用历史)
```typescript
{
  catId: string;        // 猫猫ID
  catName: string;      // 猫猫名称
  sessionId: string;    // 会话ID
  timestamp: Date;      // 调用时间
}
```

---

## WebSocket 接口（可选）

如果需要实时消息推送，可以实现 WebSocket 接口：

```
WS /api/ws/sessions/:sessionId
```

**消息格式**:
```json
{
  "type": "message" | "typing" | "status_change",
  "data": {
    // 根据 type 不同，data 内容不同
  }
}
```

**示例 - 新消息**:
```json
{
  "type": "message",
  "data": {
    "id": "msg_005",
    "type": "cat",
    "content": "好的，让我来帮你设计！",
    "sender": {
      "id": "cat_001",
      "name": "花花",
      "color": "#ff9966"
    },
    "timestamp": "2026-02-16T10:06:00Z"
  }
}
```

**示例 - 打字状态**:
```json
{
  "type": "typing",
  "data": {
    "catId": "cat_001",
    "catName": "花花",
    "isTyping": true
  }
}
```

**示例 - 状态变更**:
```json
{
  "type": "status_change",
  "data": {
    "catId": "cat_002",
    "status": "busy"
  }
}
```

---

## 错误处理

所有接口在出错时返回统一格式：

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "错误描述"
  }
}
```

**常见错误码**:
- `SESSION_NOT_FOUND`: 会话不存在
- `CAT_NOT_FOUND`: 猫猫不存在
- `INVALID_REQUEST`: 请求参数错误
- `INTERNAL_ERROR`: 服务器内部错误
