package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// 命令行参数
	var (
		configPath  = flag.String("config", "config.yaml", "配置文件路径")
		mode        = flag.String("mode", "", "运行模式: ui(交互界面), agent(Agent工作进程)")
		agentName   = flag.String("agent", "", "Agent 名称 (agent 模式必需)")
		sendTask    = flag.Bool("send", false, "发送任务模式")
		listAgents  = flag.Bool("list", false, "列出所有 Agent")
		targetAgent = flag.String("to", "", "目标 Agent 名称")
		taskContent = flag.String("task", "", "任务内容")
	)

	flag.Parse()

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

		taskID, err := scheduler.SendTask(*targetAgent, *taskContent)
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
	fmt.Println("  交互界面:      ./cat-cafe --mode ui")
	fmt.Println("  列出 Agent:    ./cat-cafe --list")
	fmt.Println("  发送任务:      ./cat-cafe --send --to 花花 --task \"实现HTTP服务器\"")
	fmt.Println("  启动 Agent:    ./cat-cafe --mode agent --agent 花花")
	fmt.Println()
	flag.PrintDefaults()
}
