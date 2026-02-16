# 更新日志

## [2026-02-17] - Agent 回复功能完成

### ✨ 新增功能

- **Agent 实时回复**: Agent 处理完成后，回复会自动添加到会话消息列表
- **结果队列机制**: 使用 Redis Streams 实现 Agent 到 API Server 的结果传递
- **完整的消息流**: 用户消息 → Agent 处理 → 自动回复，全流程打通
- **正确的消息显示**: 猫猫回复显示在对话框左侧，用户消息显示在右侧

### 🔧 技术改进

- 扩展 `TaskMessage` 结构，添加 `Result` 和 `SessionID` 字段
- `SessionManager` 新增后台监听器，实时接收 Agent 结果
- `AgentWorker` 完成任务后自动发送结果到结果队列
- 修复 Claude Code 嵌套会话冲突问题（使用 `env -u CLAUDECODE`）
- 统一消息类型：使用 `cat` 而非 `agent`，与前端组件保持一致

### 📝 文档更新

- 更新 `frontend/docs/API.md`：添加消息类型和工作流程说明
- 更新 `doc/USAGE_GUIDE.md`：添加 API 模式使用指南
- 更新 `doc/README.md`：完善架构图和消息流程说明
- 明确消息类型：`user`（右侧）、`cat`（左侧）、`system`（居中）

### 🐛 问题修复

- 修复猫猫 ID 到名字的映射问题
- 修复命令行参数传递错误
- 修复 Claude Code 嵌套会话导致的启动失败
- 添加详细的日志系统，便于调试

### 🧪 测试

- 创建 `test_api.sh` 脚本用于快速测试 API 功能
- 验证完整的消息流程：发送 → 处理 → 回复

### 📊 工作流程

```
用户发送消息
    ↓
API Server 接收并分发任务
    ↓
Agent 处理任务
    ↓
结果发送到结果队列
    ↓
API Server 接收结果
    ↓
自动添加到会话消息
    ↓
前端获取更新
```

---

## [2026-02-16] - 初始版本

### ✨ 核心功能

- 多 Agent 协作系统
- Redis Streams 消息队列
- 交互式 UI 界面
- 命令行模式
- API 服务器基础框架
