# 前端对话消失 Bug 修复方案

## 问题描述

用户反馈前端对话消息会莫名其妙消失，但后端 Session Chain（`.md` 文件）中对话记录完整存在。

**现象**：
- 对话进行中，消息突然全部消失或部分消失
- 刷新页面后消息恢复
- 问题间歇性出现，不是每次都能复现

**影响**：
- 用户体验严重受损
- 用户误以为消息丢失

## 根本原因

### 根因 1：useEffect 对象引用变化触发消息重置

**文件**: `frontend/src/components/ChatArea/index.tsx` 第 22-53 行

```tsx
useEffect(() => {
  if (currentSession) {
    messagesLoadedRef.current = false;
    pendingWsMessagesRef.current = [];
    loadMessages();  // 这里会先 setMessages(response.data) 清空再设置
    // ...
  }
}, [currentSession]);  // ← 对象引用比较
```

**问题**：
- `useEffect` 依赖 `currentSession` 对象引用
- `Sidebar` 中的 `updateSession` 会产生新的 session 对象引用（如会话名更新、消息数变化）
- 新引用触发 `useEffect` 重新执行 → `loadMessages()` → 在 API 返回前消息列表被重置
- 如果 API 请求失败或超时，消息永久消失

### 根因 2：消息 ID 不一致导致去重失效

**文件**: `src/api_server.go`

- WebSocket 推送的用户消息 ID：`msg_{uuid[:8]}`（第 447 行）
- Session Chain 读取的消息 ID：`msg_ev_{EventNo}`（第 1507 行）
- 同一条消息在两个来源有不同的 ID

**问题**：
- `addMessageIfNotExists` 基于 ID 去重，不同 ID 会导致同一消息重复
- `useEffect` 重触发时 `setMessages()` 用 Session Chain 版本覆盖，WS 推送的新消息（尚未写入 Chain）消失

### 根因 3：WebSocket 重连不重载消息

**文件**: `frontend/src/services/websocket.ts` 第 26-29 行

```tsx
connect(sessionId: string) {
  if (this.ws && this.sessionId === sessionId) {
    return; // 已连接到该会话 — 但断连重连后这里不会重新加载消息
  }
```

**问题**：
- WS 断开后，`scheduleReconnect` 调用 `createConnection` 重连
- 重连成功后没有通知前端重新拉取消息
- 断连期间的消息永久丢失（WS 没推送，前端也没主动拉取）

## 解决方案

### 修复 1：useEffect 依赖改为 session ID（字符串比较）

**文件**: `frontend/src/components/ChatArea/index.tsx`

将 `useEffect` 的依赖从 `currentSession` 对象改为 `currentSession?.id` 字符串：

```tsx
const currentSessionId = currentSession?.id;

useEffect(() => {
  if (!currentSessionId) return;

  messagesLoadedRef.current = false;
  pendingWsMessagesRef.current = [];

  loadMessages(currentSessionId);
  loadSessionMode(currentSessionId);

  wsService.connect(currentSessionId);

  const unsubscribeMessage = wsService.onMessage((message: Message) => {
    if (!messagesLoadedRef.current) {
      pendingWsMessagesRef.current.push(message);
      return;
    }
    addMessageIfNotExists(message);
    if (!isUserScrollingRef.current) {
      requestAnimationFrame(() => {
        messageListRef.current?.scrollToBottom();
      });
    }
  });

  return () => {
    unsubscribeMessage();
  };
}, [currentSessionId]);
```

**效果**：只有 session ID 真正变化时才重新加载，避免因对象引用变化导致的无意义重载。

### 修复 2：WebSocket 重连后通知前端重载消息

**文件**: `frontend/src/services/websocket.ts`

添加重连成功回调机制：

```tsx
private reconnectHandlers: Set<() => void> = new Set();

onReconnect(handler: () => void) {
  this.reconnectHandlers.add(handler);
  return () => this.reconnectHandlers.delete(handler);
}
```

在 `createConnection` 的 `onopen` 中，如果是重连（`reconnectAttempts > 0`），触发回调：

```tsx
this.ws.onopen = () => {
  console.log('[WS] 连接已建立');
  const wasReconnect = this.reconnectAttempts > 0;
  this.reconnectAttempts = 0;
  if (wasReconnect) {
    this.reconnectHandlers.forEach(handler => handler());
  }
};
```

在 `ChatArea` 中订阅重连事件，重连后重新拉取消息：

```tsx
const unsubscribeReconnect = wsService.onReconnect(() => {
  console.log('[ChatArea] WS reconnected, reloading messages');
  loadMessages(currentSessionId);
});
```

### 修复 3：统一消息 ID 生成策略（前端去重增强）

**文件**: `frontend/src/stores/appStore.ts`

增强 `addMessageIfNotExists`，不仅按 ID 去重，还按内容 + 时间戳近似去重：

```tsx
addMessageIfNotExists: (message) => set((state) => {
  // ID 精确去重
  const existsById = state.messages.some(m => m.id === message.id);
  if (existsById) return state;

  // 内容 + 时间近似去重（防止同一消息不同 ID 的情况）
  const existsByContent = state.messages.some(m =>
    m.content === message.content &&
    m.type === message.type &&
    m.sessionId === message.sessionId &&
    Math.abs(new Date(m.timestamp).getTime() - new Date(message.timestamp).getTime()) < 2000
  );
  if (existsByContent) return state;

  return { messages: [...state.messages, message] };
}),
```

## 修改文件清单

| 文件 | 修改内容 |
|------|---------|
| `frontend/src/components/ChatArea/index.tsx` | useEffect 依赖改为 sessionId；订阅 WS 重连事件 |
| `frontend/src/services/websocket.ts` | 添加重连回调机制 |
| `frontend/src/stores/appStore.ts` | 增强消息去重逻辑 |

## 测试建议

### 手动测试

1. **切换会话测试**：快速在多个会话之间切换，验证消息不会消失
2. **网络中断测试**：在开发者工具中模拟网络断连，恢复后验证消息完整
3. **长会话测试**：在长对话中发送消息，验证新消息正确显示
4. **会话更新测试**：重命名会话后，验证消息不受影响

### 验证方法

1. 打开浏览器开发者工具 Console
2. 观察 `[WS]` 和 `[ChatArea]` 开头的日志
3. 确认没有出现意外的 `loadMessages` 调用
4. 确认 WS 重连后有 `WS reconnected, reloading messages` 日志

## 性能影响

- `addMessageIfNotExists` 增加了内容比较，但消息列表通常不超过几百条，影响可忽略
- `useEffect` 依赖优化反而减少了不必要的 API 调用，性能有所提升