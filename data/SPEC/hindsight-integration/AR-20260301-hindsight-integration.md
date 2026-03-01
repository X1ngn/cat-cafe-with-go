# AR-20260301-hindsight-integration

## Hindsight 集成到猫猫咖啡屋 — 设计文档

**作者**: 花花
**日期**: 2026-03-01
**状态**: Draft
**优先级**: P0

---

## 1. 背景与动机

猫猫咖啡屋当前使用自研的 Session Chain 做短期记忆（对话历史、摘要压缩），但没有长期记忆能力。当 Session 被 seal 并压缩后，原始对话细节丢失，只保留摘要。

Hindsight 已作为 submodule 引入（`hindsight/`），它本身实现了完整的 MCP Server（基于 FastMCP），提供 retain/recall/reflect 等 29 个 MCP tools。

### 1.1 目标

通过 MCP 协议将 hindsight 接入三只猫猫，让模型自主决定何时存取长期记忆：
- 模型在对话中自行判断何时需要 retain（存储记忆）
- 模型在需要历史上下文时自行 recall（检索记忆）
- 模型在需要深度分析时自行 reflect（综合推理）

### 1.2 核心设计原则

**MCP 优先，模型自治**：不在 Go 后端硬编码 retain/recall 时机，而是将 hindsight 作为 MCP Server 暴露给模型，由模型根据对话上下文自主选择何时调用记忆工具。这比后端硬编码更灵活——模型能理解语义，知道什么值得记住、什么时候需要回忆。

### 1.3 范围

本方案涉及：
- Hindsight 作为 MCP Server 的部署
- 三只猫猫的 MCP 配置（每只猫独立 Bank）
- Go 后端的 MCP 配置生成改造

不涉及：
- Hindsight 本身的代码修改
- Bank Federation / 多 Agent 共享记忆（独立 SPEC）

---

## 2. 架构概览

```
                          ┌─────────────────────────────┐
                          │   Hindsight API Server      │
                          │   (Python/FastAPI)          │
                          │   http://localhost:8888     │
                          │                             │
                          │   /mcp/cat-花花/  ──► Bank: cat-花花  │
                          │   /mcp/cat-薇薇/  ──► Bank: cat-薇薇  │
                          │   /mcp/cat-小乔/  ──► Bank: cat-小乔  │
                          └──────────┬──────────────────┘
                                     │ MCP (HTTP transport)
                    ┌────────────────┼────────────────┐
                    │                │                │
              ┌─────▼─────┐  ┌──────▼─────┐  ┌──────▼──────┐
              │  花花       │  │  薇薇       │  │  小乔        │
              │  (claude)  │  │  (codex)   │  │  (gemini)   │
              │            │  │            │  │             │
              │  MCP:      │  │  MCP:      │  │  MCP:       │
              │  - session │  │  - session │  │  - session  │
              │    -chain  │  │    -chain  │  │    -chain   │
              │  - hindsight│ │  - hindsight│ │  - hindsight│
              └────────────┘  └────────────┘  └─────────────┘
```

**记忆分层**：
- 短期记忆（Session Chain MCP）：当前 Thread 的对话历史 + 已 seal Session 的摘要 → 由后端编排注入
- 长期记忆（Hindsight MCP）：跨 Session 的 facts、entities、relationships → 由模型自主调用

两者互补：Session Chain 提供精确的近期上下文，Hindsight 提供跨 Session 的长期知识。模型同时拥有两个 MCP Server，按需使用。

---

## 3. Hindsight 部署方案

### 3.1 部署方式

Hindsight 作为独立的 HTTP 服务运行，内置 MCP Server 默认挂载在 `/mcp` 路径。

**启动命令**：
```bash
cd hindsight/hindsight-api
uv run hindsight-api
# 默认监听 http://localhost:8888
# MCP 端点: http://localhost:8888/mcp/{bank_id}/
```

### 3.2 Bank 规划

每只猫猫一个独立 Bank，使用 **single-bank 模式**（推荐）：

| 猫猫 | Bank ID | MCP Endpoint | 说明 |
|------|---------|-------------|------|
| 花花 | `cat-花花` | `http://localhost:8888/mcp/cat-花花/` | 主架构师的记忆 |
| 薇薇 | `cat-薇薇` | `http://localhost:8888/mcp/cat-薇薇/` | 代码审查员的记忆 |
| 小乔 | `cat-小乔` | `http://localhost:8888/mcp/cat-小乔/` | UI/UX 设计师的记忆 |

Single-bank 模式的优势：
- 工具调用不需要传 `bank_id` 参数，简化模型调用
- 天然隔离，每个 MCP 连接只能访问自己的 Bank
- 暴露 26 个工具（不含 `list_banks`、`create_bank`、`get_bank_stats`）

### 3.3 Bank 初始化

Bank 在首次连接时自动创建（hindsight 的 single-bank 模式支持）。也可以提前通过 HTTP API 或 multi-bank MCP 创建：

```bash
# 通过 HTTP API 预创建 Bank
curl -X PUT http://localhost:8888/v1/default/banks/cat-花花 \
  -H "Content-Type: application/json" \
  -d '{"name": "花花", "mission": "三花猫，主架构师，负责系统设计和核心开发"}'
```

### 3.4 环境配置

Hindsight 需要的最小配置（`.env` 或环境变量）：

```bash
# LLM 配置（hindsight 内部用于 fact extraction、reflect 等）
HINDSIGHT_API_LLM_PROVIDER=anthropic
HINDSIGHT_API_LLM_API_KEY=your-key
HINDSIGHT_API_LLM_MODEL=claude-sonnet-4-20250514

# 可选：认证（本地开发可不设）
# HINDSIGHT_API_TENANT_EXTENSION=hindsight_api.extensions.builtin.tenant:ApiKeyTenantExtension
# HINDSIGHT_API_TENANT_API_KEY=your-secret-key
```

---

## 4. MCP 配置生成改造

### 4.1 当前机制

`GenerateMCPConfig()` 目前只生成 session-chain 一个 MCP Server：

```json
{
  "mcpServers": {
    "session-chain": {
      "command": "./bin/cat-cafe",
      "args": ["--mode", "mcp", "--thread", "thread-xxx"],
      "type": "stdio"
    }
  }
}
```

### 4.2 改造后

在 `mcpServers` 中新增 `hindsight`，使用 HTTP transport 连接到 hindsight 的 per-bank endpoint：

```json
{
  "mcpServers": {
    "session-chain": {
      "command": "./bin/cat-cafe",
      "args": ["--mode", "mcp", "--thread", "thread-xxx"],
      "type": "stdio"
    },
    "hindsight": {
      "url": "http://localhost:8888/mcp/cat-花花/",
      "type": "http"
    }
  }
}
```

### 4.3 配置来源

`config.yaml` 新增 hindsight 配置段：

```yaml
agents:
  - name: "花花"
    pipe: "pipe_huahua"
    cli_type: "claude"
    system_prompt_path: "prompts/calico_cat.md"
    dev_prompt_path: "prompts/dev_sop.md"
    avatar: "/images/sanhua.png"
    context_mode: "orchestrated"

  # ... 其他 agent 同理

hindsight:
  enabled: true
  base_url: "http://localhost:8888"
  # token: ""  # 可选，本地开发不需要
```

Bank ID 由代码自动生成：`cat-{agent_name}`，MCP endpoint 为 `{base_url}/mcp/{bank_id}/`。

---

## 5. 实现方案

### Phase 1：config.yaml + 配置解析

**修改文件**: `src/scheduler.go`

新增 `HindsightConfig` 结构体和 `Config` 解析：

```go
type HindsightConfig struct {
    Enabled bool   `yaml:"enabled"`
    BaseURL string `yaml:"base_url"`
    Token   string `yaml:"token,omitempty"`
}
```

### Phase 2：GenerateMCPConfig 改造

**修改文件**: `src/cli_adapter.go`

`GenerateMCPConfig` 签名扩展，接收 agent name 和 hindsight 配置：

```go
func GenerateMCPConfig(threadID, binPath, agentName string, hindsightCfg *HindsightConfig) (string, error)
```

当 `hindsightCfg.Enabled` 时，在 `mcpServers` 中追加 hindsight 条目：

```go
if hindsightCfg != nil && hindsightCfg.Enabled {
    bankID := BankIDForAgent(agentName)
    mcpURL := fmt.Sprintf("%s/mcp/%s/", hindsightCfg.BaseURL, bankID)
    servers["hindsight"] = map[string]interface{}{
        "url":  mcpURL,
        "type": "http",
    }
    if hindsightCfg.Token != "" {
        servers["hindsight"].(map[string]interface{})["headers"] = map[string]string{
            "Authorization": "Bearer " + hindsightCfg.Token,
        }
    }
}
```

### Phase 3：AgentWorker 传递配置

**修改文件**: `src/agent_worker.go`

`executeTask` 中调用 `GenerateMCPConfig` 时传入 agent name 和 hindsight 配置：

```go
mcpConfigPath, mcpErr := GenerateMCPConfig(task.SessionID, "", w.config.Name, w.hindsightCfg)
```

### Phase 4：CLI 适配确认

当前 `InvokeCLI` 中：
- `claude` CLI：已支持 `--mcp-config`，且支持 HTTP transport MCP ✅
- `gemini` CLI：已支持 `--mcp-config`，需确认 HTTP transport 支持
- `codex` CLI：当前不传 `--mcp-config`，需要调研是否支持

> **注意**：codex（薇薇）目前不支持 MCP config 注入。Phase 4 需要调研 codex 的 MCP 支持情况，如果不支持，薇薇暂时无法使用 hindsight 长期记忆。

### Phase 5：前端 Hindsight 状态面板

**新增/修改文件**:
- `frontend/src/components/StatusBar/HindsightPanel.tsx`（新增）
- `frontend/src/components/StatusBar/index.tsx`（修改，引入 HindsightPanel）
- `frontend/src/services/api.ts`（修改，新增 hindsightAPI）
- `src/api_server.go`（修改，新增 `/api/hindsight/health` 代理端点）

在 StatusBar 中新增一个 Hindsight 状态面板（放在 SessionChainPanel 下方），展示 hindsight 服务的健康状态。

**后端代理端点**：

Go 后端新增 `/api/hindsight/health`，代理请求到 hindsight 的 health API（`GET http://localhost:8888/v1/default/health`），避免前端跨域问题：

```go
// GET /api/hindsight/health
// 代理到 hindsight 服务的 health 端点
func (s *APIServer) handleHindsightHealth(w http.ResponseWriter, r *http.Request) {
    if !s.config.Hindsight.Enabled {
        json.NewEncoder(w).Encode(map[string]string{"status": "disabled"})
        return
    }
    resp, err := http.Get(s.config.Hindsight.BaseURL + "/v1/default/health")
    // ... 代理响应或返回 {"status": "unreachable"}
}
```

**前端 API**：

```typescript
export const hindsightAPI = {
  getHealth: () => api.get<HindsightHealth>('/hindsight/health'),
};
```

**HindsightPanel 组件**：

```
┌─────────────────────────────────┐
│  🧠 长期记忆 (Hindsight)         │
│                                 │
│  状态: ● 正常 / ○ 不可用 / ◌ 未启用 │
│  地址: localhost:8888            │
└─────────────────────────────────┘
```

- 轮询间隔：30 秒检查一次 health
- 三种状态：`connected`（绿色）、`unreachable`（红色）、`disabled`（灰色）
- hindsight 未启用时显示"未启用"灰色状态，不发请求

---

## 6. 文件改动清单

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `src/scheduler.go` | 修改 | 新增 `HindsightConfig` 结构体，`Config` 新增 `Hindsight` 字段 |
| `src/cli_adapter.go` | 修改 | `GenerateMCPConfig` 扩展签名，生成 hindsight MCP 条目 |
| `src/agent_worker.go` | 修改 | `executeTask` 传递 agent name 和 hindsight 配置 |
| `src/api_server.go` | 修改 | 新增 `/api/hindsight/health` 代理端点 |
| `config.yaml` | 修改 | 新增 `hindsight` 配置段 |
| `frontend/src/components/StatusBar/HindsightPanel.tsx` | 新增 | Hindsight 状态面板组件 |
| `frontend/src/components/StatusBar/index.tsx` | 修改 | 引入 HindsightPanel |
| `frontend/src/services/api.ts` | 修改 | 新增 `hindsightAPI` |
| `frontend/src/types/index.ts` | 修改 | 新增 `HindsightHealth` 类型 |

**不再需要的文件**：
| 文件 | 说明 |
|------|------|
| ~~`src/hindsight_client.go`~~ | Go SDK 封装不再需要，改为 MCP 方式接入 |
| ~~`src/session_chain.go` 修改~~ | 不再需要在 seal 时硬编码 retain |
| ~~`src/session_chain_context.go` 修改~~ | 不再需要在 prompt 构建时硬编码 recall |
| ~~`go.mod` 修改~~ | 不再需要 hindsight Go SDK 依赖 |

---

## 7. 与旧方案的对比

| 维度 | 旧方案（Go SDK 直接集成） | 新方案（MCP Server） |
|------|--------------------------|---------------------|
| 接入方式 | Go 后端调用 hindsight Go SDK | 模型通过 MCP 协议调用 hindsight |
| retain 时机 | 硬编码在 Session seal 时 | 模型自主决定 |
| recall 时机 | 硬编码在 prompt 构建时 | 模型自主决定 |
| 灵活性 | 低，只能在固定节点触发 | 高，模型理解语义，按需调用 |
| 后端改动量 | 大（6 个文件） | 小（3 个文件 + config） |
| Go SDK 依赖 | 需要 | 不需要 |
| 部署依赖 | hindsight 作为库嵌入 | hindsight 作为独立服务 |
| 多 Agent 扩展 | 每个 Agent 需要代码适配 | 只需配置 MCP endpoint |

---

## 8. 容错设计

1. **Hindsight 服务不可用**：`enabled: false` 或服务未启动时，`GenerateMCPConfig` 不生成 hindsight 条目，模型只有 session-chain MCP，不影响核心功能
2. **MCP 调用超时**：由 CLI 工具（claude/gemini）自身的 MCP 超时机制处理，模型会收到错误并自行决定是否重试
3. **Bank 不存在**：hindsight single-bank 模式下首次调用会自动创建 Bank
4. **认证失败**：hindsight 返回 401，模型收到错误提示

---

## 9. 验证方式

1. 启动 hindsight 服务，确认 `http://localhost:8888/mcp/cat-花花/` 可访问
2. 发送消息给花花，检查生成的 MCP 配置文件中包含 hindsight 条目
3. 在对话中告诉花花一些信息（如"我喜欢用 Vim"），观察模型是否自主调用 `retain`
4. 新开对话，问花花"我喜欢用什么编辑器"，观察模型是否自主调用 `recall`
5. 关闭 hindsight 服务，确认猫猫仍能正常工作（只是没有长期记忆）

---

## 10. 实现顺序

1. Phase 1：`config.yaml` + `HindsightConfig` 配置解析
2. Phase 2：`GenerateMCPConfig` 改造（核心改动）
3. Phase 3：`AgentWorker` 传递配置
4. Phase 4：CLI 适配确认（codex MCP 支持调研）
5. Phase 5：前端 Hindsight 状态面板（后端代理 + 前端组件）

---

## 11. 后续演进

- **Bank Federation**：多 Agent 共享记忆池（独立 SPEC `AR-20260301-multi-agent-shared-memory.md`）
- **System Prompt 注入**：在猫猫的 system prompt 中添加 hindsight 使用指引，引导模型更好地使用记忆工具
- **Mental Models**：为每只猫猫创建 mental models（如"用户偏好"、"项目架构"），实现更高层次的记忆抽象
- **Directives**：通过 directives 定制每只猫猫的记忆策略（如花花侧重技术决策，小乔侧重设计偏好）
