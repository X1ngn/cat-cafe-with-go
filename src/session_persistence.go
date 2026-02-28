package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// SessionData 可序列化的会话数据（不含消息，消息由 Session Chain 管理）
type SessionData struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Summary      string            `json:"summary"`
	WorkspaceID  string            `json:"workspaceId"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
	MessageCount int               `json:"messageCount"`
	CallHistory  []CallHistoryItem `json:"callHistory"`
	JoinedCats   map[string]bool   `json:"joinedCats"`
	ModeName     string            `json:"modeName"`
	ModeConfig   *ModeConfig       `json:"modeConfig"`
	ModeState    *ModeState        `json:"modeState"`
}

const (
	sessionKeyPrefix = "session:"
	sessionListKey   = "sessions:list"
)

// SaveSession 保存会话到 Redis（仅元数据，不含消息）
func (sm *SessionManager) SaveSession(sessionID string) error {
	LogDebug("[Persistence] 开始保存会话: %s", sessionID)

	sm.mu.RLock()
	ctx, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	modeName := "free_discussion"
	if ctx.Mode != nil {
		modeName = ctx.Mode.GetName()
	}

	data := SessionData{
		ID:           ctx.ID,
		Name:         ctx.Name,
		Summary:      ctx.Summary,
		WorkspaceID:  ctx.WorkspaceID,
		CreatedAt:    ctx.CreatedAt,
		UpdatedAt:    ctx.UpdatedAt,
		MessageCount: ctx.MessageCount,
		CallHistory:  ctx.CallHistory,
		JoinedCats:   ctx.JoinedCats,
		ModeName:     modeName,
		ModeConfig:   ctx.ModeConfig,
		ModeState:    ctx.ModeState,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化会话失败: %w", err)
	}

	key := sessionKeyPrefix + sessionID
	if err := sm.redisClient.Set(sm.ctx, key, jsonData, 0).Err(); err != nil {
		return fmt.Errorf("保存会话到 Redis 失败: %w", err)
	}

	if err := sm.redisClient.SAdd(sm.ctx, sessionListKey, sessionID).Err(); err != nil {
		return fmt.Errorf("添加会话到列表失败: %w", err)
	}

	LogDebug("[Persistence] 会话已保存: %s", sessionID)
	return nil
}

// LoadSession 从 Redis 加载会话（消息从 Session Chain 按需加载）
func (sm *SessionManager) LoadSession(sessionID string) error {
	key := sessionKeyPrefix + sessionID

	jsonData, err := sm.redisClient.Get(sm.ctx, key).Result()
	if err == redis.Nil {
		return fmt.Errorf("会话不存在: %s", sessionID)
	} else if err != nil {
		return fmt.Errorf("从 Redis 读取会话失败: %w", err)
	}

	var data SessionData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return fmt.Errorf("反序列化会话失败: %w", err)
	}

	scheduler, err := NewScheduler("config.yaml")
	if err != nil {
		return fmt.Errorf("创建调度器失败: %w", err)
	}

	mode, err := sm.orchestrator.registry.GetOrCreate(data.ModeName, data.ModeConfig)
	if err != nil {
		return fmt.Errorf("创建协作模式失败: %w", err)
	}

	ctx := &SessionContext{
		ID:             data.ID,
		Name:           data.Name,
		Summary:        data.Summary,
		WorkspaceID:    data.WorkspaceID,
		CreatedAt:      data.CreatedAt,
		UpdatedAt:      data.UpdatedAt,
		MessageCount:   data.MessageCount,
		Scheduler:      scheduler,
		SystemMessages: []Message{},
		CallHistory:    data.CallHistory,
		JoinedCats:     data.JoinedCats,
		Mode:           mode,
		ModeConfig:     data.ModeConfig,
		ModeState:      data.ModeState,
	}

	sm.mu.Lock()
	sm.sessions[sessionID] = ctx
	sm.mu.Unlock()

	if err := sm.orchestrator.CreateSession(sessionID, data.ModeName, data.ModeConfig); err != nil {
		return fmt.Errorf("在编排器中注册会话失败: %w", err)
	}

	LogInfo("[Persistence] 会话已加载: %s", sessionID)
	return nil
}

// LoadAllSessions 从 Redis 加载所有会话
func (sm *SessionManager) LoadAllSessions() error {
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

	if err := sm.redisClient.Del(sm.ctx, key).Err(); err != nil {
		return fmt.Errorf("删除会话数据失败: %w", err)
	}

	if err := sm.redisClient.SRem(sm.ctx, sessionListKey, sessionID).Err(); err != nil {
		return fmt.Errorf("从会话列表移除失败: %w", err)
	}

	// 联动删除 Session Chain 数据
	if sm.chainManager != nil {
		if err := sm.chainManager.DeleteChain(sessionID); err != nil {
			LogWarn("[Persistence] 删除 Session Chain 失败（非致命）: %v", err)
		} else {
			LogDebug("[Persistence] Session Chain 已删除: %s", sessionID)
		}
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
