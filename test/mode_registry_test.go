package test

import (
	"testing"
)

// MockMode 用于测试的模拟协作模式
type MockMode struct {
	name        string
	description string
	initialized bool
}

func (m *MockMode) GetName() string {
	return m.name
}

func (m *MockMode) GetDescription() string {
	return m.description
}

func (m *MockMode) OnUserMessage(sessionID string, content string, mentionedCats []string) ([]AgentCall, error) {
	return []AgentCall{}, nil
}

func (m *MockMode) OnAgentResponse(sessionID string, agentName string, response string) ([]AgentCall, error) {
	return []AgentCall{}, nil
}

func (m *MockMode) Validate() error {
	return nil
}

func (m *MockMode) Initialize(sessionID string) error {
	m.initialized = true
	return nil
}

// AgentCall 结构体（从 orchestrator.go 复制）
type AgentCall struct {
	AgentName  string
	Prompt     string
	SessionID  string
	CallerName string
	Metadata   map[string]interface{}
}

// CollaborationMode 接口（从 mode_interface.go 复制）
type CollaborationMode interface {
	GetName() string
	GetDescription() string
	OnUserMessage(sessionID string, content string, mentionedCats []string) ([]AgentCall, error)
	OnAgentResponse(sessionID string, agentName string, response string) ([]AgentCall, error)
	Validate() error
	Initialize(sessionID string) error
}

// ModeRegistry 简化版本用于测试
type ModeRegistry struct {
	modes map[string]CollaborationMode
}

func NewModeRegistry() *ModeRegistry {
	return &ModeRegistry{
		modes: make(map[string]CollaborationMode),
	}
}

func (r *ModeRegistry) Register(mode CollaborationMode) error {
	r.modes[mode.GetName()] = mode
	return nil
}

func (r *ModeRegistry) Get(name string) (CollaborationMode, bool) {
	mode, exists := r.modes[name]
	return mode, exists
}

func (r *ModeRegistry) List() []CollaborationMode {
	modes := make([]CollaborationMode, 0, len(r.modes))
	for _, mode := range r.modes {
		modes = append(modes, mode)
	}
	return modes
}

// 测试用例

func TestModeRegistry_Register(t *testing.T) {
	registry := NewModeRegistry()
	mode := &MockMode{
		name:        "test_mode",
		description: "Test Mode",
	}

	err := registry.Register(mode)
	if err != nil {
		t.Fatalf("Failed to register mode: %v", err)
	}

	// 验证模式已注册
	retrieved, exists := registry.Get("test_mode")
	if !exists {
		t.Fatal("Mode not found after registration")
	}

	if retrieved.GetName() != "test_mode" {
		t.Errorf("Expected mode name 'test_mode', got '%s'", retrieved.GetName())
	}
}

func TestModeRegistry_Get(t *testing.T) {
	registry := NewModeRegistry()
	mode := &MockMode{
		name:        "test_mode",
		description: "Test Mode",
	}

	registry.Register(mode)

	// 测试获取存在的模式
	retrieved, exists := registry.Get("test_mode")
	if !exists {
		t.Fatal("Mode should exist")
	}
	if retrieved.GetName() != "test_mode" {
		t.Errorf("Expected mode name 'test_mode', got '%s'", retrieved.GetName())
	}

	// 测试获取不存在的模式
	_, exists = registry.Get("non_existent")
	if exists {
		t.Error("Non-existent mode should not exist")
	}
}

func TestModeRegistry_List(t *testing.T) {
	registry := NewModeRegistry()

	// 注册多个模式
	mode1 := &MockMode{name: "mode1", description: "Mode 1"}
	mode2 := &MockMode{name: "mode2", description: "Mode 2"}
	mode3 := &MockMode{name: "mode3", description: "Mode 3"}

	registry.Register(mode1)
	registry.Register(mode2)
	registry.Register(mode3)

	// 获取所有模式
	modes := registry.List()

	if len(modes) != 3 {
		t.Errorf("Expected 3 modes, got %d", len(modes))
	}

	// 验证所有模式都在列表中
	modeNames := make(map[string]bool)
	for _, mode := range modes {
		modeNames[mode.GetName()] = true
	}

	if !modeNames["mode1"] || !modeNames["mode2"] || !modeNames["mode3"] {
		t.Error("Not all registered modes are in the list")
	}
}

func TestModeRegistry_EmptyList(t *testing.T) {
	registry := NewModeRegistry()

	modes := registry.List()
	if len(modes) != 0 {
		t.Errorf("Expected empty list, got %d modes", len(modes))
	}
}
