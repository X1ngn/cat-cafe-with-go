# 猫猫咖啡屋 - 完整部署指南

## ✅ 已完成的工作

### 后端实现
- ✅ HTTP API 服务器（基于 Gin）
- ✅ 每个 Session 独立的调度系统
- ✅ 自动初始化猫猫 Agent（花花、薇薇、小乔）
- ✅ 完整的 RESTful API 接口
- ✅ 会话管理、消息管理、猫猫管理
- ✅ 调用历史记录
- ✅ 并发安全（使用 sync.RWMutex）

### 前端实现
- ✅ React + TypeScript + Tailwind CSS
- ✅ 基于 Figma 设计的完整 UI
- ✅ 左侧会话列表
- ✅ 中间对话区（支持 @ 提及）
- ✅ 右侧状态栏
- ✅ API 服务层封装
- ✅ Zustand 状态管理

### 工具脚本
- ✅ start.sh - 一键启动所有服务
- ✅ stop.sh - 停止所有服务
- ✅ Makefile - 编译脚本

## 🚀 快速开始

### 1. 启动 Redis

```bash
# macOS
brew services start redis

# Linux
sudo systemctl start redis

# 验证 Redis 运行
redis-cli ping  # 应该返回 PONG
```

### 2. 启动后端服务

```bash
# 方式一：使用启动脚本（推荐）
./start.sh

# 方式二：手动启动
make build
./bin/cat-cafe --mode api --port 8080 &
./bin/cat-cafe --mode agent --agent 花花 &
./bin/cat-cafe --mode agent --agent 薇薇 &
./bin/cat-cafe --mode agent --agent 小乔 &
```

启动后你会看到：
```
🚀 启动 API 服务器...
✓ API 服务器运行在 http://localhost:8080
✓ 前端可以通过 /api 路径访问接口

可用接口:
  GET    /api/sessions
  POST   /api/sessions
  GET    /api/sessions/:id
  DELETE /api/sessions/:id
  GET    /api/sessions/:id/messages
  POST   /api/sessions/:id/messages
  GET    /api/sessions/:id/stats
  GET    /api/sessions/:id/history
  GET    /api/cats
  GET    /api/cats/:id
  GET    /api/cats/available
```

### 3. 启动前端

```bash
cd frontend
npm install
npm run dev
```

访问 http://localhost:3000

### 4. 停止服务

```bash
./stop.sh
```

## 📖 使用流程

### 创建会话并发送消息

1. **打开浏览器** → http://localhost:3000

2. **创建新会话**
   - 点击左侧"新建对话"按钮
   - 系统自动创建会话并初始化调度器
   - 所有猫猫（花花、薇薇、小乔）已就位

3. **发送消息**
   - 在输入框输入：`@花花 你好`
   - 点击发送
   - 花花会自动加入对话

4. **多猫协作**
   - 输入：`@花花 @薇薇 帮我设计一个网站`
   - 两只猫猫会同时收到任务
   - 可以在右侧看到调用历史

## 🔍 测试 API

### 创建会话
```bash
curl -X POST http://localhost:8080/api/sessions
```

响应：
```json
{
  "id": "sess_abc123",
  "name": "新对话",
  "summary": "",
  "updatedAt": "2026-02-16T10:00:00Z",
  "messageCount": 0
}
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

响应：
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

## 🏗️ 架构说明

### 数据流

```
用户输入 "@花花 你好"
    ↓
前端 (React)
    ↓
HTTP POST /api/sessions/:id/messages
    ↓
API Server (Gin)
    ↓
SessionManager
    ↓
SessionContext (独立的 Scheduler)
    ↓
Redis Streams (pipe:pipe_huahua)
    ↓
Agent Worker (花花)
    ↓
minimal-claude (Claude CLI)
    ↓
返回响应（待实现）
```

### Session 隔离

每个 Session 有：
- 独立的 Scheduler 实例
- 独立的消息队列
- 独立的 Agent 状态管理
- 独立的调用历史

Session 之间完全隔离，互不影响。

## 📁 项目结构

```
cat_coffee/
├── src/
│   ├── main.go              # 主入口（支持 api 模式）
│   ├── api_server.go        # API 服务器和 SessionManager
│   ├── scheduler.go         # 调度器核心
│   ├── agent_worker.go      # Agent 工作进程
│   └── user_interface.go    # 交互式 UI
├── frontend/
│   ├── src/
│   │   ├── components/      # React 组件
│   │   ├── services/        # API 服务
│   │   └── stores/          # 状态管理
│   └── docs/
│       ├── API.md           # 前端 API 文档
│       └── DESIGN.md        # 设计文档
├── doc/
│   ├── BACKEND_API.md       # 后端实现说明
│   └── ...
├── bin/                     # 编译产物
├── config.yaml              # 配置文件
├── start.sh                 # 启动脚本
├── stop.sh                  # 停止脚本
└── QUICKSTART.md            # 快速开始
```

## ⚙️ 配置文件

`config.yaml`:
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

## 🐛 故障排查

### Redis 连接失败
```bash
# 检查 Redis
redis-cli ping

# 启动 Redis
brew services start redis  # macOS
sudo systemctl start redis # Linux
```

### 端口被占用
```bash
# 检查端口
lsof -i :8080

# 杀死进程
kill -9 <PID>
```

### 编译失败
```bash
# 清理并重新编译
make clean
make build
```

### 前端无法连接后端
- 确认后端运行在 8080 端口
- 检查 `frontend/vite.config.ts` 的 proxy 配置
- 查看浏览器控制台的网络请求

## 📝 待完成功能

### 1. Agent 响应回传 ⚠️
目前 Agent 处理完任务后，响应还没有回传到前端。需要：
- Agent Worker 完成任务后写回响应队列
- SessionManager 监听响应队列
- 将猫猫回复添加到消息列表

### 2. WebSocket 实时推送
- 实现 WebSocket 连接
- 实时推送新消息
- 推送打字状态

### 3. Agent 状态同步
- 实时更新 Agent 的 idle/busy 状态
- 前端显示猫猫工作状态

### 4. 消息持久化
- 保存到数据库或文件
- 重启后恢复会话

## 📚 相关文档

- [后端 API 实现说明](doc/BACKEND_API.md)
- [前端 API 接口文档](frontend/docs/API.md)
- [前端设计文档](frontend/docs/DESIGN.md)
- [快速开始指南](QUICKSTART.md)

## 🎉 总结

现在你已经有了：
1. ✅ 完整的后端 API 服务器
2. ✅ 每个 Session 独立的调度系统
3. ✅ 自动初始化的猫猫 Agent
4. ✅ 基于 Figma 设计的前端界面
5. ✅ 完整的启动和停止脚本

可以开始使用猫猫咖啡屋了！🐱☕
