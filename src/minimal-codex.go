package main

import (
	"flag"
	"fmt"
	"os"
)

// 安全边界的 CLI 标志（静态可扫描）
const (
	DEFAULT_MODEL = ""
)

func main() {
	// 命令行参数定义
	var (
		model         = flag.String("model", DEFAULT_MODEL, "Codex 模型")
		sessionIDFlag = flag.String("resume", "", "恢复会话 ID")
	)

	// 手动解析参数，只解析标志，不解析 prompt 内容
	args := os.Args[1:]
	var prompt string

	// 找到第一个非标志参数（prompt）
	for i := 0; i < len(args); i++ {
		if args[i] == "-model" || args[i] == "--model" {
			if i+1 < len(args) {
				*model = args[i+1]
				i++ // 跳过值
			}
		} else if args[i] == "-resume" || args[i] == "--resume" {
			if i+1 < len(args) {
				*sessionIDFlag = args[i+1]
				i++ // 跳过值
			}
		} else {
			// 第一个非标志参数就是 prompt
			prompt = args[i]
			break
		}
	}

	// 检查是否提供了 prompt
	if prompt == "" {
		fmt.Fprintf(os.Stderr, "用法: %s [选项] \"你的问题\"\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// 配置选项
	options := AgentOptions{
		Model:     *model,
		SessionID: *sessionIDFlag, // 直接使用命令行参数，如果为空则创建新会话
	}

	// 调用 Codex 代理
	_, newSessionID, err := InvokeCLI("codex", prompt, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// 输出 Session ID 到 stdout（单独一行，方便提取）
	if newSessionID != "" {
		fmt.Printf("SESSION_ID:%s\n", newSessionID)
	}
}
