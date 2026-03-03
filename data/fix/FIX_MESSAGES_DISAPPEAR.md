# 修复方案：前端对话消失 Bug

## 问题描述

用户发送消息后，前端对话列表经常莫名其妙消失。后台 Session Chain 文件（如 S001.md）中能看到完整的对话记录，但前端显示为空或消息丢失。

## 根因分析

### 核心问题：消息 ID 不一致

系统中存在两条消息路径，它们产生的消息 ID 格式完全不同：

**路径 A：实时推送（WebSocket）**

1. SendMessage() / handleResult() 生成 ID 格式为 msg_uuid前8位（如 msg_a1b2c3d4）
2. 通过 WebSocket 推送给前端
3. 前端通过 addMessageIfNotExists() 存入 zustand store

**路径 B：API 重新加载**

1. 前端调用 GET /api/sessions/:id/messages
2. 后端 GetMessages() 从 Session Chain 读取 events
3. 调用 eventToMessage() 时重新生成 ID：fmt.Sprintf("msg_ev_%d", ev.EventNo)（如 msg_ev_1）
4. 前端调用 setMessages(response.data) 全量覆盖消息列表

| 场景 | WebSocket 推送的 ID | API 重新加载时的 ID | 结果 |
|------|---------------------|---------------------|------|
| 用户消息 | msg_a1b2c3d4 | msg_ev_1 | ID 不同 |
| 猫猫回复 | msg_e5f6g7h8 | msg_ev_2 | ID 不同 |

### 触发条件：组件频繁重新挂载

从 api.log 中观察到，在 21 秒内（01:32:45 ~ 01:33:06），前端对同一个 session 发起了 3 次完整的重载周期（每次都包含 GET messages + GET ws + GET mode 等）。这说明 React 组件在频繁 unmount/remount，每次触发 ChatArea/index.tsx 的 useEffect，导致：

    loadMessages() -> setMessages(response.data) -> 全量覆盖 -> 消息消失

### 三个加剧因素

#### 1. 写入与推送的时序竞争（Race Condition）

SendMessage() 中的执行顺序（api_server.go:462-478）：

    // 第 463 行：先通过 WebSocket 推送
    sm.wsHub.BroadcastToSession(sessionID, "message", userMsg)

    // 第 466-478 行：后写入 Session Chain
    sm.chainManager.AppendEvent(sessionID, SessionEvent{...})

如果前端在 WebSocket 推送之后、Session Chain 写入之前触发了 loadMessages()，API 返回的数据中不包含刚发送的消息，setMessages 全量覆盖后消息就消失了。

#### 2. System 消息仅存内存

GetMessages() 中（api_server.go:415-418），System 消息（会话已创建、花花已加入对话等）只保存在 SessionContext.SystemMessages 内存中，不写入 Session Chain。服务重启后这些消息全部丢失。

#### 3. setMessages 全量覆盖无合并策略

appStore.ts:63 中 setMessages 直接 set({ messages })，前端的 loadMessages()（ChatArea/index.tsx:70）直接用 API 返回值覆盖整个消息列表，没有与现有消息做合并。

---

## 修复方案

### 修改 1：SessionEvent 增加 MsgID 字段

**文件**: src/session_chain.go

在 SessionEvent 结构体（第 58-66 行）中，在 Content 和 InvocationID 之间增加一个 MsgID 字段：

    MsgID        string                `json:"msgId,omitempty"`  // 新增：原始消息 ID

### 修改 2：写入 Session Chain 时携带原始 ID

**文件**: src/api_server.go

**2a. SendMessage() 中写入用户消息时传入 ID（约第 468 行）**

在 AppendEvent 调用中新增 MsgID 字段：MsgID: userMsg.ID

**2b. handleResult() 中写入猫猫回复时传入 ID（约第 1357 行）**

在 AppendEvent 调用中新增 MsgID 字段：MsgID: agentMsg.ID

### 修改 3：eventToMessage 优先使用原始 ID

**文件**: src/api_server.go

修改 eventToMessage() 方法（约第 1493-1514 行），优先使用 MsgID。

原逻辑（第 1507 行）：

    ID: fmt.Sprintf("msg_ev_%d", ev.EventNo)

改为：

    msgID := ev.MsgID
    if msgID == "" {
        msgID = fmt.Sprintf("msg_ev_%d", ev.EventNo)  // 兼容旧数据
    }
    ...
    ID: msgID

这样 API 返回的消息 ID 就与 WebSocket 推送的一致，前端 addMessageIfNotExists 的去重逻辑也能正确工作。

### 修改 4：调整写入与推送顺序，消除竞争条件

**文件**: src/api_server.go

**4a. SendMessage() 中（约第 462-478 行）**

将 Session Chain 写入移到 WebSocket 推送之前：

    原顺序：先 BroadcastToSession -> 后 AppendEvent
    新顺序：先 AppendEvent -> 后 BroadcastToSession

这样即使前端在收到 WS 推送后立即触发 loadMessages()，API 从 Session Chain 读取时也已包含该消息。

**4b. handleResult() 中（约第 1351-1366 行）**

同样调整顺序：

    原顺序：先 BroadcastToSession -> 后 AppendEvent
    新顺序：先 AppendEvent -> 后 BroadcastToSession

### 修改 5：Markdown 格式中保存 MsgID（可选但推荐）

**文件**: src/session_chain_storage.go

在 writeSessionMarkdownToDisk 中，将 MsgID 写入 Markdown 标题行的 HTML 注释中（约第 167-175 行）。

例如原来的：

    ### #1 [12:30:00] **[用户]**

改为：

    ### #1 [12:30:00] **[用户]** <!-- msg_a1b2c3d4 -->

同时更新 readSessionMarkdownFromDisk 的解析逻辑，从 HTML 注释中提取 MsgID。

---

## 涉及文件清单

| 文件 | 修改内容 |
|------|----------|
| src/session_chain.go:58-66 | SessionEvent 增加 MsgID 字段 |
| src/api_server.go:462-478 | SendMessage() 调整写入顺序 + 传入 MsgID |
| src/api_server.go:1337-1366 | handleResult() 调整写入顺序 + 传入 MsgID |
| src/api_server.go:1493-1514 | eventToMessage() 优先使用 MsgID |
| src/session_chain_storage.go:167-175 | Markdown 写入时保存 MsgID（可选） |

## 验证方法

1. 启动服务后，发送消息并 @猫猫
2. 等待猫猫回复出现在前端
3. 切换到其他会话再切回来，确认消息不消失
4. 刷新浏览器（F5），确认消息完整保留
5. 重启后端服务，确认消息完整保留（system 消息除外，后续可单独优化）

## 备注

- 修改 1-4 为必须修改项，修改 5 为推荐项
- 所有修改向后兼容：旧数据中 MsgID 为空时，自动回退到 msg_ev_N 格式
- System 消息持久化为独立优化项，本次不涉及
