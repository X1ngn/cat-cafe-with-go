package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	// 命令行参数
	var (
		configPath  = flag.String("config", "config.yaml", "配置文件路径")
		mode        = flag.String("mode", "", "运行模式: ui(交互界面), agent(Agent工作进程), api(API服务器), mcp(MCP Server)")
		agentName   = flag.String("agent", "", "Agent 名称 (agent 模式必需)")
		threadID    = flag.String("thread", "", "Thread ID (mcp 模式必需)")
		sendTask    = flag.Bool("send", false, "发送任务模式")
		listAgents  = flag.Bool("list", false, "列出所有 Agent")
		targetAgent = flag.String("to", "", "目标 Agent 名称")
		taskContent = flag.String("task", "", "任务内容")
		port        = flag.String("port", "8080", "API 服务器端口")
	)

	flag.Parse()

	// API 服务器模式
	if *mode == "api" {
		fmt.Println("🚀 启动 API 服务器...")

		sessionManager, err := NewSessionManager(*configPath)
		if err != nil {
			log.Fatalf("初始化会话管理器失败: %v", err)
		}

		router := sessionManager.SetupRouter()

		addr := fmt.Sprintf(":%s", *port)
		fmt.Printf("✓ API 服务器运行在 http://localhost%s\n", addr)
		fmt.Println("✓ 前端可以通过 /api 路径访问接口")
		fmt.Println()
		fmt.Println("可用接口:")
		fmt.Println("  GET    /api/sessions")
		fmt.Println("  POST   /api/sessions")
		fmt.Println("  GET    /api/sessions/:id")
		fmt.Println("  DELETE /api/sessions/:id")
		fmt.Println("  GET    /api/sessions/:id/messages")
		fmt.Println("  POST   /api/sessions/:id/messages")
		fmt.Println("  GET    /api/sessions/:id/stats")
		fmt.Println("  GET    /api/sessions/:id/history")
		fmt.Println("  GET    /api/cats")
		fmt.Println("  GET    /api/cats/:id")
		fmt.Println("  GET    /api/cats/available")
		fmt.Println()

		if err := router.Run(addr); err != nil {
			log.Fatalf("启动服务器失败: %v", err)
		}
		return
	}

	// MCP Server 模式
	if *mode == "mcp" {
		if *threadID == "" {
			fmt.Fprintf(os.Stderr, "MCP 模式需要指定 --thread 参数\n")
			os.Exit(1)
		}

		dataDir := "data/session_chains"
		chainManager, err := NewSessionChainManager(dataDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建 SessionChainManager 失败: %v\n", err)
			os.Exit(1)
		}

		// 确保 chain 存在
		chainManager.GetOrCreateChain(*threadID)

		mcpServer := NewSessionChainMCPServer(chainManager, *threadID)
		if err := mcpServer.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP Server 异常退出: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 列出 Agent
	if *listAgents {
		scheduler, err := NewScheduler(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化调度器失败: %v\n", err)
			os.Exit(1)
		}
		defer scheduler.Close()

		fmt.Println("🐱 可用的 Agent:")
		fmt.Println()
		for _, agent := range scheduler.ListAgents() {
			fmt.Printf("  %s\n", agent.Name)
			fmt.Printf("    管道: %s\n", agent.Pipe)
			fmt.Printf("    执行命令: %s\n", agent.ExecCmd)
			fmt.Printf("    系统提示词: %s\n", agent.SystemPromptPath)

			state, _ := scheduler.GetAgentState(agent.Name)
			if state != nil {
				fmt.Printf("    状态: %s\n", state.Status)
			}
			fmt.Println()
		}
		return
	}

	// 发送任务模式
	if *sendTask {
		if *targetAgent == "" || *taskContent == "" {
			fmt.Fprintf(os.Stderr, "发送任务需要指定 --to 和 --task 参数\n")
			os.Exit(1)
		}

		scheduler, err := NewScheduler(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化调度器失败: %v\n", err)
			os.Exit(1)
		}
		defer scheduler.Close()

		taskID, err := scheduler.SendTask(*targetAgent, *taskContent, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "发送任务失败: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✓ 任务已发送: %s\n", taskID)
		return
	}

	// Agent 工作进程模式
	if *mode == "agent" {
		if *agentName == "" {
			fmt.Fprintf(os.Stderr, "Agent 模式需要指定 --agent 参数\n")
			os.Exit(1)
		}

		// 读取配置
		scheduler, err := NewScheduler(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化调度器失败: %v\n", err)
			os.Exit(1)
		}

		// 创建 WorkspaceManager
		workspaceManager := NewWorkspaceManager(
			scheduler.redisClient,
			scheduler.ctx,
		)

		// 获取 Agent 配置
		var agentConfig *AgentConfig
		for _, agent := range scheduler.ListAgents() {
			if agent.Name == *agentName {
				agentConfig = agent
				break
			}
		}

		if agentConfig == nil {
			fmt.Fprintf(os.Stderr, "Agent %s 不存在\n", *agentName)
			os.Exit(1)
		}

		// 创建 SessionChainManager（如果 Agent 配置了 context_mode）
		var chainManager *SessionChainManager
		if agentConfig.ContextMode != "" {
			dataDir := "data/session_chains"
			var err error
			chainManager, err = NewSessionChainManager(dataDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  创建 SessionChainManager 失败: %v（将使用旧逻辑）\n", err)
				chainManager = nil
			}
		}

		// 获取系统提示词
		systemPrompt, err := scheduler.GetSystemPrompt(*agentName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取系统提示词失败: %v\n", err)
			os.Exit(1)
		}

		// 创建 Agent 工作进程
		worker, err := NewAgentWorker(
			agentConfig,
			systemPrompt,
			scheduler.config.Redis.Addr,
			scheduler.config.Redis.Password,
			scheduler.config.Redis.DB,
			workspaceManager,
			chainManager,
			scheduler.config.Hindsight,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建 Agent 工作进程失败: %v\n", err)
			os.Exit(1)
		}

		scheduler.Close()

		// 启动 Agent
		if err := worker.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Agent 运行失败: %v\n", err)
			os.Exit(1)
		}

		return
	}

	// 交互式 UI 模式
	if *mode == "ui" {
		scheduler, err := NewScheduler(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化调度器失败: %v\n", err)
			os.Exit(1)
		}

		ui, err := NewUserInterface(
			scheduler,
			scheduler.config.Redis.Addr,
			scheduler.config.Redis.Password,
			scheduler.config.Redis.DB,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建用户界面失败: %v\n", err)
			os.Exit(1)
		}
		defer ui.Stop()

		if err := ui.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "用户界面运行失败: %v\n", err)
			os.Exit(1)
		}

		return
	}

	// 默认显示帮助
	fmt.Println("猫猫咖啡屋 - Multi-Agent 调度器")
	fmt.Println()
	fmt.Println("使用方法:")
	fmt.Println("  API 服务器:    ./cat-cafe --mode api")
	fmt.Println("  交互界面:      ./cat-cafe --mode ui")
	fmt.Println("  列出 Agent:    ./cat-cafe --list")
	fmt.Println("  发送任务:      ./cat-cafe --send --to 花花 --task \"实现HTTP服务器\"")
	fmt.Println("  启动 Agent:    ./cat-cafe --mode agent --agent 花花")
	fmt.Println()
	flag.PrintDefaults()
}
