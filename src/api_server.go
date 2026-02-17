package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gopkg.in/yaml.v2"
)

// SessionManager 管理所有会话
type SessionManager struct {
	sessions    map[string]*SessionContext
	mu          sync.RWMutex
	config      *Config
	redisClient *redis.Client
	ctx         context.Context
	cancel      context.CancelFunc
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
}

// SendMessageRequest 发送消息请求
type SendMessageRequest struct {
	Content       string   `json:"content"`
	MentionedCats []string `json:"mentionedCats"`
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

	sm := &SessionManager{
		sessions:    make(map[string]*SessionContext),
		config:      config,
		redisClient: rdb,
		ctx:         ctx,
		cancel:      cancel,
	}

	// 启动结果监听器
	go sm.listenForResults()

	return sm, nil
}

// CreateSession 创建新会话
func (sm *SessionManager) CreateSession() (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sessionID := fmt.Sprintf("sess_%s", uuid.New().String()[:8])

	// 为每个会话创建独立的调度器
	scheduler, err := NewScheduler("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("创建调度器失败: %w", err)
	}

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
	}

	sm.sessions[sessionID] = ctx

	// 添加系统欢迎消息
	welcomeMsg := Message{
		ID:        fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
		Type:      "system",
		Content:   "会话已创建，猫猫们已就位！",
		Timestamp: time.Now(),
		SessionID: sessionID,
	}
	ctx.Messages = append(ctx.Messages, welcomeMsg)

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

	// 如果有提及的猫猫，发送任务
	if len(req.MentionedCats) > 0 {
		// 创建 ID 到名字的映射
		catIDToName := map[string]string{
			"cat_001": "花花",
			"cat_002": "薇薇",
			"cat_003": "小乔",
		}

		for _, catID := range req.MentionedCats {
			catName, ok := catIDToName[catID]
			if !ok {
				LogWarn("[API] 未知的猫猫 ID: %s", catID)
				continue
			}

			LogInfo("[API] 处理猫猫提及 - ID: %s, Name: %s", catID, catName)

			// 只在猫猫第一次加入时添加系统消息
			if !ctx.JoinedCats[catID] {
				systemMsg := Message{
					ID:        fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
					Type:      "system",
					Content:   fmt.Sprintf("%s 已加入对话", catName),
					Timestamp: time.Now(),
					SessionID: sessionID,
				}
				ctx.Messages = append(ctx.Messages, systemMsg)
				ctx.JoinedCats[catID] = true // 标记该猫猫已加入
				LogDebug("[API] 已添加系统消息: %s", systemMsg.ID)
			} else {
				LogDebug("[API] 猫猫 %s 已在会话中，跳过系统消息", catName)
			}

			// 记录调用历史
			ctx.CallHistory = append(ctx.CallHistory, CallHistoryItem{
				CatID:     catID,
				CatName:   catName,
				SessionID: sessionID,
				Timestamp: time.Now(),
			})
			LogDebug("[API] 已记录调用历史 - Cat: %s", catName)

			// 发送任务到调度器
			go func(id, name string) {
				LogInfo("[API] 准备发送任务到调度器 - Cat: %s (ID: %s)", name, id)
				taskID, err := ctx.Scheduler.SendTask(name, req.Content, sessionID)
				if err != nil {
					LogError("[API] 发送任务失败 - Cat: %s, Error: %v", name, err)
				} else {
					LogInfo("[API] 任务已发送 - Cat: %s, TaskID: %s", name, taskID)
				}
			}(catID, catName)
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

		LogDebug("[API] 添加猫猫: %s, Avatar: %s", agent.Name, agent.Avatar)

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
	session, err := sm.CreateSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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

// SetupRouter 设置路由
func (sm *SessionManager) SetupRouter() *gin.Engine {
	r := gin.Default()

	// CORS 中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// 静态文件服务 - 提供头像图片
	r.Static("/images", "./images")

	api := r.Group("/api")
	{
		// 会话管理
		api.GET("/sessions", sm.handleGetSessions)
		api.POST("/sessions", sm.handleCreateSession)
		api.GET("/sessions/:sessionId", sm.handleGetSession)
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
		return fmt.Errorf("无效的任务数据")
	}

	var task TaskMessage
	if err := json.Unmarshal([]byte(taskData), &task); err != nil {
		return fmt.Errorf("解析任务失败: %w", err)
	}

	LogInfo("[API] 📥 收到 Agent 结果 - SessionID: %s, Agent: %s", task.SessionID, task.AgentName)
	LogDebug("[API] 结果内容: %s", task.Result)

	// 查找对应的会话
	sm.mu.RLock()
	ctx, exists := sm.sessions[task.SessionID]
	sm.mu.RUnlock()

	if !exists {
		LogWarn("[API] 会话不存在: %s", task.SessionID)
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

	// 解析回复中的 @ 调用，记录到调用历史
	sm.parseAndRecordCalls(ctx, task.Result, task.SessionID)

	return nil
}

// parseAndRecordCalls 解析回复中的 @ 调用并记录到调用历史
func (sm *SessionManager) parseAndRecordCalls(ctx *SessionContext, content string, sessionID string) {
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 检查是否包含 @标记
		if !strings.HasPrefix(line, "@") {
			continue
		}

		// 解析格式: @Agent 任务内容
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}

		targetAgent := strings.TrimPrefix(parts[0], "@")

		// 跳过 @铲屎官
		if targetAgent == "铲屎官" {
			continue
		}

		// 获取猫猫 ID
		catID := getCatIDByName(targetAgent)
		if catID == "cat_unknown" {
			continue
		}

		// 只在猫猫第一次被调用时添加系统消息
		if !ctx.JoinedCats[catID] {
			systemMsg := Message{
				ID:        fmt.Sprintf("msg_%s", uuid.New().String()[:8]),
				Type:      "system",
				Content:   fmt.Sprintf("%s 已加入对话", targetAgent),
				Timestamp: time.Now(),
				SessionID: sessionID,
			}
			ctx.Messages = append(ctx.Messages, systemMsg)
			ctx.JoinedCats[catID] = true
			LogDebug("[API] 猫猫互相调用 - 已添加系统消息: %s", systemMsg.ID)
		}

		// 记录调用历史
		ctx.CallHistory = append(ctx.CallHistory, CallHistoryItem{
			CatID:     catID,
			CatName:   targetAgent,
			SessionID: sessionID,
			Timestamp: time.Now(),
		})
		LogDebug("[API] 猫猫互相调用 - 已记录调用历史: %s", targetAgent)
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
