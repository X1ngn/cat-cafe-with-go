package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
)

// AgentWorker Agent 工作进程
type AgentWorker struct {
	config        *AgentConfig
	systemPrompt  string
	redisClient   *redis.Client
	ctx           context.Context
	cancel        context.CancelFunc
	streamKey     string
	consumerGroup string
	consumerName  string
	chatLogFile   string
}

// NewAgentWorker 创建 Agent 工作进程
func NewAgentWorker(config *AgentConfig, systemPrompt string, redisAddr, redisPassword string, redisDB int) (*AgentWorker, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// 测试 Redis 连接
	if err := rdb.Ping(ctx).Err(); err != nil {
		cancel()
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
	}

	streamKey := fmt.Sprintf("pipe:%s", config.Pipe)
	consumerGroup := fmt.Sprintf("group:%s", config.Name)
	consumerName := fmt.Sprintf("consumer:%s:%d", config.Name, os.Getpid())

	worker := &AgentWorker{
		config:        config,
		systemPrompt:  systemPrompt,
		redisClient:   rdb,
		ctx:           ctx,
		cancel:        cancel,
		streamKey:     streamKey,
		consumerGroup: consumerGroup,
		consumerName:  consumerName,
		chatLogFile:   "chat_history.jsonl",
	}

	// 创建消费者组
	if err := worker.createConsumerGroup(); err != nil {
		cancel()
		return nil, err
	}

	return worker, nil
}

// createConsumerGroup 创建消费者组
func (w *AgentWorker) createConsumerGroup() error {
	// 尝试创建消费者组，如果已存在则忽略错误
	err := w.redisClient.XGroupCreateMkStream(w.ctx, w.streamKey, w.consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("创建消费者组失败: %w", err)
	}
	return nil
}

// Start 启动 Agent 工作进程
func (w *AgentWorker) Start() error {
	fmt.Printf("🐱 Agent %s 启动 (管道: %s)\n", w.config.Name, w.config.Pipe)
	fmt.Printf("   监听: %s\n", w.streamKey)
	fmt.Printf("   消费者组: %s\n", w.consumerGroup)
	fmt.Printf("   消费者: %s\n", w.consumerName)
	fmt.Println()

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Printf("\n🛑 Agent %s 收到停止信号\n", w.config.Name)
		w.cancel()
	}()

	// 主循环
	for {
		select {
		case <-w.ctx.Done():
			fmt.Printf("✓ Agent %s 已停止\n", w.config.Name)
			return nil
		default:
			if err := w.processMessages(); err != nil {
				fmt.Fprintf(os.Stderr, "处理消息失败: %v\n", err)
				time.Sleep(1 * time.Second)
			}
		}
	}
}

// processMessages 处理消息
func (w *AgentWorker) processMessages() error {
	// 从消费者组读取消息
	streams, err := w.redisClient.XReadGroup(w.ctx, &redis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerName,
		Streams:  []string{w.streamKey, ">"},
		Count:    1,
		Block:    1 * time.Second,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil // 没有新消息
		}
		return err
	}

	// 处理每条消息
	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := w.handleMessage(message); err != nil {
				fmt.Fprintf(os.Stderr, "处理消息 %s 失败: %v\n", message.ID, err)
				// 重试逻辑
				w.retryMessage(message)
			} else {
				// 确认消息
				w.redisClient.XAck(w.ctx, w.streamKey, w.consumerGroup, message.ID)
			}
		}
	}

	return nil
}

// handleMessage 处理单条消息
func (w *AgentWorker) handleMessage(message redis.XMessage) error {
	taskData, ok := message.Values["task"].(string)
	if !ok {
		return fmt.Errorf("无效的任务数据")
	}

	var task TaskMessage
	if err := json.Unmarshal([]byte(taskData), &task); err != nil {
		return fmt.Errorf("解析任务失败: %w", err)
	}

	fmt.Printf("📥 Agent %s 收到任务: %s\n", w.config.Name, task.TaskID)
	fmt.Printf("   内容: %s\n", task.Content)

	// 更新状态为 processing
	task.Status = "processing"

	// 执行任务
	startTime := time.Now()
	result, err := w.executeTask(&task)
	duration := time.Since(startTime)

	if err != nil {
		task.Status = "failed"
		fmt.Fprintf(os.Stderr, "❌ 任务执行失败: %v (耗时: %v)\n", err, duration)
		return err
	}

	task.Status = "completed"
	fmt.Printf("✓ 任务完成: %s (耗时: %v)\n", task.TaskID, duration)
	fmt.Printf("   结果: %s\n", result)
	fmt.Println()

	// 解析输出中的 @标记，触发后续任务
	if err := w.parseAndDispatchTasks(result); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  解析后续任务失败: %v\n", err)
	}

	return nil
}

// executeTask 执行任务
func (w *AgentWorker) executeTask(task *TaskMessage) (string, error) {
	// 组合系统提示词和用户内容
	fullPrompt := fmt.Sprintf("%s\n\n---\n\n用户需求：\n%s", w.systemPrompt, task.Content)

	// 执行命令
	cmd := exec.CommandContext(w.ctx, w.config.ExecCmd, "-p", fullPrompt)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("执行命令失败: %w, 输出: %s", err, string(output))
	}

	return string(output), nil
}

// parseAndDispatchTasks 解析输出中的 @标记并分发任务
func (w *AgentWorker) parseAndDispatchTasks(output string) error {
	lines := strings.Split(output, "\n")

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
		taskContent := strings.TrimSpace(parts[1])

		if taskContent == "" {
			continue
		}

		// 特殊处理 @铲屎官
		if targetAgent == "铲屎官" {
			fmt.Printf("📢 %s 完成工作，等待用户输入\n", w.config.Name)
			fmt.Printf("   消息: %s\n", taskContent)
			// 留给后续扩展
			continue
		}

		// 发送任务到其他 Agent
		if err := w.sendTaskToAgent(targetAgent, taskContent); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  发送任务到 %s 失败: %v\n", targetAgent, err)
			continue
		}

		// 记录聊天
		w.logChat(w.config.Name, targetAgent, taskContent)

		fmt.Printf("🔄 %s 调用 %s\n", w.config.Name, targetAgent)
		fmt.Printf("   任务: %s\n", taskContent)
	}

	return nil
}

// sendTaskToAgent 发送任务到指定 Agent
func (w *AgentWorker) sendTaskToAgent(agentName, taskContent string) error {
	// 查询 Agent 配置
	configKey := "config:agents"
	agentsData, err := w.redisClient.Get(w.ctx, configKey).Result()
	if err != nil {
		// 如果 Redis 中没有配置，尝试从本地加载
		return w.sendTaskByPipeName(agentName, taskContent)
	}

	// 解析配置
	var agents []AgentConfig
	if err := json.Unmarshal([]byte(agentsData), &agents); err != nil {
		return w.sendTaskByPipeName(agentName, taskContent)
	}

	// 查找目标 Agent
	var targetPipe string
	for _, agent := range agents {
		if agent.Name == agentName {
			targetPipe = agent.Pipe
			break
		}
	}

	if targetPipe == "" {
		return fmt.Errorf("Agent %s 不存在", agentName)
	}

	// 创建任务
	task := TaskMessage{
		TaskID:     generateTaskID(),
		Content:    taskContent,
		Status:     "pending",
		CreatedAt:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	// 发送到 Redis
	taskData, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("序列化任务失败: %w", err)
	}

	streamKey := fmt.Sprintf("pipe:%s", targetPipe)
	_, err = w.redisClient.XAdd(w.ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"task": string(taskData),
		},
	}).Result()

	return err
}

// sendTaskByPipeName 通过管道名发送任务（备用方法）
func (w *AgentWorker) sendTaskByPipeName(agentName, taskContent string) error {
	// 简单映射：Agent名 -> 管道名
	pipeMap := map[string]string{
		"花花": "pipe_huahua",
		"薇薇": "pipe_weiwei",
		"小乔": "pipe_xiaoqiao",
	}

	targetPipe, exists := pipeMap[agentName]
	if !exists {
		return fmt.Errorf("未知的 Agent: %s", agentName)
	}

	// 创建任务
	task := TaskMessage{
		TaskID:     generateTaskID(),
		Content:    taskContent,
		Status:     "pending",
		CreatedAt:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	// 发送到 Redis
	taskData, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("序列化任务失败: %w", err)
	}

	streamKey := fmt.Sprintf("pipe:%s", targetPipe)
	_, err = w.redisClient.XAdd(w.ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"task": string(taskData),
		},
	}).Result()

	return err
}

// generateTaskID 生成任务 ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}

// retryMessage 重试消息
func (w *AgentWorker) retryMessage(message redis.XMessage) {
	taskData, ok := message.Values["task"].(string)
	if !ok {
		return
	}

	var task TaskMessage
	if err := json.Unmarshal([]byte(taskData), &task); err != nil {
		return
	}

	task.RetryCount++

	if task.RetryCount >= task.MaxRetries {
		fmt.Fprintf(os.Stderr, "❌ 任务 %s 重试次数已达上限，放弃\n", task.TaskID)
		w.redisClient.XAck(w.ctx, w.streamKey, w.consumerGroup, message.ID)
		return
	}

	fmt.Printf("🔄 重试任务 %s (第 %d 次)\n", task.TaskID, task.RetryCount)

	// 重新发送任务
	retryTaskData, _ := json.Marshal(task)
	w.redisClient.XAdd(w.ctx, &redis.XAddArgs{
		Stream: w.streamKey,
		Values: map[string]interface{}{
			"task": string(retryTaskData),
		},
	})

	// 确认原消息
	w.redisClient.XAck(w.ctx, w.streamKey, w.consumerGroup, message.ID)
}

// Stop 停止 Agent
func (w *AgentWorker) Stop() {
	w.cancel()
	w.redisClient.Close()
}

// logChat 记录聊天到文件
func (w *AgentWorker) logChat(from, to, content string) {
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
	f, err := os.OpenFile(w.chatLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(string(data) + "\n")
}
