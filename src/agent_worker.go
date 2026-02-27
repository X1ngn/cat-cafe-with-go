package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
)

// AgentWorker Agent 工作进程
type AgentWorker struct {
	config           *AgentConfig
	systemPrompt     string
	redisClient      *redis.Client
	ctx              context.Context
	cancel           context.CancelFunc
	streamKey        string
	consumerGroup    string
	consumerName     string
	chatLogFile      string
	workspaceManager *WorkspaceManager // 新增：工作区管理器
}

// NewAgentWorker 创建 Agent 工作进程
func NewAgentWorker(config *AgentConfig, systemPrompt string, redisAddr, redisPassword string, redisDB int, workspaceManager *WorkspaceManager) (*AgentWorker, error) {
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
		config:           config,
		systemPrompt:     systemPrompt,
		redisClient:      rdb,
		ctx:              ctx,
		cancel:           cancel,
		streamKey:        streamKey,
		consumerGroup:    consumerGroup,
		consumerName:     consumerName,
		chatLogFile:      "chat_history.jsonl",
		workspaceManager: workspaceManager, // 新增：注入工作区管理器
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
	LogInfo("[Agent-%s] 启动 (管道: %s)", w.config.Name, w.config.Pipe)
	LogInfo("[Agent-%s] 监听: %s", w.config.Name, w.streamKey)
	LogInfo("[Agent-%s] 消费者组: %s", w.config.Name, w.consumerGroup)
	LogInfo("[Agent-%s] 消费者: %s", w.config.Name, w.consumerName)

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		LogInfo("[Agent-%s] 收到停止信号", w.config.Name)
		w.cancel()
	}()

	// 主循环
	for {
		select {
		case <-w.ctx.Done():
			LogInfo("[Agent-%s] 已停止", w.config.Name)
			return nil
		default:
			if err := w.processMessages(); err != nil {
				LogError("[Agent-%s] 处理消息失败: %v", w.config.Name, err)
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
	LogDebug("[Agent-%s] 收到 Redis 消息: %s", w.config.Name, message.ID)

	taskData, ok := message.Values["task"].(string)
	if !ok {
		LogError("[Agent-%s] 无效的任务数据", w.config.Name)
		return fmt.Errorf("无效的任务数据")
	}

	var task TaskMessage
	if err := json.Unmarshal([]byte(taskData), &task); err != nil {
		LogError("[Agent-%s] 解析任务失败: %v", w.config.Name, err)
		return fmt.Errorf("解析任务失败: %w", err)
	}

	LogInfo("[Agent-%s] 📥 收到任务: %s", w.config.Name, task.TaskID)
	LogInfo("[Agent-%s] 任务内容: %s", w.config.Name, task.Content)

	// 更新状态为 processing
	task.Status = "processing"

	// 执行任务
	startTime := time.Now()
	result, err := w.executeTask(&task)
	duration := time.Since(startTime)

	if err != nil {
		task.Status = "failed"
		LogError("[Agent-%s] ❌ 任务执行失败: %v (耗时: %v)", w.config.Name, err, duration)
		return err
	}

	task.Status = "completed"
	LogInfo("[Agent-%s] ✓ 任务完成: %s (耗时: %v)", w.config.Name, task.TaskID, duration)
	LogDebug("[Agent-%s] 任务结果: %s", w.config.Name, result)

	// 将结果发送回结果队列
	if err := w.sendResult(&task, result); err != nil {
		LogError("[Agent-%s] 发送结果失败: %v", w.config.Name, err)
	}

	// 解析输出中的 @标记，触发后续任务
	if err := w.parseAndDispatchTasks(result, &task); err != nil {
		LogWarn("[Agent-%s] 解析后续任务失败: %v", w.config.Name, err)
	}

	return nil
}

// executeTask 执行任务
func (w *AgentWorker) executeTask(task *TaskMessage) (string, error) {
	LogDebug("[Agent-%s] 开始执行任务: %s", w.config.Name, task.TaskID)
	LogDebug("[Agent-%s] CLI 类型: %s", w.config.Name, w.config.CLIType)

	// 查询工作区路径
	var workDir string
	if task.WorkspaceID != "" {
		workspace, err := w.workspaceManager.GetWorkspace(task.WorkspaceID)
		if err != nil {
			LogWarn("[Agent-%s] 获取工作区失败: %v", w.config.Name, err)
		} else {
			workDir = workspace.Path
			LogInfo("[Agent-%s] 工作目录: %s", w.config.Name, workDir)
		}
	}

	// 获取会话历史消息
	chatHistory := w.getSessionHistory(task.SessionID)

	// 组合系统提示词、对话历史和当前任务
	var fullPrompt string
	if chatHistory != "" {
		fullPrompt = fmt.Sprintf("%s\n\n========================================\n\n【对话历史】\n%s\n========================================\n\n🎯 你是%s，请回应以下消息：\n%s\n\n请结合上面的对话历史来完成任务。", w.systemPrompt, chatHistory, w.config.Name, task.Content)
	} else {
		fullPrompt = fmt.Sprintf("%s\n\n========================================\n\n用户需求：\n%s", w.systemPrompt, task.Content)
	}

	// 打印完整 prompt 到日志，用于调试上下文超长问题
	// LogInfo("[Agent-%s] === fullPrompt 长度: %d 字符 ===", w.config.Name, len(fullPrompt))
	// LogInfo("[Agent-%s] === fullPrompt 内容开始 ===\n%s\n=== fullPrompt 内容结束 ===", w.config.Name, fullPrompt)

	// 从 Redis 获取 AI Session ID 映射
	var aiSessionID string
	if task.SessionID != "" {
		mappingKey := fmt.Sprintf("session_mapping:%s:%s", task.SessionID, w.config.Name)
		aiSessionID, _ = w.redisClient.Get(w.ctx, mappingKey).Result()
		if aiSessionID != "" {
			LogDebug("[Agent-%s] 找到 AI Session ID 映射: %s -> %s", w.config.Name, task.SessionID, aiSessionID)
		} else {
			LogDebug("[Agent-%s] 未找到 AI Session ID 映射，将创建新会话", w.config.Name)
		}
	}

	// 直接调用 InvokeAgent，不再通过 exec.Command 调用 minimal-* 二进制
	response, newSessionID, err := InvokeAgent(w.config.CLIType, fullPrompt, aiSessionID, workDir)
	if err != nil {
		LogError("[Agent-%s] 调用 CLI 失败: %v", w.config.Name, err)
		return "", fmt.Errorf("调用 %s CLI 失败: %w", w.config.CLIType, err)
	}

	LogDebug("[Agent-%s] CLI 返回 - response长度: %d, newSessionID: %s", w.config.Name, len(response), newSessionID)

	// 保存新的 AI Session ID 映射到 Redis
	if newSessionID != "" && newSessionID != aiSessionID && task.SessionID != "" {
		mappingKey := fmt.Sprintf("session_mapping:%s:%s", task.SessionID, w.config.Name)
		if err := w.redisClient.Set(w.ctx, mappingKey, newSessionID, 0).Err(); err != nil {
			LogWarn("[Agent-%s] 保存 AI Session ID 映射失败: %v", w.config.Name, err)
		} else {
			LogInfo("[Agent-%s] ✓ 已保存 AI Session ID 映射: %s -> %s", w.config.Name, task.SessionID, newSessionID)
		}
	}

	return response, nil
}

// getSessionHistory 从 Redis 获取会话历史消息并格式化
func (w *AgentWorker) getSessionHistory(sessionID string) string {
	if sessionID == "" {
		return ""
	}

	key := "session:" + sessionID
	jsonData, err := w.redisClient.Get(w.ctx, key).Result()
	if err != nil {
		LogDebug("[Agent-%s] 获取会话历史失败: %v", w.config.Name, err)
		return ""
	}

	var data SessionData
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		LogWarn("[Agent-%s] 解析会话历史失败: %v", w.config.Name, err)
		return ""
	}

	if len(data.Messages) == 0 {
		return ""
	}

	// 只取最近 20 条消息，避免上下文过长
	const maxMessages = 20
	// 每条猫猫消息最多保留 500 字符，避免长回复撑爆上下文
	const maxContentLen = 500

	messages := data.Messages
	if len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}

	// 格式化历史消息，带角色标注
	var history strings.Builder
	for _, msg := range messages {
		content := msg.Content
		switch msg.Type {
		case "user":
			name := "用户"
			if msg.Sender != nil {
				name = msg.Sender.Name
			}
			history.WriteString(fmt.Sprintf("[%s] %s\n", name, content))
		case "cat":
			name := "猫猫"
			if msg.Sender != nil {
				name = msg.Sender.Name
			}
			// 猫猫回复可能很长，做截断
			if len(content) > maxContentLen {
				content = content[:maxContentLen] + "...(已截断)"
			}
			history.WriteString(fmt.Sprintf("[%s] %s\n", name, content))
		case "system":
			history.WriteString(fmt.Sprintf("[系统] %s\n", msg.Content))
		}
	}

	LogDebug("[Agent-%s] 已加载 %d/%d 条历史消息", w.config.Name, len(messages), len(data.Messages))
	return strings.TrimSpace(history.String())
}

// sendResult 将任务结果发送到结果队列
func (w *AgentWorker) sendResult(task *TaskMessage, result string) error {
	// 如果没有 SessionID，不发送结果
	if task.SessionID == "" {
		LogDebug("[Agent-%s] 任务没有 SessionID，跳过发送结果", w.config.Name)
		return nil
	}

	// 更新任务结果
	task.Result = result
	task.Status = "completed"

	// 序列化任务
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("序列化任务失败: %w", err)
	}

	// 发送到结果队列
	resultStreamKey := "results:stream"
	_, err = w.redisClient.XAdd(w.ctx, &redis.XAddArgs{
		Stream: resultStreamKey,
		Values: map[string]interface{}{
			"task": string(taskJSON),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("发送结果到 Redis 失败: %w", err)
	}

	LogInfo("[Agent-%s] ✓ 结果已发送到队列: %s", w.config.Name, resultStreamKey)
	return nil
}

// parseAndDispatchTasks 解析输出中的 @标记并分发任务
func (w *AgentWorker) parseAndDispatchTasks(output string, currentTask *TaskMessage) error {
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

		// 跳过自己调用自己，防止无限循环
		if targetAgent == w.config.Name {
			continue
		}

		// 特殊处理 @铲屎官
		if targetAgent == "铲屎官" {
			fmt.Printf("📢 %s 完成工作，等待用户输入\n", w.config.Name)
			fmt.Printf("   消息: %s\n", taskContent)
			// 留给后续扩展
			continue
		}

		// 发送任务到其他 Agent，传递 SessionID
		if err := w.sendTaskToAgent(targetAgent, taskContent, currentTask.SessionID); err != nil {
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
func (w *AgentWorker) sendTaskToAgent(agentName, taskContent, sessionID string) error {
	// 查询 Agent 配置
	configKey := "config:agents"
	agentsData, err := w.redisClient.Get(w.ctx, configKey).Result()
	if err != nil {
		// 如果 Redis 中没有配置，尝试从本地加载
		return w.sendTaskByPipeName(agentName, taskContent, sessionID)
	}

	// 解析配置
	var agents []AgentConfig
	if err := json.Unmarshal([]byte(agentsData), &agents); err != nil {
		return w.sendTaskByPipeName(agentName, taskContent, sessionID)
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
		AgentName:  agentName,
		Content:    taskContent,
		SessionID:  sessionID,
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
func (w *AgentWorker) sendTaskByPipeName(agentName, taskContent, sessionID string) error {
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
		AgentName:  agentName,
		Content:    taskContent,
		SessionID:  sessionID,
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
