# AR-20260303-frontend-ui-enhancement

## 前端 UI 优化 — 设计文档

**作者**: 花花 🐱
**日期**: 2026-03-03
**状态**: Draft v3.1（已采纳薇薇全部审查意见，含断线重连补偿）
**优先级**: P1

---

## 1. 背景与动机

### 1.1 当前问题

铲屎官反馈了三个前端 UI 体验问题：

1. **消息缺少日期时间** — 对话框内猫猫（和用户）的消息只有内容，没有显示日期时间戳；同时 Session Chain 的 Markdown 持久化文件中，时间格式仅记录 `HH:MM:SS`，缺少日期部分
2. **侧边栏对话排序混乱** — 左侧对话列表每次打开顺序不固定，没有按最后对话时间排序
3. **摘要显示的是最早消息** — 侧边栏每个对话下方的摘要显示的是最早的消息摘要，而非最新的消息摘要

### 1.2 根因分析

#### 问题 1：消息不显示日期时间

**前端层面**：
- `frontend/src/components/ChatArea/MessageBubble.tsx` 第 20-31 行（cat 消息渲染）和第 35-44 行（user 消息渲染）中，完全没有渲染 `message.timestamp` 字段
- `Message` 类型（`frontend/src/types/index.ts` 第 9-16 行）已定义 `timestamp: Date` 字段，数据是有的，只是没显示

**后端存储层面**：
- `src/session_chain_storage.go` 第 165 行，Markdown 写入时间格式为 `e.Timestamp.Format("15:04:05")`，仅有时分秒
- 第 269-273 行，解析时用 `time.Now().Format("2006-01-02")` 补全日期，导致重启后所有历史消息的日期都变成"今天"

#### 问题 2：侧边栏对话排序混乱

- `src/api_server.go` 第 344-363 行 `ListSessions()` 方法，遍历 `map[string]*SessionContext` 构建列表，Go 的 map 遍历顺序不确定
- `frontend/src/components/Sidebar/index.tsx` 第 106-116 行 `filteredSessions` 只做了搜索过滤，没有排序逻辑

#### 问题 3：摘要显示的是最早消息

- `src/api_server.go` 第 572-579 行 `SendMessage()` 中，摘要逻辑为：
  ```go
  if ctx.Summary == "" && len(req.Content) > 0 {
      summary := req.Content
      if len(summary) > 30 {
          summary = summary[:30] + "..."
      }
      ctx.Summary = fmt.Sprintf("用户：%s", summary)
  }
  ```
  仅在 `Summary` 为空时（即第一条消息）设置摘要，之后不再更新
- `handleResult()` 中（Agent 回复处理）也没有更新摘要的逻辑

### 1.3 目标

1. 在对话框内每条消息（cat / user）下方显示 `YYYY-MM-DD HH:MM` 格式的日期时间
2. Markdown 持久化文件中记录完整日期时间 `YYYY-MM-DD HH:MM:SS`
3. 左侧对话列表按 `updatedAt` 降序排列（最近对话在最上面）
4. 对话摘要始终显示最新一条消息的摘要

---

## 2. 解决方案

### 2.1 Feature 1：消息显示日期时间

#### 2.1.1 前端 — MessageBubble 组件增加时间戳显示

在 `frontend/src/components/ChatArea/MessageBubble.tsx` 中，为 cat 和 user 类型的消息增加时间戳渲染。

**时间格式化工具函数**：
```tsx
const formatMessageTime = (timestamp: Date): string => {
  const date = new Date(timestamp);
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');
  return `${year}-${month}-${day} ${hours}:${minutes}`;
};
```

**Cat 消息渲染（约第 20-31 行）— 增加时间戳**：
```tsx
if (message.type === 'cat') {
  const cat = message.sender as { id: string; name: string; avatar: string; color?: string };
  return (
    <div className="flex items-start gap-4">
      <Avatar color={cat.color || '#ff9966'} size="md" className="rounded-3xl" avatar={cat.avatar} />
      <div>
        <div className="cat-message max-w-md prose prose-sm">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {message.content}
          </ReactMarkdown>
        </div>
        <p className="text-xs text-gray-400 mt-1 ml-1">
          {formatMessageTime(message.timestamp)}
        </p>
      </div>
    </div>
  );
}
```

**User 消息渲染（约第 35-44 行）— 增加时间戳**：
```tsx
// user message
const user = message.sender as { id: string; name: string; avatar: string };
return (
  <div className="flex items-start gap-4 justify-end">
    <div>
      <div className="user-message max-w-md prose prose-sm">
        <ReactMarkdown remarkPlugins={[remarkGfm]}>
          {message.content}
        </ReactMarkdown>
      </div>
      <p className="text-xs text-gray-400 mt-1 mr-1 text-right">
        {formatMessageTime(message.timestamp)}
      </p>
    </div>
    <Avatar color="#336699" size="md" className="rounded-xl" avatar={user?.avatar} />
  </div>
);
```

**React.memo 比较函数更新（第 46-51 行）**：
需要在 memo 比较函数中增加 `timestamp` 的比较：
```tsx
}, (prevProps, nextProps) => {
  return prevProps.message.id === nextProps.message.id &&
         prevProps.message.content === nextProps.message.content &&
         prevProps.message.type === nextProps.message.type &&
         prevProps.message.sender === nextProps.message.sender &&
         prevProps.message.timestamp === nextProps.message.timestamp;
});
```

#### 2.1.2 后端 — Markdown 持久化包含完整日期

在 `src/session_chain_storage.go` 第 165 行，将时间格式从 `15:04:05` 改为 `2006-01-02 15:04:05`：

```go
// 修改前
ts := e.Timestamp.Format("15:04:05")

// 修改后
ts := e.Timestamp.Format("2006-01-02 15:04:05")
```

对应地，解析函数 `parseEventsFromMarkdown` 中第 268-273 行需要更新解析逻辑，同时保持对旧格式的向后兼容：

**重要**：`parseEventsFromMarkdown` 需要接收 `baseDate` 参数，用于旧格式兼容时补全日期（由薇薇审查指出）。

```go
// 函数签名修改：增加 baseDate 参数
func parseEventsFromMarkdown(body string, baseDate time.Time) []SessionEvent {

// 调用处修改（ReadSessionChain 中）：
// 使用 frontmatter 中的 createdAt 作为 baseDate
events := parseEventsFromMarkdown(body, session.CreatedAt)
```

时间解析逻辑（第 268-273 行）：

```go
if strings.HasPrefix(rest, "[") {
    endBracket := strings.Index(rest, "]")
    if endBracket > 0 {
        tsStr := rest[1:endBracket]
        // 先尝试新格式（带日期）
        if t, err := time.Parse("2006-01-02 15:04:05", tsStr); err == nil {
            e.Timestamp = t
        } else {
            // 兼容旧格式（仅时间）— 使用 session 的 createdAt 日期而非 time.Now()
            dateStr := baseDate.Format("2006-01-02")
            if t, err := time.Parse("2006-01-02 15:04:05", dateStr+" "+tsStr); err == nil {
                e.Timestamp = t
            }
        }
        rest = strings.TrimSpace(rest[endBracket+1:])
    }
}
```

> **设计决策**（花花 & 薇薇达成一致）：旧格式 `HH:MM:SS` 使用 session 的 `createdAt` 日期补全，而非 `time.Now()`。这样历史消息不会被错误标记为"今天"。对于跨天对话，时间仍可能有偏差，但比伪造成当天日期要合理得多。

**Markdown 输出示例变化**：
```markdown
<!-- 修改前 -->
### #1 [14:30:05] **[用户]** <!-- msg_abc12345 -->

你好，花花

<!-- 修改后 -->
### #1 [2026-03-03 14:30:05] **[用户]** <!-- msg_abc12345 -->

你好，花花
```

### 2.2 Feature 2：侧边栏对话按时间排序

#### 2.2.1 后端 — ListSessions 返回排序后的列表

在 `src/api_server.go` 的 `ListSessions()` 方法（第 344-363 行）中，在返回前按 `UpdatedAt` 降序排序：

```go
func (sm *SessionManager) ListSessions() []Session {
    sm.mu.RLock()
    defer sm.mu.RUnlock()

    sessions := make([]Session, 0, len(sm.sessions))
    for _, ctx := range sm.sessions {
        sessions = append(sessions, Session{
            ID:            ctx.ID,
            Name:          ctx.Name,
            Summary:       ctx.Summary,
            UpdatedAt:     ctx.UpdatedAt,
            MessageCount:  ctx.MessageCount,
            WorkspaceID:   ctx.WorkspaceID,
            WorkspacePath: sm.getWorkspacePath(ctx.WorkspaceID),
        })
    }

    // 按 UpdatedAt 降序排序（最新对话排在最前面）
    sort.Slice(sessions, func(i, j int) bool {
        return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
    })

    return sessions
}
```

> 注意：`sort` 包已在 `session_chain.go` 中被 import，但 `api_server.go` 需要额外添加 `"sort"` import。

#### 2.2.2 前端 — 双重保障排序

虽然后端已返回排序后的数据，但前端也应在 `filteredSessions` 中增加排序逻辑，作为双重保障（防止 WebSocket 实时更新打乱顺序）：

`frontend/src/components/Sidebar/index.tsx` 第 106-116 行修改：

```tsx
const filteredSessions = useMemo(() => {
  let result = sessions;

  if (searchQuery.trim()) {
    const query = searchQuery.toLowerCase();
    result = result.filter(session =>
      session.name.toLowerCase().includes(query) ||
      (session.summary && session.summary.toLowerCase().includes(query))
    );
  }

  // 按 updatedAt 降序排序（最新对话排在最前面）
  return [...result].sort((a, b) => {
    return new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime();
  });
}, [sessions, searchQuery]);
```

### 2.3 Feature 3：摘要显示最新消息

#### 2.3.1 后端 — 每条消息都更新摘要

**修改 `SendMessage()` 中的摘要逻辑**（`src/api_server.go` 第 572-579 行）：

> **注意**：统一抽取摘要生成函数，使用 `[]rune` 按字符截断而非按字节，避免中文/emoji 截断乱码（由薇薇审查指出）。

**新增公共函数**（建议放在 `api_server.go` 顶部或工具函数区域）：
```go
// truncateSummary 按字符（rune）截断摘要，避免中文/emoji 截断乱码
func truncateSummary(content string, prefix string, maxRunes int) string {
    runes := []rune(content)
    if len(runes) > maxRunes {
        content = string(runes[:maxRunes]) + "..."
    }
    return fmt.Sprintf("%s：%s", prefix, content)
}
```

```go
// 修改前：仅在摘要为空时设置，且按字节截断
if ctx.Summary == "" && len(req.Content) > 0 {
    summary := req.Content
    if len(summary) > 30 {
        summary = summary[:30] + "..."
    }
    ctx.Summary = fmt.Sprintf("用户：%s", summary)
}

// 修改后：每条用户消息都更新摘要，按字符截断
if len(req.Content) > 0 {
    ctx.Summary = truncateSummary(req.Content, "用户", 30)
}
```

**修改 `handleResult()` 中增加摘要更新**（`src/api_server.go` 约第 1350-1363 行）：

在 Agent 回复消息构建后，增加摘要更新逻辑：

```go
// 在 agentMsg 构建之后，AutoSaveSession 之前添加：
// 更新摘要为最新的猫猫回复
ctx.Summary = truncateSummary(task.Result, task.AgentName, 30)
```

---

## 2.5 补充 Feature：摘要/排序持久化时序修正（由薇薇审查指出）

### 2.5.1 问题

当前 `SendMessage()` 中 `AutoSaveSession(sessionID)` 在第 495 行调用，而摘要更新在第 572-579 行。由于 `AutoSaveSession` 是异步 goroutine，摘要更新和持久化之间存在 **竞态条件**：大多数情况下 goroutine 还没执行到 `SaveSession` 时摘要就已经更新了，但这不是确定性保证。

`handleResult()` 中也是类似：`AutoSaveSession` 在第 1390 行，而我们需要在其之前更新摘要。

### 2.5.2 方案

**`SendMessage()` 中**：将摘要更新代码移动到 `AutoSaveSession` 调用之前。具体来说：

```go
// 当前顺序（有风险）：
// 1. wsHub.BroadcastToSession(...)  // 第 492 行
// 2. AutoSaveSession(sessionID)     // 第 495 行
// 3. ... 编排器处理 ...
// 4. 摘要更新                        // 第 572 行

// 修改后顺序：
// 1. 摘要更新（移到这里）
// 2. ctx.UpdatedAt = time.Now()
// 3. ctx.MessageCount++
// 4. wsHub.BroadcastToSession(...)
// 5. AutoSaveSession(sessionID)
// 6. ... 编排器处理 ...
```

**`handleResult()` 中**：在 `AutoSaveSession` 之前增加摘要更新：

```go
// 更新摘要为最新猫猫回复
ctx.Summary = truncateSummary(task.Result, task.AgentName, 30)
ctx.UpdatedAt = time.Now()

// 自动保存会话（此时 summary 已更新）
sm.AutoSaveSession(task.SessionID)
```

> **花花 & 薇薇达成一致**：将 summary/updatedAt/messageCount 的更新聚合为一个统一的"session metadata update"步骤，放在 `AutoSaveSession` 之前，确保持久化数据的正确性。

---

## 2.6 补充 Feature：会话元数据实时同步到前端（由薇薇审查指出）

### 2.6.1 问题

当前前端的会话列表只在 `App.tsx` 启动时通过 `sessionAPI.getSessions()` 拉取一次。后续发送消息和收到回复时，虽然后端更新了 `summary` 和 `updatedAt`，但前端 `sessions` store 不会收到这些变更，导致：

- 排序虽然加了，但新消息后不会实时重排
- 摘要虽然后端更新了，但前端侧边栏不会刷新

### 2.6.2 跨会话广播问题（由薇薇 v2 审查指出）

**问题**：当前 WebSocket 架构是按 session 分桶的（`websocket.go` 第 29 行：`clients map[string]map[*WSClient]bool`），前端也只连接当前查看的 session（`websocket.ts` 第 27-36 行：切换会话时 disconnect 旧连接、connect 新连接）。如果 `session_updated` 事件用 `BroadcastToSession(sessionID, ...)` 广播，**用户切走后旧会话的 WS 连接已断开，后台猫猫的异步回复推送不到前端**。

**影响场景**：用户在会话 A 发消息，然后切到会话 B。此时 A 的猫猫回复异步到达，但前端已经断开 A 的 WS 连接，左侧栏中 A 的摘要和排序无法实时更新。

### 2.6.3 方案：全局广播 + WS 事件 双管齐下

解决思路：**`session_updated` 事件不走 session 分桶广播，而走全局广播**。同时前端订阅位置从 `ChatArea`（跟随 session 生命周期）移到 `App`（全局生命周期）。

#### 后端改动

**1. `websocket.go` 新增 `BroadcastToAll` 方法：**

```go
// BroadcastToAll 向所有已连接的客户端广播消息（不分 session）
func (h *WSHub) BroadcastToAll(msgType string, data interface{}) {
    message := WSMessage{
        Type:      msgType,
        Data:      data,
        Timestamp: time.Now(),
    }
    h.mu.RLock()
    defer h.mu.RUnlock()

    for _, clients := range h.clients {
        for client := range clients {
            select {
            case client.send <- message:
            default:
                // 缓冲区满，跳过（不在读锁中删除 client，避免死锁）
            }
        }
    }
}
```

**2. `api_server.go` 中 `session_updated` 改用 `BroadcastToAll`：**

```go
// 在 SendMessage() 和 handleResult() 中：
sm.wsHub.BroadcastToAll("session_updated", map[string]interface{}{
    "id":           sessionID,
    "summary":      ctx.Summary,
    "updatedAt":    ctx.UpdatedAt,
    "messageCount": ctx.MessageCount,
})
```

> **设计决策**：只有 `session_updated` 走全局广播，其他消息类型（`message`、`history`、`chain_status`）仍然走 session 分桶广播。因为 `session_updated` 的载荷极小（仅 4 个字段），且频率不高（每条消息/回复一次），全局广播不会带来性能问题。而消息内容走全局广播则会有隐私/性能风险。

#### 前端改动

**1. `websocket.ts` 增加 `session_updated` 类型处理：**

```typescript
type WSMessageType = 'message' | 'history' | 'stats' | 'cats' | 'chain_status' | 'session_updated';

// 新增 handler 类型
type SessionUpdatedHandler = (data: { id: string; summary: string; updatedAt: string; messageCount: number }) => void;

// WebSocketService 中增加：
private sessionUpdatedHandlers: Set<SessionUpdatedHandler> = new Set();

// handleMessage 的 switch 中增加：
case 'session_updated':
    this.sessionUpdatedHandlers.forEach(handler => handler(wsMessage.data));
    break;

// 新增订阅方法：
onSessionUpdated(handler: SessionUpdatedHandler) {
    this.sessionUpdatedHandlers.add(handler);
    return () => this.sessionUpdatedHandlers.delete(handler);
}
```

**2. 订阅位置：`App.tsx`（全局级别）而非 `ChatArea/index.tsx`（会话级别）：**

```typescript
// App.tsx
import { wsService } from './services/websocket';

function App() {
  const { setSessions, updateSession } = useAppStore();

  useEffect(() => {
    loadSessions();

    // 全局订阅 session 元数据更新（不依赖当前连接的 session）
    const unsubSessionUpdate = wsService.onSessionUpdated((data) => {
      updateSession(data.id, {
        summary: data.summary,
        updatedAt: data.updatedAt,
        messageCount: data.messageCount,
      });
    });

    // 断线重连后全量刷新 sessions，补偿断线期间丢失的 session_updated 事件
    const unsubReconnect = wsService.onReconnect(() => {
      loadSessions();
    });

    return () => {
      unsubSessionUpdate();
      unsubReconnect();
    };
  }, []);

  // ...
}
```

> **关键设计点**：订阅放在 `App.tsx` 而非 `ChatArea/index.tsx`。因为 `ChatArea` 的 `useEffect` 依赖 `currentSessionId`，切换 session 会重新订阅，切走后旧订阅被清理。而 `App` 是全局不卸载的，handler 始终存在，只要任何一个 session 的 WS 连接还活着，就能收到全局广播的 `session_updated`。

> **花花 & 薇薇达成一致**：
> - v2 方案选择了 `BroadcastToSession` + `ChatArea` 订阅，薇薇正确指出这无法覆盖"用户切走后旧会话异步回复"的场景
> - v3 方案改为 `BroadcastToAll` + `App` 全局订阅，覆盖已建立任一会话 WS 连接后的跨会话更新场景
> - 仅 `session_updated` 走全局广播，其他消息类型不变，最小化改动范围
> - v3.1 补充：`App.tsx` 订阅 `onReconnect` 重连事件，重连后执行 `loadSessions()` 全量刷新，作为断线丢失事件的兜底补偿（薇薇 v3 审查指出）

---

## 3. 实现细节

### 3.1 文件改动一览

| 文件 | 改动类型 | 改动说明 | 关联 Feature |
|------|----------|----------|-------------|
| `frontend/src/components/ChatArea/MessageBubble.tsx` | 修改 | 增加时间戳显示、更新 memo 比较函数 | F1 |
| `src/session_chain_storage.go` | 修改 | 时间格式从 `HH:MM:SS` → `YYYY-MM-DD HH:MM:SS`；解析逻辑向后兼容（使用 session createdAt 日期） | F1 |
| `src/api_server.go` | 修改 | ① ListSessions 排序 ② SendMessage 摘要逻辑（rune 截断）③ handleResult 摘要逻辑 ④ 摘要更新时序修正 ⑤ 新增 session_updated WS 推送（改用 BroadcastToAll）⑥ 新增 truncateSummary 工具函数 | F2, F3, F2.5, F2.6 |
| `src/websocket.go` | 修改 | 新增 `BroadcastToAll` 全局广播方法 | F2.6（v3 新增） |
| `frontend/src/components/Sidebar/index.tsx` | 修改 | filteredSessions 增加排序 | F2 |
| `frontend/src/services/websocket.ts` | 修改 | 新增 `session_updated` 消息类型和 handler | F2.6 |
| `frontend/src/App.tsx` | 修改 | 全局订阅 `session_updated` 事件，调用 `updateSession()` | F2.6（v3 改动：从 ChatArea 移到 App） |

### 3.2 代码位置定位

#### MessageBubble.tsx（F1）
- **第 20-31 行**：cat 消息渲染 → 增加时间戳 `<p>` 标签
- **第 35-44 行**：user 消息渲染 → 增加时间戳 `<p>` 标签
- **第 46-51 行**：memo 比较函数 → 增加 timestamp 比较
- **新增**：`formatMessageTime()` 工具函数（文件顶部）

#### session_chain_storage.go（F1）
- **第 165 行**：`e.Timestamp.Format("15:04:05")` → `e.Timestamp.Format("2006-01-02 15:04:05")`
- **第 232 行**：`parseEventsFromMarkdown(body)` → `parseEventsFromMarkdown(body, session.CreatedAt)`
- **第 237 行**：函数签名增加 `baseDate time.Time` 参数
- **第 268-273 行**：解析逻辑 → 先尝试新格式，fallback 到旧格式（使用 baseDate 而非 time.Now()）

#### api_server.go（F2 + F3 + F2.5 + F2.6）
- **第 344-363 行**：`ListSessions()` → 返回前增加 `sort.Slice`
- **第 1 行**：import 中添加 `"sort"`
- **新增**：`truncateSummary()` 工具函数（按 `[]rune` 截断）
- **第 572-579 行**：`SendMessage()` 摘要逻辑 → 使用 `truncateSummary()`，移到 `AutoSaveSession` 之前
- **第 1358-1362 行**：`handleResult()` → Agent 回复后更新摘要，移到 `AutoSaveSession` 之前
- **第 492/1381 行附近**：`SendMessage()` / `handleResult()` 中 `AutoSaveSession` 之后新增 `session_updated` WS 推送

#### Sidebar/index.tsx（F2）
- **第 106-116 行**：`filteredSessions` → 增加 `.sort()` 排序逻辑

#### websocket.go（F2.6 v3 新增）
- **新增**：`BroadcastToAll()` 全局广播方法

#### websocket.ts（F2.6）
- 新增 `session_updated` 消息类型
- 新增 `SessionUpdatedHandler` 和对应的 handler 集合
- `handleMessage` switch 中新增 `session_updated` case
- 新增 `onSessionUpdated()` 订阅方法

#### App.tsx（F2.6 v3 改动）
- 新增 `useEffect` 中全局订阅 `session_updated` 事件，调用 `updateSession()`
- **注意**：v2 方案将订阅放在 `ChatArea/index.tsx`，v3 改为放在 `App.tsx`（全局级别，不随 session 切换而重建）

### 3.3 向后兼容性

| 变更点 | 向后兼容处理 |
|--------|-------------|
| Markdown 时间格式变化 | 解析函数同时支持 `HH:MM:SS` 和 `YYYY-MM-DD HH:MM:SS` 两种格式；旧格式使用 session `createdAt` 日期补全，不再伪造为当天 |
| 摘要更新逻辑变化 | 不影响已有数据，已有摘要会在下次新消息时自动更新；使用 `[]rune` 按字符截断，支持中文/emoji |
| 排序逻辑 | 不影响数据结构，纯展示层变化 |
| 新增 session_updated WS 事件 | 前端新增 handler 处理即可，旧客户端会忽略未知 WS 类型 |
| 新增 BroadcastToAll 方法 | 仅用于 `session_updated`，不影响其他消息类型的分桶广播行为；载荷极小、频率低，不会产生性能问题 |

---

## 4. 测试验证

### 4.1 Feature 1：消息时间戳

- [ ] cat 类型消息在气泡下方显示 `YYYY-MM-DD HH:MM` 格式时间
- [ ] user 类型消息在气泡下方显示 `YYYY-MM-DD HH:MM` 格式时间（右对齐）
- [ ] system 类型消息不显示时间戳
- [ ] 新写入的 Markdown 文件包含完整日期时间 `[2026-03-03 14:30:05]`
- [ ] 旧格式 Markdown 文件（仅 `[14:30:05]`）能正确解析，不会报错
- [ ] **旧格式 Markdown 文件解析后日期为 session 的 createdAt 日期，不是当天日期**（薇薇审查新增）
- [ ] 时间戳样式协调，不影响消息列表的视觉平衡

### 4.2 Feature 2：侧边栏排序

- [ ] 启动后侧边栏按最后对话时间降序排列
- [ ] 发送新消息后，当前对话自动排到最上面
- [ ] 收到猫猫回复后，当前对话自动排到最上面
- [ ] 搜索结果也保持时间排序
- [ ] 新建对话排在最上面

### 4.3 Feature 3：摘要更新

- [ ] 发送用户消息后，侧边栏摘要更新为最新用户消息摘要
- [ ] 收到猫猫回复后，侧边栏摘要更新为最新猫猫回复摘要
- [ ] 摘要格式：`用户：xxx...` 或 `花花：xxx...`
- [ ] 摘要超过 30 字符正确截断并加 `...`
- [ ] **中文、emoji、中英混合文本截断不乱码**（薇薇审查新增）
- [ ] 第一条消息和后续消息的摘要都能正确更新
- [ ] **发消息后不刷新页面，侧边栏摘要立即更新**（薇薇审查新增）
- [ ] **发消息后重启服务，摘要持久化正确、不回退**（薇薇审查新增）
- [ ] **跨会话场景：在 A 发消息后切到 B，等 A 的猫猫回复到达，左侧栏 A 的摘要和排序实时更新**（薇薇 v2 审查新增）
- [ ] **两个会话同时有未完成的异步任务，不同顺序收到回复，侧边栏始终按最新 updatedAt 排序**（薇薇 v2 审查新增）
- [ ] **断线重连补偿：WS 断开期间 A 收到猫猫回复，重连后侧边栏摘要和排序自动恢复一致**（薇薇 v3 审查新增）
- [ ] **未选中任何会话时的行为：明确此场景不在实时推送覆盖范围内，首次选中会话后自动建立连接并同步**（薇薇 v3 审查新增）

### 4.4 回归测试

- [ ] 消息发送和接收流程正常
- [ ] WebSocket 实时推送正常
- [ ] 会话持久化（Redis 保存/加载）正常
- [ ] Session Chain Markdown 读写正常
- [ ] 删除、重命名对话正常

---

## 5. 风险与注意事项

### 5.1 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 旧 Markdown 时间格式解析失败 | 历史消息时间戳丢失 | 实现双格式兼容解析，旧格式使用 session createdAt 日期补全 |
| 摘要频繁更新导致 Redis 写入压力 | 性能影响 | 摘要更新已包含在 `AutoSaveSession` 的异步保存中，无额外开销 |
| 时间戳显示占用气泡空间 | UI 显得拥挤 | 使用 `text-xs text-gray-400` 小字灰色，不喧宾夺主 |
| 前端排序与后端排序不一致 | 闪烁 | 后端先排好，前端再保障，两层一致 |
| 摘要更新与持久化的竞态条件 | 重启后摘要回退 | 将摘要更新移到 `AutoSaveSession` 之前（薇薇审查指出） |
| 中文/emoji 截断乱码 | 摘要显示异常 | 使用 `[]rune` 按字符截断（薇薇审查指出） |
| 跨会话 WS 广播不到 | 切走后旧会话的侧边栏摘要/排序不更新 | `session_updated` 改用 `BroadcastToAll` 全局广播 + `App.tsx` 全局订阅（薇薇 v2 审查指出） |
| WS 断线丢失 `session_updated` 事件 | 侧边栏摘要/排序停留在旧值 | `App.tsx` 订阅 `onReconnect`，重连后 `loadSessions()` 全量刷新（薇薇 v3 审查指出） |

### 5.2 注意事项

1. `sort` 包需要在 `api_server.go` 的 import 中添加
2. 前端排序使用 `[...result].sort()` 创建新数组，避免 mutation
3. `formatMessageTime` 中需要 `new Date(timestamp)` 处理字符串格式的时间戳（API 返回的可能是 ISO 字符串）
4. memo 比较函数增加 timestamp 比较，需注意 Date 对象的引用比较问题（可能需要比较 `.getTime()`）
5. `parseEventsFromMarkdown` 函数签名变更，所有调用处需要同步更新
6. `truncateSummary` 函数需统一使用，避免 user 和 cat 两条路径的重复截断逻辑
7. `BroadcastToAll` 仅用于 `session_updated`，其他消息类型继续用 `BroadcastToSession`，避免隐私泄露
8. `App.tsx` 中的 `session_updated` 订阅是全局级别的，不会随 session 切换而重建
9. `App.tsx` 同时订阅 `onReconnect`，断线恢复后执行 `loadSessions()` 全量刷新，确保不因丢失事件导致状态漂移
10. 实时推送方案的覆盖范围是"已建立任一会话 WS 连接后的跨会话更新"；用户未选中任何会话时不在实时推送范围内（启动后选中首个会话即可覆盖）

---

## 6. 实现计划

### Phase 1: 消息时间戳（F1）
1. 修改 `MessageBubble.tsx` — 增加时间格式化函数和时间戳渲染
2. 修改 `session_chain_storage.go` — 更新时间写入格式和解析逻辑
3. 验证新旧 Markdown 格式兼容性

### Phase 2: 侧边栏排序（F2）
1. 修改 `api_server.go` `ListSessions()` — 增加排序
2. 修改 `Sidebar/index.tsx` — `filteredSessions` 增加排序
3. 验证各种场景下排序正确性

### Phase 3: 摘要更新 + 持久化时序修正（F3 + F2.5）
1. 新增 `truncateSummary()` 工具函数（`[]rune` 截断）
2. 修改 `api_server.go` `SendMessage()` — 摘要逻辑移到 `AutoSaveSession` 之前
3. 修改 `api_server.go` `handleResult()` — Agent 回复更新摘要，移到 `AutoSaveSession` 之前
4. 验证摘要实时更新效果 + 重启后持久化正确性

### Phase 4: 会话元数据实时同步（F2.6）
1. 后端 `websocket.go` 新增 `BroadcastToAll()` 全局广播方法
2. 后端 `SendMessage()` / `handleResult()` 中新增 `session_updated` WS 推送（使用 `BroadcastToAll`）
3. 前端 `websocket.ts` 新增 `session_updated` handler
4. 前端 `App.tsx` 全局订阅 `session_updated` 事件，调用 `updateSession()`
5. 验证当前会话场景 + 跨会话场景下摘要和排序的实时更新

### Phase 5: 集成测试
1. 全流程回归测试
2. 旧数据兼容性测试
3. 性能观察

---

## 7. 验收标准

1. ✅ 对话框内 cat / user 消息下方显示 `YYYY-MM-DD HH:MM` 格式时间
2. ✅ Session Chain Markdown 文件记录完整 `YYYY-MM-DD HH:MM:SS` 时间
3. ✅ 旧格式 Markdown（仅 HH:MM:SS）能正确解析，向后兼容
4. ✅ 侧边栏对话列表按最后对话时间降序排列
5. ✅ 新消息产生后对话自动排到最前面
6. ✅ 摘要始终显示最新一条消息的摘要
7. ✅ 所有功能测试和回归测试通过
8. ✅ 旧格式 Markdown 日期使用 session createdAt 补全，不伪造为当天（v2 新增）
9. ✅ 摘要截断支持中文/emoji，不乱码（v2 新增）
10. ✅ 不刷新页面，发消息后侧边栏摘要和排序实时更新（v2 新增）
11. ✅ 发消息后重启服务，摘要持久化正确不回退（v2 新增）
12. ✅ 跨会话场景：切走后旧会话的异步回复仍能实时更新侧边栏（v3 新增）
13. ✅ 断线重连后侧边栏自动恢复一致，不需要手动刷新页面（v3.1 新增）

---

## 8. 审查记录

### 8.1 薇薇审查 — 2026-03-04

薇薇对 v1 SPEC 提出了 4 个审查意见，花花逐条分析如下：

| # | 严重级别 | 薇薇的问题 | 花花的判断 | 处理结果 |
|---|----------|-----------|-----------|----------|
| 1 | 高 | 前端会话列表没有订阅 session 元数据变更，排序和摘要无法实时更新 | **完全合理**。确认当前 WS 只推送 `message`/`history`/`chain_status` 三种类型，没有 session 元数据更新通道。`sessions` store 只在启动时拉取一次。 | ✅ 采纳 — 新增 §2.6 `session_updated` WS 事件方案 |
| 2 | 高 | 旧时间格式兼容方案仍使用 `time.Now()` 补全日期，历史数据会被错误标记为当天 | **完全合理**。旧代码原本就有这个 bug，SPEC v1 只是继承了错误。使用 session `createdAt` 日期更合理。 | ✅ 采纳 — 修改 §2.1.2，`parseEventsFromMarkdown` 增加 `baseDate` 参数 |
| 3 | 中 | 摘要截断用 `len()/[:30]` 按字节切，中文会乱码 | **完全合理**。Go 的 `string` 是 UTF-8 字节序列，直接切片会破坏多字节字符。 | ✅ 采纳 — 修改 §2.3.1，新增 `truncateSummary()` 使用 `[]rune` 截断 |
| 4 | 中 | `AutoSaveSession` 在摘要更新之前调用，存在竞态条件 | **合理但实际风险较低**。`AutoSaveSession` 是 `go func()` 异步执行，绝大多数情况下摘要更新会先于 goroutine 内的 `SaveSession` 完成。但作为工程规范，确保时序正确是应该的。 | ✅ 采纳 — 新增 §2.5，将摘要更新移到 `AutoSaveSession` 之前 |

**结论**：4 条审查意见全部采纳，已更新至 SPEC v2。

### 8.2 薇薇复核 v2 — 2026-03-04

薇薇对 v2 SPEC 进行复核后，确认前 4 条意见已落实，但发现 1 个新的边界问题：

| # | 严重级别 | 薇薇的问题 | 花花的判断 | 处理结果 |
|---|----------|-----------|-----------|----------|
| 5 | 中 | `session_updated` 走 `BroadcastToSession` + `ChatArea` 订阅，无法覆盖"用户切走后旧会话收到异步回复"的场景。WS 按 session 分桶，切走后连接断开，推送不到前端 | **完全合理**。确认了 `websocket.go` 的 `clients` 确实是 `map[string]map[*WSClient]bool` 按 session 分桶；前端 `websocket.ts` 切换会话时 disconnect 旧连接。这意味着非当前会话的 `session_updated` 事件完全丢失。 | ✅ 采纳 — 重写 §2.6，`session_updated` 改用新增的 `BroadcastToAll` 全局广播，订阅位置从 `ChatArea` 移到 `App.tsx` |

**花花的分析**：

这个问题的根源是 v2 方案复用了现有的 `BroadcastToSession` 机制，但没有意识到 **session 元数据更新需要被"所有会话"的客户端看到**（因为侧边栏显示的是全部会话的摘要和排序）。这和"消息内容只需要被当前会话的客户端看到"在广播范围上有本质区别。

v3 方案的核心改动：
1. 后端新增 `BroadcastToAll()` 方法，只用于 `session_updated`（载荷小、频率低，无性能担忧）
2. 前端订阅从 `ChatArea/index.tsx`（会话级生命周期）移到 `App.tsx`（全局级生命周期）
3. 其他消息类型不受影响，继续走 `BroadcastToSession`

**结论**：第 5 条审查意见采纳，已更新至 SPEC v3。

### 8.3 薇薇复核 v3 — 2026-03-04

薇薇对 v3 SPEC 进行复核后，确认跨会话主场景已覆盖，但提出 2 个表述和兜底问题：

| # | 严重级别 | 薇薇的问题 | 花花的判断 | 处理结果 |
|---|----------|-----------|-----------|----------|
| 6a | 低 | "彻底解决跨会话更新问题"表述过满。用户未打开任何会话时不存在 WS 连接，此时收不到全局广播 | **认可但实际影响极小**。应用启动后默认选中首个会话，"无会话连接"状态窗口极窄。但文档措辞应当准确。 | ✅ 采纳 — 措辞改为"覆盖已建立任一会话 WS 连接后的跨会话更新场景" |
| 6b | 中 | WS 断线期间丢失的 `session_updated` 事件无法恢复。现有 `onReconnect` 只被 `ChatArea` 用来重拉消息，不覆盖会话列表元数据 | **完全合理，且实现成本极低**。在 `App.tsx` 加一行 `wsService.onReconnect(() => loadSessions())` 即可。正常靠实时推送，异常靠重连全量刷新，双层保障。 | ✅ 采纳 — `App.tsx` 新增 `onReconnect` 订阅，重连后全量刷新 sessions |

**花花的分析**：

薇薇这两条意见的本质是在区分"正常路径"和"异常路径"的覆盖度。v3 已经解决了正常路径（在线跨会话同步），但异常路径（断线恢复）确实还有一个缝隙。加上 `onReconnect` 兜底后，两层保障形成了完整的容错链：

1. **在线实时**：`BroadcastToAll` + `App` 全局 `onSessionUpdated` → 即时更新
2. **断线恢复**：`App` 全局 `onReconnect` → `loadSessions()` 全量刷新 → 补齐丢失事件

**结论**：第 6a/6b 条审查意见采纳，已更新至 SPEC v3.1。至此花花和薇薇已就全部审查意见达成一致。
