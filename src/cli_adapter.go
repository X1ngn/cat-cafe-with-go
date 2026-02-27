package main

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

// getDefaultOptions 返回指定 CLI 类型的默认选项
func getDefaultOptions(cliType string) AgentOptions {
	switch cliType {
	case "claude":
		return AgentOptions{
			AllowedTools:   "Read,Edit,Glob,Grep",
			PermissionMode: "acceptEdits",
		}
	case "codex":
		return AgentOptions{}
	case "gemini":
		return AgentOptions{
			AllowedTools: "Read,Edit,Glob,Grep",
			ApprovalMode: "auto_edit",
		}
	default:
		return AgentOptions{}
	}
}
