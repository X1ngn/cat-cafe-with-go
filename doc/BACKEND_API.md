# 后端 API 实现说明

## 架构设计

### 核心特性

1. **每个 Session 独立的调度系统**
   - 每创建一个会话，就初始化一个独立的 Scheduler 实例
   - 每个 Session 有自己的消息队列和 Agent 状态管理
   - Session 之间完全隔离，互不影响

2. **自动初始化猫猫 Agent**
   - 创建 Session 时，自动注册所有配置的猫猫（花花、薇薇、小乔）
   - 每个猫猫有独立的 Redis Stream 管道
   - 猫猫状态实时跟踪（idle/busy）

3. **消息流转机制**
   ```
   用户发送消息 → SessionManager →
   → 记录用户消息 →
   → 检测 @提及 →
   → 发送任务到对应猫猫的调度器 →
   → 猫猫 Agent 处理任务 →
   → 返回响应
   ```

## 已实现的接口

### 1. 会话管理

#### GET /api/sessions
获取所有会话列表

**响应**:
```json
[
  {
    "id": "sess_abc123",
    "name": "新对话",
    "summary": "用户：你好...",
    "updatedAt": "2026-02-16T10:00:00Z",
    "messageCount": 5
  }
]
```

#### POST /api/sessions
创建新会话

**功能**:
- 生成唯一的 session ID
- 为该 session 创建独立的 Scheduler
- 初始化所有猫猫 Agent（花花、薇薇、小乔）
- 添加系统欢迎消息

**响应**:
```json
{
  "id": "sess_abc123",
  "name": "新对话",
  "summary": "",
  "updatedAt": "2026-02-16T10:00:00Z",
  "messageCount": 0
}
```

#### GET /api/sessions/:sessionId
获取会话详情

#### DELETE /api/sessions/:sessionId
删除会话

**功能**:
- 关闭该 session 的 Scheduler
- 清理所有相关资源
- 从内存中移除

### 2. 消息管理

#### GET /api/sessions/:sessionId/messages
获取会话的所有消息

**响应**:
```json
[
  {
    "id": "msg_001",
    "type": "system",
    "content": "会话已创建，猫猫们已就位！",
    "timestamp": "2026-02-16T10:00:00Z",
    "sessionId": "sess_abc123"
  },
  {
    "id": "msg_002",
    "type": "user",
    "content": "@花花 你好",
    "sender": {
      "id": "user_001",
      "name": "用户",
      "avatar": ""
    },
    "timestamp": "2026-02-16T10:01:00Z",
    "sessionId": "sess_abc123"
  }
]
```

#### POST /api/sessions/:sessionId/messages
发送消息

**请求体**:
```json
{
  "content": "@花花 帮我设计一个网站",
  "mentionedCats": ["花花"]
}
```

**功能**:
1. 记录用户消息
2. 如果有 @提及的猫猫：
   - 添加系统消息 "XXX 已加入对话"
   - 记录调用历史
   - 通过该 session 的 Scheduler 发送任务到对应猫猫
3. 更新会话摘要

**响应**:
```json
{
  "id": "msg_003",
  "type": "user",
  "content": "@花花 帮我设计一个网站",
  "sender": {
    "id": "user_001",
    "name": "用户",
    "avatar": ""
  },
  "timestamp": "2026-02-16T10:02:00Z",
  "sessionId": "sess_abc123"
}
```

#### GET /api/sessions/:sessionId/stats
获取消息统计

**响应**:
```json
{
  "totalMessages": 10,
  "catMessages": 5
}
```

### 3. 猫猫管理

#### GET /api/cats
获取所有猫猫列表

**响应**:
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
    "status": "idle"
  },
  {
    "id": "cat_003",
    "name": "小乔",
    "avatar": "",
    "color": "#cccccc",
    "status": "idle"
  }
]
```

#### GET /api/cats/:catId
获取单个猫猫信息

#### GET /api/cats/available
获取可用的猫猫（状态为 idle）

### 4. 调用历史

#### GET /api/sessions/:sessionId/history
获取该会话的调用历史

**响应**:
```json
[
  {
    "catId": "cat_001",
    "catName": "花花",
    "sessionId": "sess_abc123",
    "timestamp": "2026-02-16T10:02:00Z"
  }
]
```

## 数据结构

### SessionContext
```go
type SessionContext struct {
    ID            string              // 会话 ID
    Name          string              // 会话名称
    Summary       string              // 会话摘要
    CreatedAt     time.Time           // 创建时间
    UpdatedAt     time.Time           // 更新时间
    MessageCount  int                 // 消息数量
    Scheduler     *Scheduler          // 独立的调度器实例
    Messages      []Message           // 消息列表
    CallHistory   []CallHistoryItem   // 调用历史
    mu            sync.RWMutex        // 读写锁
}
```

### SessionManager
```go
type SessionManager struct {
    sessions map[string]*SessionContext  // 所有会话
    mu       sync.RWMutex                // 读写锁
    config   *Config                     // 配置
}
```

## 启动方式

### 1. 启动 Redis
```bash
# macOS
brew services start redis

# Linux
sudo systemctl start redis
```

### 2. 编译项目
```bash
make build
```

### 3. 启动服务（推荐使用脚本）
```bash
./start.sh
```

这会自动启动：
- API 服务器（端口 8080）
- 花花 Agent 工作进程
- 薇薇 Agent 工作进程
- 小乔 Agent 工作进程

### 4. 手动启动（可选）

**启动 API 服务器**:
```bash
./bin/cat-cafe --mode api --port 8080
```

**启动 Agent 工作进程**（需要在不同终端）:
```bash
./bin/cat-cafe --mode agent --agent 花花
./bin/cat-cafe --mode agent --agent 薇薇
./bin/cat-cafe --mode agent --agent 小乔
```

### 5. 停止服务
```bash
./stop.sh
```

或按 Ctrl+C（如果使用 start.sh）

## 测试 API

### 创建会话
```bash
curl -X POST http://localhost:8080/api/sessions
```

### 获取会话列表
```bash
curl http://localhost:8080/api/sessions
```

### 发送消息
```bash
curl -X POST http://localhost:8080/api/sessions/sess_abc123/messages \
  -H "Content-Type: application/json" \
  -d '{
    "content": "@花花 你好",
    "mentionedCats": ["花花"]
  }'
```

### 获取消息列表
```bash
curl http://localhost:8080/api/sessions/sess_abc123/messages
```

### 获取猫猫列表
```bash
curl http://localhost:8080/api/cats
```

## 工作流程

### 完整的消息流程

1. **前端发送消息**
   ```
   POST /api/sessions/sess_123/messages
   {
     "content": "@花花 @薇薇 帮我设计一个网站",
     "mentionedCats": ["花花", "薇薇"]
   }
   ```

2. **后端处理**
   - SessionManager 找到对应的 SessionContext
   - 记录用户消息到 Messages 列表
   - 遍历 mentionedCats：
     - 添加系统消息 "花花 已加入对话"
     - 添加系统消息 "薇薇 已加入对话"
     - 记录调用历史
     - 通过 SessionContext.Scheduler 发送任务：
       ```go
       scheduler.SendTask("花花", "@花花 @薇薇 帮我设计一个网站")
       scheduler.SendTask("薇薇", "@花花 @薇薇 帮我设计一个网站")
       ```

3. **调度器处理**
   - Scheduler 将任务序列化为 JSON
   - 发送到 Redis Stream（pipe:pipe_huahua, pipe:pipe_weiwei）
   - 更新 Agent 状态为 busy

4. **Agent 工作进程处理**
   - Agent Worker 从 Redis Stream 读取任务
   - 调用对应的 CLI 工具（minimal-claude, minimal-codex）
   - 执行任务并返回结果

5. **结果返回**（待实现）
   - Agent 完成任务后，将结果写回 Redis
   - SessionManager 监听结果
   - 添加猫猫消息到 Messages 列表
   - 前端通过轮询或 WebSocket 获取新消息

## 待完成功能

### 1. Agent 响应回传
目前 Agent 处理完任务后，结果还没有回传到 SessionContext。需要：
- Agent Worker 完成任务后，将结果发送到响应队列
- SessionManager 监听响应队列
- 将猫猫的回复添加到 Messages 列表

### 2. WebSocket 实时推送
- 实现 WebSocket 连接
- 当有新消息时，实时推送给前端
- 推送打字状态、Agent 状态变更

### 3. Agent 状态同步
- 实时更新 Agent 的 idle/busy 状态
- 前端可以看到哪些猫猫正在工作

### 4. 消息持久化
- 将消息保存到数据库或文件
- 重启后可以恢复会话

## 配置文件

使用现有的 `config.yaml`:
```yaml
agents:
  - name: "花花"
    pipe: "pipe_huahua"
    exec_cmd: "./minimal-claude"
    system_prompt_path: "prompts/calico_cat.md"

  - name: "薇薇"
    pipe: "pipe_weiwei"
    exec_cmd: "./minimal-codex"
    system_prompt_path: "prompts/lihua_cat.md"

  - name: "小乔"
    pipe: "pipe_xiaoqiao"
    exec_cmd: "./minimal-gemini"
    system_prompt_path: "prompts/silver_cat.md"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0
```

## 依赖

- Go 1.21+
- Redis 6.0+
- Gin Web Framework
- go-redis
- google/uuid

## 目录结构

```
cat_coffee/
├── src/
│   ├── main.go              # 主入口（新版本，支持 API 模式）
│   ├── api_server.go        # API 服务器实现
│   ├── scheduler.go         # 调度器
│   ├── agent_worker.go      # Agent 工作进程
│   └── ...
├── frontend/                # 前端项目
├── bin/                     # 编译产物
├── config.yaml              # 配置文件
├── start.sh                 # 启动脚本
├── stop.sh                  # 停止脚本
└── Makefile                 # 编译脚本
```

## 注意事项

1. **Redis 必须运行**: 所有通信都通过 Redis Streams
2. **Agent 进程必须启动**: 否则任务无法被处理
3. **端口占用**: 确保 8080 端口未被占用
4. **并发安全**: 使用了 sync.RWMutex 保证线程安全
5. **内存管理**: Session 数据存储在内存中，重启会丢失

## 下一步

1. 完成 Agent 响应回传机制
2. 实现 WebSocket 实时通信
3. 添加消息持久化
4. 实现 Agent 状态实时同步
5. 添加错误处理和重试机制
