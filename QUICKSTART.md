# 猫猫咖啡屋 - 快速启动指南

## 🚀 快速启动

### 前置条件

1. **安装 Redis**
   ```bash
   # macOS
   brew install redis
   brew services start redis

   # Linux
   sudo apt-get install redis-server
   sudo systemctl start redis
   ```

2. **安装 Go 1.21+**
   ```bash
   go version  # 确认版本
   ```

3. **安装 Node.js 18+**（前端需要）
   ```bash
   node --version  # 确认版本
   ```

### 一键启动

```bash
# 1. 编译项目
make build

# 2. 启动所有服务（API + 所有 Agent）
./start.sh

# 3. 在另一个终端，启动前端
cd frontend
npm install
npm run dev
```

访问 http://localhost:3000 即可使用！

### 停止服务

```bash
./stop.sh
```

## 📁 项目结构

```
cat_coffee/
├── src/                     # 后端 Go 代码
│   ├── main.go             # 主入口（支持 API 模式）
│   ├── api_server.go       # API 服务器实现
│   ├── scheduler.go        # 调度器核心
│   └── agent_worker.go     # Agent 工作进程
├── frontend/               # 前端 React 项目
│   ├── src/
│   │   ├── components/    # React 组件
│   │   ├── services/      # API 服务
│   │   └── stores/        # 状态管理
│   └── docs/
│       ├── API.md         # API 接口文档
│       └── DESIGN.md      # 前端设计文档
├── doc/                    # 后端文档
│   ├── BACKEND_API.md     # 后端 API 实现说明
│   └── ...
├── config.yaml             # 系统配置
├── start.sh                # 启动脚本
└── stop.sh                 # 停止脚本
```

## 🎯 核心特性

### 1. 每个会话独立的调度系统
- 创建会话时自动初始化独立的 Scheduler
- 每个会话有自己的消息队列和 Agent 状态
- 会话之间完全隔离

### 2. 多 Agent 协作
- 支持 @ 提及多个猫猫
- 猫猫们可以协同工作
- 实时状态监控

### 3. 基于 Figma 设计的 UI
- 完全还原 Figma 设计稿
- React + TypeScript + Tailwind CSS
- 响应式布局

## 🔧 手动启动（开发模式）

### 后端

```bash
# 终端 1: 启动 API 服务器
./bin/cat-cafe --mode api --port 8080

# 终端 2: 启动花花
./bin/cat-cafe --mode agent --agent 花花

# 终端 3: 启动薇薇
./bin/cat-cafe --mode agent --agent 薇薇

# 终端 4: 启动小乔
./bin/cat-cafe --mode agent --agent 小乔
```

### 前端

```bash
cd frontend
npm run dev
```

## 📖 文档

- [后端 API 实现说明](doc/BACKEND_API.md)
- [前端 API 接口文档](frontend/docs/API.md)
- [前端设计文档](frontend/docs/DESIGN.md)
- [项目总览](frontend/docs/OVERVIEW.md)

## 🧪 测试 API

```bash
# 创建会话
curl -X POST http://localhost:8080/api/sessions

# 获取会话列表
curl http://localhost:8080/api/sessions

# 发送消息
curl -X POST http://localhost:8080/api/sessions/sess_xxx/messages \
  -H "Content-Type: application/json" \
  -d '{"content": "@花花 你好", "mentionedCats": ["花花"]}'

# 获取消息
curl http://localhost:8080/api/sessions/sess_xxx/messages

# 获取猫猫列表
curl http://localhost:8080/api/cats
```

## 🐱 可用的猫猫

- **花花** - 三花猫，使用 Claude
- **薇薇** - 狸花猫，使用 Codex
- **小乔** - 银渐层，使用 Gemini

## 🛠️ 开发

### 编译

```bash
make build
```

### 运行测试

```bash
make test
```

### 列出所有 Agent

```bash
./bin/cat-cafe --list
```

## 📝 使用示例

1. 打开浏览器访问 http://localhost:3000
2. 点击"新建对话"创建会话
3. 在输入框输入 `@花花 你好`
4. 花花会自动加入对话并回复
5. 可以同时 @ 多个猫猫：`@花花 @薇薇 帮我设计一个网站`

## 🔍 故障排查

### Redis 连接失败
```bash
# 检查 Redis 是否运行
redis-cli ping

# 应该返回 PONG
```

### 端口被占用
```bash
# 检查端口占用
lsof -i :8080

# 杀死占用进程
kill -9 <PID>
```

### 前端无法连接后端
- 确认后端 API 服务器运行在 8080 端口
- 检查 `frontend/vite.config.ts` 中的 proxy 配置

## 🎨 架构说明

```
用户 → 前端 (React) → API 服务器 (Go + Gin)
                           ↓
                    SessionManager
                           ↓
                    独立的 Scheduler (每个 Session)
                           ↓
                    Redis Streams
                           ↓
                    Agent Workers (花花、薇薇、小乔)
                           ↓
                    CLI 工具 (Claude, Codex, Gemini)
```

## 📦 依赖

### 后端
- Go 1.21+
- Redis 6.0+
- github.com/gin-gonic/gin
- github.com/go-redis/redis/v8
- github.com/google/uuid

### 前端
- React 18
- TypeScript 5
- Tailwind CSS 3
- Zustand 4
- Axios
- Vite 5

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT
