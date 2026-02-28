package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// --- JSON-RPC 2.0 协议类型 ---

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- MCP 协议类型 ---

type mcpToolDef struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	InputSchema mcpToolInputSchema  `json:"inputSchema"`
}

type mcpToolInputSchema struct {
	Type       string                       `json:"type"`
	Properties map[string]mcpPropertySchema `json:"properties"`
	Required   []string                     `json:"required,omitempty"`
}

type mcpPropertySchema struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// --- SessionChainMCPServer ---

// SessionChainMCPServer MCP Server 实现（stdio 模式）
type SessionChainMCPServer struct {
	chainManager *SessionChainManager
	threadID     string
}

// NewSessionChainMCPServer 创建 MCP Server
func NewSessionChainMCPServer(chainManager *SessionChainManager, threadID string) *SessionChainMCPServer {
	return &SessionChainMCPServer{
		chainManager: chainManager,
		threadID:     threadID,
	}
}

// Start 启动 MCP Server（读 stdin，写 stdout）
func (s *SessionChainMCPServer) Start() error {
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("读取 stdin 失败: %w", err)
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, -32700, "Parse error")
			continue
		}

		resp := s.handleRequest(&req)
		s.writeResponse(resp)
	}
}

// handleRequest 路由请求到对应处理函数
func (s *SessionChainMCPServer) handleRequest(req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "notifications/initialized":
		// 客户端通知，无需响应
		return nil
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)},
		}
	}
}

// handleInitialize 处理 MCP 初始化
func (s *SessionChainMCPServer) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "session-chain",
				"version": "1.0.0",
			},
		},
	}
}

// handleToolsList 返回可用工具列表
func (s *SessionChainMCPServer) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	tools := []mcpToolDef{
		{
			Name:        "list_session_chain",
			Description: "列出当前 thread 的所有 session 列表（状态、token数、序号）",
			InputSchema: mcpToolInputSchema{
				Type: "object",
				Properties: map[string]mcpPropertySchema{
					"catId": {Type: "string", Description: "猫猫名称"},
				},
				Required: []string{"catId"},
			},
		},
		{
			Name:        "read_session_events",
			Description: "分页读取某个 session 的完整记录。view 模式: chat（人类可读）| handoff（交接摘要）| raw（原始数据）",
			InputSchema: mcpToolInputSchema{
				Type: "object",
				Properties: map[string]mcpPropertySchema{
					"sessionId": {Type: "string", Description: "Session ID（如 S001）"},
					"cursor":    {Type: "number", Description: "分页起始位置，默认 0"},
					"limit":     {Type: "number", Description: "每页数量，默认 50"},
					"view":      {Type: "string", Description: "视图模式: chat | handoff | raw"},
				},
				Required: []string{"sessionId"},
			},
		},
		{
			Name:        "read_invocation_detail",
			Description: "查看某一次猫猫调用的完整输入/输出",
			InputSchema: mcpToolInputSchema{
				Type: "object",
				Properties: map[string]mcpPropertySchema{
					"invocationId": {Type: "string", Description: "Invocation ID"},
				},
				Required: []string{"invocationId"},
			},
		},
		{
			Name:        "session_search",
			Description: "跨所有 session 的全文搜索，返回匹配片段和定位指针",
			InputSchema: mcpToolInputSchema{
				Type: "object",
				Properties: map[string]mcpPropertySchema{
					"query": {Type: "string", Description: "搜索关键词"},
					"limit": {Type: "number", Description: "最大返回数量，默认 10"},
				},
				Required: []string{"query"},
			},
		},
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]interface{}{"tools": tools},
	}
}

// handleToolsCall 处理工具调用
func (s *SessionChainMCPServer) handleToolsCall(req *jsonRPCRequest) *jsonRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "Invalid params"},
		}
	}

	var result *mcpToolResult

	switch params.Name {
	case "list_session_chain":
		result = s.callListSessionChain(params.Arguments)
	case "read_session_events":
		result = s.callReadSessionEvents(params.Arguments)
	case "read_invocation_detail":
		result = s.callReadInvocationDetail(params.Arguments)
	case "session_search":
		result = s.callSessionSearch(params.Arguments)
	default:
		result = &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("未知工具: %s", params.Name)}},
			IsError: true,
		}
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

// --- 工具实现 ---

func (s *SessionChainMCPServer) callListSessionChain(args json.RawMessage) *mcpToolResult {
	var input struct {
		CatID string `json:"catId"`
	}
	json.Unmarshal(args, &input)

	summaries, err := s.chainManager.MCPListSessionChain(s.threadID, input.CatID)
	if err != nil {
		return &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("错误: %v", err)}},
			IsError: true,
		}
	}

	data, _ := json.MarshalIndent(summaries, "", "  ")
	return &mcpToolResult{
		Content: []mcpContent{{Type: "text", Text: string(data)}},
	}
}

func (s *SessionChainMCPServer) callReadSessionEvents(args json.RawMessage) *mcpToolResult {
	var input struct {
		SessionID string `json:"sessionId"`
		Cursor    int    `json:"cursor"`
		Limit     int    `json:"limit"`
		View      string `json:"view"`
	}
	json.Unmarshal(args, &input)

	if input.Limit <= 0 {
		input.Limit = 50
	}
	if input.View == "" {
		input.View = "chat"
	}

	events, nextCursor, err := s.chainManager.MCPReadSessionEvents(input.SessionID, input.Cursor, input.Limit, input.View)
	if err != nil {
		return &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("错误: %v", err)}},
			IsError: true,
		}
	}

	result := map[string]interface{}{
		"events":     events,
		"nextCursor": nextCursor,
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return &mcpToolResult{
		Content: []mcpContent{{Type: "text", Text: string(data)}},
	}
}

func (s *SessionChainMCPServer) callReadInvocationDetail(args json.RawMessage) *mcpToolResult {
	var input struct {
		InvocationID string `json:"invocationId"`
	}
	json.Unmarshal(args, &input)

	inv, err := s.chainManager.MCPReadInvocationDetail(input.InvocationID)
	if err != nil {
		return &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("错误: %v", err)}},
			IsError: true,
		}
	}

	data, _ := json.MarshalIndent(inv, "", "  ")
	return &mcpToolResult{
		Content: []mcpContent{{Type: "text", Text: string(data)}},
	}
}

func (s *SessionChainMCPServer) callSessionSearch(args json.RawMessage) *mcpToolResult {
	var input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	json.Unmarshal(args, &input)

	if input.Limit <= 0 {
		input.Limit = 10
	}

	results, err := s.chainManager.MCPSessionSearch(s.threadID, input.Query, input.Limit)
	if err != nil {
		return &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("错误: %v", err)}},
			IsError: true,
		}
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return &mcpToolResult{
		Content: []mcpContent{{Type: "text", Text: string(data)}},
	}
}

// --- 输出辅助 ---

func (s *SessionChainMCPServer) writeResponse(resp *jsonRPCResponse) {
	if resp == nil {
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	fmt.Fprintf(os.Stdout, "%s\n", data)
}

func (s *SessionChainMCPServer) writeError(id interface{}, code int, message string) {
	resp := &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	}
	s.writeResponse(resp)
}
