# IPD 协作模式使用指南

## 概述

IPD（Integrated Product Development）模式是一个企业级研发流程协作模式，支持 TR3 和 TR5 两个关键评审点。

## 流程图

```
TR3 阶段：
  编码 (coding)
    ↓
  代码审查 (review)
    ↓
  技术辩论 (debate)
    ↓
  最终批准 (final_approval)
    ↓
  [进入 TR5]

TR5 阶段：
  编码 (coding)
    ↓
  代码审查 (review)
    ↓
  技术辩论 (debate)
    ↓
  最终批准 (final_approval)
    ↓
  [完成]
```

## 角色分工

- **花花（架构师）**：负责技术辩论和最终批准决策
- **薇薇（审查专家）**：负责代码审查
- **用户（开发者）**：提交代码、响应审查意见

## API 使用

### 1. 切换到 IPD 模式

```bash
curl -X PUT http://localhost:8080/api/sessions/{sessionId}/mode \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "ipd_dev"
  }'
```

### 2. 提交代码

```bash
curl -X POST http://localhost:8080/api/sessions/{sessionId}/mode/ipd/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "submit_code",
    "params": {
      "pr_url": "https://github.com/user/repo/pull/123"
    }
  }'
```

### 3. 批准代码

```bash
curl -X POST http://localhost:8080/api/sessions/{sessionId}/mode/ipd/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "approve"
  }'
```

### 4. 拒绝代码

```bash
curl -X POST http://localhost:8080/api/sessions/{sessionId}/mode/ipd/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "reject",
    "params": {
      "reason": "代码质量不符合要求，请修复以下问题：..."
    }
  }'
```

### 5. 开始技术辩论

```bash
curl -X POST http://localhost:8080/api/sessions/{sessionId}/mode/ipd/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "start_debate"
  }'
```

### 6. 最终批准

```bash
curl -X POST http://localhost:8080/api/sessions/{sessionId}/mode/ipd/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "final_approve"
  }'
```

### 7. 推进到下一阶段

```bash
curl -X POST http://localhost:8080/api/sessions/{sessionId}/mode/ipd/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "advance_phase"
  }'
```

## 前端集成

### TypeScript 类型定义

```typescript
// IPD 阶段
export type IPDPhase = 'tr3' | 'tr5';

// IPD 子阶段
export type IPDSubPhase = 'coding' | 'review' | 'debate' | 'final_approval';

// IPD 状态
export interface IPDState {
  phase: IPDPhase;
  subPhase: IPDSubPhase;
  participants: string[];
  approvals: Record<string, boolean>;
  prInfo?: {
    url: string;
    author: string;
    comments: string[];
  };
  reviewRound: number;
  startTime: string;
}

// IPD 动作请求
export interface IPDActionRequest {
  action: 'submit_code' | 'approve' | 'reject' | 'start_debate' | 'final_approve' | 'advance_phase';
  params?: Record<string, any>;
}
```

### API 调用示例

```typescript
import axios from 'axios';

// 提交代码
async function submitCode(sessionId: string, prUrl: string) {
  const response = await axios.post(
    `/api/sessions/${sessionId}/mode/ipd/action`,
    {
      action: 'submit_code',
      params: { pr_url: prUrl }
    }
  );
  return response.data;
}

// 批准代码
async function approveCode(sessionId: string) {
  const response = await axios.post(
    `/api/sessions/${sessionId}/mode/ipd/action`,
    { action: 'approve' }
  );
  return response.data;
}

// 拒绝代码
async function rejectCode(sessionId: string, reason: string) {
  const response = await axios.post(
    `/api/sessions/${sessionId}/mode/ipd/action`,
    {
      action: 'reject',
      params: { reason }
    }
  );
  return response.data;
}
```

## 使用场景

### 场景 1：正常流程

1. 用户提交代码（附带 PR 链接）
2. 薇薇自动开始审查
3. 薇薇批准后，花花组织技术辩论
4. 花花做出最终批准决策
5. 系统推进到 TR5 阶段
6. 重复步骤 1-4
7. 完成 IPD 流程

### 场景 2：审查不通过

1. 用户提交代码
2. 薇薇审查发现问题，拒绝
3. 系统回退到编码阶段
4. 用户修改代码后重新提交
5. 继续正常流程

### 场景 3：手动控制流程

用户可以通过 API 手动触发各个阶段的转换：

```bash
# 手动开始辩论（跳过审查）
curl -X POST .../mode/ipd/action -d '{"action": "start_debate"}'

# 手动推进阶段
curl -X POST .../mode/ipd/action -d '{"action": "advance_phase"}'
```

## 状态查询

### 获取当前模式状态

```bash
curl http://localhost:8080/api/sessions/{sessionId}/mode
```

响应示例：

```json
{
  "mode": "ipd_dev",
  "description": "IPD 研发流程模式，支持 TR3/TR5 评审",
  "config": {
    "name": "ipd_dev",
    "enabled": true
  },
  "state": {
    "customState": {
      "phase": "tr3",
      "sub_phase": "review",
      "participants": ["花花", "薇薇"],
      "approvals": {
        "薇薇": true
      },
      "pr_info": {
        "url": "https://github.com/user/repo/pull/123",
        "author": "用户",
        "comments": []
      },
      "review_round": 1,
      "start_time": "2024-02-18T10:00:00Z"
    },
    "lastUpdateTime": "2024-02-18T10:05:00Z"
  }
}
```

## 注意事项

1. **阶段限制**：某些动作只能在特定阶段执行
   - `submit_code` 只能在 `coding` 阶段
   - `approve/reject` 只能在 `review` 阶段
   - `start_debate` 只能在 `review` 阶段之后
   - `final_approve` 只能在 `debate` 阶段之后

2. **权限控制**：当前版本未实现严格的权限控制，建议在前端限制用户操作

3. **状态持久化**：状态保存在内存中，重启服务器会丢失

4. **并发控制**：同一会话的多个请求会串行处理

## 扩展开发

### 添加新的子阶段

1. 在 `mode_ipd.go` 中定义新的 `IPDSubPhase` 常量
2. 在状态机中添加转换逻辑
3. 在 `handleIPDAction` 中添加对应的动作处理

### 自定义审批流程

可以通过修改 `allApproved` 函数来实现自定义的审批逻辑：

```go
// 示例：要求至少 2 个审查者批准
func (m *IPDMode) allApproved(state *IPDState) bool {
    return len(state.Approvals) >= 2
}
```

### 集成外部系统

可以在 `handleSubmitCode` 中集成 GitHub API：

```go
// 自动获取 PR 信息
func (m *IPDMode) fetchPRInfo(prURL string) (*PRInfo, error) {
    // 调用 GitHub API
    // ...
}
```

## 故障排查

### 问题：动作执行失败

**原因**：当前阶段不允许该动作

**解决**：检查当前状态，确保在正确的阶段执行动作

### 问题：状态不更新

**原因**：编排器未正确处理消息

**解决**：检查日志，确认 `HandleUserMessage` 是否被调用

### 问题：Agent 未响应

**原因**：调度器未正确发送任务

**解决**：检查 Redis 连接和 Agent 进程状态

## 参考资料

- [IPD 流程介绍](https://en.wikipedia.org/wiki/Integrated_product_development)
- [编排层架构文档](./ORCHESTRATION.md)
- [API 文档](./API.md)
