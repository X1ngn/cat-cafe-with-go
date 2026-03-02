package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"
)

// AgentOptions 包含所有可能的代理CLI配置选项
type AgentOptions struct {
	Model          string
	AllowedTools   string
	PermissionMode string // 用于 Claude CLI
	ApprovalMode   string // 用于 Gemini CLI
	SessionID      string // 用于 --resume
	WorkDir        string // 工作目录
	MCPConfigPath  string // MCP 配置文件路径
}

// InvokeCLI 调用指定的 CLI 工具并处理其流式输出。
// 它根据 cliName 动态构建命令行参数，并解析对应的 JSON 输出格式。
// 返回助手的回复内容（如果找到）和会话ID。
func InvokeCLI(cliName, prompt string, options AgentOptions) (string, string, error) {
	var args []string
	var assistantResponse string
	var sessionID string

	// 根据 CLI 名称构建不同的参数
	switch cliName {
	case "claude":
		args = append(args, "-p", prompt, "--output-format", "stream-json", "--verbose")
		if options.Model != "" {
			args = append(args, "--model", options.Model)
		}
		if options.SessionID != "" {
			args = append(args, "--resume", options.SessionID)
		}
		if options.AllowedTools != "" {
			args = append(args, "--allowedTools", options.AllowedTools)
		}
		if options.PermissionMode != "" {
			args = append(args, "--permission-mode", options.PermissionMode)
		}
		if options.MCPConfigPath != "" {
			args = append(args, "--mcp-config", options.MCPConfigPath)
		}
	case "gemini":
		args = append(args, "-p", prompt, "--output-format", "stream-json")
		if options.Model != "" {
			args = append(args, "--model", options.Model)
		}
		if options.SessionID != "" {
			args = append(args, "--resume", options.SessionID)
		}
		if options.AllowedTools != "" {
			args = append(args, "--allowed-tools", options.AllowedTools)
		}
		if options.ApprovalMode != "" {
			args = append(args, "--approval-mode", options.ApprovalMode)
		}
		if options.MCPConfigPath != "" {
			args = append(args, "--mcp-config", options.MCPConfigPath)
		}
	case "codex":
		// Codex 使用不同的命令格式，通过 stdin 传递 prompt 避免参数解析问题
		if options.SessionID != "" {
			args = []string{"exec", "resume", "--json", "--skip-git-repo-check", options.SessionID, "-"}
		} else {
			args = []string{"exec", "--json", "--full-auto", "--skip-git-repo-check"}
			if options.Model != "" {
				args = append(args, "--model", options.Model)
			}
			args = append(args, "-")
		}
	default:
		return "", "", fmt.Errorf("不支持的 CLI 工具: %s", cliName)
	}

	cmd := exec.Command(cliName, args...)

	// 设置工作目录
	if options.WorkDir != "" {
		cmd.Dir = options.WorkDir
	}

	// 清理 CLAUDECODE 环境变量，避免嵌套会话错误
	env := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	// 对于 codex，通过 stdin 传递 prompt
	if cliName == "codex" {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return "", "", fmt.Errorf("无法获取 stdin: %w", err)
		}
		go func() {
			defer stdin.Close()
			// 确保 prompt 是有效的 UTF-8
			validPrompt := ensureValidUTF8(prompt)
			stdin.Write([]byte(validPrompt))
		}()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", fmt.Errorf("无法获取 stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", "", fmt.Errorf("无法获取 stderr: %w", err)
	}

	// 使用 WaitGroup 等待所有 goroutine 完成
	var wg sync.WaitGroup
	wg.Add(2)

	// 收集 stderr 输出
	var stderrOutput strings.Builder

	// 处理 stderr
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		// 增加缓冲区大小到 1MB，避免 "token too long" 错误
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			stderrOutput.WriteString(line)
			stderrOutput.WriteString("\n")
		}
	}()

	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("无法启动 %s 命令: %w", cliName, err)
	}

	// 处理 stdout
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		// 增大 buffer 到 1MB，避免长行被截断导致响应丢失
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()

			// 根据 CLI 类型解析 JSON
			switch cliName {
			case "claude":
				var event struct {
					Type      string `json:"type"`
					Subtype   string `json:"subtype,omitempty"`
					SessionID string `json:"session_id,omitempty"`
					Message   struct {
						Content []struct {
							Type string `json:"type"`
							Text string `json:"text"`
						} `json:"content"`
					} `json:"message,omitempty"`
				}
				if err := json.Unmarshal([]byte(line), &event); err == nil {
					if event.Type == "system" && event.Subtype == "init" && event.SessionID != "" {
						sessionID = event.SessionID
					} else if event.Type == "assistant" {
						for _, contentBlock := range event.Message.Content {
							if contentBlock.Type == "text" {
								assistantResponse += contentBlock.Text
							}
						}
					}
				}
			case "gemini":
				var event struct {
					Type      string `json:"type"`
					SessionID string `json:"session_id,omitempty"`
					Role      string `json:"role,omitempty"`
					Content   string `json:"content,omitempty"`
				}
				if err := json.Unmarshal([]byte(line), &event); err == nil {
					if event.Type == "init" && event.SessionID != "" {
						sessionID = event.SessionID
					} else if event.Type == "message" && event.Role == "assistant" && event.Content != "" {
						assistantResponse += event.Content
					}
				}
			case "codex":
				var event struct {
					Type      string `json:"type"`
					ThreadID  string `json:"thread_id,omitempty"`
					SessionID string `json:"session_id,omitempty"`
					Item      struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"item,omitempty"`
				}
				if err := json.Unmarshal([]byte(line), &event); err == nil {
					if event.Type == "thread.started" && event.ThreadID != "" {
						sessionID = event.ThreadID
					} else if event.Type == "session_start" && event.SessionID != "" {
						sessionID = event.SessionID
					} else if event.Type == "item.completed" && event.Item.Type == "agent_message" && event.Item.Text != "" {
						assistantResponse += event.Item.Text
					}
				}
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "读取 %s 输出时出错: %v\n", cliName, err)
		}
	}()

	wg.Wait() // 等待所有 goroutine 完成

	if err := cmd.Wait(); err != nil {
		errMsg := fmt.Sprintf("命令 %s 执行失败: %v", cliName, err)
		if stderrOutput.Len() > 0 {
			errMsg += fmt.Sprintf("\nstderr: %s", stderrOutput.String())
		}
		return assistantResponse, sessionID, fmt.Errorf("%s", errMsg)
	}

	return assistantResponse, sessionID, nil
}

// ensureValidUTF8 确保字符串是有效的 UTF-8 编码
// 将所有无效的 UTF-8 字节序列替换为 Unicode 替换字符 (U+FFFD)
func ensureValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	// 使用 strings.ToValidUTF8 替换无效字符
	// 替换字符使用 � (U+FFFD, Unicode replacement character)
	return strings.ToValidUTF8(s, "�")
}
