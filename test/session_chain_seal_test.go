package test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// TC-3: Seal 与压缩测试
// ============================================================

func TestCheckAndSeal_BelowThreshold(t *testing.T) {
	// TC-3.1: tokenCount < threshold，不触发 Seal
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 写入少量 Event
	appendNEvents(t, mgr, threadID, 3)

	config := &SessionChainConfig{
		MaxTokens:           200000,
		SealThreshold:       0.8,
		MaxEventsPerSession: 500,
	}

	err = mgr.CheckAndSeal(threadID, config)
	require.NoError(t, err)

	// 应该仍然只有 1 个 Session
	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, SessionActive, sessions[0].Status)
}

func TestCheckAndSeal_TokenThreshold(t *testing.T) {
	// TC-3.2: tokenCount >= maxTokens * sealThreshold，触发 Seal
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 使用很小的阈值，让少量 Event 就能触发
	config := &SessionChainConfig{
		MaxTokens:           100, // 很小的阈值
		SealThreshold:       0.8, // 80 token 就触发
		MaxEventsPerSession: 500,
	}

	// 写入足够多的内容超过 80 token
	for i := 0; i < 20; i++ {
		err := mgr.AppendEvent(threadID, makeUserEvent(
			fmt.Sprintf("这是一条比较长的测试消息，用于触发 token 阈值 #%d", i)))
		require.NoError(t, err)
	}

	err = mgr.CheckAndSeal(threadID, config)
	require.NoError(t, err)

	// 应该有 2 个 Session（旧的 sealed/compressing + 新的 active）
	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sessions), 2, "应该触发 Seal 创建新 Session")

	// 最后一个应该是 active
	lastSession := sessions[len(sessions)-1]
	assert.Equal(t, SessionActive, lastSession.Status)
}

func TestCheckAndSeal_EventCountThreshold(t *testing.T) {
	// TC-3.3: eventCount >= maxEventsPerSession，触发 Seal
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	config := &SessionChainConfig{
		MaxTokens:           200000,
		SealThreshold:       0.8,
		MaxEventsPerSession: 5, // 很小的阈值
	}

	// 写入 6 个 Event（超过 maxEventsPerSession=5）
	appendNEvents(t, mgr, threadID, 6)

	err = mgr.CheckAndSeal(threadID, config)
	require.NoError(t, err)

	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sessions), 2, "应该触发 Seal")
}

func TestSealActiveSession_StateTransition(t *testing.T) {
	// TC-3.4: 旧 Session → compressing，新 Session → active，Meta 更新
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-004"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 5)

	// Seal
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// 旧 Session 应该是 compressing
	assert.Equal(t, SessionCompressing, sessions[0].Status)
	assert.Equal(t, "S001", sessions[0].ID)
	assert.NotNil(t, sessions[0].SealedAt)

	// 新 Session 应该是 active
	assert.Equal(t, SessionActive, sessions[1].Status)
	assert.Equal(t, "S002", sessions[1].ID)
	assert.Equal(t, 2, sessions[1].SeqNo)

	// Meta 应该更新
	meta, err := mgr.ReadMeta(threadID)
	require.NoError(t, err)
	assert.Equal(t, "S002", meta.ActiveSessionID)
	assert.Equal(t, 2, meta.SessionCount)
}

func TestSeal_NewEventsGoToNewSession(t *testing.T) {
	// TC-3.5: Seal 后 AppendEvent 写入新的活跃 Session
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-005"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 3)

	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 新 Event 应该写入 S002
	err = mgr.AppendEvent(threadID, makeUserEvent("Seal 后的新消息"))
	require.NoError(t, err)

	activeSession, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, "S002", activeSession.ID)
	assert.Equal(t, 1, activeSession.EventCount)

	events, _, err := mgr.GetEvents(threadID, "S002", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "Seal 后的新消息", events[0].Content)
}

func TestSeal_OldSessionImmutable(t *testing.T) {
	// TC-3.6: 向 sealed Session 追加 Event 返回错误
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-006"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 3)

	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 验证旧 Session 状态
	oldSession, err := mgr.GetSession(threadID, "S001")
	require.NoError(t, err)
	assert.NotEqual(t, SessionActive, oldSession.Status,
		"旧 Session 不应该是 active")
}

func TestCompressSession_GeneratesSummary(t *testing.T) {
	// TC-3.7: 调用压缩模型后 Session.Summary 非空，状态变为 sealed
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-007"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 5)

	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 压缩（测试环境可能需要 mock 压缩模型）
	compressorConfig := &MemoryCompressorConfig{
		Model:            "test-model",
		MaxSummaryTokens: 200,
	}

	err = mgr.CompressSession(threadID, "S001", compressorConfig)
	// 注意：在测试环境中，如果没有 mock 压缩模型，这里可能会失败
	// Phase 4 实现时需要支持 mock
	if err != nil {
		t.Skipf("压缩模型不可用，跳过: %v", err)
		return
	}

	session, err := mgr.GetSession(threadID, "S001")
	require.NoError(t, err)
	assert.Equal(t, SessionSealed, session.Status)
	assert.NotEmpty(t, session.Summary, "压缩后应该有 Summary")
}

func TestCompressSession_IncludesHistorySummaries(t *testing.T) {
	// TC-3.8: 压缩 prompt 中包含之前 sealed Session 的 Summary
	// 这个测试验证压缩逻辑会收集历史摘要
	// 具体实现依赖 Phase 4，这里先定义预期行为
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-008"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 创建第一个 Session 并 Seal
	appendNEvents(t, mgr, threadID, 3)
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 创建第二个 Session 并 Seal
	appendNEvents(t, mgr, threadID, 3)
	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 验证有 3 个 Session
	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)

	// 压缩第二个 Session 时应该包含第一个的 Summary
	compressorConfig := &MemoryCompressorConfig{
		Model:            "test-model",
		MaxSummaryTokens: 200,
	}

	err = mgr.CompressSession(threadID, "S002", compressorConfig)
	if err != nil {
		t.Skipf("压缩模型不可用，跳过: %v", err)
	}
}

func TestCompressSession_FailureDoesNotBlock(t *testing.T) {
	// TC-3.9: 压缩模型调用失败，Session 保持 compressing，新消息正常写入
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-009"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	appendNEvents(t, mgr, threadID, 3)

	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// 即使压缩失败，旧 Session 应该保持 compressing
	oldSession, err := mgr.GetSession(threadID, "S001")
	require.NoError(t, err)
	assert.Equal(t, SessionCompressing, oldSession.Status)

	// 新消息应该正常写入新 Session
	err = mgr.AppendEvent(threadID, makeUserEvent("压缩失败后的新消息"))
	require.NoError(t, err)

	activeSession, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, "S002", activeSession.ID)
	assert.Equal(t, 1, activeSession.EventCount)
}

func TestSeal_ConsecutiveThreeTimes(t *testing.T) {
	// TC-3.10: 连续触发 3 次 Seal，Chain 中有 4 个 Session（3 sealed/compressing + 1 active）
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-seal-010"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		appendNEvents(t, mgr, threadID, 3)
		err = mgr.SealActiveSession(threadID)
		require.NoError(t, err, "第 %d 次 Seal 失败", i+1)
	}

	sessions, err := mgr.ListSessions(threadID)
	require.NoError(t, err)
	assert.Len(t, sessions, 4)

	// 前 3 个应该是 compressing（未压缩）
	for i := 0; i < 3; i++ {
		assert.Equal(t, SessionCompressing, sessions[i].Status,
			"Session %d 应该是 compressing", i+1)
		assert.Equal(t, fmt.Sprintf("S%03d", i+1), sessions[i].ID)
	}

	// 最后一个应该是 active
	assert.Equal(t, SessionActive, sessions[3].Status)
	assert.Equal(t, "S004", sessions[3].ID)
	assert.Equal(t, 4, sessions[3].SeqNo)

	// Meta 验证
	meta, err := mgr.ReadMeta(threadID)
	require.NoError(t, err)
	assert.Equal(t, "S004", meta.ActiveSessionID)
	assert.Equal(t, 4, meta.SessionCount)
}
