package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
)

// generateUUID 生成简单的 UUID
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// UserInterface 用户交互界面
type UserInterface struct {
	scheduler   *Scheduler
	redisClient *redis.Client
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewUserInterface 创建用户界面
func NewUserInterface(scheduler *Scheduler, redisAddr, redisPassword string, redisDB int) (*UserInterface, error) {
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

	return &UserInterface{
		scheduler:   scheduler,
		redisClient: rdb,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

// Start 启动用户界面
func (ui *UserInterface) Start() error {
	fmt.Println("🐱 猫猫咖啡屋 - 交互式界面")
	fmt.Println()
	fmt.Println("使用方法:")
	fmt.Println("  @花花 你的任务内容")
	fmt.Println("  @薇薇 你的任务内容")
	fmt.Println("  @小乔 你的任务内容")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  /list   - 列出所有 Agent")
	fmt.Println("  /help   - 显示帮助")
	fmt.Println("  /exit   - 退出")
	fmt.Println()

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n👋 再见！")
		ui.cancel()
	}()

	// 读取用户输入
	scanner := bufio.NewScanner(os.Stdin)
	for {
		select {
		case <-ui.ctx.Done():
			return nil
		default:
			fmt.Print("> ")
			if !scanner.Scan() {
				return nil
			}

			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}

			if err := ui.handleInput(input); err != nil {
				fmt.Fprintf(os.Stderr, "❌ 错误: %v\n", err)
			}
		}
	}
}

// handleInput 处理用户输入
func (ui *UserInterface) handleInput(input string) error {
	// 处理命令
	if strings.HasPrefix(input, "/") {
		return ui.handleCommand(input)
	}

	// 处理 @Agent 格式
	if strings.HasPrefix(input, "@") {
		return ui.handleAgentTask(input)
	}

	fmt.Println("❌ 无效的输入格式")
	fmt.Println("   使用 @Agent 发送任务，例如: @花花 实现HTTP服务器")
	fmt.Println("   使用 /help 查看帮助")
	return nil
}

// handleCommand 处理命令
func (ui *UserInterface) handleCommand(cmd string) error {
	switch cmd {
	case "/list":
		return ui.listAgents()
	case "/help":
		ui.showHelp()
		return nil
	case "/exit", "/quit":
		fmt.Println("👋 再见！")
		ui.cancel()
		return nil
	default:
		fmt.Printf("❌ 未知命令: %s\n", cmd)
		fmt.Println("   使用 /help 查看可用命令")
		return nil
	}
}

// handleAgentTask 处理 @Agent 任务
func (ui *UserInterface) handleAgentTask(input string) error {
	// 解析格式: @Agent 任务内容
	parts := strings.SplitN(input, " ", 2)
	if len(parts) < 2 {
		return fmt.Errorf("格式错误，正确格式: @Agent 任务内容")
	}

	agentName := strings.TrimPrefix(parts[0], "@")
	taskContent := strings.TrimSpace(parts[1])

	if taskContent == "" {
		return fmt.Errorf("任务内容不能为空")
	}

	// 检查 Agent 是否存在
	agent, exists := ui.scheduler.agents[agentName]
	if !exists {
		fmt.Printf("❌ Agent '%s' 不存在\n", agentName)
		fmt.Println("   使用 /list 查看可用的 Agent")
		return nil
	}

	// 创建任务
	task := TaskMessage{
		TaskID:     generateUUID(),
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

	streamKey := fmt.Sprintf("pipe:%s", agent.Pipe)
	_, err = ui.redisClient.XAdd(ui.ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{
			"task": string(taskData),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("发送任务失败: %w", err)
	}

	fmt.Printf("✓ 任务已发送给 %s\n", agentName)
	fmt.Printf("  任务ID: %s\n", task.TaskID)
	fmt.Println()

	return nil
}

// listAgents 列出所有 Agent
func (ui *UserInterface) listAgents() error {
	fmt.Println("🐱 可用的 Agent:")
	fmt.Println()

	for name, agent := range ui.scheduler.agents {
		state, _ := ui.scheduler.GetAgentState(name)
		status := "unknown"
		if state != nil {
			status = state.Status
		}
		fmt.Printf("  @%s\n", name)
		fmt.Printf("    管道: %s\n", agent.Pipe)
		fmt.Printf("    状态: %s\n", status)
		fmt.Println()
	}

	return nil
}

// showHelp 显示帮助
func (ui *UserInterface) showHelp() {
	fmt.Println("🐱 猫猫咖啡屋 - 帮助")
	fmt.Println()
	fmt.Println("发送任务:")
	fmt.Println("  @花花 实现一个HTTP服务器")
	fmt.Println("  @薇薇 审查代码安全性")
	fmt.Println("  @小乔 设计登录页面")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  /list   - 列出所有可用的 Agent")
	fmt.Println("  /help   - 显示此帮助信息")
	fmt.Println("  /exit   - 退出程序")
	fmt.Println()
}

// Stop 停止用户界面
func (ui *UserInterface) Stop() {
	ui.cancel()
	ui.redisClient.Close()
}
