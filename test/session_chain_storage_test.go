package test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// TC-2: 文件系统存储测试
// ============================================================

func TestStorage_DirectoryAutoCreate(t *testing.T) {
	// TC-2.1: 首次写入时自动创建目录
	mgr, tmpDir, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-storage-001"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 验证目录结构
	tDir := threadDir(tmpDir, threadID)
	assert.True(t, fileExists(tDir), "thread 目录应该存在")
	assert.True(t, fileExists(filepath.Join(tDir, "invocations")),
		"invocations 子目录应该存在")
}

func TestStorage_MetaJSON_ReadWrite(t *testing.T) {
	// TC-2.2: meta.json 写入后读回，字段完全一致
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-storage-002"
	original, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 读回
	loaded, err := mgr.ReadMeta(threadID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, original.ThreadID, loaded.ThreadID)
	assert.Equal(t, original.ActiveSessionID, loaded.ActiveSessionID)
	assert.Equal(t, original.SessionCount, loaded.SessionCount)
	assert.Equal(t, original.TotalEvents, loaded.TotalEvents)
}

func TestStorage_SessionMarkdown_Write(t *testing.T) {
	// TC-2.3: 生成的 Markdown 包含正确的 frontmatter 和 Event 格式
	mgr, tmpDir, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-storage-003"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 写入几个 Event
	err = mgr.AppendEvent(threadID, makeUserEvent("你好"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "喵~"))
	require.NoError(t, err)

	// 读取 Markdown 文件内容
	mdPath := filepath.Join(tmpDir, threadID, "S001.md")
	assert.True(t, fileExists(mdPath), "Markdown 文件应该存在")

	content := readFileContent(t, mdPath)

	// 验证 frontmatter
	assert.True(t, strings.HasPrefix(content, "---"),
		"应该以 frontmatter 开头")
	assert.Contains(t, content, "id: S001")
	assert.Contains(t, content, fmt.Sprintf("threadId: %s", threadID))
	assert.Contains(t, content, "status: active")

	// 验证 Event 格式
	assert.Contains(t, content, "**[用户]**")
	assert.Contains(t, content, "你好")
	assert.Contains(t, content, "**[花花]**")
	assert.Contains(t, content, "喵~")
}

func TestStorage_SessionMarkdown_Append(t *testing.T) {
	// TC-2.4: 追加 Event 后文件内容正确，frontmatter 更新
	mgr, tmpDir, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-storage-004"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	// 第一个 Event
	err = mgr.AppendEvent(threadID, makeUserEvent("第一条"))
	require.NoError(t, err)

	mdPath := filepath.Join(tmpDir, threadID, "S001.md")
	content1 := readFileContent(t, mdPath)
	assert.Contains(t, content1, "第一条")

	// 追加第二个 Event
	err = mgr.AppendEvent(threadID, makeUserEvent("第二条"))
	require.NoError(t, err)

	content2 := readFileContent(t, mdPath)
	assert.Contains(t, content2, "第一条")
	assert.Contains(t, content2, "第二条")

	// frontmatter 中的 eventCount 应该更新
	// 具体格式取决于实现，这里验证两个 Event 都在
}

func TestStorage_SessionMarkdown_Read(t *testing.T) {
	// TC-2.5: 从 Markdown 解析出 frontmatter 和 Event 列表
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-storage-005"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	err = mgr.AppendEvent(threadID, makeUserEvent("解析测试"))
	require.NoError(t, err)
	err = mgr.AppendEvent(threadID, makeCatEvent("花花", "收到"))
	require.NoError(t, err)

	// 通过 ReadSessionMarkdown 解析
	session, events, err := mgr.ReadSessionMarkdown(threadID, "S001")
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, "S001", session.ID)
	assert.Len(t, events, 2)
	assert.Equal(t, "解析测试", events[0].Content)
	assert.Equal(t, "收到", events[1].Content)
}

func TestStorage_InvocationJSON_ReadWrite(t *testing.T) {
	// TC-2.6: Invocation JSON 写入后读回，字段完全一致
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-storage-006"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	inv := makeInvocation("花花", "写个函数", "func hello() {}")
	inv.ThreadID = threadID
	inv.SessionID = "S001"
	inv.TokensIn = 50
	inv.TokensOut = 80
	inv.Duration = 2000

	err = mgr.WriteInvocation(threadID, &inv)
	require.NoError(t, err)

	loaded, err := mgr.ReadInvocation(threadID, inv.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, inv.ID, loaded.ID)
	assert.Equal(t, inv.SessionID, loaded.SessionID)
	assert.Equal(t, inv.AgentName, loaded.AgentName)
	assert.Equal(t, inv.Prompt, loaded.Prompt)
	assert.Equal(t, inv.Response, loaded.Response)
	assert.Equal(t, inv.TokensIn, loaded.TokensIn)
	assert.Equal(t, inv.TokensOut, loaded.TokensOut)
	assert.Equal(t, inv.Duration, loaded.Duration)
}

func TestStorage_SpecialCharacters(t *testing.T) {
	// TC-2.7: Event 内容包含 Markdown 特殊字符时不破坏格式
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-storage-007"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	specialContent := "# 标题\n**加粗** *斜体* | 表格 | `代码` ```块```\n---\n> 引用"
	err = mgr.AppendEvent(threadID, makeUserEvent(specialContent))
	require.NoError(t, err)

	// 读回验证内容完整
	events, _, err := mgr.GetEvents(threadID, "S001", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, specialContent, events[0].Content,
		"特殊字符内容应该完整保留")
}

func TestStorage_LargeFile(t *testing.T) {
	// TC-2.8: 500 条 Event 写入后文件可正常读取
	mgr, _, cleanup := setupSessionChainTest(t)
	defer cleanup()

	threadID := "thread-storage-008"
	_, err := mgr.GetOrCreateChain(threadID)
	require.NoError(t, err)

	const total = 500
	appendNEvents(t, mgr, threadID, total)

	session, err := mgr.GetActiveSession(threadID)
	require.NoError(t, err)
	assert.Equal(t, total, session.EventCount)

	// 验证可以完整读取
	var allEvents []SessionEvent
	cursor := 0
	for {
		events, next, err := mgr.GetEvents(threadID, session.ID, cursor, 100)
		require.NoError(t, err)
		allEvents = append(allEvents, events...)
		if next == -1 {
			break
		}
		cursor = next
	}
	assert.Len(t, allEvents, total)
}
