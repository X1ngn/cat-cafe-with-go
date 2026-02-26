package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v2"
)

// SessionManager 管理所有会话
type SessionManager struct {
	sessions         map[string]*SessionContext
	mu               sync.RWMutex
	config           *Config
	redisClient      *redis.Client
	ctx              context.Context
	cancel           context.CancelFunc
	orchestrator     *Orchestrator      // 新增：编排器
	wsHub            *WSHub             // 新增：WebSocket Hub
	workspaceManager *WorkspaceManager  // 新增：工作区管理器
}

// SessionContext 会话上下文，每个会话有独立的调度器
type SessionContext struct {
	ID            string
	Name          string
	Summary       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	MessageCount  int
	Scheduler     *Scheduler
	Messages      []Message
	CallHistory   []CallHistoryItem
	JoinedCats    map[string]bool // 记录已加入的猫猫，避免重复显示系统消息
	Mode          CollaborationMode // 新增：当前协作模式
	ModeConfig    *ModeConfig       // 新增：模式配置
	ModeState     *ModeState        // 新增：模式状态
	WorkspaceID   string            // 新增：关联的工作区 ID
	mu            sync.RWMutex
}

// Message 消息结构
type Message struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"` // cat, user, system
	Content   string      `json:"content"`
	Sender    *Sender     `json:"sender,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	SessionID string      `json:"sessionId"`
}

// Sender 发送者信息
type Sender struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Color  string `json:"color,omitempty"`
}

// Cat 猫猫信息
type Cat struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Color  string `json:"color"`
	Status string `json:"status"` // idle, busy, offline
}

// Session 会话信息
type Session struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Summary      string    `json:"summary"`
	UpdatedAt    time.Time `json:"updatedAt"`
	MessageCount int       `json:"messageCount"`
}

// MessageStats 消息统计
type MessageStats struct {
	TotalMessages int `json:"totalMessages"`
	CatMessages   int `json:"catMessages"`
}

// CallHistoryItem 调用历史项
type CallHistoryItem struct {
	CatID     string    `json:"catId"`
	CatName   string    `json:"catName"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Prompt    string    `json:"prompt"`    // 调用时的提示词
	Response  string    `json:"response"`  // 猫猫的回复
}

// SendMessageRequest 发送消息请求
type SendMessageRequest struct {
	Content       string   `json:"content"`
	MentionedCats []string `json:"mentionedCats"`
}

// SwitchModeRequest 切换模式请求
type SwitchModeRequest struct {
	Mode       string                 `json:"mode"`
	ModeConfig map[string]interface{} `json:"modeConfig,omitempty"`
}

// NewSessionManager 创建会话管理器
func NewSessionManager(configPath string) (*SessionManager, error) {
	// 读取配置
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Redis.Addr,
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// 测试 Redis 连接
	if err := rdb.Ping(ctx).Err(); err != nil {
		cancel()
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
	}

	// 创建一个临时调度器用于编排器（编排器需要调度器来发送任务）
	// 注意：每个会话仍然有自己的调度器
	tempScheduler, err := NewScheduler(configPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("创建调度器失败: %w", err)
	}

	// 创建编排器，默认使用自由讨论模式
	orchestrator := NewOrchestrator(tempScheduler, "free_discussion")
	orchestrator.SetAgentConfigs(config.Agents)

	// 创建 WebSocket Hub
	wsHub := NewWSHub()
	go wsHub.Run()

	// 创建工作区管理器
	workspaceManager := NewWorkspaceManager(rdb, ctx)

	sm := &SessionManager{
		sessions:         make(map[string]*SessionContext),
		config:           config,
		redisClient:      rdb,
		ctx:              ctx,
		cancel:           cancel,
		orchestrator:     orchestrator,
		wsHub:            wsHub,
		workspaceManager: workspaceManager,
	}

	// 启动结果监听器
	go sm.listenForResults()

	// 从 Redis 加载已有的会话
	if err := sm.LoadAllSessions(); err != nil {
		LogWarn("[API] 加载会话失败: %v", err)
	}

	return sm, nil
}

// CreateSession 创建新会话
func (sm *SessionManager) CreateSession() (*Session, error) {
	LogDebug("[API] 开始创建会话")
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sessionID := fmt.Sprintf("sess_%s", uuid.New().String()[:8])
	LogDebug("[API] 生成会话 ID: %s", sessionID)

	// 为每个会话创建独立的调度器
	LogDebug("[API] 创建调度器: %s", sessionID)
	scheduler, err := NewScheduler("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("创建调度器失败: %w", err)
	}

	// 创建默认模式配置
	LogDebug("[API] 创建模式配置: %s", sessionID)
	modeConfig := &ModeConfig{
		Name:    "free_discussion",
		Enabled: true,
	}

	// 从编排器获取模式实例
	LogDebug("[API] 获取模式实例: %s", sessionID)
	mode, err := sm.orchestrator.registry.GetOrCreate("free_discussion", modeConfig)
	if err != nil {
		return nil, fmt.Errorf("创建协作模式失败: %w", err)
	}

	LogDebug("[API] 创建会话上下文: %s", sessionID)
	ctx := &SessionContext{
		ID:           sessionID,
		Name:         "新对话",
		Summary:      "",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		MessageCount: 0,
		Scheduler:    scheduler,
		Messages:     make([]Message, 0),
		CallHistory:  make([]CallHistoryItem, 0),
		JoinedCats:   make(map[string]bool), // 初始化已加入猫猫的映射
		Mode:         mode,
		ModeConfig:   modeConfig,
		ModeState: &ModeState{
			CustomState:    make(map[string]interface{}),
			LastUpdateTime: time.Now(),
		},
	}

	sm.sessions[sessionID] = ctx

	// 在编排器中注册会话
	LogDebug("[API] 在编排器中注册会话: %s", sessionID)
	if err := sm.orchestrator.CreateSession(sessionID, "free_discussion", modeConfig); err != nil {
		delete(sm.sessions, sessionID)
		return nil, fmt.Errorf("在编排器中注册会话失败: %w", err)
	}

	// 添加系统欢迎消息
	LogDebug("[API] 添加欢迎消息: %s", sessionID)
	welcomeMsg := Message{
		ID:        fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
		Type:      "system",
		Content:   "会话已创建，当前模式：自由讨论",
		Timestamp: time.Now(),
		SessionID: sessionID,
	}
	ctx.Messages = append(ctx.Messages, welcomeMsg)

	// 先解锁，再保存会话到 Redis（避免死锁）
	sm.mu.Unlock()
	LogDebug("[API] 保存会话到 Redis: %s", sessionID)
	if err := sm.SaveSession(sessionID); err != nil {
		LogError("[API] 保存新会话失败: %v", err)
	}
	sm.mu.Lock() // 重新加锁以便 defer 正常解锁

	LogDebug("[API] 会话创建完成: %s", sessionID)
	return &Session{
		ID:           ctx.ID,
		Name:         ctx.Name,
		Summary:      ctx.Summary,
		UpdatedAt:    ctx.UpdatedAt,
		MessageCount: ctx.MessageCount,
	}, nil
}

// GetSession 获取会话
func (sm *SessionManager) GetSession(sessionID string) (*Session, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	ctx, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("会话不存在")
	}

	return &Session{
		ID:           ctx.ID,
		Name:         ctx.Name,
		Summary:      ctx.Summary,
		UpdatedAt:    ctx.UpdatedAt,
		MessageCount: ctx.MessageCount,
	}, nil
}

// UpdateSessionName 更新会话名称
func (sm *SessionManager) UpdateSessionName(sessionID string, name string) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ctx, exists := sm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("会话不存在")
	}

	// 更新名称和时间戳
	ctx.Name = name
	ctx.UpdatedAt = time.Now()

	// 保存到 Redis
	sm.mu.Unlock()
	if err := sm.SaveSession(sessionID); err != nil {
		LogError("[API] 保存重命名后的会话失败: %v", err)
	}
	sm.mu.Lock()

	return &Session{
		ID:           ctx.ID,
		Name:         ctx.Name,
		Summary:      ctx.Summary,
		UpdatedAt:    ctx.UpdatedAt,
		MessageCount: ctx.MessageCount,
	}, nil
}

// ListSessions 列出所有会话
func (sm *SessionManager) ListSessions() []Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]Session, 0, len(sm.sessions))
	for _, ctx := range sm.sessions {
		sessions = append(sessions, Session{
			ID:           ctx.ID,
			Name:         ctx.Name,
			Summary:      ctx.Summary,
			UpdatedAt:    ctx.UpdatedAt,
			MessageCount: ctx.MessageCount,
		})
	}

	return sessions
}

// DeleteSession 删除会话
func (sm *SessionManager) DeleteSession(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	ctx, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("会话不存在")
	}

	// 关闭调度器
	ctx.Scheduler.Close()

	// 从编排器中删除会话
	sm.orchestrator.DeleteSession(sessionID)

	// 从 Redis 删除会话
	if err := sm.DeleteSessionFromRedis(sessionID); err != nil {
		LogError("[API] 从 Redis 删除会话失败: %v", err)
	}

	delete(sm.sessions, sessionID)

	return nil
}

// GetMessages 获取会话消息
func (sm *SessionManager) GetMessages(sessionID string) ([]Message, error) {
	sm.mu.RLock()
	ctx, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("会话不存在")
	}

	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	return ctx.Messages, nil
}

// SendMessage 发送消息
func (sm *SessionManager) SendMessage(sessionID string, req SendMessageRequest) (*Message, error) {
	LogDebug("[API] 收到发送消息请求 - SessionID: %s, Content: %s, MentionedCats: %v",
		sessionID, req.Content, req.MentionedCats)

	sm.mu.RLock()
	ctx, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		LogError("[API] 会话不存在: %s", sessionID)
		return nil, fmt.Errorf("会话不存在")
	}

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	// 添加用户消息
	userMsg := Message{
		ID:      fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
		Type:    "user",
		Content: req.Content,
		Sender: &Sender{
			ID:     "user_001",
			Name:   "用户",
			Avatar: sm.config.User.Avatar,
		},
		Timestamp: time.Now(),
		SessionID: sessionID,
	}
	ctx.Messages = append(ctx.Messages, userMsg)
	ctx.MessageCount++
	ctx.UpdatedAt = time.Now()
	LogDebug("[API] 已添加用户消息: %s", userMsg.ID)

	// 通过 WebSocket 推送新消息
	sm.wsHub.BroadcastToSession(sessionID, "message", userMsg)

	// 自动保存会话
	sm.AutoSaveSession(sessionID)

	// 如果有提及的猫猫，通过编排器处理
	if len(req.MentionedCats) > 0 {
		// 将猫猫 ID 转换为名称
		catIDToName := map[string]string{
			"cat_001": "花花",
			"cat_002": "薇薇",
			"cat_003": "小乔",
		}

		mentionedNames := make([]string, 0, len(req.MentionedCats))
		for _, catID := range req.MentionedCats {
			if name, ok := catIDToName[catID]; ok {
				mentionedNames = append(mentionedNames, name)
			}
		}

		// 通过编排器处理用户消息
		calls, err := sm.orchestrator.HandleUserMessage(sessionID, req.Content, mentionedNames)
		if err != nil {
			LogError("[API] 编排器处理用户消息失败: %v", err)
			return nil, fmt.Errorf("处理消息失败: %w", err)
		}

		LogInfo("[API] 编排器返回 %d 个猫猫调用", len(calls))

		// 处理每个调用
		for _, call := range calls {
			catID := getCatIDByName(call.AgentName)

			// 只在猫猫第一次加入时添加系统消息
			if !ctx.JoinedCats[catID] {
				systemMsg := Message{
					ID:        fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
					Type:      "system",
					Content:   fmt.Sprintf("%s 已加入对话", call.AgentName),
					Timestamp: time.Now(),
					SessionID: sessionID,
				}
				ctx.Messages = append(ctx.Messages, systemMsg)
				ctx.JoinedCats[catID] = true
				LogDebug("[API] 已添加系统消息: %s", systemMsg.ID)

				// 通过 WebSocket 推送系统消息
				sm.wsHub.BroadcastToSession(sessionID, "message", systemMsg)
			} else {
				LogDebug("[API] 猫猫 %s 已在会话中，跳过系统消息", call.AgentName)
			}

			// 记录调用历史
			ctx.CallHistory = append(ctx.CallHistory, CallHistoryItem{
				CatID:     catID,
				CatName:   call.AgentName,
				SessionID: sessionID,
				Timestamp: time.Now(),
				Prompt:    call.Prompt,
				Response:  "", // 回复稍后在 handleResult 中更新
			})
			LogDebug("[API] 已记录调用历史 - Cat: %s", call.AgentName)

			// 通过 WebSocket 推送调用历史更新
			sm.wsHub.BroadcastToSession(sessionID, "history", ctx.CallHistory)

			// 发送任务到调度器
			go func(agentCall AgentCall) {
				LogInfo("[API] 准备发送任务到调度器 - Cat: %s", agentCall.AgentName)
				taskID, err := ctx.Scheduler.SendTask(agentCall.AgentName, agentCall.Prompt, sessionID)
				if err != nil {
					LogError("[API] 发送任务失败 - Cat: %s, Error: %v", agentCall.AgentName, err)
				} else {
					LogInfo("[API] 任务已发送 - Cat: %s, TaskID: %s", agentCall.AgentName, taskID)
				}
			}(call)
		}
	}

	// 更新摘要
	if ctx.Summary == "" && len(req.Content) > 0 {
		summary := req.Content
		if len(summary) > 30 {
			summary = summary[:30] + "..."
		}
		ctx.Summary = fmt.Sprintf("用户：%s", summary)
	}

	LogInfo("[API] 消息发送完成 - MessageID: %s", userMsg.ID)
	return &userMsg, nil
}

// GetMessageStats 获取消息统计
func (sm *SessionManager) GetMessageStats(sessionID string) (*MessageStats, error) {
	sm.mu.RLock()
	ctx, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("会话不存在")
	}

	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	catMessages := 0
	for _, msg := range ctx.Messages {
		if msg.Type == "cat" {
			catMessages++
		}
	}

	return &MessageStats{
		TotalMessages: len(ctx.Messages),
		CatMessages:   catMessages,
	}, nil
}

// GetCallHistory 获取调用历史
func (sm *SessionManager) GetCallHistory(sessionID string) ([]CallHistoryItem, error) {
	sm.mu.RLock()
	ctx, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("会话不存在")
	}

	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	return ctx.CallHistory, nil
}

// GetCats 获取所有猫猫
func (sm *SessionManager) GetCats() []Cat {
	// 从配置文件构建猫猫列表
	cats := make([]Cat, 0, len(sm.config.Agents))

	LogDebug("[API] 配置中的 Agent 数量: %d", len(sm.config.Agents))

	catIDMap := map[string]string{
		"花花": "cat_001",
		"薇薇": "cat_002",
		"小乔": "cat_003",
	}

	catColorMap := map[string]string{
		"花花": "#ff9966",
		"薇薇": "#d9bf99",
		"小乔": "#cccccc",
	}

	for _, agent := range sm.config.Agents {
		catID := catIDMap[agent.Name]
		color := catColorMap[agent.Name]

		LogDebug("[API] aaaa添加猫猫: %s, Avatar: %s", agent.Name, agent.Avatar)

		cats = append(cats, Cat{
			ID:     catID,
			Name:   agent.Name,
			Avatar: agent.Avatar,
			Color:  color,
			Status: "idle",
		})
	}

	LogDebug("[API] 返回猫猫列表，数量: %d", len(cats))

	return cats
}

// API 路由处理函数

func (sm *SessionManager) handleGetSessions(c *gin.Context) {
	sessions := sm.ListSessions()
	c.JSON(http.StatusOK, sessions)
}

func (sm *SessionManager) handleCreateSession(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		WorkspaceID string `json:"workspace_id"` // 可选的工作区 ID
	}
	// 使用 ShouldBindJSON 而不是 BindJSON，允许空 body
	_ = c.ShouldBindJSON(&req)

	session, err := sm.CreateSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 如果提供了 workspace_id，更新会话
	if req.WorkspaceID != "" {
		sm.mu.Lock()
		if ctx, exists := sm.sessions[session.ID]; exists {
			ctx.WorkspaceID = req.WorkspaceID
		}
		sm.mu.Unlock()

		// 保存到 Redis
		sm.AutoSaveSession(session.ID)
	}

	// 如果提供了名称，更新会话名称
	if req.Name != "" {
		session, _ = sm.UpdateSessionName(session.ID, req.Name)
	} else {
		// 即使没有提供名称，也要重新获取会话以包含工作区信息
		session, _ = sm.GetSession(session.ID)
	}

	c.JSON(http.StatusOK, session)
}

func (sm *SessionManager) handleGetSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	session, err := sm.GetSession(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

func (sm *SessionManager) handleUpdateSession(c *gin.Context) {
	sessionID := c.Param("sessionId")

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
		return
	}

	session, err := sm.UpdateSessionName(sessionID, req.Name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

func (sm *SessionManager) handleDeleteSession(c *gin.Context) {
	sessionID := c.Param("sessionId")
	err := sm.DeleteSession(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (sm *SessionManager) handleGetMessages(c *gin.Context) {
	sessionID := c.Param("sessionId")
	messages, err := sm.GetMessages(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, messages)
}

func (sm *SessionManager) handleSendMessage(c *gin.Context) {
	sessionID := c.Param("sessionId")

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	message, err := sm.SendMessage(sessionID, req)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, message)
}

func (sm *SessionManager) handleGetMessageStats(c *gin.Context) {
	sessionID := c.Param("sessionId")
	stats, err := sm.GetMessageStats(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (sm *SessionManager) handleGetCats(c *gin.Context) {
	cats := sm.GetCats()
	c.JSON(http.StatusOK, cats)
}

func (sm *SessionManager) handleGetCat(c *gin.Context) {
	catID := c.Param("catId")
	cats := sm.GetCats()

	for _, cat := range cats {
		if cat.ID == catID {
			c.JSON(http.StatusOK, cat)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "猫猫不存在"})
}

func (sm *SessionManager) handleGetAvailableCats(c *gin.Context) {
	cats := sm.GetCats()
	available := make([]Cat, 0)

	for _, cat := range cats {
		if cat.Status == "idle" {
			available = append(available, cat)
		}
	}

	c.JSON(http.StatusOK, available)
}

func (sm *SessionManager) handleGetCallHistory(c *gin.Context) {
	sessionID := c.Param("sessionId")
	history, err := sm.GetCallHistory(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, history)
}

// handleGetModes 获取所有可用模式
func (sm *SessionManager) handleGetModes(c *gin.Context) {
	modes := sm.orchestrator.ListModes()
	c.JSON(http.StatusOK, modes)
}

// handleGetSessionMode 获取会话当前模式
func (sm *SessionManager) handleGetSessionMode(c *gin.Context) {
	sessionID := c.Param("sessionId")

	sm.mu.RLock()
	ctx, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}

	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"mode":        ctx.Mode.GetName(),
		"description": ctx.Mode.GetDescription(),
		"config":      ctx.ModeConfig,
		"state":       ctx.ModeState,
	})
}

// handleSwitchMode 切换会话模式
func (sm *SessionManager) handleSwitchMode(c *gin.Context) {
	sessionID := c.Param("sessionId")

	var req SwitchModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sm.mu.RLock()
	ctx, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}

	// 创建模式配置
	modeConfig := &ModeConfig{
		Name:    req.Mode,
		Enabled: true,
		Config:  req.ModeConfig,
	}

	// 通过编排器切换模式
	if err := sm.orchestrator.SwitchMode(sessionID, req.Mode, modeConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新会话上下文
	ctx.mu.Lock()
	mode, _ := sm.orchestrator.registry.GetOrCreate(req.Mode, modeConfig)
	ctx.Mode = mode
	ctx.ModeConfig = modeConfig
	ctx.ModeState = &ModeState{
		CustomState:    make(map[string]interface{}),
		LastUpdateTime: time.Now(),
	}
	ctx.mu.Unlock()

	// 添加系统消息
	ctx.mu.Lock()
	systemMsg := Message{
		ID:        fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
		Type:      "system",
		Content:   fmt.Sprintf("协作模式已切换为：%s", mode.GetDescription()),
		Timestamp: time.Now(),
		SessionID: sessionID,
	}
	ctx.Messages = append(ctx.Messages, systemMsg)
	ctx.mu.Unlock()

	// 自动保存会话
	sm.AutoSaveSession(sessionID)

	c.JSON(http.StatusOK, gin.H{
		"mode":        mode.GetName(),
		"description": mode.GetDescription(),
	})
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，生产环境应该限制
	},
}

// handleWebSocket 处理 WebSocket 连接
func (sm *SessionManager) handleWebSocket(c *gin.Context) {
	sessionID := c.Param("sessionId")

	// 检查会话是否存在
	sm.mu.RLock()
	_, exists := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}

	// 升级 HTTP 连接为 WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		LogError("[WS] 升级连接失败: %v", err)
		return
	}

	// 创建客户端
	client := &WSClient{
		conn:      conn,
		send:      make(chan WSMessage, 256),
		sessionID: sessionID,
	}

	// 注册客户端
	sm.wsHub.register <- client

	// 启动读写协程
	go client.writePump()
	go client.readPump(sm.wsHub)

	LogInfo("[WS] WebSocket 连接已建立 - SessionID: %s", sessionID)
}

// SetupRouter 设置路由
func (sm *SessionManager) SetupRouter() *gin.Engine {
	r := gin.Default()

	// 禁用自动重定向
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	// CORS 中间件
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		LogDebug("[CORS] Request from origin: %s, method: %s, path: %s", origin, c.Request.Method, c.Request.URL.Path)

		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			LogDebug("[CORS] Handling OPTIONS preflight request")
			c.AbortWithStatus(204)
			return
		}

		c.Next()

		LogDebug("[CORS] Response headers: %v", c.Writer.Header())
	})

	// 静态文件服务 - 提供头像图片
	r.Static("/images", "./images")

	api := r.Group("/api")
	{
		// WebSocket 连接
		api.GET("/sessions/:sessionId/ws", sm.handleWebSocket)

		// 会话管理（支持带/不带尾部斜杠）
		api.GET("/sessions", sm.handleGetSessions)
		api.GET("/sessions/", sm.handleGetSessions)
		api.POST("/sessions", sm.handleCreateSession)
		api.POST("/sessions/", sm.handleCreateSession)
		api.GET("/sessions/:sessionId", sm.handleGetSession)
		api.PUT("/sessions/:sessionId", sm.handleUpdateSession)
		api.DELETE("/sessions/:sessionId", sm.handleDeleteSession)

		// 消息管理
		api.GET("/sessions/:sessionId/messages", sm.handleGetMessages)
		api.POST("/sessions/:sessionId/messages", sm.handleSendMessage)
		api.GET("/sessions/:sessionId/stats", sm.handleGetMessageStats)

		// 猫猫管理
		api.GET("/cats", sm.handleGetCats)
		api.GET("/cats/:catId", sm.handleGetCat)
		api.GET("/cats/available", sm.handleGetAvailableCats)

		// 调用历史
		api.GET("/sessions/:sessionId/history", sm.handleGetCallHistory)

		// 模式管理
		api.GET("/modes", sm.handleGetModes)
		api.GET("/sessions/:sessionId/mode", sm.handleGetSessionMode)
		api.PUT("/sessions/:sessionId/mode", sm.handleSwitchMode)

		// 工作区管理
		api.GET("/workspaces", sm.handleGetWorkspaces)
		api.POST("/workspaces", sm.handleCreateWorkspace)
		api.GET("/workspaces/:workspaceId", sm.handleGetWorkspace)
		api.PUT("/workspaces/:workspaceId", sm.handleUpdateWorkspace)
		api.DELETE("/workspaces/:workspaceId", sm.handleDeleteWorkspace)

		// 部署管理
		api.POST("/workspaces/:workspaceId/deploy-test", sm.handleDeployToTest)
		api.POST("/deployments/:deploymentId/promote", sm.handlePromoteToProduction)
		api.GET("/deployments/:deploymentId", sm.handleGetDeployment)
		api.GET("/workspaces/:workspaceId/deployments", sm.handleGetDeployments)
	}

	return r
}

// loadConfig 加载配置（简化版）
func loadConfig(path string) (*Config, error) {
	// 读取配置文件
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &config, nil
}

// listenForResults 监听 Agent 返回的结果
func (sm *SessionManager) listenForResults() {
	resultStreamKey := "results:stream"
	consumerGroup := "api-server"
	consumerName := fmt.Sprintf("consumer-%d", os.Getpid())

	// 创建消费者组
	sm.redisClient.XGroupCreateMkStream(sm.ctx, resultStreamKey, consumerGroup, "0").Err()

	LogInfo("[API] 开始监听结果队列: %s", resultStreamKey)

	for {
		select {
		case <-sm.ctx.Done():
			LogInfo("[API] 结果监听器已停止")
			return
		default:
			// 从消费者组读取消息
			streams, err := sm.redisClient.XReadGroup(sm.ctx, &redis.XReadGroupArgs{
				Group:    consumerGroup,
				Consumer: consumerName,
				Streams:  []string{resultStreamKey, ">"},
				Count:    1,
				Block:    1 * time.Second,
			}).Result()

			if err != nil {
				if err != redis.Nil {
					LogError("[API] 读取结果队列失败: %v", err)
				}
				continue
			}

			// 处理每条消息
			for _, stream := range streams {
				for _, message := range stream.Messages {
					if err := sm.handleResult(message); err != nil {
						LogError("[API] 处理结果失败: %v", err)
					} else {
						// 确认消息
						sm.redisClient.XAck(sm.ctx, resultStreamKey, consumerGroup, message.ID)
					}
				}
			}
		}
	}
}

// handleResult 处理单个结果消息
func (sm *SessionManager) handleResult(message redis.XMessage) error {
	taskData, ok := message.Values["task"].(string)
	if !ok {
		LogError("[API] message task处理结果失败")
		return fmt.Errorf("无效的任务数据")
	}

	var task TaskMessage
	if err := json.Unmarshal([]byte(taskData), &task); err != nil {
		LogError("[API] Unmarshal处理结果失败: %v", err)
		return fmt.Errorf("解析任务失败: %w", err)
	}

	LogInfo("[API] 📥 收到 Agent 结果 - SessionID: %s, Agent: %s", task.SessionID, task.AgentName)
	LogDebug("[API] 结果内容: %s", task.Result)

	// 查找对应的会话
	sm.mu.RLock()
	ctx, exists := sm.sessions[task.SessionID]
	sm.mu.RUnlock()

	if !exists {
		// 打印当前所有会话 ID 用于调试
		sm.mu.RLock()
		sessionIDs := make([]string, 0, len(sm.sessions))
		for sid := range sm.sessions {
			sessionIDs = append(sessionIDs, sid)
		}
		sm.mu.RUnlock()
		LogWarn("[API] 会话不存在: %s, 当前会话列表: %v", task.SessionID, sessionIDs)
		return fmt.Errorf("会话不存在: %s", task.SessionID)
	}

	// 添加 Agent 回复消息
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	agentMsg := Message{
		ID:        fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
		Type:      "cat",
		Content:   task.Result,
		Timestamp: time.Now(),
		SessionID: task.SessionID,
		Sender:    sm.getCatInfoByName(task.AgentName),
	}

	ctx.Messages = append(ctx.Messages, agentMsg)
	ctx.MessageCount++
	ctx.UpdatedAt = time.Now()

	LogInfo("[API] ✓ Agent 消息已添加 - MessageID: %s, Agent: %s", agentMsg.ID, task.AgentName)

	// 通过 WebSocket 推送猫猫消息
	sm.wsHub.BroadcastToSession(task.SessionID, "message", agentMsg)

	// 更新调用历史中的 Response
	sm.updateCallHistoryResponse(ctx, task.AgentName, task.Result)

	// 通过 WebSocket 推送调用历史更新
	sm.wsHub.BroadcastToSession(task.SessionID, "history", ctx.CallHistory)

	// 自动保存会话
	sm.AutoSaveSession(task.SessionID)

	// 通过编排器处理猫猫回复，获取下一步需要调用的猫猫
	calls, err := sm.orchestrator.HandleAgentResponse(task.SessionID, task.AgentName, task.Result)
	if err != nil {
		LogError("[API] 编排器处理猫猫回复失败: %v", err)
		return fmt.Errorf("处理猫猫回复失败: %w", err)
	}

	LogInfo("[API] 编排器返回 %d 个后续猫猫调用", len(calls))

	// 处理每个后续调用
	for _, call := range calls {
		catID := getCatIDByName(call.AgentName)

		// 只在猫猫第一次被调用时添加系统消息
		if !ctx.JoinedCats[catID] {
			systemMsg := Message{
				ID:        fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
				Type:      "system",
				Content:   fmt.Sprintf("%s 已加入对话", call.AgentName),
				Timestamp: time.Now(),
				SessionID: task.SessionID,
			}
			ctx.Messages = append(ctx.Messages, systemMsg)
			ctx.JoinedCats[catID] = true
			LogDebug("[API] 猫猫互相调用 - 已添加系统消息: %s", systemMsg.ID)

			// 通过 WebSocket 推送系统消息
			sm.wsHub.BroadcastToSession(task.SessionID, "message", systemMsg)
		}

		// 记录调用历史
		ctx.CallHistory = append(ctx.CallHistory, CallHistoryItem{
			CatID:     catID,
			CatName:   call.AgentName,
			SessionID: task.SessionID,
			Timestamp: time.Now(),
			Prompt:    call.Prompt,
			Response:  "", // 回复稍后更新
		})
		LogDebug("[API] 猫猫互相调用 - 已记录调用历史: %s", call.AgentName)

		// 通过 WebSocket 推送调用历史更新
		sm.wsHub.BroadcastToSession(task.SessionID, "history", ctx.CallHistory)

		// 发送任务到调度器
		go func(agentCall AgentCall) {
			LogInfo("[API] 猫猫互相调用 - 准备发送任务: %s", agentCall.AgentName)
			taskID, err := ctx.Scheduler.SendTask(agentCall.AgentName, agentCall.Prompt, task.SessionID)
			if err != nil {
				LogError("[API] 猫猫互相调用 - 发送任务失败: %s, Error: %v", agentCall.AgentName, err)
			} else {
				LogInfo("[API] 猫猫互相调用 - 任务已发送: %s, TaskID: %s", agentCall.AgentName, taskID)
			}
		}(call)
	}

	return nil
}

// updateCallHistoryResponse 更新调用历史中的 Response
func (sm *SessionManager) updateCallHistoryResponse(ctx *SessionContext, catName string, response string) {
	// 从后往前查找最近一次该猫猫的调用记录（Response 为空的）
	for i := len(ctx.CallHistory) - 1; i >= 0; i-- {
		if ctx.CallHistory[i].CatName == catName && ctx.CallHistory[i].Response == "" {
			ctx.CallHistory[i].Response = response
			LogDebug("[API] 已更新调用历史 Response - Cat: %s", catName)
			break
		}
	}
}

// getCatIDByName 根据猫猫名字获取 ID
func getCatIDByName(name string) string {
	catMap := map[string]string{
		"花花": "cat_001",
		"薇薇": "cat_002",
		"小乔": "cat_003",
	}
	if id, ok := catMap[name]; ok {
		return id
	}
	return "cat_unknown"
}

// getCatInfoByName 根据猫猫名字获取完整信息
func (sm *SessionManager) getCatInfoByName(name string) *Sender {
	catIDMap := map[string]string{
		"花花": "cat_001",
		"薇薇": "cat_002",
		"小乔": "cat_003",
	}

	catColorMap := map[string]string{
		"花花": "#ff9966",
		"薇薇": "#d9bf99",
		"小乔": "#cccccc",
	}

	// 从配置中查找头像
	avatar := ""
	for _, agent := range sm.config.Agents {
		if agent.Name == name {
			avatar = agent.Avatar
			break
		}
	}

	return &Sender{
		ID:     catIDMap[name],
		Name:   name,
		Avatar: avatar,
		Color:  catColorMap[name],
	}
}

// 工作区 API 处理函数

func (sm *SessionManager) handleGetWorkspaces(c *gin.Context) {
	workspaces := sm.workspaceManager.ListWorkspaces()
	c.JSON(http.StatusOK, workspaces)
}

func (sm *SessionManager) handleCreateWorkspace(c *gin.Context) {
	var req struct {
		Path string        `json:"path" binding:"required"`
		Type WorkspaceType `json:"type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	workspace, err := sm.workspaceManager.CreateWorkspace(req.Path, req.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, workspace)
}

func (sm *SessionManager) handleGetWorkspace(c *gin.Context) {
	workspaceID := c.Param("workspaceId")
	workspace, err := sm.workspaceManager.GetWorkspace(workspaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, workspace)
}

func (sm *SessionManager) handleUpdateWorkspace(c *gin.Context) {
	workspaceID := c.Param("workspaceId")

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	workspace, err := sm.workspaceManager.UpdateWorkspace(workspaceID, updates)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, workspace)
}

func (sm *SessionManager) handleDeleteWorkspace(c *gin.Context) {
	workspaceID := c.Param("workspaceId")
	err := sm.workspaceManager.DeleteWorkspace(workspaceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (sm *SessionManager) handleDeployToTest(c *gin.Context) {
	workspaceID := c.Param("workspaceId")

	deployment, err := sm.workspaceManager.DeployToTest(workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, deployment)
}

func (sm *SessionManager) handlePromoteToProduction(c *gin.Context) {
	deploymentID := c.Param("deploymentId")

	err := sm.workspaceManager.PromoteToProduction(deploymentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	deployment, _ := sm.workspaceManager.GetDeployment(deploymentID)
	c.JSON(http.StatusOK, deployment)
}

func (sm *SessionManager) handleGetDeployment(c *gin.Context) {
	deploymentID := c.Param("deploymentId")

	deployment, err := sm.workspaceManager.GetDeployment(deploymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, deployment)
}

func (sm *SessionManager) handleGetDeployments(c *gin.Context) {
	workspaceID := c.Param("workspaceId")

	deployments := sm.workspaceManager.ListDeployments(workspaceID)
	c.JSON(http.StatusOK, deployments)
}
