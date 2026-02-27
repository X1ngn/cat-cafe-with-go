package main

import (
	"fmt"
	"os"
)

func main() {
	// 命令行参数解析
	var (
		model     string
		sessionID string
	)

	args := os.Args[1:]
	var prompt string

	for i := 0; i < len(args); i++ {
		if args[i] == "-model" || args[i] == "--model" {
			if i+1 < len(args) {
				model = args[i+1]
				i++
			}
		} else if args[i] == "-resume" || args[i] == "--resume" {
			if i+1 < len(args) {
				sessionID = args[i+1]
				i++
			}
		} else {
			prompt = args[i]
			break
		}
	}

	if prompt == "" {
		fmt.Fprintf(os.Stderr, "用法: %s [选项] \"你的问题\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		fmt.Fprintf(os.Stderr, "  --model           Claude 模型\n")
		fmt.Fprintf(os.Stderr, "  --resume          恢复会话 ID\n")
		os.Exit(1)
	}

	options := AgentOptions{
		Model:          model,
		AllowedTools:   "Read,Edit,Glob,Grep",
		PermissionMode: "acceptEdits",
		SessionID:      sessionID,
	}

	_, _, err := InvokeCLI("claude", prompt, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}
