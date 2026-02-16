package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// AgentOptions 包含所有可能的代理CLI配置选项
type AgentOptions struct {
	Model          string
	AllowedTools   string
	PermissionMode string // 用于 Claude CLI
	ApprovalMode   string // 用于 Gemini CLI
	SessionID      string // 用于 --resume
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

	// 对于 codex，通过 stdin 传递 prompt
	if cliName == "codex" {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return "", "", fmt.Errorf("无法获取 stdin: %w", err)
		}
		go func() {
			defer stdin.Close()
			stdin.Write([]byte(prompt))
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
								fmt.Print(contentBlock.Text) // 实时打印
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
						fmt.Print(event.Content) // 实时打印
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
						fmt.Print(event.Item.Text) // 实时打印
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

// getSessionFilePath 返回用于存储给定 CLI 工具的会话 ID 的文件路径。
func getSessionFilePath(cliName string) string {
	return filepath.Join(".", fmt.Sprintf(".%s_session", cliName))
}

// LoadSessionID 从文件中加载指定 CLI 工具的会话 ID。
func LoadSessionID(cliName string) (string, error) {
	filePath := getSessionFilePath(cliName)
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // 文件不存在不是错误，表示没有会话
		}
		return "", fmt.Errorf("无法读取会话文件 %s: %w", filePath, err)
	}
	return strings.TrimSpace(string(content)), nil
}

// SaveSessionID 将指定 CLI 工具的会话 ID 保存到文件中。
func SaveSessionID(cliName, sessionID string) error {
	filePath := getSessionFilePath(cliName)
	err := os.WriteFile(filePath, []byte(sessionID), 0644)
	if err != nil {
		return fmt.Errorf("无法写入会话文件 %s: %w", filePath, err)
	}
	return nil
}
