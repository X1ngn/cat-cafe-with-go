# 编排/治理层设计文档

## 状态

**✅ 已实现** - 2024年2月

本文档描述了猫猫咖啡屋的编排/治理层设计，该层位于调度器之上，负责管理不同的协作模式。

## 概述

编排/治理层是一个抽象层，用于管理猫猫们的协作方式。它允许系统支持多种协作模式，每种模式定义了猫猫们如何响应用户消息和互相调用。

### 核心概念

- **协作模式 (Collaboration Mode)**: 定义猫猫们如何协作的规则
- **编排器 (Orchestrator)**: 管理会话和模式的核心组件
- **模式注册表 (Mode Registry)**: 存储和管理所有可用的协作模式
- **会话 (Session)**: 每个会话可以独立选择协作模式

## 架构设计

### 层次结构

```
┌─────────────────────────────────────┐
│         API Server                  │
│  (处理 HTTP 请求，管理会话)          │
└─────────────────┬───────────────────┘
                  │
┌─────────────────▼───────────────────┐
│         Orchestrator                │
│  (编排器，管理模式和会话)            │
└─────────────────┬───────────────────┘
                  │
        ┌─────────┴─────────┐
        │                   │
┌───────▼────────┐  ┌──────▼──────────┐
│ Mode Registry  │  │   Scheduler     │
│ (模式注册表)    │  │   (调度器)      │
└────────────────┘  └─────────────────┘
        │
┌───────▼────────────────────────────┐
│     Collaboration Modes            │
│  - FreeDiscussionMode              │
│  - SOPMode (未来)                  │
│  - CustomMode (未来)               │
└────────────────────────────────────┘
```

### 核心组件

#### 1. CollaborationMode 接口

定义了所有协作模式必须实现的方法：

```go
type CollaborationMode interface {
    GetName() string
    GetDescription() string
    OnUserMessage(sessionID string, content string, mentionedCats []string) ([]AgentCall, error)
    OnAgentResponse(sessionID string, agentName string, response string) ([]AgentCall, error)
    Validate() error
    Initialize(sessionID string) error
}
```

#### 2. Orchestrator (编排器)

负责：
- 管理会话和模式的映射
- 处理用户消息和猫猫回复
- 协调模式切换
- 维护会话状态

#### 3. ModeRegistry (模式注册表)

负责：
- 注册和存储协作模式
- 创建模式实例
- 查询可用模式

## 已实现的协作模式

### 1. 自由讨论模式 (FreeDiscussionMode)

**名称**: `free_discussion`

**描述**: 猫猫们可以自由互相调用，没有固定流程

**特点**:
- 用户可以 @ 任意猫猫
- 猫猫可以在回复中 @ 其他猫猫
- 没有调用顺序限制
- 支持多轮对话

**实现**:
- 解析用户消息中的 @ 提及
- 解析猫猫回复中的 @ 调用
- 为每个被 @ 的猫猫创建任务

## API 集成

### 新增 API 端点

#### 1. 获取所有可用模式

```
GET /api/modes
```

响应：
```json
[
  {
    "name": "free_discussion",
    "description": "自由讨论模式 - 猫猫们可以自由互相调用"
  }
]
```

#### 2. 获取会话当前模式

```
GET /api/sessions/:sessionId/mode
```

响应：
```json
{
  "mode": "free_discussion",
  "description": "自由讨论模式 - 猫猫们可以自由互相调用",
  "config": {
    "name": "free_discussion",
    "enabled": true
  },
  "state": {
    "custom_state": {},
    "last_update_time": "2024-02-17T10:00:00Z"
  }
}
```

#### 3. 切换会话模式

```
PUT /api/sessions/:sessionId/mode
```

请求体：
```json
{
  "mode": "free_discussion",
  "modeConfig": {
    "option1": "value1"
  }
}
```

响应：
```json
{
  "mode": "free_discussion",
  "description": "自由讨论模式 - 猫猫们可以自由互相调用"
}
```

## 数据结构

### ModeConfig

```go
type ModeConfig struct {
    Name    string                 `json:"name"`
    Enabled bool                   `json:"enabled"`
    Config  map[string]interface{} `json:"config,omitempty"`
}
```

### ModeState

```go
type ModeState struct {
    CurrentStep    string                 `json:"current_step,omitempty"`
    StepHistory    []string               `json:"step_history,omitempty"`
    CustomState    map[string]interface{} `json:"custom_state,omitempty"`
    LastUpdateTime time.Time              `json:"last_update_time"`
}
```

### AgentCall

```go
type AgentCall struct {
    AgentName  string                 `json:"agent_name"`
    Prompt     string                 `json:"prompt"`
    SessionID  string                 `json:"session_id"`
    CallerName string                 `json:"caller_name"`
    Metadata   map[string]interface{} `json:"metadata,omitempty"`
}
```

## 扩展性设计

### 添加新的协作模式

1. 实现 `CollaborationMode` 接口
2. 在 `mode_registry.go` 中注册模式
3. 更新 API 文档

示例：

```go
type SOPMode struct {
    config *ModeConfig
    steps  []string
}

func (m *SOPMode) GetName() string {
    return "sop"
}

func (m *SOPMode) GetDescription() string {
    return "SOP 流程模式 - 按预定义流程执行"
}

func (m *SOPMode) OnUserMessage(sessionID string, content string, mentionedCats []string) ([]AgentCall, error) {
    // 实现 SOP 逻辑
    return calls, nil
}

// ... 实现其他方法
```

### 模式配置

每个模式可以有自己的配置选项，通过 `ModeConfig.Config` 传递：

```json
{
  "mode": "sop",
  "modeConfig": {
    "steps": ["花花", "薇薇", "小乔"],
    "strict": true
  }
}
```

## 实现细节

### 会话创建流程

1. API Server 收到创建会话请求
2. 创建 SessionContext，默认使用 `free_discussion` 模式
3. 在 Orchestrator 中注册会话
4. 初始化模式
5. 返回会话信息

### 消息处理流程

#### 用户消息

1. API Server 收到用户消息
2. 调用 `Orchestrator.HandleUserMessage()`
3. 当前模式处理消息，返回需要调用的猫猫列表
4. API Server 通过调度器发送任务
5. 更新会话状态和调用历史

#### 猫猫回复

1. API Server 从 Redis 接收猫猫回复
2. 添加回复消息到会话
3. 调用 `Orchestrator.HandleAgentResponse()`
4. 当前模式处理回复，返回后续调用列表
5. API Server 发送后续任务

### 模式切换流程

1. API Server 收到切换模式请求
2. 验证新模式是否存在
3. 调用 `Orchestrator.SwitchMode()`
4. 初始化新模式
5. 更新会话上下文
6. 添加系统消息通知用户

## 测试策略

### 单元测试

- 测试每个协作模式的逻辑
- 测试模式注册表的注册和查询
- 测试编排器的会话管理

### 集成测试

- 测试完整的消息处理流程
- 测试模式切换功能
- 测试多会话并发场景

## 未来扩展

### 计划中的协作模式

1. **SOP 流程模式**
   - 按预定义步骤执行
   - 支持条件分支
   - 支持循环和重试

2. **投票模式**
   - 多个猫猫并行处理
   - 结果投票决策
   - 支持加权投票

3. **专家路由模式**
   - 根据任务类型自动选择专家猫猫
   - 支持技能标签匹配
   - 动态路由决策

### 高级特性

- 模式热重载
- 模式版本管理
- 模式性能监控
- 模式 A/B 测试

## 总结

编排/治理层为猫猫咖啡屋提供了灵活的协作模式管理能力。通过清晰的接口定义和模块化设计，系统可以轻松扩展支持新的协作模式，满足不同场景的需求。

当前实现的自由讨论模式已经完全集成到系统中，为未来添加更多模式奠定了坚实的基础。
