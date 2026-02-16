package test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"gopkg.in/yaml.v3"
)

// 从主包复制必要的类型定义

// AgentConfig Agent 配置结构
type AgentConfig struct {
	Name             string `yaml:"name"`
	Pipe             string `yaml:"pipe"`
	ExecCmd          string `yaml:"exec_cmd"`
	SystemPromptPath string `yaml:"system_prompt_path"`
}

// Config 系统配置
type Config struct {
	Agents []AgentConfig `yaml:"agents"`
	Redis  RedisConfig   `yaml:"redis"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// TaskMessage 任务消息结构
type TaskMessage struct {
	TaskID      string    `json:"task_id"`
	AgentName   string    `json:"agent_name"`
	Content     string    `json:"content"`
	RetryCount  int       `json:"retry_count"`
	MaxRetries  int       `json:"max_retries"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
}

// AgentState Agent 状态
type AgentState struct {
	Name       string
	Status     string
	LastTaskID string
	UpdatedAt  time.Time
}

// Scheduler 调度器
type Scheduler struct {
	config        *Config
	redisClient   *redis.Client
	ctx           context.Context
	agents        map[string]*AgentConfig
	agentStates   map[string]*AgentState
	systemPrompts map[string]string
}

// NewScheduler 创建新的调度器
func NewScheduler(configPath string) (*Scheduler, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Redis.Addr,
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
	})

	ctx := context.Background()

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
	}

	if err := scheduler.registerAgents(); err != nil {
		return nil, err
	}

	return scheduler, nil
}

// registerAgents 注册所有 Agent
func (s *Scheduler) registerAgents() error {
	for i := range s.config.Agents {
		agent := &s.config.Agents[i]

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
	}

	return nil
}

// SendTask 发送任务到指定 Agent
func (s *Scheduler) SendTask(agentName, content string) (string, error) {
	agent, exists := s.agents[agentName]
	if !exists {
		return "", fmt.Errorf("Agent %s 不存在", agentName)
	}

	taskID := fmt.Sprintf("task_%s_%d", agentName, time.Now().UnixNano())

	task := TaskMessage{
		TaskID:     taskID,
		AgentName:  agentName,
		Content:    content,
		RetryCount: 0,
		MaxRetries: 3,
		CreatedAt:  time.Now(),
		Status:     "pending",
	}

	taskData, err := json.Marshal(task)
	if err != nil {
		return "", fmt.Errorf("序列化任务失败: %w", err)
	}

	streamKey := fmt.Sprintf("pipe:%s", agent.Pipe)
	_, err = s.redisClient.XAdd(s.ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"task": string(taskData),
		},
	}).Result()

	if err != nil {
		return "", fmt.Errorf("发送任务到 Redis Stream 失败: %w", err)
	}

	return taskID, nil
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
