# 🐱 猫猫咖啡屋 - Multi-Agent 调度器

一个基于 Go 的多 Agent 协作开发系统，由三只可爱的猫咪 Agent 组成。

## 🎭 猫咪团队

- **🐱 三花猫·花花** (Claude Opus) - 主架构师 & 核心开发
- **🐱 狸花猫·薇薇** (Codex) - 代码审查 & 安全测试
- **🐱 银渐层·小乔** (Gemini) - 视觉设计

## 🚀 快速开始

### 1. 安装依赖

```bash
# 安装 Redis
brew install redis

# 下载 Go 依赖
go mod download
```

### 2. 编译

```bash
make build
```

### 3. 启动 Redis

```bash
redis-server --daemonize yes
```

### 4. 使用

#### 方式一：一键启动（推荐）

使用便捷脚本启动所有服务（API Server + 所有 Agent）：

```bash
# 启动所有服务
./start.sh

# 停止所有服务
./stop.sh
```

启动后：
- API 服务器运行在 `http://localhost:8080`
- 所有猫猫 Agent 自动启动并等待任务
- 可以通过 API 或前端界面与猫猫们交互

#### 方式二：交互式界面

```bash
# 启动交互式 UI
./bin/cat-cafe --mode ui

# 在交互界面中使用 @ 发送任务
> @花花 实现一个 HTTP 服务器
> @薇薇 审查代码安全性
> @小乔 设计登录页面

# 查看可用命令
> /help
> /list
> /exit
```

#### 方式三：命令行模式

```bash
# 列出所有 Agent
./bin/cat-cafe --list

# 发送任务
./bin/cat-cafe --send --to 花花 --task "实现一个 HTTP 服务器"
```

#### 方式四：API 模式

```bash
# 手动启动 API 服务器
./bin/cat-cafe --mode api --port 8080

# 手动启动 Agent Workers（在不同终端）
./bin/cat-cafe --mode agent --agent 花花
./bin/cat-cafe --mode agent --agent 薇薇
./bin/cat-cafe --mode agent --agent 小乔
```

**API 使用示例：**

```bash
# 1. 创建会话
curl -X POST http://localhost:8080/api/sessions

# 2. 发送消息
curl -X POST http://localhost:8080/api/sessions/sess_xxx/messages \
  -H "Content-Type: application/json" \
  -d '{
    "content": "@花花 你好",
    "mentionedCats": ["cat_001"]
  }'

# 3. 获取消息（包含 Agent 回复）
curl http://localhost:8080/api/sessions/sess_xxx/messages
```

详细 API 文档：`frontend/docs/API.md`

## 🧪 运行测试

```bash
make test
```

**测试结果**: ✅ 所有测试通过 (8/8)

详见 [TEST_REPORT.md](TEST_REPORT.md)

## 📁 项目结构

```
.
├── src/                     # 源代码
│   ├── main.go              # 主程序
│   ├── scheduler.go         # 调度器
│   ├── agent_worker.go      # Agent 工作进程
│   ├── user_interface.go    # 交互式用户界面
│   ├── api_server.go        # API 服务器
│   ├── logger.go            # 日志系统
│   ├── orchestrator.go      # 编排器
│   ├── mode_interface.go    # 协作模式接口
│   ├── mode_registry.go     # 模式注册表
│   ├── mode_free_discussion.go  # 自由讨论模式
│   ├── minimal-claude.go    # Claude CLI 包装器
│   ├── minimal-codex.go     # Codex CLI 包装器
│   ├── minimal-gemini.go    # Gemini CLI 包装器
│   └── invoke.go            # CLI 调用核心逻辑
├── bin/                     # 编译产物
├── doc/                     # 后端文档
│   ├── README.md            # 项目说明
│   ├── BACKEND_API.md       # API 文档
│   ├── ORCHESTRATION_DESIGN.md  # 编排层设计
│   └── ...                  # 其他文档
├── frontend/                # 前端项目
│   ├── src/                 # React 源代码
│   └── docs/                # 前端文档
│       └── API.md           # API 接口文档
├── test/                    # 测试文件
├── prompts/                 # Agent 系统提示词
│   ├── calico_cat.md        # 花花的提示词
│   ├── lihua_cat.md         # 薇薇的提示词
│   └── silver_cat.md        # 小乔的提示词
├── config.yaml              # 配置文件
├── Makefile                 # 编译脚本
├── start.sh                 # 启动脚本
├── stop.sh                  # 停止脚本
├── test_api.sh              # API 测试脚本
└── chat_history.jsonl       # 聊天记录
```

## 🛠 工作流程

### 架构

```
前端界面 / API 客户端
    ↓
API 服务器 (SessionManager)
    ↓
编排器 (Orchestrator)
    ├─ 协作模式管理
    └─ 会话状态管理
    ↓ 发送任务
调度器 (Scheduler)
    ↓
Redis Streams (任务队列)
    ↓
┌──────────┬──────────┬──────────┐
│  花花     │  薇薇     │  小乔     │
│ Worker   │ Worker   │ Worker   │
└──────────┴──────────┴──────────┘
    ↓           ↓           ↓
执行任务 (调用 Claude/Codex/Gemini)
    ↓
Redis Streams (结果队列)
    ↓
API 服务器接收结果
    ↓
编排器处理回复 (解析 @ 调用)
    ↓
添加到会话消息列表
    ↓
前端轮询获取更新
```

### 协作模式

系统支持多种协作模式，每个会话可以独立选择：

- **自由讨论模式** (默认) - 猫猫们可以自由互相调用
- **SOP 流程模式** (计划中) - 按预定义流程执行
- **更多模式** - 可扩展支持

详见 [ORCHESTRATION_DESIGN.md](ORCHESTRATION_DESIGN.md)

### 消息流程

1. **用户发送消息** → API Server 接收
2. **提及猫猫** → 任务发送到对应 Agent 的队列
3. **Agent 处理** → 调用 AI 模型生成回复
4. **结果返回** → 通过结果队列发送回 API Server
5. **消息更新** → 自动添加到会话消息列表
6. **前端自动获取** → 智能轮询获取最新消息
   - 发送消息后：1 秒快速轮询
   - 收到回复后：3 秒慢速轮询
   - 无需手动刷新页面

## 📚 文档

- [API.md](../frontend/docs/API.md) - API 接口文档
- [BACKEND_API.md](BACKEND_API.md) - 后端 API 实现说明
- [ORCHESTRATION_DESIGN.md](ORCHESTRATION_DESIGN.md) - 编排/治理层设计
- [COLLABORATION.md](COLLABORATION.md) - Agent 协作机制详解
- [SPEC.md](SPEC.md) - 系统设计规范
- [TEST_SPEC.md](TEST_SPEC.md) - 测试规范
- [TEST_REPORT.md](TEST_REPORT.md) - 测试报告
- [USAGE_GUIDE.md](USAGE_GUIDE.md) - 详细使用指南

## 🎯 核心特性

- ✅ **RESTful API** - 完整的 HTTP API 接口，支持前端集成
- ✅ **会话管理** - 多会话支持，每个会话独立的调度器
- ✅ **编排/治理层** - 支持多种协作模式，可扩展的模式管理
- ✅ **协作模式** - 自由讨论模式已实现，支持运行时切换
- ✅ **实时回复** - Agent 处理完成后自动将回复添加到会话
- ✅ **智能轮询** - 前端自动获取新消息，发送后快速轮询，收到回复后慢速轮询
- ✅ **调用历史** - 实时追踪所有被调用的猫猫，支持展开查看完整 Prompt 和 Response
- ✅ **Markdown 渲染** - 消息支持丰富的格式化显示（粗体、代码块、列表、表格等）
- ✅ **交互式 UI** - 使用 @Agent 语法轻松发送任务
- ✅ **Agent 协作** - Agent 可以相互调用，自动化工作流
- ✅ **头像显示** - 用户和猫猫都有专属头像，猫猫互相调用时头像正确显示
- ✅ **@ 提及菜单** - 输入 @ 弹出猫猫选择菜单，支持头像显示和智能关闭
- ✅ **系统消息优化** - 每只猫猫只在首次加入时显示"已加入对话"
- ✅ **Redis Streams 消息队列** - 可靠的消息传递
- ✅ **无状态通信** - Agent 独立运行，易于扩展
- ✅ **YAML 配置** - 灵活的 Agent 管理
- ✅ **重试机制** - 失败任务自动重试
- ✅ **聊天记录** - 消息落盘到 chat_history.jsonl
- ✅ **完整测试** - 100% 测试覆盖

## 📝 配置示例

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

## 🧹 清理

```bash
make clean
```

## 📄 许可证

MIT License
