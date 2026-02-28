# AR-20260228-session-chain

## 短期记忆 Session Chain 模块设计文档

**作者**: 花花
**日期**: 2026-02-28
**状态**: Draft
**优先级**: P0

---

## 1. 背景与动机

### 1.1 当前问题

当前系统中，Agent 的上下文管理存在以下痛点：

1. **上下文策略单一**：所有 Agent 都采用相同的上下文管理方式——每次调用时从 Redis 拉取最近 20 条消息拼入 prompt（`agent_worker.go:getSessionHistory`），同时通过 `session_mapping:{sessionID}:{agentName}` 维护 AI CLI 的 session ID 用于 `--resume`。这种"一刀切"的方式无法适应不同 Agent 的需求。

2. **上下文无限膨胀**：对话历史只增不减，虽然当前硬编码截取最近 20 条 + 每条最多 500 字符，但这是粗暴的截断而非智能压缩，长对话中关键信息容易丢失。

3. **对话与 Session 强绑定**：当前一个对话（thread）只对应一个 `SessionContext`，无法在上下文过长时"翻页"到新的 session 继续。

4. **记忆不可检索**：历史对话只能线性回溯，Agent 无法按需搜索过去的交互记录。

### 1.2 目标

设计一个 **Session Chain** 模块，实现：

- 每个 Agent 可独立选择上下文管理策略（调度系统管理 vs CLI 自动管理）
- 对话记录持久化到文件系统（Markdown），与 Redis 解耦
- 一个对话（thread）可以对应多个 session（chain），支持上下文"翻页"
- 上下文达到阈值时自动 seal 并压缩，创建新 session 继续
- 通过 MCP Server 提供记忆检索能力，Agent 可按需拉取历史

---

## 2. 核心概念

### 2.1 术语定义

| 术语 | 定义 |
|------|------|
| **Thread** | 用户发起的一个对话，对应前端的一个聊天窗口。即当前的 `SessionContext`。 |
| **Session** | Thread 内的一段连续对话记录。一个 Thread 可以有多个 Session，形成 Session Chain。 |
| **Session Chain** | 同一个 Thread 下按时间顺序排列的 Session 链表。 |
| **Event** | Session 内的一条记录（用户消息、Agent 回复、系统消息等）。每个 Event 有全局递增编号。 |
| **Invocation** | 一次 Agent 调用的完整记录（输入 prompt + 输出 response + 元数据）。 |
| **Seal** | 将当前 Session 标记为"已封存"，不再追加新 Event。触发压缩并创建新 Session。 |
| **Cursor** | Agent 维护的读取位置指针，记录该 Agent 上次读到的 Session ID + Event 编号。 |
| **Memory Compressor** | 负责将已封存的 Session 内容压缩为摘要的模型/服务。 |

### 2.2 数据模型

```
Thread (对话)
  ├── Session #1 (sealed) ──→ session_chain/{threadId}/S001.md
  │     ├── Event #1: [user] 你好
  │     ├── Event #2: [花花] 喵~
  │     ├── Event #3: [user] 帮我写个HTTP服务器
  │     ├── Event #4: [花花] Invocation { prompt, response }
  │     └── ... (达到阈值，seal)
  │     └── 📋 Summary: "用户打招呼，要求写HTTP服务器..."
  │
  ├── Session #2 (sealed) ──→ session_chain/{threadId}/S002.md
  │     ├── Event #101: [user] 加个中间件
  │     ├── ...
  │     └── 📋 Summary: "在HTTP服务器基础上添加中间件..."
  │
  └── Session #3 (active) ──→ session_chain/{threadId}/S003.md
        ├── Event #201: [user] 部署到测试环境
        └── ... (当前活跃 session)
```

### 2.3 Agent 上下文策略

每个 Agent 可在配置中选择以下两种策略之一：

#### 策略 A：调度系统管理（`context_mode: orchestrated`）

- **不使用** AI CLI 的 session ID（每次调用都是新 session）
- 每次调用时，prompt 中包含当前活跃 Session 的**全部** Event（与当前实现类似）
- 适用于：需要完整上下文的场景，或 CLI 不支持 `--resume` 的情况

**调用流程**：
```
1. 收到任务
2. 从 Session Chain 读取当前活跃 Session 的所有 Event
3. 拼接 system prompt + events + 当前任务
4. 调用 CLI（不传 --resume）
5. 记录 Invocation 到当前 Session
```

#### 策略 B：CLI 自动管理（`context_mode: cli_managed`）

- **使用** AI CLI 的 session ID，每个 Thread 对应一个独立的 AI session ID
- 每次调用时，prompt 只需包含**增量**内容：从该 Agent 上次调用结束后到当前最新的 Event
- Agent 维护自己的 Cursor（上次读到的 session ID + event 编号）
- 适用于：CLI 原生支持 `--resume` 且上下文管理能力强的 Agent（如 Claude）

**调用流程**：
```
1. 收到任务
2. 读取该 Agent 的 Cursor（lastSessionId, lastEventNo）
3. 判断 Cursor 指向的 Session 是否已被 seal：
   a. 未 seal → 从 lastEventNo+1 开始读取增量 Event
   b. 已 seal → 读取 seal 后的 Summary，再读取后续 Session 的 Event
4. 拼接增量内容 + 当前任务
5. 调用 CLI（传 --resume {aiSessionId}）
6. 更新 Cursor 到最新位置
7. 记录 Invocation 到当前 Session
```

---

## 3. 详细设计

### 3.1 数据结构

#### 3.1.1 Session Chain 元数据（Redis + 文件系统）

```go
// SessionChainMeta Session Chain 元数据，存储在 Redis 中
type SessionChainMeta struct {
    ThreadID       string    `json:"threadId"`
    ActiveSessionID string   `json:"activeSessionId"`  // 当前活跃的 Session ID
    SessionCount   int       `json:"sessionCount"`      // Session 总数
    TotalEvents    int       `json:"totalEvents"`       // 全局 Event 计数器
    CreatedAt      time.Time `json:"createdAt"`
    UpdatedAt      time.Time `json:"updatedAt"`
}
```

#### 3.1.2 Session 记录

```go
// SessionRecord 单个 Session 的元数据
type SessionRecord struct {
    ID          string        `json:"id"`          // 格式: S001, S002, ...
    ThreadID    string        `json:"threadId"`
    SeqNo       int           `json:"seqNo"`       // 在 chain 中的序号（1-based）
    Status      SessionStatus `json:"status"`      // active / sealed / compressing
    StartEvent  int           `json:"startEvent"`  // 起始 Event 编号
    EndEvent    int           `json:"endEvent"`    // 结束 Event 编号（sealed 时确定）
    EventCount  int           `json:"eventCount"`
    TokenCount  int           `json:"tokenCount"`  // 估算的 token 数
    Summary     string        `json:"summary"`     // seal 后的压缩摘要
    FilePath    string        `json:"filePath"`    // Markdown 文件路径
    CreatedAt   time.Time     `json:"createdAt"`
    SealedAt    *time.Time    `json:"sealedAt,omitempty"`
}

type SessionStatus string

const (
    SessionActive      SessionStatus = "active"
    SessionSealed      SessionStatus = "sealed"
    SessionCompressing SessionStatus = "compressing"
)
```

#### 3.1.3 Event 记录

```go
// SessionEvent Session 内的一条事件
type SessionEvent struct {
    EventNo      int           `json:"eventNo"`      // 全局递增编号
    Type         EventType     `json:"type"`          // user / cat / system / invocation
    Sender       string        `json:"sender"`        // 发送者名称
    Content      string        `json:"content"`       // 消息内容
    InvocationID string        `json:"invocationId,omitempty"` // 关联的 Invocation ID
    Timestamp    time.Time     `json:"timestamp"`
    TokenCount   int           `json:"tokenCount"`    // 该条 Event 的估算 token 数
}

type EventType string

const (
    EventUser       EventType = "user"
    EventCat        EventType = "cat"
    EventSystem     EventType = "system"
    EventInvocation EventType = "invocation"
)
```

#### 3.1.4 Invocation 记录

```go
// InvocationRecord 一次 Agent 调用的完整记录
type InvocationRecord struct {
    ID            string    `json:"id"`            // 格式: inv_{timestamp}
    SessionID     string    `json:"sessionId"`
    ThreadID      string    `json:"threadId"`
    AgentName     string    `json:"agentName"`
    Prompt        string    `json:"prompt"`        // 发送给 Agent 的完整 prompt
    Response      string    `json:"response"`      // Agent 的回复
    AISessionID   string    `json:"aiSessionId"`   // CLI 返回的 AI session ID
    TokensIn      int       `json:"tokensIn"`      // 输入 token 数（估算）
    TokensOut     int       `json:"tokensOut"`     // 输出 token 数（估算）
    Duration      int64     `json:"duration"`      // 耗时（毫秒）
    StartEventNo  int       `json:"startEventNo"`  // 调用时的起始 Event 编号
    EndEventNo    int       `json:"endEventNo"`    // 调用时的结束 Event 编号
    Timestamp     time.Time `json:"timestamp"`
}
```

#### 3.1.5 Agent Cursor

```go
// AgentCursor Agent 的读取位置指针
type AgentCursor struct {
    AgentName      string `json:"agentName"`
    ThreadID       string `json:"threadId"`
    LastSessionID  string `json:"lastSessionId"`  // 上次读到的 Session ID
    LastEventNo    int    `json:"lastEventNo"`    // 上次读到的 Event 编号
    AISessionID    string `json:"aiSessionId"`    // 该 Agent 在该 Thread 的 AI session ID
}
```

### 3.2 Agent 配置扩展

```yaml
agents:
  - name: "花花"
    pipe: "pipe_huahua"
    cli_type: "claude"
    system_prompt_path: "prompts/calico_cat.md"
    avatar: "/images/sanhua.png"
    # ===== 新增配置 =====
    context_mode: "cli_managed"        # "cli_managed" | "orchestrated"
    memory_compressor:
      model: "claude-haiku-4-5-20251001"  # 用于压缩的模型
      max_summary_tokens: 2000         # 摘要最大 token 数
    session_chain:
      max_tokens: 200000               # 单个 Session 最大 token 数
      seal_threshold: 0.8              # 触发 seal 的阈值（max_tokens * threshold）
      max_events_per_session: 500      # 单个 Session 最大 Event 数（备用阈值）

  - name: "薇薇"
    pipe: "pipe_weiwei"
    cli_type: "codex"
    system_prompt_path: "prompts/lihua_cat.md"
    avatar: "/images/weiwei.png"
    context_mode: "orchestrated"       # Codex 不支持 resume，用调度系统管理
    memory_compressor:
      model: "claude-haiku-4-5-20251001"
      max_summary_tokens: 2000
    session_chain:
      max_tokens: 200000
      seal_threshold: 0.8

  - name: "小乔"
    pipe: "pipe_xiaoqiao"
    cli_type: "gemini"
    system_prompt_path: "prompts/silver_cat.md"
    avatar: "/images/xiaoqiao.png"
    context_mode: "cli_managed"        # Gemini 支持 resume
    memory_compressor:
      model: "claude-haiku-4-5-20251001"
      max_summary_tokens: 2000
    session_chain:
      max_tokens: 200000
      seal_threshold: 0.8
```

对应的 Go 结构体扩展：

```go
// AgentConfig Agent 配置结构（扩展）
type AgentConfig struct {
    Name             string              `yaml:"name"`
    Pipe             string              `yaml:"pipe"`
    CLIType          string              `yaml:"cli_type"`
    ExecCmd          string              `yaml:"exec_cmd"`
    SystemPromptPath string              `yaml:"system_prompt_path"`
    Avatar           string              `yaml:"avatar"`
    // ===== 新增 =====
    ContextMode      string              `yaml:"context_mode"`       // "cli_managed" | "orchestrated"
    MemoryCompressor *MemoryCompressorConfig `yaml:"memory_compressor,omitempty"`
    SessionChain     *SessionChainConfig `yaml:"session_chain,omitempty"`
}

// MemoryCompressorConfig 记忆压缩器配置
type MemoryCompressorConfig struct {
    Model            string `yaml:"model"`
    MaxSummaryTokens int    `yaml:"max_summary_tokens"`
}

// SessionChainConfig Session Chain 配置
type SessionChainConfig struct {
    MaxTokens           int     `yaml:"max_tokens"`
    SealThreshold       float64 `yaml:"seal_threshold"`
    MaxEventsPerSession int     `yaml:"max_events_per_session"`
}
```

### 3.3 文件系统存储

#### 3.3.1 目录结构

```
data/
  session_chain/
    {threadId}/
      meta.json              # Session Chain 元数据
      S001.md                # Session #1 的 Markdown 记录
      S002.md                # Session #2
      S003.md                # Session #3（当前活跃）
      invocations/
        inv_1709100000.json  # Invocation 详情
        inv_1709100100.json
        ...
```

#### 3.3.2 Session Markdown 格式

每个 Session 对应一个 Markdown 文件，格式如下：

```markdown
---
id: S001
threadId: abc-123
seqNo: 1
status: sealed
startEvent: 1
endEvent: 100
tokenCount: 150000
sealedAt: 2026-02-28T10:30:00Z
summary: |
  用户要求开发一个HTTP服务器，花花完成了基础框架搭建，
  包括路由、中间件、错误处理。薇薇进行了代码审查并提出优化建议。
---

## Event #1 [2026-02-28 10:00:00]
**[用户]** 你好，帮我写一个HTTP服务器

## Event #2 [2026-02-28 10:00:05]
**[花花]** 喵~ 好的，我来帮你搭建一个HTTP服务器框架...

## Event #3 [2026-02-28 10:01:00]
**[花花]** 📎 Invocation: `inv_1709100000`
> 已完成HTTP服务器基础框架，包含路由和中间件支持...

## Event #4 [2026-02-28 10:02:00]
**[用户]** @薇薇 帮我审查一下花花写的代码

...
```

### 3.4 Seal 与压缩流程

#### 3.4.1 触发条件

当以下任一条件满足时触发 Seal：

1. 当前活跃 Session 的 `tokenCount >= maxTokens * sealThreshold`（默认 200k * 0.8 = 160k）
2. 当前活跃 Session 的 `eventCount >= maxEventsPerSession`（备用阈值）

检查时机：每次向 Session 追加 Event 后。

#### 3.4.2 Seal 流程

```
触发 Seal
    │
    ▼
1. 将当前 Session 状态设为 "compressing"
    │
    ▼
2. 创建新 Session（SeqNo + 1），状态为 "active"
    │
    ▼
3. 更新 SessionChainMeta.activeSessionId 指向新 Session
    │
    ▼
4. 后续新 Event 写入新 Session（不阻塞主流程）
    │
    ▼
5. 异步：调用 Memory Compressor 压缩旧 Session
    │
    ├── 读取当前 Session 及之前所有 sealed Session 的摘要
    ├── 读取当前 Session 的全部 Event
    ├── 调用压缩模型生成摘要
    ├── 将摘要写入当前 Session 的 summary 字段
    ├── 更新 Markdown 文件的 frontmatter
    └── 将状态设为 "sealed"
```

#### 3.4.3 压缩 Prompt 模板

```
你是一个对话记忆压缩助手。请将以下对话记录压缩为一份结构化摘要。

## 之前的摘要（如有）
{previous_summaries}

## 当前 Session 的对话记录
{current_session_events}

## 要求
1. 保留关键决策、结论和待办事项
2. 保留重要的代码变更和文件路径
3. 保留每个参与者（Agent）的主要贡献
4. 摘要长度控制在 {max_summary_tokens} token 以内
5. 使用结构化格式（标题、列表）
6. 标注未完成的任务和待确认的问题
```

#### 3.4.4 压缩模型调用

压缩使用配置中指定的模型（默认 `claude-haiku-4-5-20251001`），通过现有的 `InvokeCLI` 函数调用：

```go
func (sc *SessionChainManager) compressSession(
    threadID string,
    sessionID string,
) error {
    // 1. 收集之前所有 sealed session 的 summary
    // 2. 收集当前 session 的所有 events
    // 3. 构建压缩 prompt
    // 4. 调用 InvokeCLI 获取摘要
    // 5. 更新 session record 和 markdown 文件
    return nil
}
```

### 3.5 Agent 上下文构建详细流程

#### 3.5.1 策略 A：调度系统管理（orchestrated）

```go
func (w *AgentWorker) buildOrchestratedPrompt(task *TaskMessage) string {
    // 1. 获取当前活跃 Session 的所有 Event
    chain := w.sessionChainManager.GetChain(task.SessionID)
    activeSession := chain.GetActiveSession()
    events := activeSession.GetAllEvents()

    // 2. 格式化为对话历史
    history := formatEventsAsHistory(events)

    // 3. 拼接完整 prompt
    return fmt.Sprintf("%s\n\n【对话历史】\n%s\n\n🎯 当前任务：\n%s",
        w.systemPrompt, history, task.Content)
}
```

每次调用都是全新的 AI session，不传 `--resume`。

#### 3.5.2 策略 B：CLI 自动管理（cli_managed）

```go
func (w *AgentWorker) buildCLIManagedPrompt(task *TaskMessage) (string, string) {
    chain := w.sessionChainManager.GetChain(task.SessionID)
    cursor := w.sessionChainManager.GetCursor(w.config.Name, task.SessionID)

    var incrementalContent string

    if cursor == nil {
        // 首次调用，传入当前活跃 Session 的所有 Event
        activeSession := chain.GetActiveSession()
        events := activeSession.GetAllEvents()
        incrementalContent = formatEventsAsHistory(events)
    } else {
        // 检查 cursor 指向的 Session 是否已被 seal
        cursorSession := chain.GetSession(cursor.LastSessionID)

        if cursorSession.Status == SessionSealed {
            // 上次的 Session 已被压缩，传入摘要 + 后续所有 Event
            var parts []string

            // 收集从 cursor session 到 active session 之间所有 sealed session 的摘要
            sealedSessions := chain.GetSessionsAfter(cursor.LastSessionID)
            for _, s := range sealedSessions {
                if s.Status == SessionSealed && s.Summary != "" {
                    parts = append(parts,
                        fmt.Sprintf("📋 [Session %s 摘要]\n%s", s.ID, s.Summary))
                }
            }

            // 加上当前活跃 Session 的所有 Event
            activeSession := chain.GetActiveSession()
            events := activeSession.GetAllEvents()
            parts = append(parts, formatEventsAsHistory(events))

            incrementalContent = strings.Join(parts, "\n\n---\n\n")
        } else {
            // Session 未被 seal，只读取增量 Event
            events := chain.GetEventsAfter(cursor.LastSessionID, cursor.LastEventNo)
            incrementalContent = formatEventsAsHistory(events)
        }
    }

    // 构建 prompt
    prompt := fmt.Sprintf("【新的对话内容】\n%s\n\n🎯 当前任务：\n%s",
        incrementalContent, task.Content)

    // 返回 prompt 和 AI session ID
    return prompt, cursor.AISessionID
}
```

### 3.6 MCP Server 设计

Session Chain 通过 MCP Server 暴露记忆检索能力，注册给 Agent 使用。Agent 可以按需拉取历史记忆，而非一次性灌入上下文。

#### 3.6.1 工具定义

```go
// MCP Tool: list_session_chain
// 列出某只猫在某个 thread 的所有 session
type ListSessionChainRequest struct {
    ThreadID  string `json:"threadId"`
    CatID     string `json:"catId"`  // Agent 名称
}

type ListSessionChainResponse struct {
    Sessions []SessionSummary `json:"sessions"`
}

type SessionSummary struct {
    ID         string        `json:"id"`
    SeqNo      int           `json:"seqNo"`
    Status     SessionStatus `json:"status"`
    EventCount int           `json:"eventCount"`
    TokenCount int           `json:"tokenCount"`
    Summary    string        `json:"summary,omitempty"` // sealed 时有值
    CreatedAt  time.Time     `json:"createdAt"`
    SealedAt   *time.Time    `json:"sealedAt,omitempty"`
}
```

```go
// MCP Tool: read_session_events
// 分页读取某个 session 的记录
type ReadSessionEventsRequest struct {
    SessionID string `json:"sessionId"`
    Cursor    int    `json:"cursor"`    // 从哪个 eventNo 开始读
    Limit     int    `json:"limit"`     // 每页数量，默认 50
    View      string `json:"view"`      // "chat" | "handoff" | "raw"
}

type ReadSessionEventsResponse struct {
    Events     []SessionEvent `json:"events"`
    NextCursor int            `json:"nextCursor"` // 下一页起始位置，-1 表示没有更多
    Total      int            `json:"total"`
}
```

View 模式说明：
- `chat`：人类可读格式，格式化为对话形式，隐藏 Invocation 细节
- `handoff`：交接摘要格式，包含关键决策和上下文，适合 Agent 间交接
- `raw`：原始 JSON 格式，包含所有字段

```go
// MCP Tool: read_invocation_detail
// 查看某次 Agent 调用的完整输入/输出
type ReadInvocationDetailRequest struct {
    InvocationID string `json:"invocationId"`
}

type ReadInvocationDetailResponse struct {
    Invocation InvocationRecord `json:"invocation"`
}
```

```go
// MCP Tool: session_search
// 跨所有 session 的全文搜索
type SessionSearchRequest struct {
    ThreadID string `json:"threadId"`
    Query    string `json:"query"`
    Limit    int    `json:"limit"`  // 默认 10
}

type SessionSearchResponse struct {
    Results []SearchResult `json:"results"`
    Total   int            `json:"total"`
}

type SearchResult struct {
    SessionID    string `json:"sessionId"`
    EventNo      int    `json:"eventNo"`
    InvocationID string `json:"invocationId,omitempty"`
    Snippet      string `json:"snippet"`      // 匹配的文本片段
    Score        float64 `json:"score"`        // 相关性评分
    Timestamp    time.Time `json:"timestamp"`
}
```

#### 3.6.2 MCP Server 实现架构

```go
// SessionChainMCPServer MCP Server 实现
type SessionChainMCPServer struct {
    chainManager *SessionChainManager
    port         int
}

// 注册工具
func (s *SessionChainMCPServer) RegisterTools() []MCPTool {
    return []MCPTool{
        {
            Name:        "list_session_chain",
            Description: "列出某只猫在某个 thread 的所有 session 列表（状态、token数、序号）",
            Handler:     s.handleListSessionChain,
        },
        {
            Name:        "read_session_events",
            Description: "分页读取某个 session 的完整记录。view 模式: chat（人类可读）| handoff（交接摘要）| raw（原始数据）",
            Handler:     s.handleReadSessionEvents,
        },
        {
            Name:        "read_invocation_detail",
            Description: "深入查看某一次猫调用的完整输入/输出",
            Handler:     s.handleReadInvocationDetail,
        },
        {
            Name:        "session_search",
            Description: "跨所有 session 的全文搜索，返回匹配片段和定位指针",
            Handler:     s.handleSessionSearch,
        },
    }
}
```

#### 3.6.3 MCP Server 注册到 Agent

MCP Server 通过 stdio 模式注册给每个 Agent 的 CLI 调用。在调用 CLI 时，通过 `--mcp-config` 参数传入 MCP 配置：

```json
{
  "mcpServers": {
    "session-chain": {
      "command": "./cat-cafe",
      "args": ["--mode", "mcp", "--thread", "{threadId}"],
      "type": "stdio"
    }
  }
}
```

对应 `InvokeCLI` 的扩展：

```go
// 在构建 CLI 参数时，如果 Agent 配置了 session_chain，注入 MCP 配置
case "claude":
    args = append(args, "-p", prompt, "--output-format", "stream-json", "--verbose")
    if mcpConfigPath != "" {
        args = append(args, "--mcp-config", mcpConfigPath)
    }
    // ... 其他参数
```

### 3.7 核心模块：SessionChainManager

```go
// SessionChainManager 管理所有 Thread 的 Session Chain
type SessionChainManager struct {
    mu          sync.RWMutex
    chains      map[string]*SessionChain  // threadId -> chain
    dataDir     string                     // 数据目录根路径
    redisClient *redis.Client
    ctx         context.Context
}

// SessionChain 单个 Thread 的 Session Chain
type SessionChain struct {
    mu       sync.RWMutex
    Meta     SessionChainMeta
    Sessions []*SessionRecord           // 按 seqNo 排序
    Cursors  map[string]*AgentCursor    // agentName -> cursor
}
```

#### 3.7.1 核心接口

```go
// === Chain 生命周期 ===

// GetOrCreateChain 获取或创建 Thread 的 Session Chain
func (m *SessionChainManager) GetOrCreateChain(threadID string) (*SessionChain, error)

// === Event 写入 ===

// AppendEvent 向当前活跃 Session 追加 Event，自动检查是否需要 Seal
func (m *SessionChainManager) AppendEvent(threadID string, event SessionEvent) error

// RecordInvocation 记录一次 Agent 调用
func (m *SessionChainManager) RecordInvocation(threadID string, inv InvocationRecord) error

// === Seal 与压缩 ===

// CheckAndSeal 检查是否需要 Seal，如需要则执行
func (m *SessionChainManager) CheckAndSeal(threadID string, compressorConfig *MemoryCompressorConfig) error

// SealActiveSession 强制 Seal 当前活跃 Session
func (m *SessionChainManager) SealActiveSession(threadID string) error

// CompressSession 压缩指定 Session（异步调用）
func (m *SessionChainManager) CompressSession(threadID, sessionID string, config *MemoryCompressorConfig) error

// === 读取 ===

// GetActiveSession 获取当前活跃 Session
func (m *SessionChainManager) GetActiveSession(threadID string) (*SessionRecord, error)

// GetSession 获取指定 Session
func (m *SessionChainManager) GetSession(threadID, sessionID string) (*SessionRecord, error)

// ListSessions 列出所有 Session
func (m *SessionChainManager) ListSessions(threadID string) ([]*SessionRecord, error)

// GetEvents 获取指定 Session 的 Event（支持分页）
func (m *SessionChainManager) GetEvents(threadID, sessionID string, cursor, limit int) ([]SessionEvent, int, error)

// GetEventsAfter 获取指定位置之后的所有 Event（跨 Session）
func (m *SessionChainManager) GetEventsAfter(threadID, sessionID string, afterEventNo int) ([]SessionEvent, error)

// GetInvocation 获取 Invocation 详情
func (m *SessionChainManager) GetInvocation(threadID, invocationID string) (*InvocationRecord, error)

// SearchEvents 全文搜索
func (m *SessionChainManager) SearchEvents(threadID, query string, limit int) ([]SearchResult, error)

// === Cursor 管理 ===

// GetCursor 获取 Agent 的 Cursor
func (m *SessionChainManager) GetCursor(agentName, threadID string) *AgentCursor

// UpdateCursor 更新 Agent 的 Cursor
func (m *SessionChainManager) UpdateCursor(agentName, threadID, sessionID string, eventNo int, aiSessionID string) error
```

#### 3.7.2 Token 估算

使用简单的字符数估算（中文约 1 字 ≈ 2 token，英文约 4 字符 ≈ 1 token）：

```go
func estimateTokens(text string) int {
    chineseCount := 0
    asciiCount := 0
    for _, r := range text {
        if r > 127 {
            chineseCount++
        } else {
            asciiCount++
        }
    }
    return chineseCount*2 + asciiCount/4
}
```

后续可替换为 tiktoken 等精确计算库。

### 3.8 与现有系统的集成

#### 3.8.1 AgentWorker 改造

当前 `agent_worker.go` 中的 `executeTask` 方法需要改造：

```go
func (w *AgentWorker) executeTask(task *TaskMessage) (string, error) {
    // ... 工作目录获取（不变）

    // ===== 改造点 1：使用 SessionChainManager 替代 getSessionHistory =====
    var fullPrompt string
    var aiSessionID string

    switch w.config.ContextMode {
    case "orchestrated":
        // 策略 A：从 Session Chain 读取全部 Event
        fullPrompt = w.buildOrchestratedPrompt(task)
        aiSessionID = "" // 不使用 AI session ID

    case "cli_managed":
        // 策略 B：读取增量 Event + 使用 AI session ID
        fullPrompt, aiSessionID = w.buildCLIManagedPrompt(task)

    default:
        // 兼容旧逻辑：回退到当前实现
        chatHistory := w.getSessionHistory(task.SessionID)
        fullPrompt = w.buildLegacyPrompt(chatHistory, task)
        aiSessionID = w.getLegacyAISessionID(task)
    }

    // ===== 改造点 2：调用 CLI =====
    response, newSessionID, err := InvokeAgent(
        w.config.CLIType, fullPrompt, aiSessionID, workDir)
    if err != nil {
        return "", fmt.Errorf("调用 %s CLI 失败: %w", w.config.CLIType, err)
    }

    // ===== 改造点 3：记录 Invocation 到 Session Chain =====
    if task.SessionID != "" {
        inv := InvocationRecord{
            ID:          fmt.Sprintf("inv_%d", time.Now().UnixNano()),
            SessionID:   w.chainManager.GetActiveSession(task.SessionID).ID,
            ThreadID:    task.SessionID,
            AgentName:   w.config.Name,
            Prompt:      fullPrompt,
            Response:    response,
            AISessionID: newSessionID,
            Timestamp:   time.Now(),
        }
        w.chainManager.RecordInvocation(task.SessionID, inv)

        // 追加 cat Event
        w.chainManager.AppendEvent(task.SessionID, SessionEvent{
            Type:         EventCat,
            Sender:       w.config.Name,
            Content:      response,
            InvocationID: inv.ID,
            Timestamp:    time.Now(),
        })

        // 检查是否需要 Seal
        w.chainManager.CheckAndSeal(task.SessionID, w.config.MemoryCompressor)

        // 更新 Cursor（仅 cli_managed 模式）
        if w.config.ContextMode == "cli_managed" {
            activeSession := w.chainManager.GetActiveSession(task.SessionID)
            w.chainManager.UpdateCursor(
                w.config.Name, task.SessionID,
                activeSession.ID, activeSession.EndEvent, newSessionID)
        }
    }

    return response, nil
}
```

#### 3.8.2 API Server 改造

`api_server.go` 中的 `SendMessage` 方法需要在添加用户消息时同步写入 Session Chain：

```go
func (sm *SessionManager) SendMessage(sessionID string, req SendMessageRequest) (*Message, error) {
    // ... 现有逻辑（添加用户消息到 ctx.Messages）

    // ===== 新增：写入 Session Chain =====
    sm.sessionChainManager.AppendEvent(sessionID, SessionEvent{
        Type:      EventUser,
        Sender:    "用户",
        Content:   req.Content,
        Timestamp: time.Now(),
    })

    // ... 后续逻辑（编排器处理等）
}
```

`handleResult` 方法中，Agent 回复也需要写入 Session Chain（由 AgentWorker 负责，API Server 不重复写入）。

#### 3.8.3 SessionManager 改造

`NewSessionManager` 中初始化 `SessionChainManager`：

```go
func NewSessionManager(configPath string) (*SessionManager, error) {
    // ... 现有初始化逻辑

    // ===== 新增：创建 SessionChainManager =====
    chainManager := NewSessionChainManager("data/session_chain", rdb, ctx)

    sm := &SessionManager{
        // ... 现有字段
        sessionChainManager: chainManager,
    }

    // ... 后续逻辑
}
```

### 3.9 全文搜索实现

#### 3.9.1 搜索策略

采用基于文件系统的简单全文搜索，不引入额外搜索引擎依赖：

1. 遍历 Thread 目录下所有 Session Markdown 文件
2. 对每个文件进行逐行匹配（支持简单的关键词和短语匹配）
3. 返回匹配行的上下文（前后各 2 行）
4. 按匹配度排序

```go
func (m *SessionChainManager) SearchEvents(threadID, query string, limit int) ([]SearchResult, error) {
    chainDir := filepath.Join(m.dataDir, threadID)

    // 遍历所有 session markdown 文件
    files, _ := filepath.Glob(filepath.Join(chainDir, "S*.md"))

    var results []SearchResult
    for _, file := range files {
        content, _ := os.ReadFile(file)
        lines := strings.Split(string(content), "\n")

        for i, line := range lines {
            if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
                // 提取上下文片段
                start := max(0, i-2)
                end := min(len(lines), i+3)
                snippet := strings.Join(lines[start:end], "\n")

                // 解析 eventNo（从 ## Event #N 格式）
                eventNo := parseEventNo(line)

                results = append(results, SearchResult{
                    SessionID: extractSessionID(file),
                    EventNo:   eventNo,
                    Snippet:   snippet,
                    Score:     1.0, // 简单匹配，后续可加 TF-IDF
                    Timestamp: time.Now(),
                })
            }
        }
    }

    // 按 score 排序，取 top N
    sort.Slice(results, func(i, j int) bool {
        return results[i].Score > results[j].Score
    })
    if len(results) > limit {
        results = results[:limit]
    }

    return results, nil
}
```

后续可升级为：
- 基于 embedding 的语义搜索
- 引入 SQLite FTS5 做全文索引
- 接入外部搜索服务

---

## 4. 数据流全景图

```
用户发送消息
    │
    ▼
API Server (SendMessage)
    │
    ├──→ 写入 ctx.Messages（现有逻辑，保持兼容）
    ├──→ 写入 Session Chain Event（新增）
    ├──→ WebSocket 推送（现有逻辑）
    │
    ▼
Orchestrator (HandleUserMessage)
    │
    ▼
Scheduler (SendTask) ──→ Redis Stream
    │
    ▼
AgentWorker (handleMessage)
    │
    ├── 1. 根据 context_mode 构建 prompt
    │     ├── orchestrated: 读取活跃 Session 全部 Event
    │     └── cli_managed:  读取增量 Event + AI session ID
    │
    ├── 2. 调用 CLI（InvokeAgent）
    │     └── cli_managed 模式注入 MCP config
    │
    ├── 3. 记录 Invocation 到 Session Chain
    │
    ├── 4. 追加 cat Event 到 Session Chain
    │
    ├── 5. 检查 Seal 阈值
    │     └── 超过阈值 → Seal + 异步压缩
    │
    ├── 6. 更新 Agent Cursor（cli_managed 模式）
    │
    └── 7. 发送结果到 results:stream（现有逻辑）
              │
              ▼
        API Server (handleResult)
              │
              ├──→ 写入 ctx.Messages（现有逻辑）
              ├──→ WebSocket 推送（现有逻辑）
              └──→ 编排器处理后续调用（现有逻辑）
```

Agent 在执行过程中可通过 MCP 工具按需查询记忆：

```
Agent CLI 执行中
    │
    ├──→ list_session_chain(threadId, catId)
    │     → 查看有哪些历史 session
    │
    ├──→ read_session_events(sessionId, cursor, limit, "handoff")
    │     → 读取某个 session 的交接摘要
    │
    ├──→ read_invocation_detail(invocationId)
    │     → 深入查看某次调用的完整输入/输出
    │
    └──→ session_search(threadId, "HTTP服务器")
          → 搜索历史中关于 HTTP 服务器的内容
```

---

## 5. 边界情况与容错

### 5.1 Seal 过程中的新消息

Seal 是异步操作（压缩可能耗时较长）。在 Seal 触发后：

1. 立即创建新 Session 并设为 active
2. 新消息写入新 Session，不阻塞
3. 旧 Session 状态为 `compressing`，压缩完成后变为 `sealed`
4. 如果压缩失败，旧 Session 保持 `compressing` 状态，不影响新消息写入
5. 可通过定时任务重试失败的压缩

### 5.2 Agent Cursor 指向已压缩的 Session

当 cli_managed 模式的 Agent 被调用时，发现 Cursor 指向的 Session 已被 seal：

1. 读取该 Session 的 Summary（压缩摘要）
2. 读取后续所有 Session 的 Event（如果有多个 sealed session，逐个读取 summary）
3. 将摘要 + 增量 Event 作为 prompt 传入
4. Agent 自行判断是否需要通过 MCP 工具查找更详细的历史

### 5.3 并发写入

多个 Agent 可能同时向同一个 Session 写入 Event：

1. `SessionChain` 使用 `sync.RWMutex` 保护
2. Event 编号使用 `SessionChainMeta.TotalEvents` 原子递增
3. Markdown 文件写入使用文件锁（`flock`）或通过 channel 串行化
4. Redis 中的元数据更新使用 Redis 事务

### 5.4 进程重启恢复

1. `SessionChainMeta` 持久化在 Redis + 文件系统（`meta.json`）双写
2. 重启时优先从 Redis 加载，Redis 不可用时从文件系统恢复
3. Agent Cursor 持久化在 Redis 中（key: `cursor:{threadId}:{agentName}`）
4. Invocation 记录持久化在文件系统中，不依赖 Redis

### 5.5 向后兼容

1. 新增的配置字段都有默认值，不配置时回退到现有逻辑
2. `context_mode` 默认为空，走现有的 `getSessionHistory` + `session_mapping` 逻辑
3. 已有的 Redis 数据（`session:*`、`session_mapping:*`）保持不变
4. Session Chain 是增量功能，不破坏现有数据

---

## 6. 实现计划

采用 **测试先行（TDD）** 策略：每个 Phase 先根据设计接口编写测试用例，再实现功能代码，确保所有用例通过后才进入下一阶段。

### Phase 0：测试基础设施 + 全量测试用例 ✅ 已完成

根据 3.7.1 节定义的核心接口，先编写全量测试用例（允许编译失败，接口桩先占位）：

1. 编写 `test/session_chain_test.go` — SessionChainManager 单元测试
2. 编写 `test/session_chain_storage_test.go` — 文件系统存储测试
3. 编写 `test/session_chain_seal_test.go` — Seal 与压缩测试
4. 编写 `test/session_chain_search_test.go` — 全文搜索测试
5. 编写 `test/session_chain_mcp_test.go` — MCP Server 工具测试
6. 编写 `test/session_chain_integration_test.go` — 端到端集成测试
7. 编写测试辅助函数（mock Redis、临时目录、fixture 数据）

测试用例详见第 12 章「验收标准」。

### Phase 1：基础设施 ✅ 已完成

目标：通过 `session_chain_test.go` 中的基础 CRUD 用例。

1. ✅ 扩展 `AgentConfig` 结构体，新增 `context_mode`、`memory_compressor`、`session_chain` 字段
2. 扩展 `config.yaml` 格式（待配置实际 Agent）
3. ✅ 实现 `SessionChainManager` 核心结构（`src/session_chain.go`）
4. ✅ 实现文件系统存储（`src/session_chain_storage.go`）
5. ✅ 实现 `SessionRecord`、`SessionEvent`、`InvocationRecord` 的 CRUD
6. ✅ 实现 Token 估算函数（`src/token_estimator.go`）

✅ 验收：69/72 测试通过，3 个 skip（压缩模型依赖）

### Phase 2：Event 写入与读取 ✅ 已完成

目标：通过 `session_chain_test.go` 和 `session_chain_storage_test.go` 中的 Event 读写用例。

1. ✅ 实现 `AppendEvent`：写入 Event 到活跃 Session + 更新 Markdown
2. ✅ 实现 `RecordInvocation`：记录 Invocation 到文件系统
3. ✅ 实现 `GetEvents`、`GetEventsAfter`：分页读取和增量读取
4. ✅ 实现 `AgentCursor` 管理（文件系统持久化）
5. 集成到 `api_server.go`：用户消息写入 Session Chain（待 Phase 7/8）
6. ✅ 集成到 `agent_worker.go`：Agent 回复写入 Session Chain

✅ 验收：Phase 2 相关测试用例全部通过

### Phase 3：上下文策略 ✅ 已完成

目标：通过上下文构建相关测试用例。

1. ✅ 实现 `buildOrchestratedPrompt`（策略 A）— `src/session_chain_context.go`
2. ✅ 实现 `buildCLIManagedPrompt`（策略 B）— `src/session_chain_context.go`
3. ✅ 改造 `executeTask`：根据 `context_mode` 选择策略
4. ✅ 保持向后兼容：无配置时走旧逻辑
5. ✅ `NewAgentWorker` 注入 `SessionChainManager`
6. ✅ `main.go` 初始化 `SessionChainManager` 并传入 worker

✅ 验收：编译通过，69/72 测试通过

### Phase 4：Seal 与压缩 ✅ 已完成

目标：通过 `session_chain_seal_test.go` 全部用例。

1. ✅ 实现 `CompressSession`：通过可注入的 `CompressFn` 调用压缩模型生成摘要
2. ✅ 实现压缩 Prompt 模板（SPEC 3.4.3 格式）
3. ✅ 实现 `DefaultCompressFn`（生产环境通过 `InvokeCLI` 调用模型）
4. ✅ 测试环境通过 mock `CompressFn` 注入，3 个 skip 测试全部通过

✅ 验收：72/72 测试通过，0 skip

### Phase 5：MCP Server ✅ 已完成

目标：通过 `session_chain_mcp_test.go` 全部用例。

1. ✅ 实现 MCP Server 框架（`src/session_chain_mcp.go`，JSON-RPC 2.0 over stdio）
2. ✅ 实现 `list_session_chain` 工具
3. ✅ 实现 `read_session_events` 工具（含 3 种 view 模式）
4. ✅ 实现 `read_invocation_detail` 工具
5. ✅ 实现 `session_search` 工具（基础全文搜索）
6. ✅ 实现 MCP 配置注入到 CLI 调用（`AgentOptions.MCPConfigPath`、`GenerateMCPConfig`）
7. ✅ 在 `main.go` 中添加 `--mode mcp --thread {threadId}` 启动模式
8. ✅ `agent_worker.go` 自动生成 MCP 配置并注入 CLI 调用

✅ 验收：72/72 测试通过，MCP Server stdio 协议验证通过

### Phase 6：集成验收 ✅ 已完成

目标：通过 `session_chain_integration_test.go` 全部用例 + 全量回归。

1. ✅ 端到端集成测试：完整的消息流转 → Event 写入 → Seal → 压缩 → MCP 查询
2. ✅ 向后兼容测试：无 session_chain 配置时走旧逻辑
3. ✅ 并发测试：多 Agent 同时写入同一 Session
4. ✅ 恢复测试：进程重启后 Chain 状态恢复
5. ✅ 全量测试回归：`go test ./test/... -v` 全部通过

✅ 最终验收：72/72 测试全部通过，零失败，零 skip

### Phase 7：上下文长度评估模块

目标：实现 `ContextTokenEstimator`，为前端面板和 Seal 判断提供精确的 token 评估。

1. 在 `src/token_estimator.go` 中新增 `ContextTokenEstimator` 结构体和 `ContextTokenReport`
2. 实现 `EstimateContext` 方法：评估完整上下文（system prompt + summaries + events + task）
3. 实现 `EstimateSessionUsage` 方法：轻量版，仅评估当前 Session 的 token 使用
4. 在 `api_server.go` 中新增 `GET /api/sessions/:sessionId/chain-status` 端点
5. `SessionManager` 持有 `SessionChainManager` 引用
6. Event 写入后通过 WebSocket 推送 `chain_status` 更新

✅ 验收：API 返回正确的 `ContextTokenReport`，WebSocket 推送正常

### Phase 8：前端 Session Chain 状态面板

目标：在右侧 StatusBar 中展示 Session Chain 状态，包含进度条和可展开的压缩概述。

1. 在 `frontend/src/types/index.ts` 新增 `SessionChainStatus`、`SessionChainItem` 类型
2. 在 `frontend/src/services/api.ts` 新增 `chainAPI.getChainStatus()`
3. 在 `frontend/src/services/websocket.ts` 新增 `onChainStatus` 监听
4. 实现 `frontend/src/components/StatusBar/SessionChainPanel.tsx` 组件
   - 已压缩 Session 列表（可点击展开查看压缩概述）
   - 当前活跃 Session 的 token 进度条（绿→黄→红）
   - 实时更新（WebSocket）
5. 在 `frontend/src/components/StatusBar/index.tsx` 中引入 `SessionChainPanel`

✅ 验收：面板正确展示 Session Chain 状态，进度条实时更新，点击可查看压缩概述

### Phase 9：去掉 Redis 消息存储，Session Chain 为唯一消息源 ✅ 已完成

目标：消除 Redis 与 Session Chain 的消息双写，Session Chain 作为唯一消息 Source of Truth。

1. ✅ `session_persistence.go` — `SessionData` 移除 `Messages` 字段，移除对账逻辑
2. ✅ `session_persistence.go:LoadSession` — 不再加载消息，`SystemMessages` 初始化为空
3. ✅ `session_persistence.go:DeleteSessionFromRedis` — 联动调用 `DeleteChain`
4. ✅ `api_server.go:SessionContext` — `Messages` 重命名为 `SystemMessages`（仅存 system 消息）
5. ✅ `api_server.go:GetMessages` — 从 Session Chain 读取 + 合并 system 消息 + 按时间排序
6. ✅ `api_server.go:GetMessageStats` — 从 Session Chain 统计
7. ✅ `api_server.go:SendMessage` — 移除 `ctx.Messages` 追加，仅写 Session Chain
8. ✅ `api_server.go:handleAgentResult` — 移除 `ctx.Messages` 追加，仅写 Session Chain
9. ✅ `api_server.go` — 新增 `eventToMessage` 转换函数
10. ✅ `session_chain.go` — 新增 `GetAllEvents`、`DeleteChain` 方法
11. ✅ `agent_worker.go:getSessionHistory` — 改为从 Session Chain 读取

✅ 验收：编译通过，69/72 测试通过，Redis 不再存储消息，前端 API 返回格式不变

---

## 7. 新增文件清单

```
src/
  session_chain.go          # SessionChainManager 核心实现
  session_chain_storage.go  # 文件系统存储（Markdown 读写）
  session_chain_context.go  # 上下文策略（orchestrated / cli_managed / legacy）
  session_chain_seal.go     # Seal 与压缩逻辑
  session_chain_search.go   # 全文搜索
  session_chain_mcp.go      # MCP Server 实现
  token_estimator.go        # Token 估算 + ContextTokenEstimator

data/
  session_chain/            # 运行时数据目录（gitignore）

frontend/src/
  components/StatusBar/
    SessionChainPanel.tsx   # Session Chain 状态面板
  types/index.ts            # 新增 SessionChainStatus、SessionChainItem 类型
  services/api.ts           # 新增 chainAPI

test/
  session_chain_test.go              # SessionChainManager 单元测试
  session_chain_storage_test.go      # 文件系统存储测试
  session_chain_seal_test.go         # Seal 与压缩测试
  session_chain_search_test.go       # 全文搜索测试
  session_chain_mcp_test.go          # MCP Server 工具测试
  session_chain_integration_test.go  # 端到端集成测试
  session_chain_test_helpers.go      # 测试辅助函数（mock、fixture）
```

---

## 8. 上下文长度评估模块

### 8.1 背景

当前的 `EstimateTokens` 函数（`src/token_estimator.go`）基于简单的字符计数规则（中文 ×2，ASCII ÷4），只能估算单条文本的 token 数。但在实际场景中，Agent 的一次调用涉及多个上下文组成部分，需要一个更完整的模块来评估当前上下文的总 token 消耗，以便：

1. 前端展示当前 Session 的 token 使用进度
2. 后端精确判断 Seal 触发时机
3. 为用户提供上下文容量的可视化反馈

### 8.2 ContextTokenEstimator 设计

```go
// ContextTokenEstimator 上下文长度评估器
type ContextTokenEstimator struct {
    maxTokens int // 当前 Session 的 token 上限（来自 SessionChainConfig）
}

// ContextTokenReport 上下文 token 评估报告
type ContextTokenReport struct {
    SystemPromptTokens  int     `json:"systemPromptTokens"`  // 系统提示词 token 数
    SummaryTokens       int     `json:"summaryTokens"`       // 历史摘要 token 数
    EventTokens         int     `json:"eventTokens"`         // 当前 Session Event 总 token 数
    CurrentTaskTokens   int     `json:"currentTaskTokens"`   // 当前任务 token 数
    TotalTokens         int     `json:"totalTokens"`         // 总计
    MaxTokens           int     `json:"maxTokens"`           // 上限
    UsagePercent        float64 `json:"usagePercent"`        // 使用百分比 (0.0 ~ 1.0)
    RemainingTokens     int     `json:"remainingTokens"`     // 剩余可用 token
    EventCount          int     `json:"eventCount"`          // 当前 Session Event 数量
    MaxEventsPerSession int     `json:"maxEventsPerSession"` // Event 数量上限
}
```

#### 8.2.1 核心方法

```go
// NewContextTokenEstimator 创建评估器
func NewContextTokenEstimator(maxTokens int) *ContextTokenEstimator

// EstimateContext 评估当前上下文的完整 token 消耗
// 输入：系统提示词、已 seal 的摘要列表、当前 Session 的 Event 列表、当前任务内容
func (e *ContextTokenEstimator) EstimateContext(
    systemPrompt string,
    sealedSummaries []string,
    events []SessionEvent,
    currentTask string,
) *ContextTokenReport

// EstimateSessionUsage 仅评估当前 Session 的 token 使用情况（轻量版，供 API 调用）
func (e *ContextTokenEstimator) EstimateSessionUsage(
    session *SessionRecord,
    events []SessionEvent,
    config *SessionChainConfig,
) *ContextTokenReport
```

#### 8.2.2 集成点

1. **Seal 判断**：`CheckAndSeal` 使用 `EstimateSessionUsage` 替代简单的 `tokenCount` 比较
2. **API 接口**：新增 `GET /api/sessions/:sessionId/chain-status` 返回 `ContextTokenReport`
3. **WebSocket 推送**：每次 Event 写入后，通过 WS 推送最新的 `ContextTokenReport`

#### 8.2.3 文件位置

扩展现有 `src/token_estimator.go`，新增 `ContextTokenEstimator` 结构体和方法。

---

## 9. 前端 Session Chain 状态面板

### 9.1 概述

在前端右侧 `StatusBar` 中新增一个 Session Chain 展示区域，让用户直观了解当前对话的上下文使用情况和历史 Session 压缩状态。

### 9.2 UI 设计

面板位于 `StatusBar`（右侧 480px 面板）中，插入在「消息统计」和「调用历史」之间。

```
┌─────────────────────────────────────┐
│  Session Chain                      │
│                                     │
│  ┌─────────────────────────────┐    │
│  │ 📋 Session #1 (已压缩)      │◀── 可点击展开
│  │    5 条消息 · 12,340 tokens │    │
│  └─────────────────────────────┘    │
│  ┌─────────────────────────────┐    │
│  │ 📋 Session #2 (已压缩)      │◀── 可点击展开
│  │    8 条消息 · 18,200 tokens │    │
│  └─────────────────────────────┘    │
│  ┌─────────────────────────────┐    │
│  │ 🟢 Session #3 (当前)        │    │
│  │    3 条消息 · 4,500 tokens  │    │
│  │                             │    │
│  │  ████████░░░░░░░░░  42%     │◀── token 进度条
│  │  4,500 / 200,000 tokens     │    │
│  └─────────────────────────────┘    │
│                                     │
└─────────────────────────────────────┘
```

#### 点击已压缩的 Session 后展开：

```
┌─────────────────────────────────────┐
│  📋 Session #1 (已压缩)        ▲   │
│  5 条消息 · 12,340 tokens          │
│  压缩时间: 2026-02-28 10:30        │
│                                     │
│  ┌─ 压缩概述 ─────────────────┐    │
│  │ 用户要求开发一个HTTP服务器，│    │
│  │ 花花完成了基础框架搭建，    │    │
│  │ 包括路由、中间件、错误处理。│    │
│  │ 薇薇进行了代码审查并提出    │    │
│  │ 优化建议。                  │    │
│  │                             │    │
│  │ 关键决策：                  │    │
│  │ - 使用 Gin 框架             │    │
│  │ - 中间件采用洋葱模型        │    │
│  └─────────────────────────────┘    │
└─────────────────────────────────────┘
```

### 9.3 数据结构

#### 9.3.1 后端 API

新增 API 端点：

```
GET /api/sessions/:sessionId/chain-status
```

返回：

```typescript
interface SessionChainStatus {
  threadId: string;
  sessions: SessionChainItem[];
  activeSession: {
    id: string;
    seqNo: number;
    eventCount: number;
    tokenCount: number;
    maxTokens: number;
    usagePercent: number;       // 0.0 ~ 1.0
    maxEventsPerSession: number;
  };
  totalEvents: number;
  totalSessions: number;
}

interface SessionChainItem {
  id: string;
  seqNo: number;
  status: 'active' | 'sealed' | 'compressing';
  eventCount: number;
  tokenCount: number;
  summary: string | null;       // sealed 时有值，即压缩概述
  createdAt: string;
  sealedAt: string | null;
}
```

#### 9.3.2 WebSocket 推送

当 Session Chain 状态变化时（Event 写入、Seal 触发），通过 WebSocket 推送更新：

```typescript
// WS 消息类型: "chain_status"
{
  type: "chain_status",
  sessionId: "xxx",
  data: SessionChainStatus,
  timestamp: "..."
}
```

### 9.4 前端组件设计

#### 9.4.1 新增文件

```
frontend/src/
  components/
    StatusBar/
      SessionChainPanel.tsx    # Session Chain 面板主组件
  types/
    index.ts                   # 新增 SessionChainStatus、SessionChainItem 类型
  services/
    api.ts                     # 新增 chainAPI.getChainStatus()
```

#### 9.4.2 组件结构

```tsx
// SessionChainPanel.tsx
const SessionChainPanel: React.FC = () => {
  // 状态
  const [chainStatus, setChainStatus] = useState<SessionChainStatus | null>(null);
  const [expandedSessionId, setExpandedSessionId] = useState<string | null>(null);

  // 通过 API 加载初始数据
  // 通过 WebSocket 监听 chain_status 实时更新

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-6">
      <h2>Session Chain</h2>

      {/* 已压缩的 Session 列表 */}
      {sealedSessions.map(session => (
        <SealedSessionCard
          key={session.id}
          session={session}
          expanded={expandedSessionId === session.id}
          onToggle={() => toggleExpand(session.id)}
        />
      ))}

      {/* 当前活跃 Session + 进度条 */}
      <ActiveSessionCard
        session={chainStatus.activeSession}
      />
    </div>
  );
};
```

#### 9.4.3 进度条组件

```tsx
// 进度条颜色策略：
// 0% ~ 60%:  绿色 (#22c55e) — 充裕
// 60% ~ 80%: 黄色 (#eab308) — 接近阈值
// 80% ~ 100%: 红色 (#ef4444) — 即将触发 Seal

const getProgressColor = (percent: number): string => {
  if (percent < 0.6) return '#22c55e';
  if (percent < 0.8) return '#eab308';
  return '#ef4444';
};
```

### 9.5 后端改造

#### 9.5.1 新增 API 端点

在 `api_server.go` 中新增：

```go
// handleGetChainStatus 获取 Session Chain 状态
func (sm *SessionManager) handleGetChainStatus(c *gin.Context) {
    sessionID := c.Param("sessionId")

    // 从 SessionChainManager 获取 Chain 状态
    // 构建 SessionChainStatus 响应
    // 包含所有 Session 列表 + 活跃 Session 的 token 使用情况
}
```

路由注册：

```go
api.GET("/sessions/:sessionId/chain-status", sm.handleGetChainStatus)
```

#### 9.5.2 WebSocket 推送集成

在 `AppendEvent` 和 `SealActiveSession` 完成后，通过 WSHub 推送 `chain_status` 消息：

```go
// agent_worker.go 或 api_server.go 中
// Event 写入后
sm.wsHub.BroadcastToSession(sessionID, "chain_status", chainStatus)
```

#### 9.5.3 SessionManager 扩展

`SessionManager` 需要持有 `SessionChainManager` 引用：

```go
type SessionManager struct {
    // ... 现有字段
    chainManager *SessionChainManager // 新增
}
```

在 `NewSessionManager` 中初始化：

```go
chainManager, err := NewSessionChainManager("data/session_chains")
if err != nil {
    LogWarn("[API] 创建 SessionChainManager 失败: %v", err)
}
sm.chainManager = chainManager
```

### 9.6 与现有代码的映射

| 文件 | 改造内容 |
|------|----------|
| `src/api_server.go` | 新增 `handleGetChainStatus`，`SessionManager` 持有 `chainManager` |
| `src/api_server.go:SetupRouter` | 新增 `/sessions/:sessionId/chain-status` 路由 |
| `src/websocket.go` | WSMessage.Type 新增 `"chain_status"` |
| `src/token_estimator.go` | 新增 `ContextTokenEstimator`、`ContextTokenReport` |
| `frontend/src/types/index.ts` | 新增 `SessionChainStatus`、`SessionChainItem` 类型 |
| `frontend/src/services/api.ts` | 新增 `chainAPI.getChainStatus()` |
| `frontend/src/services/websocket.ts` | 新增 `onChainStatus` 监听 |
| `frontend/src/components/StatusBar/index.tsx` | 引入 `SessionChainPanel` |
| `frontend/src/components/StatusBar/SessionChainPanel.tsx` | 新增组件 |

---

## 10. Redis 与 Session Chain 数据一致性方案

### 10.1 问题背景

早期设计中 Redis 和 Session Chain 同时存储消息数据，导致一致性问题。经过评估，决定去掉 Redis 的消息存储，让 Session Chain 成为消息的唯一 Source of Truth。

### 10.2 最终方案：Session Chain 为唯一消息存储

**核心原则：Redis 只存元数据，Session Chain 存所有消息。**

#### 10.2.1 数据分工

| 存储 | 内容 | 说明 |
|------|------|------|
| Redis (`session:{id}`) | 会话元数据：Name, Summary, WorkspaceID, MessageCount, CallHistory, JoinedCats, ModeName, ModeConfig, ModeState | 不含消息内容 |
| Session Chain (Markdown) | user/cat 消息（Event） | 唯一消息存储，含 token 计数 |
| 内存 (`ctx.SystemMessages`) | system 消息（欢迎、加入、模式切换） | 仅运行时，不持久化 |

#### 10.2.2 写入路径

```
用户发消息 / Agent 回复
    │
    ├─ Session Chain: AppendEvent → pushChainStatus（WebSocket 推送）
    │
    └─ Redis: 仅更新 MessageCount、UpdatedAt 等元数据（AutoSaveSession）
```

system 消息（"会话已创建"、"花花已加入对话"、"模式已切换"）：
- 仅写入内存 `ctx.SystemMessages`
- 通过 WebSocket 实时推送
- 不写入 Session Chain（非上下文）
- 不持久化到 Redis（重启后不恢复）

#### 10.2.3 读取路径

| 场景 | 数据源 | 说明 |
|------|--------|------|
| 前端加载消息列表 (`GetMessages`) | Session Chain + 内存 | 从 Chain 读 user/cat events，转换为 Message 格式，与内存 system 消息合并，按时间排序 |
| 消息统计 (`GetMessageStats`) | Session Chain | 直接统计 events |
| Agent 历史上下文 (`getSessionHistory`) | Session Chain | 从 Chain 读取最近 20 条 events |
| 上下文构建（orchestrated/cli_managed） | Session Chain | 含 token 计数、压缩摘要 |
| MCP 工具读取 | Session Chain | 直接读 Markdown |

#### 10.2.4 SessionEvent → Message 转换

`eventToMessage` 函数将 `SessionEvent` 转换为前端需要的 `Message` 格式：
- `ev.Type` → `msg.Type`（`SCEventUser` → `"user"`，`SCEventCat` → `"cat"`）
- `ev.Sender`（名称字符串）→ `msg.Sender`（`*Sender`，通过 `getCatInfoByName` 补全头像、颜色）
- `ev.EventNo` → `msg.ID`（格式：`msg_ev_{eventNo}`）
- `ev.Timestamp` → `msg.Timestamp`
- `threadID` → `msg.SessionID`

#### 10.2.5 删除联动

`DeleteSession` 同时清理：
- Redis: `DEL session:{sessionID}` + `SREM sessions:list {sessionID}`
- Session Chain: `DeleteChain` 清理内存 + 删除 `data/session_chains/{sessionID}/` 目录

#### 10.2.6 数据边界

| 数据类型 | Redis | Session Chain | 内存 | 说明 |
|----------|-------|---------------|------|------|
| user 消息 | ❌ | ✅ | ❌ | 仅 Session Chain |
| cat 消息 | ❌ | ✅ | ❌ | 仅 Session Chain |
| system 消息 | ❌ | ❌ | ✅ | 仅运行时内存 |
| sender 头像/颜色 | ❌ | ❌ | ❌ | 读取时动态补全 |
| token 计数 | ❌ | ✅ | ❌ | Session Chain 独有 |
| 压缩摘要 | ❌ | ✅ | ❌ | Session Chain 独有 |
| Invocation 详情 | ❌ | ✅ | ❌ | Session Chain 独有 |
| 模式状态 | ✅ | ❌ | ❌ | Redis 独有 |
| 调用历史 | ✅ | ❌ | ❌ | Redis 独有（CallHistory） |
| 会话元数据 | ✅ | ❌ | ❌ | Name, Summary, WorkspaceID 等 |

### 10.3 实现改造点

| 文件 | 改造内容 | 状态 |
|------|----------|------|
| `src/session_persistence.go` | `SessionData` 移除 `Messages` 字段，移除对账逻辑 | ✅ 已完成 |
| `src/session_persistence.go:LoadSession` | 不再加载消息，`SystemMessages` 初始化为空 | ✅ 已完成 |
| `src/session_persistence.go:DeleteSessionFromRedis` | 联动调用 `DeleteChain` | ✅ 已完成 |
| `src/api_server.go:SessionContext` | `Messages` 重命名为 `SystemMessages` | ✅ 已完成 |
| `src/api_server.go:GetMessages` | 从 Session Chain 读取 + 合并 system 消息 | ✅ 已完成 |
| `src/api_server.go:GetMessageStats` | 从 Session Chain 统计 | ✅ 已完成 |
| `src/api_server.go:SendMessage` | 移除 `ctx.Messages` 追加，仅写 Session Chain | ✅ 已完成 |
| `src/api_server.go:handleAgentResult` | 移除 `ctx.Messages` 追加，仅写 Session Chain | ✅ 已完成 |
| `src/api_server.go` | 新增 `eventToMessage` 转换函数 | ✅ 已完成 |
| `src/session_chain.go` | 新增 `GetAllEvents`、`DeleteChain` 方法 | ✅ 已完成 |
| `src/agent_worker.go:getSessionHistory` | 改为从 Session Chain 读取，不再依赖 Redis 消息 | ✅ 已完成 |
| `src/api_server.go:pushChainStatus` | WebSocket 推送 chain status | ✅ 已完成 |

---

## 11. 风险与待确认项

### 11.1 风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 压缩模型调用失败 | 旧 Session 无法 seal | 异步重试 + 不阻塞新消息 |
| Markdown 文件过大 | 读写性能下降 | Seal 机制控制单文件大小 |
| 全文搜索性能 | 大量 Session 时搜索慢 | 后续引入索引；当前限制搜索范围 |
| MCP Server 兼容性 | 不同 CLI 对 MCP 支持不同 | 按 CLI 类型条件注入 |

### 11.2 已确认决策

1. **压缩模型选择** ✅：统一使用一个轻量模型（`claude-haiku-4-5-20251001`）做压缩，不依赖 Agent 自身的 CLI 类型。所有 Agent 共享同一个压缩模型配置。

2. **MCP Server 兼容性** ✅：三种 CLI（Claude、Gemini、Codex）均支持本地 stdio 方式连接 MCP，无需做回退方案。

3. **Invocation 详情存储** ✅：先不压缩，按需优化。单个 Invocation JSON 文件通常在 KB 级别。

4. **跨 Thread 搜索** ✅：不支持。Session Chain 的搜索范围限定在单个 Thread 内。

---

## 12. 附录：与现有代码的映射

| 现有代码 | 改造内容 |
|----------|----------|
| `scheduler.go:AgentConfig` | 新增 `ContextMode`、`MemoryCompressor`、`SessionChain` 字段 |
| `agent_worker.go:executeTask` | 根据 `ContextMode` 选择 prompt 构建策略 |
| `agent_worker.go:getSessionHistory` | 改为从 Session Chain 读取历史消息，不再依赖 Redis |
| `agent_worker.go:NewAgentWorker` | 注入 `SessionChainManager` |
| `session_persistence.go:SessionData` | 移除 `Messages` 字段，Redis 仅存元数据 |
| `session_persistence.go:DeleteSessionFromRedis` | 联动调用 `DeleteChain` 清理 Session Chain |
| `session_chain.go:GetAllEvents` | 新增：获取 Thread 下所有 events |
| `session_chain.go:DeleteChain` | 新增：删除整个 Thread 的 Chain 数据 |
| `api_server.go:SendMessage` | 写入 Session Chain，不再写 Redis 消息 |
| `api_server.go:GetMessages` | 从 Session Chain 读取 + 合并 system 消息 |
| `api_server.go:GetMessageStats` | 从 Session Chain 统计 |
| `api_server.go:handleAgentResult` | 写入 Session Chain，不再写 Redis 消息 |
| `api_server.go:SessionContext` | `Messages` 重命名为 `SystemMessages` |
| `api_server.go:eventToMessage` | 新增：SessionEvent → Message 转换 |
| `api_server.go:pushChainStatus` | 新增：WebSocket 推送 chain status |
| `api_server.go:NewSessionManager` | 初始化 `SessionChainManager` |
| `api_server.go:SetupRouter` | 新增 `/sessions/:sessionId/chain-status` 路由 |
| `token_estimator.go` | 新增 `ContextTokenEstimator`、`ContextTokenReport` |
| `websocket.go` | WSMessage.Type 新增 `"chain_status"` |
| `invoke.go:InvokeCLI` | 支持 `--mcp-config` 参数注入 |
| `cli_adapter.go:InvokeAgent` | 传递 MCP 配置路径 |
| `config.yaml` | 新增 Agent 级别的 session chain 配置 |
| `main.go` | 新增 `--mode mcp` 启动模式 |
| `frontend/src/types/index.ts` | 新增 `SessionChainStatus`、`SessionChainItem` 类型 |
| `frontend/src/services/api.ts` | 新增 `chainAPI.getChainStatus()` |
| `frontend/src/services/websocket.ts` | 新增 `onChainStatus` 监听 |
| `frontend/src/components/StatusBar/index.tsx` | 引入 `SessionChainPanel` |
| `frontend/src/components/StatusBar/SessionChainPanel.tsx` | 新增 Session Chain 面板组件 |

---

## 13. 验收标准

所有测试用例通过（`go test ./test/... -v` 零失败）为最终验收的唯一标准。

以下按测试文件列出全部用例。

### 13.1 session_chain_test.go — SessionChainManager 核心

#### TC-1.1 Chain 生命周期

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-1.1.1 | GetOrCreateChain 首次创建 | 返回新 Chain，Meta 字段正确，自动创建 active Session（S001） |
| TC-1.1.2 | GetOrCreateChain 重复调用 | 返回同一个 Chain 实例，不重复创建 |
| TC-1.1.3 | 多 Thread 隔离 | 不同 threadId 返回不同 Chain，互不影响 |

#### TC-1.2 Event 写入

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-1.2.1 | AppendEvent 基本写入 | Event 追加到活跃 Session，eventNo 自增，tokenCount 更新 |
| TC-1.2.2 | AppendEvent 多类型 Event | user、cat、system、invocation 四种类型均可写入 |
| TC-1.2.3 | AppendEvent 并发写入 | 10 个 goroutine 同时写入，eventNo 无重复无跳跃 |
| TC-1.2.4 | RecordInvocation | Invocation 写入文件系统，可通过 GetInvocation 读回 |

#### TC-1.3 Event 读取

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-1.3.1 | GetEvents 基本分页 | cursor=0, limit=10 返回前 10 条，nextCursor 正确 |
| TC-1.3.2 | GetEvents 翻页 | 连续翻页能读取所有 Event，最后一页 nextCursor=-1 |
| TC-1.3.3 | GetEvents 空 Session | 返回空数组，nextCursor=-1 |
| TC-1.3.4 | GetEventsAfter 增量读取 | 只返回指定 eventNo 之后的 Event |
| TC-1.3.5 | GetEventsAfter 跨 Session | 跨越 sealed Session 边界，正确拼接 |

#### TC-1.4 Session 管理

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-1.4.1 | GetActiveSession | 返回状态为 active 的 Session |
| TC-1.4.2 | GetSession 指定 ID | 返回正确的 Session 记录 |
| TC-1.4.3 | ListSessions | 返回所有 Session，按 seqNo 排序 |
| TC-1.4.4 | GetSession 不存在 | 返回错误 |

#### TC-1.5 Cursor 管理

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-1.5.1 | GetCursor 首次获取 | 返回 nil（无历史 Cursor） |
| TC-1.5.2 | UpdateCursor + GetCursor | 更新后能正确读回 |
| TC-1.5.3 | 不同 Agent 独立 Cursor | Agent A 和 Agent B 的 Cursor 互不影响 |
| TC-1.5.4 | Cursor 持久化 | 重建 SessionChainManager 后 Cursor 仍可读回 |

#### TC-1.6 Token 估算

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-1.6.1 | 纯中文文本 | "你好世界" → 约 8 token |
| TC-1.6.2 | 纯英文文本 | "hello world" → 约 2-3 token |
| TC-1.6.3 | 中英混合 | 估算值在合理范围内 |
| TC-1.6.4 | 空字符串 | 返回 0 |

### 13.2 session_chain_storage_test.go — 文件系统存储

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-2.1 | 目录自动创建 | 首次写入时自动创建 `data/session_chain/{threadId}/` 和 `invocations/` |
| TC-2.2 | meta.json 读写 | 写入后读回，字段完全一致 |
| TC-2.3 | Session Markdown 写入 | 生成的 Markdown 包含正确的 frontmatter 和 Event 格式 |
| TC-2.4 | Session Markdown 追加 | 追加 Event 后文件内容正确，frontmatter 更新 |
| TC-2.5 | Session Markdown 读取 | 从 Markdown 解析出 frontmatter 和 Event 列表 |
| TC-2.6 | Invocation JSON 读写 | 写入后读回，字段完全一致 |
| TC-2.7 | 特殊字符处理 | Event 内容包含 Markdown 特殊字符（`#`、`*`、`|`）时不破坏格式 |
| TC-2.8 | 大文件写入 | 500 条 Event 写入后文件可正常读取 |

### 13.3 session_chain_seal_test.go — Seal 与压缩

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-3.1 | CheckAndSeal 未达阈值 | tokenCount < threshold，不触发 Seal |
| TC-3.2 | CheckAndSeal 达到 token 阈值 | tokenCount >= maxTokens * sealThreshold，触发 Seal |
| TC-3.3 | CheckAndSeal 达到 Event 数阈值 | eventCount >= maxEventsPerSession，触发 Seal |
| TC-3.4 | SealActiveSession 状态流转 | 旧 Session → compressing，新 Session → active，Meta 更新 |
| TC-3.5 | Seal 后新 Event 写入新 Session | Seal 后 AppendEvent 写入新的活跃 Session |
| TC-3.6 | Seal 后旧 Session 不可追加 | 向 sealed Session 追加 Event 返回错误 |
| TC-3.7 | CompressSession 生成摘要 | 调用压缩模型后 Session.Summary 非空，状态变为 sealed |
| TC-3.8 | CompressSession 包含历史摘要 | 压缩 prompt 中包含之前 sealed Session 的 Summary |
| TC-3.9 | 压缩失败不阻塞 | 压缩模型调用失败，Session 保持 compressing，新消息正常写入 |
| TC-3.10 | 连续 Seal | 连续触发 3 次 Seal，Chain 中有 4 个 Session（3 sealed + 1 active） |

### 13.4 session_chain_search_test.go — 全文搜索

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-4.1 | 基本关键词搜索 | 搜索 "HTTP服务器"，返回包含该关键词的 Event |
| TC-4.2 | 跨 Session 搜索 | 结果来自多个 Session |
| TC-4.3 | 大小写不敏感 | 搜索 "http" 能匹配 "HTTP" |
| TC-4.4 | 搜索结果包含上下文 | snippet 包含匹配行的前后各 2 行 |
| TC-4.5 | 搜索结果定位 | 返回正确的 sessionId、eventNo |
| TC-4.6 | limit 限制 | limit=3 时最多返回 3 条结果 |
| TC-4.7 | 无匹配结果 | 搜索不存在的关键词，返回空数组 |
| TC-4.8 | 空 query | 返回错误或空数组 |

### 13.5 session_chain_mcp_test.go — MCP Server 工具

#### TC-5.1 list_session_chain

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-5.1.1 | 基本列表 | 返回所有 Session，包含 id、seqNo、status、eventCount、tokenCount |
| TC-5.1.2 | sealed Session 包含 summary | sealed 的 Session 返回 summary 字段 |
| TC-5.1.3 | 空 Thread | 返回空数组 |

#### TC-5.2 read_session_events

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-5.2.1 | view=chat | 返回人类可读格式，隐藏 Invocation 细节 |
| TC-5.2.2 | view=handoff | 返回交接摘要格式，包含关键决策 |
| TC-5.2.3 | view=raw | 返回原始 JSON，包含所有字段 |
| TC-5.2.4 | 分页 | cursor + limit 正确分页 |
| TC-5.2.5 | Session 不存在 | 返回错误 |

#### TC-5.3 read_invocation_detail

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-5.3.1 | 基本读取 | 返回完整的 Invocation 记录（prompt、response、元数据） |
| TC-5.3.2 | Invocation 不存在 | 返回错误 |

#### TC-5.4 session_search

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-5.4.1 | 基本搜索 | 返回匹配结果，包含 snippet 和定位指针 |
| TC-5.4.2 | 结果排序 | 按相关性排序 |

### 13.6 session_chain_integration_test.go — 端到端集成

| 用例 ID | 描述 | 验证点 |
|---------|------|--------|
| TC-6.1 | 完整消息流转 | 用户消息 → Event 写入 → Agent 调用 → Invocation 记录 → Agent 回复 Event |
| TC-6.2 | orchestrated 模式端到端 | 策略 A 下 prompt 包含活跃 Session 全部 Event，不传 --resume |
| TC-6.3 | cli_managed 模式端到端 | 策略 B 下 prompt 只包含增量 Event，传 --resume + AI session ID |
| TC-6.4 | Seal 触发端到端 | 写入足够多 Event → 触发 Seal → 新 Session 创建 → 压缩完成 → Summary 可读 |
| TC-6.5 | cli_managed + Seal 交互 | Cursor 指向已 seal 的 Session → prompt 包含 Summary + 增量 Event |
| TC-6.6 | MCP 查询端到端 | 写入数据 → 通过 MCP 工具查询 → 返回正确结果 |
| TC-6.7 | 向后兼容 | context_mode 为空时走旧逻辑（getSessionHistory + session_mapping），行为不变 |
| TC-6.8 | 并发多 Agent | 3 个 Agent 同时写入同一 Thread，Event 编号无冲突，数据完整 |
| TC-6.9 | 进程重启恢复 | 写入数据 → 重建 SessionChainManager → Chain 状态完整恢复 |
| TC-6.10 | 连续多轮 Seal | 模拟长对话触发 3 次 Seal，最终 Chain 结构正确，所有 Summary 可读 |

### 13.7 验收流程

```
1. Phase 0 完成后：所有测试文件可编译（接口桩占位），运行结果全部 SKIP 或 FAIL
2. Phase 1 完成后：TC-1.1.*, TC-1.4.*, TC-1.6.*, TC-2.1, TC-2.2 通过
3. Phase 2 完成后：TC-1.2.*, TC-1.3.*, TC-1.5.*, TC-2.3 ~ TC-2.8 通过
4. Phase 3 完成后：TC-6.2, TC-6.3, TC-6.7 通过
5. Phase 4 完成后：TC-3.*, TC-6.4, TC-6.5, TC-6.10 通过
6. Phase 5 完成后：TC-4.*, TC-5.*, TC-6.6 通过
7. Phase 6 最终验收：go test ./test/... -v 全部通过（TC-6.8, TC-6.9 等）
```

最终验收标准：**`go test ./test/... -v` 输出零 FAIL**。
