package test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// TC-6: 端到端集成测试
// ============================================================

func TestIntegration_FullMessageFlow(t *testing.T) {
	// TC-6.1: 用户消息 → Event 写入 → Agent 调用 → Invocation 记录 → Agent 回复 Event
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 1. 用户发送消息
	err = mgr.AppendEvent(threadID, makeUserEvent("帮我写一个HTTP服务器"))
	require.NoError(t, err)

	// 2. 记录 Agent 调用
	inv := makeInvocation("花花", "system prompt + 帮我写一个HTTP服务器", "好的，代码如下...")
	inv.ThreadID = threadID
	inv.SessionID = "S001"
	inv.StartEventNo = 1
	inv.EndEventNo = 1
	err = mgr.RecordInvocation(threadID, inv)
	require.NoError(t, err)

	// 3. Agent 回复
	err = mgr.AppendEvent(threadID, SessionEvent{
		Type:         EventCat,
		Sender:       "花花",
		Content:      "好的，代码如下...",
		InvocationID: inv.ID,
	})
	require.NoError(t, err)

	// 验证完整流程
	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, 2, session.EventCount)

	events, _, err := mgr.GetEvents(threadID, session.ID, 0, 10)
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, EventUser, events[0].Type)
	assert.Equal(t, EventCat, events[1].Type)
	assert.Equal(t, inv.ID, events[1].InvocationID)

	// 验证 Invocation 可查
	loadedInv, err := mgr.GetInvocation(threadID, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, "花花", loadedInv.AgentName)
}

func TestIntegration_OrchestratedMode(t *testing.T) {
	// TC-6.2: 策略 A 下 prompt 包含活跃 Session 全部 Event，不传 --resume
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 模拟多轮对话
	err = mgr.AppendEvent(threadID, makeUserEvent("第一条消息"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "第一条回复"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("第二条消息"))
	require.NoError(t, err)

	// orchestrated 模式：读取活跃 Session 全部 Event
	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)

	events, _, err := mgr.GetEvents(threadID, session.ID, 0, 1000)
	require.NoError(t, err)
	assert.Len(t, events, 3, "orchestrated 模式应该拿到全部 Event")

	// 不应该有 AI session ID（每次新建）
	cursor := mgr.GetCursor("花花", threadID)
	// orchestrated 模式不维护 cursor，应该是 nil
	assert.Nil(t, cursor, "orchestrated 模式不应该有 Cursor")
}

func TestIntegration_CLIManagedMode(t *testing.T) {
	// TC-6.3: 策略 B 下 prompt 只包含增量 Event，传 --resume + AI session ID
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 模拟第一轮调用
	err = mgr.AppendEvent(threadID, makeUserEvent("第一条消息"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "第一条回复"))
	require.NoError(t, err)

	// 更新 Cursor（模拟第一轮调用结束）
	err = mgr.UpdateCursor("花花", threadID, "S001", 2, "ai-session-001")
	require.NoError(t, err)

	// 模拟第二轮：新增消息
	err = mgr.AppendEvent(threadID, makeUserEvent("第二条消息"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("薇薇", "薇薇的回复"))
	require.NoError(t, err)

	// cli_managed 模式：只读取增量 Event
	cursor := mgr.GetCursor("花花", threadID)
	require.NotNil(t, cursor)
	assert.Equal(t, "ai-session-001", cursor.AISessionID)

	events, err := mgr.GetEventsAfter(threadID, cursor.LastSessionID, cursor.LastEventNo)
	require.NoError(t, err)
	assert.Len(t, events, 2, "应该只有增量的 2 条 Event")
	assert.Equal(t, "第二条消息", events[0].Content)
	assert.Equal(t, "薇薇的回复", events[1].Content)
}

func TestIntegration_SealEndToEnd(t *testing.T) {
	// TC-6.4: 写入足够多 Event → 触发 Seal → 新 Session 创建 → 压缩完成 → Summary 可读
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-004"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	config := &SessionChainConfig{
		MaxTokens:           100,
		SealThreshold:       0.8,
		MaxEventsPerSession: 5,
	}

	// 写入超过阈值的 Event
	for i := 0; i < 6; i++ {
		err := mgr.AppendEvent(threadID, makeUserEvent(
			fmt.Sprintf("消息 #%d 用于触发 Seal", i+1)))
		require.NoError(t, err)
	}

	err = mgr.CheckAndSeal(threadID, config)
	require.NoError(t, err)

	// 验证 Seal 发生
	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sessions), 2)

	// 新 Session 应该是 active
	activeSession, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, SessionActive, activeSession.Status)
	assert.NotEqual(t, "S001", activeSession.ID, "活跃 Session 应该是新的")
}

func TestIntegration_CLIManagedWithSeal(t *testing.T) {
	// TC-6.5: Cursor 指向已 seal 的 Session → prompt 包含 Summary + 增量 Event
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-005"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 第一轮：写入 Event 并设置 Cursor
	appendNEvents(t, mgr, threadID, 5)
	err = mgr.UpdateCursor("花花", threadID, "S001", 5, "ai-session-old")
	require.NoError(t, err)

	// Seal 第一个 Session
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 在新 Session 中写入更多 Event
	err = mgr.AppendEvent(threadID, makeUserEvent("Seal 后的新消息"))
	require.NoError(t, err)

	// 读取 Cursor
	cursor := mgr.GetCursor("花花", threadID)
	require.NotNil(t, cursor)
	assert.Equal(t, "S001", cursor.LastSessionID)

	// 检查 Cursor 指向的 Session 是否已 seal
	cursorSession, err := mgr.GetSession(threadID, cursor.LastSessionID)
	require.NoError(t, err)
	assert.NotEqual(t, SessionActive, cursorSession.Status,
		"Cursor 指向的 Session 应该已被 seal")

	// 应该能读取后续 Session 的 Event
	events, err := mgr.GetEventsAfter(threadID, cursor.LastSessionID, cursor.LastEventNo)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(events), 1, "应该能读到 Seal 后的新 Event")
}

func TestIntegration_MCPQueryEndToEnd(t *testing.T) {
	// TC-6.6: 写入数据 → 通过 MCP 工具查询 → 返回正确结果
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-006"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 写入数据
	err = mgr.AppendEvent(threadID, makeUserEvent("实现 WebSocket 功能"))
	require.NoError(t, err)

	inv := makeInvocation("花花", "实现 WebSocket", "WebSocket 代码完成")
	inv.ThreadID = threadID
	inv.SessionID = "S001"
	err = mgr.RecordInvocation(threadID, inv)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "WebSocket 代码完成"))
	require.NoError(t, err)

	// MCP: list_session_chain
	summaries, err := mgr.MCPListSessionChain(threadID, "花花")
	require.NoError(t, err)
	assert.Len(t, summaries, 1)
	assert.Equal(t, "S001", summaries[0].ID)

	// MCP: read_session_events
	events, _, err := mgr.MCPReadSessionEvents("S001", 0, 10, "raw")
	require.NoError(t, err)
	assert.Len(t, events, 2)

	// MCP: read_invocation_detail
	loadedInv, err := mgr.MCPReadInvocationDetail(inv.ID)
	require.NoError(t, err)
	assert.Equal(t, "花花", loadedInv.AgentName)

	// MCP: session_search
	results, err := mgr.MCPSessionSearch(threadID, "WebSocket", 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestIntegration_BackwardCompatibility(t *testing.T) {
	// TC-6.7: context_mode 为空时走旧逻辑，行为不变
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-007"

	// 即使没有初始化 Chain，基本操作不应 panic
	// 旧逻辑走 getSessionHistory + session_mapping
	// 新模块应该优雅降级

	// GetOrCreateChain 应该正常工作
	meta, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)
	assert.NotNil(t, meta)

	// 没有配置 context_mode 时，Cursor 为 nil 是正常的
	cursor := mgr.GetCursor("花花", threadID)
	assert.Nil(t, cursor, "未配置时 Cursor 应该为 nil")
}

func TestIntegration_ConcurrentMultiAgent(t *testing.T) {
	// TC-6.8: 3 个 Agent 同时写入同一 Thread，Event 编号无冲突，数据完整
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-008"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	agents := []string{"花花", "薇薇", "小乔"}
	const eventsPerAgent = 10

	var wg sync.WaitGroup
	wg.Add(len(agents))

	for _, agent := range agents {
		go func(agentName string) {
			defer wg.Done()
			for i := 0; i < eventsPerAgent; i++ {
				err := mgr.AppendEvent(threadID, makeCatEvent(agentName,
					fmt.Sprintf("%s 的消息 #%d", agentName, i+1)))
				assert.NoError(t, err)
			}
		}(agent)
	}
	wg.Wait()

	// 验证总数
	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	totalExpected := len(agents) * eventsPerAgent
	assert.Equal(t, totalExpected, session.EventCount,
		"应该有 %d 条 Event", totalExpected)

	// 验证 eventNo 无冲突
	events, _, err := mgr.GetEvents(threadID, session.ID, 0, totalExpected+10)
	require.NoError(t, err)
	assert.Len(t, events, totalExpected)

	seen := make(map[int]bool)
	for _, e := range events {
		assert.False(t, seen[e.EventNo], "eventNo %d 重复", e.EventNo)
		seen[e.EventNo] = true
	}
	for i := 1; i <= totalExpected; i++ {
		assert.True(t, seen[i], "缺少 eventNo %d", i)
	}
}

func TestIntegration_ProcessRestart(t *testing.T) {
	// TC-6.9: 写入数据 → 重建 SessionChainManager → Chain 状态完整恢复
	_, tmpDir, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-009"

	// 第一个实例：写入数据
	mgr1, err := NewSessionChainManager(tmpDir)
	require.NoError(t, err)

	_, err = mgr1.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr1, threadID, 5)

	err = mgr1.UpdateCursor("花花", threadID, "S001", 5, "ai-session-xyz")
	require.NoError(t, err)

	// 第二个实例：模拟重启
	mgr2, err := NewSessionChainManager(tmpDir)
	require.NoError(t, err)

	// 验证 Meta 恢复
	meta, err := mgr2.ReadMeta(threadID)
	require.NoError(t, err)
	assert.Equal(t, threadID, meta.ThreadID)
	assert.Equal(t, "S001", meta.ActiveSessionID)
	assert.Equal(t, 5, meta.TotalEvents)

	// 验证 Session 恢复
	session, err := mgr2.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, "S001", session.ID)
	assert.Equal(t, 5, session.EventCount)

	// 验证 Event 恢复
	events, _, err := mgr2.GetEvents(threadID, "S001", 0, 10)
	require.NoError(t, err)
	assert.Len(t, events, 5)

	// 验证 Cursor 恢复
	cursor := mgr2.GetCursor("花花", threadID)
	require.NotNil(t, cursor)
	assert.Equal(t, 5, cursor.LastEventNo)
	assert.Equal(t, "ai-session-xyz", cursor.AISessionID)
}

func TestIntegration_ConsecutiveMultiSeal(t *testing.T) {
	// TC-6.10: 模拟长对话触发 3 次 Seal，最终 Chain 结构正确，所有 Summary 可读
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-010"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 3 轮 Seal
	for round := 0; round < 3; round++ {
		for i := 0; i < 5; i++ {
			err := mgr.AppendEvent(threadID, makeUserEvent(
				fmt.Sprintf("第 %d 轮消息 #%d", round+1, i+1)))
			require.NoError(t, err)
		}
		err = mgr.SealActiveSession(threadID)
		require.NoError(t, err, "第 %d 次 Seal 失败", round+1)
	}

	// 在最新 Session 中写入一些消息
	err = mgr.AppendEvent(threadID, makeUserEvent("最新的消息"))
	require.NoError(t, err)

	// 验证 Chain 结构
	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	assert.Len(t, sessions, 4, "应该有 4 个 Session（3 sealed + 1 active）")

	// 前 3 个是 compressing
	for i := 0; i < 3; i++ {
		assert.Equal(t, SessionCompressing, sessions[i].Status)
		assert.Equal(t, i+1, sessions[i].SeqNo)
		assert.Equal(t, 5, sessions[i].EventCount)
	}

	// 最后一个是 active
	assert.Equal(t, SessionActive, sessions[3].Status)
	assert.Equal(t, 4, sessions[3].SeqNo)
	assert.Equal(t, 1, sessions[3].EventCount)

	// 验证全局 Event 编号连续
	meta, err := mgr.ReadMeta(threadID)
	require.NoError(t, err)
	assert.Equal(t, 16, meta.TotalEvents, "3×5 + 1 = 16 个 Event")
}

func TestIntegration_CompressThenMCPQuery(t *testing.T) {
	// TC-6.11: 超长上下文 → Seal → 压缩生成 Summary → 新 Session 继续对话
	//          → Agent 通过 MCP 工具读取旧 Session 的 Summary 和 Events
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-011"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 1. 模拟超长对话（S001 写入大量 Event）
	for i := 0; i < 10; i++ {
		err = mgr.AppendEvent(threadID, makeUserEvent(
			fmt.Sprintf("用户第 %d 条消息：讨论 WebSocket 实现细节", i+1)))
		require.NoError(t, err)
		err = mgr.AppendEvent(threadID, makeCatEvent("花花",
			fmt.Sprintf("花花第 %d 条回复：WebSocket 代码片段 %d", i+1, i+1)))
		require.NoError(t, err)
	}

	// 2. Seal S001
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 验证 S001 状态为 compressing
	s001, err := mgr.GetSession(threadID, "S001")
	require.NoError(t, err)
	assert.Equal(t, SessionCompressing, s001.Status)

	// 3. 压缩 S001 → 生成 Summary，状态变为 sealed
	compressorConfig := &MemoryCompressorConfig{
		Model:            "test-model",
		MaxSummaryTokens: 200,
	}
	err = mgr.CompressSession(threadID, "S001", compressorConfig)
	require.NoError(t, err)

	s001, err = mgr.GetSession(threadID, "S001")
	require.NoError(t, err)
	assert.Equal(t, SessionSealed, s001.Status)
	assert.NotEmpty(t, s001.Summary, "压缩后 S001 应该有 Summary")

	// 4. 在新 Session（S002）中继续对话
	err = mgr.AppendEvent(threadID, makeUserEvent("继续上次的 WebSocket 话题，帮我加上心跳检测"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "好的，基于之前的实现，加上心跳..."))
	require.NoError(t, err)

	// 5. Agent 通过 MCP list_session_chain 查看 Chain 全貌
	summaries, err := mgr.MCPListSessionChain(threadID, "花花")
	require.NoError(t, err)
	require.Len(t, summaries, 2, "应该有 2 个 Session")

	// S001: sealed，有 summary
	assert.Equal(t, "S001", summaries[0].ID)
	assert.Equal(t, SessionSealed, summaries[0].Status)
	assert.NotEmpty(t, summaries[0].Summary, "MCP 返回的 S001 应该有 Summary")
	assert.Equal(t, 20, summaries[0].EventCount)

	// S002: active，无 summary
	assert.Equal(t, "S002", summaries[1].ID)
	assert.Equal(t, SessionActive, summaries[1].Status)
	assert.Empty(t, summaries[1].Summary)
	assert.Equal(t, 2, summaries[1].EventCount)

	// 6. Agent 通过 MCP read_session_events 读取旧 Session 的 Events
	oldEvents, nextCursor, err := mgr.MCPReadSessionEvents("S001", 0, 50, "chat")
	require.NoError(t, err)
	assert.Len(t, oldEvents, 20, "S001 应该有 20 条 Event")
	assert.Equal(t, -1, nextCursor, "20 条一页读完，无下一页")

	// 验证旧 Session 的 Event 内容完整
	assert.Equal(t, EventUser, oldEvents[0].Type)
	assert.Contains(t, oldEvents[0].Content, "WebSocket")
	assert.Equal(t, EventCat, oldEvents[1].Type)
	assert.Equal(t, "花花", oldEvents[1].Sender)

	// 7. Agent 通过 MCP read_session_events 读取当前 Session
	newEvents, _, err := mgr.MCPReadSessionEvents("S002", 0, 50, "chat")
	require.NoError(t, err)
	assert.Len(t, newEvents, 2)
	assert.Contains(t, newEvents[0].Content, "心跳检测")

	// 8. Agent 通过 MCP session_search 搜索跨 Session 的关键词
	results, err := mgr.MCPSessionSearch(threadID, "WebSocket", 50)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2, "WebSocket 应该在多个 Event 中出现")

	// 验证搜索结果跨越了两个 Session
	sessionIDs := make(map[string]bool)
	for _, r := range results {
		sessionIDs[r.SessionID] = true
	}
	assert.True(t, sessionIDs["S001"], "搜索结果应该包含 S001 的内容")
	assert.True(t, sessionIDs["S002"], "搜索结果应该包含 S002 的内容")
}

func TestIntegration_MultiSealCompressThenContinue(t *testing.T) {
	// TC-6.12: 多轮 Seal+压缩 → 后续压缩 prompt 包含之前的 Summary → MCP 可查所有 Summary
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-integ-012"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	compressorConfig := &MemoryCompressorConfig{
		Model:            "test-model",
		MaxSummaryTokens: 200,
	}

	// 第一轮：写入 → Seal → 压缩
	for i := 0; i < 5; i++ {
		mgr.AppendEvent(threadID, makeUserEvent(fmt.Sprintf("第一轮消息 #%d", i+1)))
	}
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)
	err = mgr.CompressSession(threadID, "S001", compressorConfig)
	require.NoError(t, err)

	// 第二轮：写入 → Seal → 压缩（应该包含 S001 的 Summary）
	for i := 0; i < 5; i++ {
		mgr.AppendEvent(threadID, makeUserEvent(fmt.Sprintf("第二轮消息 #%d", i+1)))
	}
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)
	err = mgr.CompressSession(threadID, "S002", compressorConfig)
	require.NoError(t, err)

	// 第三轮：继续在 S003 中对话
	err = mgr.AppendEvent(threadID, makeUserEvent("第三轮的新消息"))
	require.NoError(t, err)

	// 验证：MCP list 返回 3 个 Session，前 2 个有 Summary
	summaries, err := mgr.MCPListSessionChain(threadID, "花花")
	require.NoError(t, err)
	require.Len(t, summaries, 3)

	assert.Equal(t, SessionSealed, summaries[0].Status)
	assert.NotEmpty(t, summaries[0].Summary, "S001 应该有 Summary")

	assert.Equal(t, SessionSealed, summaries[1].Status)
	assert.NotEmpty(t, summaries[1].Summary, "S002 应该有 Summary")

	assert.Equal(t, SessionActive, summaries[2].Status)
	assert.Equal(t, 1, summaries[2].EventCount)

	// 验证：cli_managed 模式下 Cursor 指向已压缩的 Session
	// 模拟 Agent 上次读到 S001 的最后一条
	err = mgr.UpdateCursor("花花", threadID, "S001", 5, "ai-session-old")
	require.NoError(t, err)

	cursor := mgr.GetCursor("花花", threadID)
	require.NotNil(t, cursor)

	// Agent 发现 Cursor 指向的 Session 已 sealed，通过 MCP 获取 Summary
	cursorSession, err := mgr.GetSession(threadID, cursor.LastSessionID)
	require.NoError(t, err)
	assert.Equal(t, SessionSealed, cursorSession.Status)
	assert.NotEmpty(t, cursorSession.Summary, "Agent 应该能读到压缩摘要")

	// Agent 通过 GetEventsAfter 获取增量 Event
	incrementalEvents, err := mgr.GetEventsAfter(threadID, cursor.LastSessionID, cursor.LastEventNo)
	require.NoError(t, err)
	assert.Equal(t, 6, len(incrementalEvents), "S002 的 5 条 + S003 的 1 条 = 6 条增量")
}
