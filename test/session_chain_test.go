package test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// TC-1: SessionChainManager 核心单元测试
// ============================================================

// --- TC-1.1 Chain 生命周期 ---

func TestGetOrCreateChain_FirstCreate(t *testing.T) {
	// TC-1.1.1: 首次创建返回新 Chain，Meta 字段正确，自动创建 active Session（S001）
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-001"
	meta, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)
	require.NotNil(t, meta)

	assert.Equal(t, threadID, meta.ThreadID)
	assert.Equal(t, "S001", meta.ActiveSessionID)
	assert.Equal(t, 1, meta.SessionCount)
	assert.Equal(t, 0, meta.TotalEvents)
	assert.False(t, meta.CreatedAt.IsZero())
	assert.False(t, meta.UpdatedAt.IsZero())

	// 验证 active session 存在
	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, "S001", session.ID)
	assert.Equal(t, SessionActive, session.Status)
	assert.Equal(t, 1, session.SeqNo)
}

func TestGetOrCreateChain_Idempotent(t *testing.T) {
	// TC-1.1.2: 重复调用返回同一个 Chain 实例，不重复创建
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-002"
	meta1, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	meta2, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	assert.Equal(t, meta1.ThreadID, meta2.ThreadID)
	assert.Equal(t, meta1.ActiveSessionID, meta2.ActiveSessionID)
	assert.Equal(t, 1, meta2.SessionCount, "不应重复创建 Session")
}

func TestGetOrCreateChain_MultiThreadIsolation(t *testing.T) {
	// TC-1.1.3: 不同 threadId 返回不同 Chain，互不影响
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	meta1, err := mgr.GetOrCreateChain("thread-A")
	require.NoError(t, err)

	meta2, err := mgr.GetOrCreateChain("thread-B")
	require.NoError(t, err)

	assert.NotEqual(t, meta1.ThreadID, meta2.ThreadID)
	assert.Equal(t, "thread-A", meta1.ThreadID)
	assert.Equal(t, "thread-B", meta2.ThreadID)
}

// --- TC-1.2 Event 写入 ---

func TestAppendEvent_Basic(t *testing.T) {
	// TC-1.2.1: Event 追加到活跃 Session，eventNo 自增，tokenCount 更新
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-write-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 追加第一个 Event
	err = mgr.AppendEvent(threadID, makeUserEvent("你好"))
	require.NoError(t, err)

	// 追加第二个 Event
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "喵~ 你好呀"))
	require.NoError(t, err)

	// 验证 Session 状态
	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, 2, session.EventCount)
	assert.Greater(t, session.TokenCount, 0, "tokenCount 应该大于 0")

	// 验证 Event 内容
	events, nextCursor, err := mgr.GetEvents(threadID, session.ID, 0, 10)
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, -1, nextCursor, "只有 2 条，不应有下一页")
	assert.Equal(t, 1, events[0].EventNo)
	assert.Equal(t, 2, events[1].EventNo)
	assert.Equal(t, "你好", events[0].Content)
	assert.Equal(t, "喵~ 你好呀", events[1].Content)
}

func TestAppendEvent_MultiType(t *testing.T) {
	// TC-1.2.2: user、cat、system、invocation 四种类型均可写入
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-write-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	types := []struct {
		eventType EventType
		sender    string
		content   string
	}{
		{EventUser, "用户", "用户消息"},
		{EventCat, "花花", "猫猫回复"},
		{EventSystem, "系统", "系统通知"},
		{EventInvocation, "花花", "调用记录"},
	}

	for _, tc := range types {
		err := mgr.AppendEvent(threadID, makeEvent(tc.eventType, tc.sender, tc.content))
		require.NoError(t, err, "写入 %s 类型 Event 失败", tc.eventType)
	}

	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, 4, session.EventCount)
}

func TestAppendEvent_Concurrent(t *testing.T) {
	// TC-1.2.3: 10 个 goroutine 同时写入，eventNo 无重复无跳跃
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-write-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			err := mgr.AppendEvent(threadID, makeUserEvent(
				"并发消息 from goroutine"))
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// 验证 eventNo 连续无重复
	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, goroutines, session.EventCount)

	events, _, err := mgr.GetEvents(threadID, session.ID, 0, goroutines+10)
	require.NoError(t, err)
	assert.Len(t, events, goroutines)

	seen := make(map[int]bool)
	for _, e := range events {
		assert.False(t, seen[e.EventNo], "eventNo %d 重复", e.EventNo)
		seen[e.EventNo] = true
	}
	// 验证连续性：1 到 goroutines
	for i := 1; i <= goroutines; i++ {
		assert.True(t, seen[i], "缺少 eventNo %d", i)
	}
}

func TestRecordInvocation(t *testing.T) {
	// TC-1.2.4: Invocation 写入文件系统，可通过 GetInvocation 读回
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-write-004"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	inv := makeInvocation("花花", "请帮我写代码", "好的，代码如下...")
	inv.ThreadID = threadID
	inv.TokensIn = 100
	inv.TokensOut = 200
	inv.Duration = 1500

	err = mgr.RecordInvocation(threadID, inv)
	require.NoError(t, err)

	// 读回验证
	loaded, err := mgr.GetInvocation(threadID, inv.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, inv.ID, loaded.ID)
	assert.Equal(t, inv.AgentName, loaded.AgentName)
	assert.Equal(t, inv.Prompt, loaded.Prompt)
	assert.Equal(t, inv.Response, loaded.Response)
	assert.Equal(t, inv.TokensIn, loaded.TokensIn)
	assert.Equal(t, inv.TokensOut, loaded.TokensOut)
	assert.Equal(t, inv.Duration, loaded.Duration)
}

// --- TC-1.3 Event 读取 ---

func TestGetEvents_BasicPagination(t *testing.T) {
	// TC-1.3.1: cursor=0, limit=10 返回前 10 条，nextCursor 正确
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-read-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 25)

	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)

	events, nextCursor, err := mgr.GetEvents(threadID, session.ID, 0, 10)
	require.NoError(t, err)
	assert.Len(t, events, 10)
	assert.Greater(t, nextCursor, 0, "应该有下一页")
	assert.Equal(t, 1, events[0].EventNo)
	assert.Equal(t, 10, events[9].EventNo)
}

func TestGetEvents_FullPagination(t *testing.T) {
	// TC-1.3.2: 连续翻页能读取所有 Event，最后一页 nextCursor=-1
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-read-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	total := 23
	appendNEvents(t, mgr, threadID, total)

	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)

	var allEvents []SessionEvent
	cursor := 0
	for {
		events, next, err := mgr.GetEvents(threadID, session.ID, cursor, 10)
		require.NoError(t, err)
		allEvents = append(allEvents, events...)
		if next == -1 {
			break
		}
		cursor = next
	}

	assert.Len(t, allEvents, total)
}

func TestGetEvents_EmptySession(t *testing.T) {
	// TC-1.3.3: 空 Session 返回空数组，nextCursor=-1
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-read-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)

	events, nextCursor, err := mgr.GetEvents(threadID, session.ID, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Equal(t, -1, nextCursor)
}

func TestGetEventsAfter_Incremental(t *testing.T) {
	// TC-1.3.4: 只返回指定 eventNo 之后的 Event
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-read-004"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 10)

	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)

	// 读取 eventNo > 5 的 Event
	events, err := mgr.GetEventsAfter(threadID, session.ID, 5)
	require.NoError(t, err)
	assert.Len(t, events, 5)
	assert.Equal(t, 6, events[0].EventNo)
	assert.Equal(t, 10, events[4].EventNo)
}

func TestGetEventsAfter_CrossSession(t *testing.T) {
	// TC-1.3.5: 跨越 sealed Session 边界，正确拼接
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-read-005"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 写入一些 Event
	appendNEvents(t, mgr, threadID, 5)

	// 手动 Seal
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 在新 Session 中写入更多 Event
	appendNEvents(t, mgr, threadID, 5)

	// 从第一个 Session 的 eventNo 3 开始读取，应该跨 Session
	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	events, err := mgr.GetEventsAfter(threadID, sessions[0].ID, 3)
	require.NoError(t, err)
	// 应该包含 Session 1 的 event 4,5 + Session 2 的 event 6,7,8,9,10
	assert.Len(t, events, 7)
	assert.Equal(t, 4, events[0].EventNo)
	assert.Equal(t, 10, events[6].EventNo)
}

// --- TC-1.4 Session 管理 ---

func TestGetActiveSession(t *testing.T) {
	// TC-1.4.1: 返回状态为 active 的 Session
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-session-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, SessionActive, session.Status)
}

func TestGetSession_ByID(t *testing.T) {
	// TC-1.4.2: 返回正确的 Session 记录
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-session-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	session, err := mgr.GetSession(threadID, "S001")
	require.NoError(t, err)
	assert.Equal(t, "S001", session.ID)
	assert.Equal(t, threadID, session.ThreadID)
}

func TestListSessions_Sorted(t *testing.T) {
	// TC-1.4.3: 返回所有 Session，按 seqNo 排序
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-session-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// Seal 两次，创建 3 个 Session
	appendNEvents(t, mgr, threadID, 3)
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 3)
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
	assert.Equal(t, 1, sessions[0].SeqNo)
	assert.Equal(t, 2, sessions[1].SeqNo)
	assert.Equal(t, 3, sessions[2].SeqNo)
}

func TestGetSession_NotFound(t *testing.T) {
	// TC-1.4.4: 返回错误
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-session-004"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	_, err = mgr.GetSession(threadID, "S999")
	assert.Error(t, err)
}

// --- TC-1.5 Cursor 管理 ---

func TestGetCursor_FirstTime(t *testing.T) {
	// TC-1.5.1: 返回 nil（无历史 Cursor）
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	cursor := mgr.GetCursor("花花", "thread-cursor-001")
	assert.Nil(t, cursor)
}

func TestCursor_UpdateAndGet(t *testing.T) {
	// TC-1.5.2: 更新后能正确读回
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-cursor-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.UpdateCursor("花花", threadID, "S001", 10, "ai-session-abc")
	require.NoError(t, err)

	cursor := mgr.GetCursor("花花", threadID)
	require.NotNil(t, cursor)
	assert.Equal(t, "花花", cursor.AgentName)
	assert.Equal(t, threadID, cursor.ThreadID)
	assert.Equal(t, "S001", cursor.LastSessionID)
	assert.Equal(t, 10, cursor.LastEventNo)
	assert.Equal(t, "ai-session-abc", cursor.AISessionID)
}

func TestCursor_AgentIsolation(t *testing.T) {
	// TC-1.5.3: Agent A 和 Agent B 的 Cursor 互不影响
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-cursor-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.UpdateCursor("花花", threadID, "S001", 10, "ai-a")
	require.NoError(t, err)

	err = mgr.UpdateCursor("薇薇", threadID, "S001", 5, "ai-b")
	require.NoError(t, err)

	cursorA := mgr.GetCursor("花花", threadID)
	cursorB := mgr.GetCursor("薇薇", threadID)

	require.NotNil(t, cursorA)
	require.NotNil(t, cursorB)
	assert.Equal(t, 10, cursorA.LastEventNo)
	assert.Equal(t, 5, cursorB.LastEventNo)
	assert.Equal(t, "ai-a", cursorA.AISessionID)
	assert.Equal(t, "ai-b", cursorB.AISessionID)
}

func TestCursor_Persistence(t *testing.T) {
	// TC-1.5.4: 重建 SessionChainManager 后 Cursor 仍可读回
	_, tmpDir, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-cursor-004"

	// 第一个 manager 实例
	mgr1, err := NewSessionChainManager(tmpDir)
	require.NoError(t, err)
	_, err = mgr1.GetOrCreateChain(threadID)
	require.NoError(t, err)
	err = mgr1.UpdateCursor("花花", threadID, "S001", 15, "ai-persist")
	require.NoError(t, err)

	// 第二个 manager 实例（模拟重启）
	mgr2, err := NewSessionChainManager(tmpDir)
	require.NoError(t, err)

	cursor := mgr2.GetCursor("花花", threadID)
	require.NotNil(t, cursor, "重启后 Cursor 应该可读")
	assert.Equal(t, 15, cursor.LastEventNo)
	assert.Equal(t, "ai-persist", cursor.AISessionID)
}

// --- TC-1.6 Token 估算 ---

func TestEstimateTokens_Chinese(t *testing.T) {
	// TC-1.6.1: 纯中文文本
	tokens := EstimateTokens("你好世界")
	assert.Equal(t, 8, tokens, "4 个中文字 × 2 = 8 token")
}

func TestEstimateTokens_English(t *testing.T) {
	// TC-1.6.2: 纯英文文本
	tokens := EstimateTokens("hello world")
	assert.True(t, tokens >= 2 && tokens <= 4,
		"'hello world' 应该约 2-3 token，实际: %d", tokens)
}

func TestEstimateTokens_Mixed(t *testing.T) {
	// TC-1.6.3: 中英混合
	tokens := EstimateTokens("Hello 你好 World 世界")
	assert.Greater(t, tokens, 0, "混合文本 token 应该大于 0")
}

func TestEstimateTokens_Empty(t *testing.T) {
	// TC-1.6.4: 空字符串
	tokens := EstimateTokens("")
	assert.Equal(t, 0, tokens)
}
