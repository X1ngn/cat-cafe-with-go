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

#### 方式一：交互式界面（推荐）

```bash
# 启动交互式 UI
./cat-cafe --mode ui

# 在交互界面中使用 @ 发送任务
> @花花 实现一个 HTTP 服务器
> @薇薇 审查代码安全性
> @小乔 设计登录页面

# 查看可用命令
> /help
> /list
> /exit
```

#### 方式二：命令行模式

```bash
# 列出所有 Agent
./cat-cafe --list

# 发送任务
./cat-cafe --send --to 花花 --task "实现一个 HTTP 服务器"
```

#### 启动 Agent Worker

```bash
# 在不同终端启动各个 Agent
./cat-cafe --mode agent --agent 花花
./cat-cafe --mode agent --agent 薇薇
./cat-cafe --mode agent --agent 小乔
```

## 🧪 运行测试

```bash
# 启动 Redis
redis-server --daemonize yes

# 运行测试
cd test && go test -v -count=1
```

**测试结果**: ✅ 所有测试通过 (7/7)

详见 [TEST_REPORT.md](TEST_REPORT.md)

## 📁 项目结构

```
.
├── main.go                  # 主程序
├── scheduler.go             # 调度器
├── agent_worker.go          # Agent 工作进程
├── user_interface.go        # 交互式用户界面
├── config.yaml              # 配置文件
├── minimal-claude.go        # Claude CLI 包装器
├── minimal-codex.go         # Codex CLI 包装器
├── minimal-gemini.go        # Gemini CLI 包装器
├── invoke.go                # CLI 调用核心逻辑
├── prompts/                 # Agent 系统提示词
│   ├── calico_cat.md        # 花花的提示词
│   ├── lihua_cat.md         # 薇薇的提示词
│   └── silver_cat.md        # 小乔的提示词
├── test/                    # 测试文件
│   ├── scheduler_test.go    # 单元测试
│   └── scheduler_wrapper.go # 测试包装器
├── SPEC.md                  # 系统设计规范
├── TEST_SPEC.md             # 测试规范
├── TEST_REPORT.md           # 测试报告
├── USAGE_GUIDE.md           # 使用指南
└── Makefile                 # 编译脚本
```

## 🛠 工作流程

### 架构

```
用户交互界面 (UI)
    ↓ @花花 任务内容
调度器 (Scheduler)
    ↓
Redis Streams (消息队列)
    ↓
┌──────────┬──────────┬──────────┐
│  花花     │  薇薇     │  小乔     │
│ Worker   │ Worker   │ Worker   │
└──────────┴──────────┴──────────┘
    ↓           ↓           ↓
执行任务并返回结果
```

### 完整流程示例

#### 方式一：用户直接发送任务

使用交互式 UI:

```bash
./cat-cafe --mode ui

> @花花 设计用户认证系统
✓ 任务已发送给 花花

> @薇薇 审查认证系统的安全性
✓ 任务已发送给 薇薇

> @小乔 设计登录界面
✓ 任务已发送给 小乔
```

#### 方式二：Agent 自动协作（推荐）

Agent 可以在输出中使用 @标记调用其他 Agent，实现自动化工作流:

```bash
> @花花 开发一个用户登录系统

花花输出:
【系统设计】
...架构设计完成...

@薇薇 请审查这个登录系统的安全性
[架构文档]

---

薇薇自动收到任务并输出:
【审查报告】
发现3个安全问题...

@花花 请修复这些安全问题
[问题列表]

---

花花自动收到任务并输出:
【修复完成】
已修复所有安全问题...

@小乔 请设计登录界面
[需求说明]

---

小乔自动收到任务并输出:
【设计方案】
登录界面设计完成...

@铲屎官 用户登录系统开发完成，请查看
[完整方案]
```

或使用命令行:

```bash
./cat-cafe --send --to 花花 --task "设计用户认证系统"
./cat-cafe --send --to 薇薇 --task "审查认证系统的安全性"
./cat-cafe --send --to 小乔 --task "设计登录界面"
```

## 📚 文档

- [COLLABORATION.md](COLLABORATION.md) - Agent 协作机制详解
- [SPEC.md](SPEC.md) - 系统设计规范
- [TEST_SPEC.md](TEST_SPEC.md) - 测试规范
- [TEST_REPORT.md](TEST_REPORT.md) - 测试报告
- [USAGE_GUIDE.md](USAGE_GUIDE.md) - 详细使用指南

## 🎯 核心特性

- ✅ **交互式 UI** - 使用 @Agent 语法轻松发送任务
- ✅ **Agent 协作** - Agent 可以相互调用，自动化工作流
- ✅ **Redis Streams 消息队列** - 可靠的消息传递
- ✅ **无状态通信** - Agent 独立运行，易于扩展
- ✅ **YAML 配置** - 灵活的 Agent 管理
- ✅ **重试机制** - 失败任务自动重试
- ✅ **顺序执行** - 保证任务按序处理
- ✅ **动态扩展** - 轻松添加新 Agent
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

## 📊 测试覆盖

- ✅ Agent 注册与配置
- ✅ 任务发送测试
- ✅ 无状态通信测试
- ✅ 消息可靠性测试
- ✅ 顺序任务执行
- ✅ 新增 Agent 测试
- ✅ 配置文件安全性测试

详见 [TEST_REPORT.md](TEST_REPORT.md)

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License