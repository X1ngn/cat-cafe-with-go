package main

import (
	"fmt"
	"sync"
)

// ModeRegistry 模式注册表
// 管理所有可用的协作模式
type ModeRegistry struct {
	mu        sync.RWMutex
	factories map[string]ModeFactory
	modes     map[string]CollaborationMode
}

// NewModeRegistry 创建新的模式注册表
func NewModeRegistry() *ModeRegistry {
	return &ModeRegistry{
		factories: make(map[string]ModeFactory),
		modes:     make(map[string]CollaborationMode),
	}
}

// Register 注册一个模式工厂
func (r *ModeRegistry) Register(name string, factory ModeFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("mode %s already registered", name)
	}

	r.factories[name] = factory
	return nil
}

// Create 创建一个模式实例
func (r *ModeRegistry) Create(name string, config *ModeConfig) (CollaborationMode, error) {
	r.mu.RLock()
	factory, exists := r.factories[name]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("mode %s not found", name)
	}

	mode, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create mode %s: %w", name, err)
	}

	// 验证模式
	if err := mode.Validate(); err != nil {
		return nil, fmt.Errorf("mode %s validation failed: %w", name, err)
	}

	return mode, nil
}

// Get 获取已创建的模式实例（如果存在）
func (r *ModeRegistry) Get(name string) (CollaborationMode, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mode, exists := r.modes[name]
	return mode, exists
}

// GetOrCreate 获取或创建模式实例
func (r *ModeRegistry) GetOrCreate(name string, config *ModeConfig) (CollaborationMode, error) {
	// 先尝试获取已存在的实例
	if mode, exists := r.Get(name); exists {
		return mode, nil
	}

	// 不存在则创建新实例
	mode, err := r.Create(name, config)
	if err != nil {
		return nil, err
	}

	// 缓存实例
	r.mu.Lock()
	r.modes[name] = mode
	r.mu.Unlock()

	return mode, nil
}

// List 列出所有已注册的模式名称
func (r *ModeRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// ListModes 列出所有已注册的模式详细信息
func (r *ModeRegistry) ListModes() []ModeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]ModeInfo, 0, len(r.factories))
	for name := range r.factories {
		// 创建临时实例以获取描述
		mode, err := r.Create(name, &ModeConfig{Name: name})
		if err != nil {
			continue
		}

		infos = append(infos, ModeInfo{
			Name:        name,
			Description: mode.GetDescription(),
		})
	}
	return infos
}

// Exists 检查模式是否已注册
func (r *ModeRegistry) Exists(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.factories[name]
	return exists
}

// ModeInfo 模式信息
type ModeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GlobalModeRegistry 全局模式注册表
var GlobalModeRegistry = NewModeRegistry()

// RegisterMode 注册模式到全局注册表（便捷函数）
func RegisterMode(name string, factory ModeFactory) error {
	return GlobalModeRegistry.Register(name, factory)
}

// GetMode 从全局注册表获取模式（便捷函数）
func GetMode(name string, config *ModeConfig) (CollaborationMode, error) {
	return GlobalModeRegistry.GetOrCreate(name, config)
}

// ListAvailableModes 列出所有可用模式（便捷函数）
func ListAvailableModes() []ModeInfo {
	return GlobalModeRegistry.ListModes()
}
