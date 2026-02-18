package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// SessionData 可序列化的会话数据
type SessionData struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Summary      string              `json:"summary"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
	MessageCount int                 `json:"messageCount"`
	Messages     []Message           `json:"messages"`
	CallHistory  []CallHistoryItem   `json:"callHistory"`
	JoinedCats   map[string]bool     `json:"joinedCats"`
	ModeName     string              `json:"modeName"`
	ModeConfig   *ModeConfig         `json:"modeConfig"`
	ModeState    *ModeState          `json:"modeState"`
}

const (
	sessionKeyPrefix = "session:"
	sessionListKey   = "sessions:list"
)

// SaveSession 保存会话到 Redis
func (sm *SessionManager) SaveSession(sessionID string) error {
	LogDebug("[Persistence] 开始保存会话: %s", sessionID)

	sm.mu.RLock()
	ctx, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	LogDebug("[Persistence] 获取会话锁: %s", sessionID)
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	LogDebug("[Persistence] 获取模式名称: %s", sessionID)
	// 获取模式名称，如果失败使用默认值
	modeName := "free_discussion"
	if ctx.Mode != nil {
		modeName = ctx.Mode.GetName()
	}
	LogDebug("[Persistence] 模式名称: %s", modeName)

	// 构建可序列化的数据
	LogDebug("[Persistence] 构建会话数据: %s", sessionID)
	data := SessionData{
		ID:           ctx.ID,
		Name:         ctx.Name,
		Summary:      ctx.Summary,
		CreatedAt:    ctx.CreatedAt,
		UpdatedAt:    ctx.UpdatedAt,
		MessageCount: ctx.MessageCount,
		Messages:     ctx.Messages,
		CallHistory:  ctx.CallHistory,
		JoinedCats:   ctx.JoinedCats,
		ModeName:     modeName,
		ModeConfig:   ctx.ModeConfig,
		ModeState:    ctx.ModeState,
	}

	// 序列化为 JSON
	LogDebug("[Persistence] 序列化会话数据: %s", sessionID)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}

	// 保存到 Redis
	LogDebug("[Persistence] 保存到 Redis: %s", sessionID)
	key := sessionKeyPrefix + sessionID
	if err := sm.redisClient.Set(sm.ctx, key, jsonData, 0).Err(); err != nil {
		return fmt.Errorf("保存会话到 Redis 失败: %w", err)
	}

	// 添加到会话列表
	LogDebug("[Persistence] 添加到会话列表: %s", sessionID)
	if err := sm.redisClient.SAdd(sm.ctx, sessionListKey, sessionID).Err(); err != nil {
		return fmt.Errorf("添加会话到列表失败: %w", err)
	}

	LogDebug("[Persistence] 会话已保存: %s", sessionID)
	return nil
}

// LoadSession 从 Redis 加载会话
func (sm *SessionManager) LoadSession(sessionID string) error {
	key := sessionKeyPrefix + sessionID

	// 从 Redis 读取
	jsonData, err := sm.redisClient.Get(sm.ctx, key).Result()
	if err == redis.Nil {
		return fmt.Errorf("会话不存在: %s", sessionID)
	} else if err != nil {
		return fmt.Errorf("从 Redis 读取会话失败: %w", err)
	}

	// 反序列化
	var data SessionData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return fmt.Errorf("反序列化会话失败: %w", err)
	}

	// 为会话创建独立的调度器
	scheduler, err := NewScheduler("config.yaml")
	if err != nil {
		return fmt.Errorf("创建调度器失败: %w", err)
	}

	// 从编排器获取模式实例
	mode, err := sm.orchestrator.registry.GetOrCreate(data.ModeName, data.ModeConfig)
	if err != nil {
		return fmt.Errorf("创建协作模式失败: %w", err)
	}

	// 重建会话上下文
	ctx := &SessionContext{
		ID:           data.ID,
		Name:         data.Name,
		Summary:      data.Summary,
		CreatedAt:    data.CreatedAt,
		UpdatedAt:    data.UpdatedAt,
		MessageCount: data.MessageCount,
		Scheduler:    scheduler,
		Messages:     data.Messages,
		CallHistory:  data.CallHistory,
		JoinedCats:   data.JoinedCats,
		Mode:         mode,
		ModeConfig:   data.ModeConfig,
		ModeState:    data.ModeState,
	}

	sm.mu.Lock()
	sm.sessions[sessionID] = ctx
	sm.mu.Unlock()

	// 在编排器中注册会话
	if err := sm.orchestrator.CreateSession(sessionID, data.ModeName, data.ModeConfig); err != nil {
		return fmt.Errorf("在编排器中注册会话失败: %w", err)
	}

	LogInfo("[Persistence] 会话已加载: %s", sessionID)
	return nil
}

// LoadAllSessions 从 Redis 加载所有会话
func (sm *SessionManager) LoadAllSessions() error {
	// 获取所有会话 ID
	sessionIDs, err := sm.redisClient.SMembers(sm.ctx, sessionListKey).Result()
	if err != nil {
		return fmt.Errorf("获取会话列表失败: %w", err)
	}

	LogInfo("[Persistence] 开始加载 %d 个会话", len(sessionIDs))

	successCount := 0
	for _, sessionID := range sessionIDs {
		if err := sm.LoadSession(sessionID); err != nil {
			LogError("[Persistence] 加载会话失败 %s: %v", sessionID, err)
			continue
		}
		successCount++
	}

	LogInfo("[Persistence] 成功加载 %d/%d 个会话", successCount, len(sessionIDs))
	return nil
}

// DeleteSessionFromRedis 从 Redis 删除会话
func (sm *SessionManager) DeleteSessionFromRedis(sessionID string) error {
	key := sessionKeyPrefix + sessionID

	// 删除会话数据
	if err := sm.redisClient.Del(sm.ctx, key).Err(); err != nil {
		return fmt.Errorf("删除会话数据失败: %w", err)
	}

	// 从会话列表中移除
	if err := sm.redisClient.SRem(sm.ctx, sessionListKey, sessionID).Err(); err != nil {
		return fmt.Errorf("从会话列表移除失败: %w", err)
	}

	LogDebug("[Persistence] 会话已从 Redis 删除: %s", sessionID)
	return nil
}

// AutoSaveSession 自动保存会话（在消息更新后调用）
func (sm *SessionManager) AutoSaveSession(sessionID string) {
	go func() {
		if err := sm.SaveSession(sessionID); err != nil {
			LogError("[Persistence] 自动保存会话失败 %s: %v", sessionID, err)
		}
	}()
}
