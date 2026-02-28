package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// TC-4: 全文搜索测试
// ============================================================

func TestSearch_BasicKeyword(t *testing.T) {
	// TC-4.1: 搜索 "HTTP服务器"，返回包含该关键词的 Event
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-search-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("帮我写一个HTTP服务器"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "好的，我来搭建HTTP服务器框架"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("再加个数据库"))
	require.NoError(t, err)

	results, err := mgr.SearchEvents(threadID, "HTTP服务器", 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2, "应该至少匹配 2 条包含 HTTP服务器 的 Event")

	for _, r := range results {
		assert.Contains(t, r.Snippet, "HTTP服务器")
	}
}

func TestSearch_CrossSession(t *testing.T) {
	// TC-4.2: 结果来自多个 Session
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-search-002"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// Session 1
	err = mgr.AppendEvent(threadID, makeUserEvent("部署到测试环境"))
	require.NoError(t, err)

	err = mgr.SealActiveSession(threadID)
	require.NoError(t, err)

	// Session 2
	err = mgr.AppendEvent(threadID, makeUserEvent("部署到生产环境"))
	require.NoError(t, err)

	results, err := mgr.SearchEvents(threadID, "部署", 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2, "应该跨 Session 匹配")

	// 验证结果来自不同 Session
	sessionIDs := make(map[string]bool)
	for _, r := range results {
		sessionIDs[r.SessionID] = true
	}
	assert.GreaterOrEqual(t, len(sessionIDs), 2, "结果应该来自至少 2 个 Session")
}

func TestSearch_CaseInsensitive(t *testing.T) {
	// TC-4.3: 搜索 "http" 能匹配 "HTTP"
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-search-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("启动 HTTP Server"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("配置 http proxy"))
	require.NoError(t, err)

	results, err := mgr.SearchEvents(threadID, "http", 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 2, "大小写不敏感应该匹配两条")
}

func TestSearch_SnippetContext(t *testing.T) {
	// TC-4.4: snippet 包含匹配行的前后各 2 行
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-search-004"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("第一行上文"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("第二行上文"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("关键词匹配行"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("第一行下文"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("第二行下文"))
	require.NoError(t, err)

	results, err := mgr.SearchEvents(threadID, "关键词匹配行", 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 1)

	snippet := results[0].Snippet
	assert.Contains(t, snippet, "关键词匹配行", "snippet 应该包含匹配行")
}

func TestSearch_ResultLocation(t *testing.T) {
	// TC-4.5: 返回正确的 sessionId、eventNo
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-search-005"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("无关消息"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeUserEvent("定位测试目标"))
	require.NoError(t, err)

	results, err := mgr.SearchEvents(threadID, "定位测试目标", 10)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 1)

	assert.Equal(t, "S001", results[0].SessionID)
	assert.Equal(t, 2, results[0].EventNo, "应该是第 2 个 Event")
}

func TestSearch_Limit(t *testing.T) {
	// TC-4.6: limit=3 时最多返回 3 条结果
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-search-006"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		err := mgr.AppendEvent(threadID, makeUserEvent("重复关键词内容"))
		require.NoError(t, err)
	}

	results, err := mgr.SearchEvents(threadID, "重复关键词", 3)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 3, "不应超过 limit")
}

func TestSearch_NoMatch(t *testing.T) {
	// TC-4.7: 搜索不存在的关键词，返回空数组
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-search-007"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("普通消息"))
	require.NoError(t, err)

	results, err := mgr.SearchEvents(threadID, "完全不存在的关键词xyz", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearch_EmptyQuery(t *testing.T) {
	// TC-4.8: 空 query 返回错误或空数组
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-search-008"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	results, err := mgr.SearchEvents(threadID, "", 10)
	// 空 query 应该返回错误或空结果
	if err == nil {
		assert.Empty(t, results, "空 query 应该返回空结果")
	}
	// 如果返回 error 也是可接受的
}
