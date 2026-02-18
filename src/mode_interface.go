package main

import (
	"time"
)

// CollaborationMode 协作模式接口
// 定义了猫猫们如何协作完成任务的规则
type CollaborationMode interface {
	// GetName 返回模式名称
	GetName() string

	// GetDescription 返回模式描述
	GetDescription() string

	// OnUserMessage 处理用户消息，返回需要调用的猫猫列表
	// sessionID: 会话 ID
	// content: 用户消息内容
	// mentionedCats: 用户 @ 提及的猫猫名称列表
	// 返回: 需要调用的猫猫列表和错误
	OnUserMessage(sessionID string, content string, mentionedCats []string) ([]AgentCall, error)

	// OnAgentResponse 处理猫猫回复，返回下一步需要调用的猫猫列表
	// sessionID: 会话 ID
	// agentName: 回复的猫猫名称
	// response: 猫猫的回复内容
	// 返回: 下一步需要调用的猫猫列表和错误
	OnAgentResponse(sessionID string, agentName string, response string) ([]AgentCall, error)

	// Validate 验证模式配置是否有效
	Validate() error

	// Initialize 初始化模式（可选）
	// 在模式被首次使用时调用
	Initialize(sessionID string) error
}

// AgentCall 表示一次猫猫调用
type AgentCall struct {
	// AgentName 要调用的猫猫名称
	AgentName string

	// Prompt 发送给猫猫的提示词
	Prompt string

	// SessionID 会话 ID
	SessionID string

	// CallerName 调用者名称（用户或其他猫猫）
	CallerName string

	// Metadata 额外的元数据
	Metadata map[string]interface{}
}

// ModeConfig 模式配置
type ModeConfig struct {
	// Name 模式名称
	Name string `json:"name"`

	// Enabled 是否启用
	Enabled bool `json:"enabled"`

	// Config 模式特定的配置
	Config map[string]interface{} `json:"config,omitempty"`
}

// ModeState 模式状态
// 用于存储模式运行时的状态信息
type ModeState struct {
	// CurrentStep 当前步骤（用于流程模式）
	CurrentStep string `json:"current_step,omitempty"`

	// StepHistory 步骤历史
	StepHistory []string `json:"step_history,omitempty"`

	// CustomState 自定义状态数据
	CustomState map[string]interface{} `json:"custom_state,omitempty"`

	// LastUpdateTime 最后更新时间
	LastUpdateTime time.Time `json:"last_update_time"`
}

// ModeFactory 模式工厂函数
type ModeFactory func(config *ModeConfig) (CollaborationMode, error)
