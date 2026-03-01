package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// InvokeAgent 调用指定类型的 AI Agent
// cliType: "claude" / "codex" / "gemini"
// prompt: 完整的 prompt 内容
// aiSessionID: 可选的 AI session ID（用于 --resume）
// workDir: 可选的工作目录
// 返回: response, newSessionID, error
func InvokeAgent(cliType, prompt, aiSessionID, workDir string) (string, string, error) {
	options := getDefaultOptions(cliType)
	options.SessionID = aiSessionID
	options.WorkDir = workDir
	return InvokeCLI(cliType, prompt, options)
}

// InvokeAgentWithMCP 调用 AI Agent 并注入 MCP 配置
func InvokeAgentWithMCP(cliType, prompt, aiSessionID, workDir, mcpConfigPath string) (string, string, error) {
	options := getDefaultOptions(cliType)
	options.SessionID = aiSessionID
	options.WorkDir = workDir
	options.MCPConfigPath = mcpConfigPath
	return InvokeCLI(cliType, prompt, options)
}

// getDefaultOptions 返回指定 CLI 类型的默认选项
func getDefaultOptions(cliType string) AgentOptions {
	switch cliType {
	case "claude":
		return AgentOptions{
			PermissionMode: "dontAsk",
			AllowedTools:   "mcp__hindsight__*,mcp__session-chain__*,mcp__github__*,mcp__figma__*,mcp__ide__*",
		}
	case "codex":
		return AgentOptions{} // 已在 invoke.go 中硬编码 --full-auto
	case "gemini":
		return AgentOptions{
			ApprovalMode: "yolo",
			AllowedTools: "mcp__hindsight__*,mcp__session-chain__*,mcp__TalkToFigma__*",
		}
	default:
		return AgentOptions{}
	}
}

// GenerateMCPConfig 生成 MCP 配置文件，返回临时文件路径
// threadID: 当前对话的 Thread ID
// binPath: cat-cafe 可执行文件路径（为空时使用默认值）
// agentName: Agent 名称（用于生成 hindsight bank ID）
// hindsightCfg: Hindsight 配置（为 nil 或 Enabled=false 时不生成 hindsight 条目）
func GenerateMCPConfig(threadID, binPath, agentName string, hindsightCfg *HindsightConfig) (string, error) {
	if binPath == "" {
		// 尝试找到当前可执行文件路径
		exe, err := os.Executable()
		if err != nil {
			binPath = "./bin/cat-cafe"
		} else {
			binPath = exe
		}
	}

	servers := map[string]interface{}{
		"session-chain": map[string]interface{}{
			"command": binPath,
			"args":    []string{"--mode", "mcp", "--thread", threadID},
			"type":    "stdio",
		},
	}

	// 追加 hindsight MCP 条目
	if hindsightCfg != nil && hindsightCfg.Enabled && hindsightCfg.BaseURL != "" {
		bankID := BankIDForAgent(agentName)
		mcpURL := fmt.Sprintf("%s/mcp/%s/", hindsightCfg.BaseURL, bankID)
		entry := map[string]interface{}{
			"url":  mcpURL,
			"type": "http",
		}
		if hindsightCfg.Token != "" {
			entry["headers"] = map[string]string{
				"Authorization": "Bearer " + hindsightCfg.Token,
			}
		}
		servers["hindsight"] = entry
	}

	config := map[string]interface{}{
		"mcpServers": servers,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 MCP 配置失败: %w", err)
	}

	// 写入临时文件
	tmpDir := filepath.Join(os.TempDir(), "cat-cafe-mcp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("创建 MCP 临时目录失败: %w", err)
	}

	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("mcp_%s.json", threadID))
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return "", fmt.Errorf("写入 MCP 配置文件失败: %w", err)
	}

	return tmpFile, nil
}
