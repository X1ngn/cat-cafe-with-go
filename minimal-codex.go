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

	flag.Parse()

	// 检查是否提供了 prompt
	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "用法: %s [选项] \"你的问题\"\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// 获取用户输入的问题
	prompt := flag.Arg(0)

	// 配置选项
	options := AgentOptions{
		Model: *model,
	}

	// 如果没有通过命令行提供 session ID，则尝试从文件加载
	if *sessionIDFlag == "" {
		loadedSessionID, err := LoadSessionID("codex")
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载 Codex 会话失败: %v\n", err)
			os.Exit(1)
		}
		options.SessionID = loadedSessionID
	} else {
		options.SessionID = *sessionIDFlag
	}

	// 调用 Codex 代理
	_, newSessionID, err := InvokeCLI("codex", prompt, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// 如果返回了新的 session ID 并且与当前使用的不同，则保存
	if newSessionID != "" && newSessionID != options.SessionID {
		if err := SaveSessionID("codex", newSessionID); err != nil {
			fmt.Fprintf(os.Stderr, "保存 Codex 会话失败: %v\n", err)
			os.Exit(1)
		}
	}
}
