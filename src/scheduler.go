package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"gopkg.in/yaml.v3"
)

// AgentConfig Agent 配置结构
type AgentConfig struct {
	Name             string `yaml:"name"`
	Pipe             string `yaml:"pipe"`
	ExecCmd          string `yaml:"exec_cmd"`
	SystemPromptPath string `yaml:"system_prompt_path"`
	Avatar           string `yaml:"avatar"`
}

// Config 系统配置
type Config struct {
	Agents []AgentConfig `yaml:"agents"`
	Redis  RedisConfig   `yaml:"redis"`
	User   UserConfig    `yaml:"user"`
}

// UserConfig 用户配置
type UserConfig struct {
	Avatar string `yaml:"avatar"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// TaskMessage 任务消息结构
type TaskMessage struct {
	TaskID      string                 `json:"task_id"`
	AgentName   string                 `json:"agent_name"`
	Content     string                 `json:"content"`
	Result      string                 `json:"result,omitempty"`     // Agent 执行结果
	SessionID   string                 `json:"session_id,omitempty"` // 关联的会话 ID
	RetryCount  int                    `json:"retry_count"`
	MaxRetries  int                    `json:"max_retries"`
	CreatedAt   time.Time              `json:"created_at"`
	Status      string                 `json:"status"` // pending, processing, completed, failed
	Timestamp   time.Time              `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// AgentState Agent 状态
type AgentState struct {
	Name       string
	Status     string // idle, busy
	LastTaskID string
	UpdatedAt  time.Time
}

// ChatRecord 聊天记录
type ChatRecord struct {
	Timestamp time.Time `json:"timestamp"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Content   string    `json:"content"`
}

// Scheduler 调度器
type Scheduler struct {
	config        *Config
	redisClient   *redis.Client
	ctx           context.Context
	agents        map[string]*AgentConfig
	agentStates   map[string]*AgentState
	systemPrompts map[string]string
	chatLogFile   string
}

// NewScheduler 创建新的调度器
func NewScheduler(configPath string) (*Scheduler, error) {
	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Redis.Addr,
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
	})

	ctx := context.Background()

	// 测试 Redis 连接
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
	}

	scheduler := &Scheduler{
		config:        &config,
		redisClient:   rdb,
		ctx:           ctx,
		agents:        make(map[string]*AgentConfig),
		agentStates:   make(map[string]*AgentState),
		systemPrompts: make(map[string]string),
		chatLogFile:   "chat_history.jsonl",
	}

	// 注册所有 Agent
	if err := scheduler.registerAgents(); err != nil {
		return nil, err
	}

	return scheduler, nil
}

// registerAgents 注册所有 Agent
func (s *Scheduler) registerAgents() error {
	for i := range s.config.Agents {
		agent := &s.config.Agents[i]

		// 读取系统提示词
		promptData, err := os.ReadFile(agent.SystemPromptPath)
		if err != nil {
			return fmt.Errorf("读取 Agent %s 的系统提示词失败: %w", agent.Name, err)
		}

		s.agents[agent.Name] = agent
		s.systemPrompts[agent.Name] = string(promptData)
		s.agentStates[agent.Name] = &AgentState{
			Name:      agent.Name,
			Status:    "idle",
			UpdatedAt: time.Now(),
		}

		fmt.Printf("✓ 注册 Agent: %s (管道: %s)\n", agent.Name, agent.Pipe)
	}

	return nil
}

// SendTask 发送任务到指定 Agent
func (s *Scheduler) SendTask(agentName, content, sessionID string) (string, error) {
	return s.SendTaskFrom("铲屎官", agentName, content, sessionID)
}

// SendTaskFrom 从指定发送者发送任务到指定 Agent
func (s *Scheduler) SendTaskFrom(from, agentName, content, sessionID string) (string, error) {
	LogDebug("[Scheduler] 准备发送任务 - From: %s, To: %s, Content: %s, SessionID: %s", from, agentName, content, sessionID)

	agent, exists := s.agents[agentName]
	if !exists {
		LogError("[Scheduler] Agent 不存在: %s, 可用的 Agents: %v", agentName, s.getAgentNames())
		return "", fmt.Errorf("Agent %s 不存在", agentName)
	}

	LogDebug("[Scheduler] 找到 Agent: %s, Pipe: %s", agentName, agent.Pipe)

	// 记录聊天
	s.logChat(from, agentName, content)

	// 生成任务 ID
	taskID := fmt.Sprintf("task_%s_%d", agentName, time.Now().UnixNano())
	LogDebug("[Scheduler] 生成任务 ID: %s", taskID)

	// 创建任务消息
	task := TaskMessage{
		TaskID:     taskID,
		AgentName:  agentName,
		Content:    content,
		SessionID:  sessionID,
		RetryCount: 0,
		MaxRetries: 3,
		CreatedAt:  time.Now(),
		Status:     "pending",
	}

	// 序列化任务
	taskData, err := json.Marshal(task)
	if err != nil {
		LogError("[Scheduler] 序列化任务失败: %v", err)
		return "", fmt.Errorf("序列化任务失败: %w", err)
	}

	// 发送到 Redis Stream
	streamKey := fmt.Sprintf("pipe:%s", agent.Pipe)
	LogInfo("[Scheduler] 发送任务到 Redis Stream: %s", streamKey)
	_, err = s.redisClient.XAdd(s.ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"task": string(taskData),
		},
	}).Result()

	if err != nil {
		LogError("[Scheduler] 发送任务到 Redis Stream 失败: %v", err)
		return "", fmt.Errorf("发送任务到 Redis Stream 失败: %w", err)
	}

	LogInfo("[Scheduler] ✓ 任务已发送: %s -> %s (管道: %s)", taskID, agentName, agent.Pipe)
	return taskID, nil
}

// getAgentNames 获取所有 Agent 名称（用于调试）
func (s *Scheduler) getAgentNames() []string {
	names := make([]string, 0, len(s.agents))
	for name := range s.agents {
		names = append(names, name)
	}
	return names
}

// GetAgentState 获取 Agent 状态
func (s *Scheduler) GetAgentState(agentName string) (*AgentState, error) {
	state, exists := s.agentStates[agentName]
	if !exists {
		return nil, fmt.Errorf("Agent %s 不存在", agentName)
	}
	return state, nil
}

// UpdateAgentState 更新 Agent 状态
func (s *Scheduler) UpdateAgentState(agentName, status, taskID string) error {
	state, exists := s.agentStates[agentName]
	if !exists {
		return fmt.Errorf("Agent %s 不存在", agentName)
	}

	state.Status = status
	state.LastTaskID = taskID
	state.UpdatedAt = time.Now()

	return nil
}

// ListAgents 列出所有 Agent
func (s *Scheduler) ListAgents() []*AgentConfig {
	agents := make([]*AgentConfig, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, agent)
	}
	return agents
}

// GetSystemPrompt 获取 Agent 的系统提示词
func (s *Scheduler) GetSystemPrompt(agentName string) (string, error) {
	prompt, exists := s.systemPrompts[agentName]
	if !exists {
		return "", fmt.Errorf("Agent %s 不存在", agentName)
	}
	return prompt, nil
}

// Close 关闭调度器
func (s *Scheduler) Close() error {
	return s.redisClient.Close()
}

// logChat 记录聊天到文件
func (s *Scheduler) logChat(from, to, content string) {
	record := ChatRecord{
		Timestamp: time.Now(),
		From:      from,
		To:        to,
		Content:   content,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	f, err := os.OpenFile(s.chatLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(string(data) + "\n")
}
