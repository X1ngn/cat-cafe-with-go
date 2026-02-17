package test

import (
	"testing"
)

// FreeDiscussionMode 简化版本用于测试
type FreeDiscussionMode struct {
	name        string
	description string
}

func NewFreeDiscussionMode() *FreeDiscussionMode {
	return &FreeDiscussionMode{
		name:        "free_discussion",
		description: "自由讨论模式 - 猫猫们可以自由互相调用",
	}
}

func (m *FreeDiscussionMode) GetName() string {
	return m.name
}

func (m *FreeDiscussionMode) GetDescription() string {
	return m.description
}

func (m *FreeDiscussionMode) OnUserMessage(sessionID string, content string, mentionedCats []string) ([]AgentCall, error) {
	calls := make([]AgentCall, 0)

	for _, catName := range mentionedCats {
		calls = append(calls, AgentCall{
			AgentName:  catName,
			Prompt:     content,
			SessionID:  sessionID,
			CallerName: "user",
		})
	}

	return calls, nil
}

func (m *FreeDiscussionMode) OnAgentResponse(sessionID string, agentName string, response string) ([]AgentCall, error) {
	calls := make([]AgentCall, 0)

	// 解析回复中的 @ 提及
	mentionedCats := parseMentions(response)

	for _, catName := range mentionedCats {
		calls = append(calls, AgentCall{
			AgentName:  catName,
			Prompt:     response,
			SessionID:  sessionID,
			CallerName: agentName,
		})
	}

	return calls, nil
}

func (m *FreeDiscussionMode) Validate() error {
	return nil
}

func (m *FreeDiscussionMode) Initialize(sessionID string) error {
	return nil
}

// parseMentions 解析文本中的 @ 提及
func parseMentions(text string) []string {
	mentions := make([]string, 0)

	// 使用更灵活的方式解析，支持中文标点符号
	runes := []rune(text)
	i := 0

	for i < len(runes) {
		if runes[i] == '@' {
			// 找到 @，开始提取猫猫名字
			i++
			start := i

			// 提取名字直到遇到空格或标点符号
			for i < len(runes) {
				r := runes[i]
				// 如果是空格或标点符号，停止
				if r == ' ' || r == '，' || r == '。' || r == '！' || r == '？' ||
				   r == ',' || r == '.' || r == '!' || r == '?' || r == '\n' || r == '\t' {
					break
				}
				i++
			}

			if i > start {
				catName := string(runes[start:i])
				if catName != "" {
					mentions = append(mentions, catName)
				}
			}
		} else {
			i++
		}
	}

	return mentions
}

// 测试用例

func TestFreeDiscussionMode_GetName(t *testing.T) {
	mode := NewFreeDiscussionMode()

	if mode.GetName() != "free_discussion" {
		t.Errorf("Expected name 'free_discussion', got '%s'", mode.GetName())
	}
}

func TestFreeDiscussionMode_GetDescription(t *testing.T) {
	mode := NewFreeDiscussionMode()

	desc := mode.GetDescription()
	if desc == "" {
		t.Error("Description should not be empty")
	}
}

func TestFreeDiscussionMode_OnUserMessage_SingleMention(t *testing.T) {
	mode := NewFreeDiscussionMode()
	sessionID := "test_session"
	content := "你好"
	mentionedCats := []string{"花花"}

	calls, err := mode.OnUserMessage(sessionID, content, mentionedCats)
	if err != nil {
		t.Fatalf("OnUserMessage failed: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}

	call := calls[0]
	if call.AgentName != "花花" {
		t.Errorf("Expected agent name '花花', got '%s'", call.AgentName)
	}
	if call.Prompt != content {
		t.Errorf("Expected prompt '%s', got '%s'", content, call.Prompt)
	}
	if call.SessionID != sessionID {
		t.Errorf("Expected session ID '%s', got '%s'", sessionID, call.SessionID)
	}
	if call.CallerName != "user" {
		t.Errorf("Expected caller name 'user', got '%s'", call.CallerName)
	}
}

func TestFreeDiscussionMode_OnUserMessage_MultipleMentions(t *testing.T) {
	mode := NewFreeDiscussionMode()
	sessionID := "test_session"
	content := "帮我审查代码"
	mentionedCats := []string{"花花", "薇薇", "小乔"}

	calls, err := mode.OnUserMessage(sessionID, content, mentionedCats)
	if err != nil {
		t.Fatalf("OnUserMessage failed: %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("Expected 3 calls, got %d", len(calls))
	}

	// 验证所有猫猫都被调用
	calledCats := make(map[string]bool)
	for _, call := range calls {
		calledCats[call.AgentName] = true
		if call.Prompt != content {
			t.Errorf("Expected prompt '%s', got '%s'", content, call.Prompt)
		}
		if call.SessionID != sessionID {
			t.Errorf("Expected session ID '%s', got '%s'", sessionID, call.SessionID)
		}
	}

	if !calledCats["花花"] || !calledCats["薇薇"] || !calledCats["小乔"] {
		t.Error("Not all mentioned cats were called")
	}
}

func TestFreeDiscussionMode_OnUserMessage_NoMentions(t *testing.T) {
	mode := NewFreeDiscussionMode()
	sessionID := "test_session"
	content := "你好"
	mentionedCats := []string{}

	calls, err := mode.OnUserMessage(sessionID, content, mentionedCats)
	if err != nil {
		t.Fatalf("OnUserMessage failed: %v", err)
	}

	if len(calls) != 0 {
		t.Errorf("Expected 0 calls, got %d", len(calls))
	}
}

func TestFreeDiscussionMode_OnAgentResponse_WithMentions(t *testing.T) {
	mode := NewFreeDiscussionMode()
	sessionID := "test_session"
	agentName := "花花"
	response := "好的，@薇薇 请帮忙审查一下代码"

	calls, err := mode.OnAgentResponse(sessionID, agentName, response)
	if err != nil {
		t.Fatalf("OnAgentResponse failed: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}

	call := calls[0]
	if call.AgentName != "薇薇" {
		t.Errorf("Expected agent name '薇薇', got '%s'", call.AgentName)
	}
	if call.Prompt != response {
		t.Errorf("Expected prompt '%s', got '%s'", response, call.Prompt)
	}
	if call.CallerName != agentName {
		t.Errorf("Expected caller name '%s', got '%s'", agentName, call.CallerName)
	}
}

func TestFreeDiscussionMode_OnAgentResponse_MultipleMentions(t *testing.T) {
	mode := NewFreeDiscussionMode()
	sessionID := "test_session"
	agentName := "花花"
	response := "@薇薇 @小乔 请帮忙审查代码和设计"

	calls, err := mode.OnAgentResponse(sessionID, agentName, response)
	if err != nil {
		t.Fatalf("OnAgentResponse failed: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("Expected 2 calls, got %d", len(calls))
	}

	// 验证所有提及的猫猫都被调用
	calledCats := make(map[string]bool)
	for _, call := range calls {
		calledCats[call.AgentName] = true
	}

	if !calledCats["薇薇"] || !calledCats["小乔"] {
		t.Error("Not all mentioned cats were called")
	}
}

func TestFreeDiscussionMode_OnAgentResponse_NoMentions(t *testing.T) {
	mode := NewFreeDiscussionMode()
	sessionID := "test_session"
	agentName := "花花"
	response := "好的，我来处理这个任务"

	calls, err := mode.OnAgentResponse(sessionID, agentName, response)
	if err != nil {
		t.Fatalf("OnAgentResponse failed: %v", err)
	}

	if len(calls) != 0 {
		t.Errorf("Expected 0 calls, got %d", len(calls))
	}
}

func TestFreeDiscussionMode_Validate(t *testing.T) {
	mode := NewFreeDiscussionMode()

	err := mode.Validate()
	if err != nil {
		t.Errorf("Validate should not return error, got: %v", err)
	}
}

func TestFreeDiscussionMode_Initialize(t *testing.T) {
	mode := NewFreeDiscussionMode()
	sessionID := "test_session"

	err := mode.Initialize(sessionID)
	if err != nil {
		t.Errorf("Initialize should not return error, got: %v", err)
	}
}

func TestParseMentions_Single(t *testing.T) {
	text := "你好 @花花"
	mentions := parseMentions(text)

	if len(mentions) != 1 {
		t.Fatalf("Expected 1 mention, got %d", len(mentions))
	}

	if mentions[0] != "花花" {
		t.Errorf("Expected mention '花花', got '%s'", mentions[0])
	}
}

func TestParseMentions_Multiple(t *testing.T) {
	text := "@花花 @薇薇 请帮忙"
	mentions := parseMentions(text)

	if len(mentions) != 2 {
		t.Fatalf("Expected 2 mentions, got %d", len(mentions))
	}

	mentionMap := make(map[string]bool)
	for _, m := range mentions {
		mentionMap[m] = true
	}

	if !mentionMap["花花"] || !mentionMap["薇薇"] {
		t.Error("Not all mentions were parsed correctly")
	}
}

func TestParseMentions_WithPunctuation(t *testing.T) {
	text := "@花花，请帮忙。@薇薇！"
	mentions := parseMentions(text)

	if len(mentions) != 2 {
		t.Fatalf("Expected 2 mentions, got %d", len(mentions))
	}

	mentionMap := make(map[string]bool)
	for _, m := range mentions {
		mentionMap[m] = true
	}

	if !mentionMap["花花"] || !mentionMap["薇薇"] {
		t.Error("Mentions with punctuation were not parsed correctly")
	}
}

func TestParseMentions_None(t *testing.T) {
	text := "没有提及任何猫猫"
	mentions := parseMentions(text)

	if len(mentions) != 0 {
		t.Errorf("Expected 0 mentions, got %d", len(mentions))
	}
}
