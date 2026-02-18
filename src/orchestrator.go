package main

import (
	"fmt"
	"sync"
	"time"
)

// Orchestrator 编排器
// 协调协作模式和调度器，管理猫猫之间的协作流程
type Orchestrator struct {
	mu            sync.RWMutex
	registry      *ModeRegistry
	scheduler     *Scheduler
	defaultMode   string
	sessions      map[string]*OrchestratorSession
	agentConfigs  map[string]*AgentConfig // 猫猫配置映射
}

// OrchestratorSession 编排器会话
type OrchestratorSession struct {
	SessionID  string
	Mode       CollaborationMode
	ModeConfig *ModeConfig
	ModeState  *ModeState
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewOrchestrator 创建新的编排器
func NewOrchestrator(scheduler *Scheduler, defaultMode string) *Orchestrator {
	return &Orchestrator{
		registry:     GlobalModeRegistry,
		scheduler:    scheduler,
		defaultMode:  defaultMode,
		sessions:     make(map[string]*OrchestratorSession),
		agentConfigs: make(map[string]*AgentConfig),
	}
}

// SetAgentConfigs 设置猫猫配置
func (o *Orchestrator) SetAgentConfigs(configs []AgentConfig) {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i := range configs {
		o.agentConfigs[configs[i].Name] = &configs[i]
	}
}

// CreateSession 创建新会话
func (o *Orchestrator) CreateSession(sessionID string, modeName string, modeConfig *ModeConfig) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	// 检查会话是否已存在
	if _, exists := o.sessions[sessionID]; exists {
		return fmt.Errorf("session %s already exists", sessionID)
	}

	// 如果未指定模式，使用默认模式
	if modeName == "" {
		modeName = o.defaultMode
	}

	// 如果未提供配置，创建默认配置
	if modeConfig == nil {
		modeConfig = &ModeConfig{
			Name:    modeName,
			Enabled: true,
		}
	}

	// 创建模式实例
	mode, err := o.registry.GetOrCreate(modeName, modeConfig)
	if err != nil {
		return fmt.Errorf("failed to create mode: %w", err)
	}

	// 创建会话
	session := &OrchestratorSession{
		SessionID:  sessionID,
		Mode:       mode,
		ModeConfig: modeConfig,
		ModeState: &ModeState{
			CustomState:    make(map[string]interface{}),
			LastUpdateTime: time.Now(),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	o.sessions[sessionID] = session

	// 初始化模式
	if err := mode.Initialize(sessionID); err != nil {
		delete(o.sessions, sessionID)
		return fmt.Errorf("failed to initialize mode: %w", err)
	}

	return nil
}

// GetSession 获取会话
func (o *Orchestrator) GetSession(sessionID string) (*OrchestratorSession, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	session, exists := o.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session, nil
}

// DeleteSession 删除会话
func (o *Orchestrator) DeleteSession(sessionID string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if _, exists := o.sessions[sessionID]; !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	delete(o.sessions, sessionID)
	return nil
}

// SwitchMode 切换会话的协作模式
func (o *Orchestrator) SwitchMode(sessionID string, modeName string, modeConfig *ModeConfig) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	session, exists := o.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// 如果未提供配置，创建默认配置
	if modeConfig == nil {
		modeConfig = &ModeConfig{
			Name:    modeName,
			Enabled: true,
		}
	}

	// 创建新模式实例
	mode, err := o.registry.GetOrCreate(modeName, modeConfig)
	if err != nil {
		return fmt.Errorf("failed to create mode: %w", err)
	}

	// 更新会话
	session.Mode = mode
	session.ModeConfig = modeConfig
	session.ModeState = &ModeState{
		CustomState:    make(map[string]interface{}),
		LastUpdateTime: time.Now(),
	}
	session.UpdatedAt = time.Now()

	// 初始化新模式
	if err := mode.Initialize(sessionID); err != nil {
		return fmt.Errorf("failed to initialize mode: %w", err)
	}

	return nil
}

// HandleUserMessage 处理用户消息
func (o *Orchestrator) HandleUserMessage(sessionID string, content string, mentionedCats []string) ([]AgentCall, error) {
	session, err := o.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// 调用模式处理用户消息
	calls, err := session.Mode.OnUserMessage(sessionID, content, mentionedCats)
	if err != nil {
		return nil, fmt.Errorf("mode failed to handle user message: %w", err)
	}

	// 更新会话状态
	o.mu.Lock()
	session.ModeState.LastUpdateTime = time.Now()
	session.UpdatedAt = time.Now()
	o.mu.Unlock()

	return calls, nil
}

// HandleAgentResponse 处理猫猫回复
func (o *Orchestrator) HandleAgentResponse(sessionID string, agentName string, response string) ([]AgentCall, error) {
	session, err := o.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// 调用模式处理猫猫回复
	calls, err := session.Mode.OnAgentResponse(sessionID, agentName, response)
	if err != nil {
		return nil, fmt.Errorf("mode failed to handle agent response: %w", err)
	}

	// 更新会话状态
	o.mu.Lock()
	session.ModeState.LastUpdateTime = time.Now()
	session.UpdatedAt = time.Now()
	o.mu.Unlock()

	return calls, nil
}

// ExecuteCalls 执行猫猫调用
func (o *Orchestrator) ExecuteCalls(calls []AgentCall) error {
	for _, call := range calls {
		// 验证猫猫是否存在
		if _, exists := o.agentConfigs[call.AgentName]; !exists {
			return fmt.Errorf("agent %s not found", call.AgentName)
		}

		// 发送任务到调度器（使用调度器的 SendTask 方法）
		_, err := o.scheduler.SendTask(call.AgentName, call.Prompt, call.SessionID)
		if err != nil {
			return fmt.Errorf("failed to send task to agent %s: %w", call.AgentName, err)
		}
	}

	return nil
}

// ListModes 列出所有可用模式
func (o *Orchestrator) ListModes() []ModeInfo {
	return o.registry.ListModes()
}

// GetCurrentMode 获取会话当前使用的模式
func (o *Orchestrator) GetCurrentMode(sessionID string) (string, error) {
	session, err := o.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	return session.Mode.GetName(), nil
}

// generateOrchestratorTaskID 生成任务 ID（编排器专用）
func generateOrchestratorTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}
