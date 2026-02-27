package main

import (
	"flag"
	"fmt"
	"os"
)

// 安全边界的 CLI 标志（静态可扫描）
const (
	DEFAULT_ALLOWED_TOOLS   = "Read,Edit,Glob,Grep"
	DEFAULT_PERMISSION_MODE = "acceptEdits"
	DEFAULT_MODEL           = ""
)

func main() {
	// 命令行参数定义
	var (
		model          = flag.String("model", DEFAULT_MODEL, "Claude 模型 (sonnet/opus/haiku)")
		allowedTools   = flag.String("allowed-tools", DEFAULT_ALLOWED_TOOLS, "允许的工具列表")
		permissionMode = flag.String("permission-mode", DEFAULT_PERMISSION_MODE, "权限模式")
		sessionIDFlag  = flag.String("resume", "", "恢复会话 ID")
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
		Model:          *model,
		AllowedTools:   *allowedTools,
		PermissionMode: *permissionMode,
		SessionID:      *sessionIDFlag, // 直接使用命令行参数，如果为空则创建新会话
	}

	// 调用 Claude 代理
	_, newSessionID, err := InvokeCLI("claude", prompt, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	// 输出 Session ID 到 stdout（单独一行，方便提取）
	if newSessionID != "" {
		fmt.Printf("SESSION_ID:%s\n", newSessionID)
	}
}
