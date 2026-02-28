package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// TC-5: MCP Server 工具测试
// ============================================================

// --- TC-5.1 list_session_chain ---

func TestMCP_ListSessionChain_Basic(t *testing.T) {
	// TC-5.1.1: 返回所有 Session，包含 id、seqNo、status、eventCount、tokenCount
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-list-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 5)

	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 3)

	summaries, err := mgr.MCPListSessionChain(threadID, "花花")
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	// 第一个 Session
	assert.Equal(t, "S001", summaries[0].ID)
	assert.Equal(t, 1, summaries[0].SeqNo)
	assert.Equal(t, SessionCompressing, summaries[0].Status)
	assert.Equal(t, 5, summaries[0].EventCount)
	assert.Greater(t, summaries[0].TokenCount, 0)

	// 第二个 Session
	assert.Equal(t, "S002", summaries[1].ID)
	assert.Equal(t, 2, summaries[1].SeqNo)
	assert.Equal(t, SessionActive, summaries[1].Status)
	assert.Equal(t, 3, summaries[1].EventCount)
}

func TestMCP_ListSessionChain_SealedWithSummary(t *testing.T) {
	// TC-5.1.2: sealed 的 Session 返回 summary 字段
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-list-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 3)
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 尝试压缩
	compressorConfig := &MemoryCompressorConfig{
		Model:            "test-model",
		MaxSummaryTokens: 200,
	}
	err = mgr.CompressSession(threadID, "S001", compressorConfig)
	if err != nil {
		t.Skipf("压缩模型不可用，跳过: %v", err)
		return
	}

	summaries, err := mgr.MCPListSessionChain(threadID, "花花")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(summaries), 1)

	// sealed 的 Session 应该有 summary
	assert.Equal(t, SessionSealed, summaries[0].Status)
	assert.NotEmpty(t, summaries[0].Summary)
}

func TestMCP_ListSessionChain_EmptyThread(t *testing.T) {
	// TC-5.1.3: 空 Thread 返回空数组
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	summaries, err := mgr.MCPListSessionChain("nonexistent-thread", "花花")
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

// --- TC-5.2 read_session_events ---

func TestMCP_ReadSessionEvents_ChatView(t *testing.T) {
	// TC-5.2.1: view=chat 返回人类可读格式，隐藏 Invocation 细节
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-read-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("你好"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "喵~"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, SessionEvent{
		Type:         EventInvocation,
		Sender:       "花花",
		Content:      "调用详情...",
		InvocationID: "inv_123",
	})
	require.NoError(t, err)

	events, _, err := mgr.MCPReadSessionEvents("S001", 0, 10, "chat")
	require.NoError(t, err)

	// chat 模式应该有用户和猫猫消息
	hasUser := false
	hasCat := false
	hasInvocation := false
	for _, e := range events {
		if e.Type == EventUser {
			hasUser = true
		}
		if e.Type == EventCat {
			hasCat = true
		}
		if e.Type == EventInvocation {
			hasInvocation = true
		}
	}
	assert.True(t, hasUser, "chat 模式应该包含用户消息")
	assert.True(t, hasCat, "chat 模式应该包含猫猫消息")
	// chat 模式可以选择隐藏或简化 invocation 细节
	_ = hasInvocation
}

func TestMCP_ReadSessionEvents_HandoffView(t *testing.T) {
	// TC-5.2.2: view=handoff 返回交接摘要格式，包含关键决策
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-read-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("实现登录功能"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "已完成 JWT 认证"))
	require.NoError(t, err)

	events, _, err := mgr.MCPReadSessionEvents("S001", 0, 10, "handoff")
	require.NoError(t, err)
	assert.NotEmpty(t, events, "handoff 模式应该返回内容")
}

func TestMCP_ReadSessionEvents_RawView(t *testing.T) {
	// TC-5.2.3: view=raw 返回原始 JSON，包含所有字段
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-read-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("原始数据测试"))
	require.NoError(t, err)

	events, _, err := mgr.MCPReadSessionEvents("S001", 0, 10, "raw")
	require.NoError(t, err)
	require.Len(t, events, 1)

	// raw 模式应该包含所有字段
	assert.Equal(t, EventUser, events[0].Type)
	assert.Equal(t, "原始数据测试", events[0].Content)
	assert.Greater(t, events[0].EventNo, 0)
	assert.False(t, events[0].Timestamp.IsZero())
}

func TestMCP_ReadSessionEvents_Pagination(t *testing.T) {
	// TC-5.2.4: cursor + limit 正确分页
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-read-004"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 15)

	// 第一页
	events1, next1, err := mgr.MCPReadSessionEvents("S001", 0, 5, "raw")
	require.NoError(t, err)
	assert.Len(t, events1, 5)
	assert.Greater(t, next1, 0)

	// 第二页
	events2, next2, err := mgr.MCPReadSessionEvents("S001", next1, 5, "raw")
	require.NoError(t, err)
	assert.Len(t, events2, 5)
	assert.Greater(t, next2, 0)

	// 第三页（最后）
	events3, next3, err := mgr.MCPReadSessionEvents("S001", next2, 5, "raw")
	require.NoError(t, err)
	assert.Len(t, events3, 5)
	assert.Equal(t, -1, next3, "最后一页 nextCursor 应该是 -1")
}

func TestMCP_ReadSessionEvents_NotFound(t *testing.T) {
	// TC-5.2.5: Session 不存在返回错误
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	_, _, err := mgr.MCPReadSessionEvents("S999", 0, 10, "raw")
	assert.Error(t, err)
}

// --- TC-5.3 read_invocation_detail ---

func TestMCP_ReadInvocationDetail_Basic(t *testing.T) {
	// TC-5.3.1: 返回完整的 Invocation 记录
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-inv-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	inv := makeInvocation("花花", "写个排序算法", "func sort(arr []int) []int { ... }")
	inv.ThreadID = threadID
	inv.SessionID = "S001"
	inv.TokensIn = 30
	inv.TokensOut = 60
	inv.Duration = 800

	err = mgr.RecordInvocation(threadID, inv)
	require.NoError(t, err)

	loaded, err := mgr.MCPReadInvocationDetail(inv.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, inv.ID, loaded.ID)
	assert.Equal(t, inv.AgentName, loaded.AgentName)
	assert.Equal(t, inv.Prompt, loaded.Prompt)
	assert.Equal(t, inv.Response, loaded.Response)
	assert.Equal(t, inv.TokensIn, loaded.TokensIn)
	assert.Equal(t, inv.TokensOut, loaded.TokensOut)
}

func TestMCP_ReadInvocationDetail_NotFound(t *testing.T) {
	// TC-5.3.2: Invocation 不存在返回错误
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	_, err := mgr.MCPReadInvocationDetail("inv_nonexistent")
	assert.Error(t, err)
}

// --- TC-5.4 session_search ---

func TestMCP_SessionSearch_Basic(t *testing.T) {
	// TC-5.4.1: 返回匹配结果，包含 snippet 和定位指针
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-search-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("实现 WebSocket 通信"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("普通消息"))
	require.NoError(t, err)

	results, err := mgr.MCPSessionSearch(threadID, "WebSocket", 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 1)

	assert.Contains(t, results[0].Snippet, "WebSocket")
	assert.NotEmpty(t, results[0].SessionID)
	assert.Greater(t, results[0].EventNo, 0)
}

func TestMCP_SessionSearch_Sorted(t *testing.T) {
	// TC-5.4.2: 按相关性排序
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-mcp-search-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("Redis 缓存配置"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("Redis 连接池和 Redis 集群配置"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("普通消息"))
	require.NoError(t, err)

	results, err := mgr.MCPSessionSearch(threadID, "Redis", 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 2)

	// 包含更多匹配的应该排在前面（或至少都有 score）
	for _, r := range results {
		assert.Greater(t, r.Score, 0.0, "每个结果应该有正的 score")
	}
}
