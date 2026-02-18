package main

import (
	"fmt"
	"strings"
)

// FreeDiscussionMode 自由讨论模式
// 猫猫可以随意互相 @ 调用，没有流程约束
type FreeDiscussionMode struct {
	name        string
	description string
}

// NewFreeDiscussionMode 创建自由讨论模式
func NewFreeDiscussionMode(config *ModeConfig) (CollaborationMode, error) {
	return &FreeDiscussionMode{
		name:        "free_discussion",
		description: "自由讨论模式 - 猫猫可以随意互相调用，适合开放式协作",
	}, nil
}

// GetName 返回模式名称
func (m *FreeDiscussionMode) GetName() string {
	return m.name
}

// GetDescription 返回模式描述
func (m *FreeDiscussionMode) GetDescription() string {
	return m.description
}

// OnUserMessage 处理用户消息
func (m *FreeDiscussionMode) OnUserMessage(sessionID string, content string, mentionedCats []string) ([]AgentCall, error) {
	calls := []AgentCall{}

	// 为每个被提及的猫猫创建调用
	for _, catName := range mentionedCats {
		calls = append(calls, AgentCall{
			AgentName:  catName,
			Prompt:     content,
			SessionID:  sessionID,
			CallerName: "用户",
			Metadata: map[string]interface{}{
				"source": "user_message",
			},
		})
	}

	return calls, nil
}

// OnAgentResponse 处理猫猫回复
func (m *FreeDiscussionMode) OnAgentResponse(sessionID string, agentName string, response string) ([]AgentCall, error) {
	// 解析回复中的 @ 调用
	calls := m.parseAtMentions(response, sessionID, agentName)
	return calls, nil
}

// Validate 验证模式配置
func (m *FreeDiscussionMode) Validate() error {
	// 自由讨论模式没有特殊配置要求
	return nil
}

// Initialize 初始化模式
func (m *FreeDiscussionMode) Initialize(sessionID string) error {
	// 自由讨论模式不需要特殊初始化
	return nil
}

// parseAtMentions 解析文本中的 @ 提及
// 支持格式：
// - @猫猫名 任务内容
// - @猫猫名\n任务内容
func (m *FreeDiscussionMode) parseAtMentions(text string, sessionID string, callerName string) []AgentCall {
	calls := []AgentCall{}

	// 按行分割文本
	lines := strings.Split(text, "\n")

	var currentCat string
	var currentPrompt strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 检查是否是新的 @ 提及
		if strings.HasPrefix(line, "@") {
			// 保存之前的提及
			if currentCat != "" && currentPrompt.Len() > 0 {
				prompt := strings.TrimSpace(currentPrompt.String())
				if currentCat != "铲屎官" && prompt != "" {
					calls = append(calls, AgentCall{
						AgentName:  currentCat,
						Prompt:     prompt,
						SessionID:  sessionID,
						CallerName: callerName,
						Metadata: map[string]interface{}{
							"source":       "agent_response",
							"caller_agent": callerName,
						},
					})
				}
			}

			// 解析新的 @ 提及
			parts := strings.SplitN(line[1:], " ", 2)
			currentCat = strings.TrimSpace(parts[0])
			currentPrompt.Reset()

			// 如果同一行有内容，添加到 prompt
			if len(parts) > 1 {
				currentPrompt.WriteString(strings.TrimSpace(parts[1]))
			}
		} else if currentCat != "" && line != "" {
			// 继续添加到当前 prompt
			if currentPrompt.Len() > 0 {
				currentPrompt.WriteString("\n")
			}
			currentPrompt.WriteString(line)
		}
	}

	// 保存最后一个提及
	if currentCat != "" && currentPrompt.Len() > 0 {
		prompt := strings.TrimSpace(currentPrompt.String())
		if currentCat != "铲屎官" && prompt != "" {
			calls = append(calls, AgentCall{
				AgentName:  currentCat,
				Prompt:     prompt,
				SessionID:  sessionID,
				CallerName: callerName,
				Metadata: map[string]interface{}{
					"source":       "agent_response",
					"caller_agent": callerName,
				},
			})
		}
	}

	return calls
}

// init 注册自由讨论模式到全局注册表
func init() {
	err := RegisterMode("free_discussion", NewFreeDiscussionMode)
	if err != nil {
		fmt.Printf("Failed to register free_discussion mode: %v\n", err)
	}
}
