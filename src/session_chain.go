package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// --- 常量 ---

const (
	SCSessionActive      SessionChainStatus = "active"
	SCSessionSealed      SessionChainStatus = "sealed"
	SCSessionCompressing SessionChainStatus = "compressing"

	SCEventUser       SessionChainEventType = "user"
	SCEventCat        SessionChainEventType = "cat"
	SCEventSystem     SessionChainEventType = "system"
	SCEventInvocation SessionChainEventType = "invocation"
)

// --- 类型定义 ---

type SessionChainStatus string
type SessionChainEventType string

// SessionChainMeta Session Chain 元数据
type SessionChainMeta struct {
	ThreadID        string    `json:"threadId"        yaml:"threadId"`
	ActiveSessionID string    `json:"activeSessionId" yaml:"activeSessionId"`
	SessionCount    int       `json:"sessionCount"    yaml:"sessionCount"`
	TotalEvents     int       `json:"totalEvents"     yaml:"totalEvents"`
	CreatedAt       time.Time `json:"createdAt"       yaml:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"       yaml:"updatedAt"`
}

// SessionRecord 单个 Session 的元数据
type SessionRecord struct {
	ID         string             `json:"id"         yaml:"id"`
	ThreadID   string             `json:"threadId"   yaml:"threadId"`
	SeqNo      int                `json:"seqNo"      yaml:"seqNo"`
	Status     SessionChainStatus `json:"status"     yaml:"status"`
	StartEvent int                `json:"startEvent" yaml:"startEvent"`
	EndEvent   int                `json:"endEvent"   yaml:"endEvent"`
	EventCount int                `json:"eventCount" yaml:"eventCount"`
	TokenCount int                `json:"tokenCount" yaml:"tokenCount"`
	Summary    string             `json:"summary"    yaml:"summary"`
	FilePath   string             `json:"filePath"   yaml:"filePath"`
	CreatedAt  time.Time          `json:"createdAt"  yaml:"createdAt"`
	SealedAt   *time.Time         `json:"sealedAt,omitempty" yaml:"sealedAt,omitempty"`
}

// SessionEvent Session 内的一条事件
type SessionEvent struct {
	EventNo      int               `json:"eventNo"`
	Type         SessionChainEventType `json:"type"`
	Sender       string            `json:"sender"`
	Content      string            `json:"content"`
	MsgID        string            `json:"msgId,omitempty"`
	InvocationID string            `json:"invocationId,omitempty"`
	Timestamp    time.Time         `json:"timestamp"`
	TokenCount   int               `json:"tokenCount"`
}

// InvocationRecord 一次 Agent 调用的完整记录
type InvocationRecord struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"sessionId"`
	ThreadID     string    `json:"threadId"`
	AgentName    string    `json:"agentName"`
	Prompt       string    `json:"prompt"`
	Response     string    `json:"response"`
	AISessionID  string    `json:"aiSessionId"`
	TokensIn     int       `json:"tokensIn"`
	TokensOut    int       `json:"tokensOut"`
	Duration     int64     `json:"duration"`
	StartEventNo int       `json:"startEventNo"`
	EndEventNo   int       `json:"endEventNo"`
	Timestamp    time.Time `json:"timestamp"`
}

// AgentCursor Agent 的读取位置指针
type AgentCursor struct {
	AgentName     string `json:"agentName"`
	ThreadID      string `json:"threadId"`
	LastSessionID string `json:"lastSessionId"`
	LastEventNo   int    `json:"lastEventNo"`
	AISessionID   string `json:"aiSessionId"`
}

// MemoryCompressorConfig 记忆压缩器配置
type MemoryCompressorConfig struct {
	Model            string `yaml:"model"`
	MaxSummaryTokens int    `yaml:"max_summary_tokens"`
}

// SessionChainConfig Session Chain 配置
type SessionChainConfig struct {
	MaxTokens           int     `yaml:"max_tokens"`
	SealThreshold       float64 `yaml:"seal_threshold"`
	MaxEventsPerSession int     `yaml:"max_events_per_session"`
}

// SearchResult 搜索结果
type SearchResult struct {
	SessionID    string    `json:"sessionId"`
	EventNo      int       `json:"eventNo"`
	InvocationID string    `json:"invocationId,omitempty"`
	Snippet      string    `json:"snippet"`
	Score        float64   `json:"score"`
	Timestamp    time.Time `json:"timestamp"`
}

// SessionSummary MCP 返回的 Session 摘要
type SessionSummary struct {
	ID         string             `json:"id"`
	SeqNo      int                `json:"seqNo"`
	Status     SessionChainStatus `json:"status"`
	EventCount int                `json:"eventCount"`
	TokenCount int                `json:"tokenCount"`
	Summary    string             `json:"summary,omitempty"`
	CreatedAt  time.Time          `json:"createdAt"`
	SealedAt   *time.Time         `json:"sealedAt,omitempty"`
}

// --- SessionChainManager ---

// CompressFunc 压缩函数签名：接收 prompt，返回摘要文本
type CompressFunc func(prompt string, config *MemoryCompressorConfig) (string, error)

// SessionChainManager 管理所有 Thread 的 Session Chain
type SessionChainManager struct {
	dataDir    string
	mu         sync.Mutex
	metas      map[string]*SessionChainMeta
	sessions   map[string]map[string]*SessionRecord
	events     map[string]map[string][]SessionEvent
	cursors    map[string]*AgentCursor
	CompressFn CompressFunc // 可注入的压缩函数，默认调用 InvokeCLI
}

// NewSessionChainManager 创建 SessionChainManager
func NewSessionChainManager(dataDir string) (*SessionChainManager, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	mgr := &SessionChainManager{
		dataDir:  dataDir,
		metas:    make(map[string]*SessionChainMeta),
		sessions: make(map[string]map[string]*SessionRecord),
		events:   make(map[string]map[string][]SessionEvent),
		cursors:  make(map[string]*AgentCursor),
	}
	if err := mgr.loadFromDisk(); err != nil {
		return nil, err
	}
	return mgr, nil
}

func cursorKey(agentName, threadID string) string {
	return agentName + ":" + threadID
}

func sessionIDFromSeq(seq int) string {
	return fmt.Sprintf("S%03d", seq)
}

// --- Chain 生命周期 ---

// GetOrCreateChain 获取或创建 Thread 的 Session Chain
func (m *SessionChainManager) GetOrCreateChain(threadID string) (*SessionChainMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if meta, ok := m.metas[threadID]; ok {
		return meta, nil
	}

	now := time.Now()
	firstSessionID := sessionIDFromSeq(1)

	meta := &SessionChainMeta{
		ThreadID:        threadID,
		ActiveSessionID: firstSessionID,
		SessionCount:    1,
		TotalEvents:     0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	session := &SessionRecord{
		ID:         firstSessionID,
		ThreadID:   threadID,
		SeqNo:      1,
		Status:     SCSessionActive,
		StartEvent: 1,
		EndEvent:   0,
		EventCount: 0,
		TokenCount: 0,
		FilePath:   m.sessionMarkdownPath(threadID, firstSessionID),
		CreatedAt:  now,
	}

	tDir := m.threadPath(threadID)
	if err := os.MkdirAll(filepath.Join(tDir, "invocations"), 0755); err != nil {
		return nil, fmt.Errorf("创建 thread 目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(tDir, "cursors"), 0755); err != nil {
		return nil, fmt.Errorf("创建 cursors 目录失败: %w", err)
	}

	if err := m.writeMetaToDisk(threadID, meta); err != nil {
		return nil, err
	}
	if err := m.writeSessionMarkdownToDisk(threadID, session, nil); err != nil {
		return nil, err
	}

	m.metas[threadID] = meta
	m.sessions[threadID] = map[string]*SessionRecord{firstSessionID: session}
	m.events[threadID] = map[string][]SessionEvent{firstSessionID: {}}

	return meta, nil
}

// DeleteChain 删除整个 Thread 的 Session Chain 数据（内存 + 磁盘）
func (m *SessionChainManager) DeleteChain(threadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清理内存
	delete(m.metas, threadID)
	delete(m.sessions, threadID)
	delete(m.events, threadID)

	// 删除磁盘目录
	dirPath := m.threadPath(threadID)
	if _, err := os.Stat(dirPath); err == nil {
		if err := os.RemoveAll(dirPath); err != nil {
			return fmt.Errorf("删除 Session Chain 目录失败: %w", err)
		}
	}

	return nil
}

// --- Event 写入 ---

// AppendEvent 向当前活跃 Session 追加 Event
func (m *SessionChainManager) AppendEvent(threadID string, event SessionEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta, ok := m.metas[threadID]
	if !ok {
		return fmt.Errorf("thread %s 不存在，请先调用 GetOrCreateChain", threadID)
	}

	activeID := meta.ActiveSessionID
	session, ok := m.sessions[threadID][activeID]
	if !ok {
		return fmt.Errorf("活跃 Session %s 不存在", activeID)
	}
	if session.Status != SCSessionActive {
		return fmt.Errorf("Session %s 不是 active 状态", activeID)
	}

	meta.TotalEvents++
	event.EventNo = meta.TotalEvents
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.TokenCount = EstimateTokens(event.Content)

	session.EventCount++
	session.EndEvent = event.EventNo
	session.TokenCount += event.TokenCount
	meta.UpdatedAt = time.Now()

	m.events[threadID][activeID] = append(m.events[threadID][activeID], event)

	evts := m.events[threadID][activeID]
	if err := m.writeSessionMarkdownToDisk(threadID, session, evts); err != nil {
		return err
	}
	return m.writeMetaToDisk(threadID, meta)
}

// RecordInvocation 记录一次 Agent 调用
func (m *SessionChainManager) RecordInvocation(threadID string, inv InvocationRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.metas[threadID]; !ok {
		return fmt.Errorf("thread %s 不存在", threadID)
	}
	return m.writeInvocationToDisk(threadID, &inv)
}

// GetInvocation 获取 Invocation 详情
func (m *SessionChainManager) GetInvocation(threadID, invocationID string) (*InvocationRecord, error) {
	return m.readInvocationFromDisk(threadID, invocationID)
}

// --- Event 读取 ---

// GetEvents 获取指定 Session 的 Event（支持分页）
func (m *SessionChainManager) GetEvents(threadID, sessionID string, cursor, limit int) ([]SessionEvent, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	evtMap, ok := m.events[threadID]
	if !ok {
		return nil, -1, fmt.Errorf("thread %s 不存在", threadID)
	}
	evts, ok := evtMap[sessionID]
	if !ok {
		return nil, -1, fmt.Errorf("Session %s 不存在", sessionID)
	}

	if cursor >= len(evts) {
		return []SessionEvent{}, -1, nil
	}
	end := cursor + limit
	if end >= len(evts) {
		return evts[cursor:], -1, nil
	}
	return evts[cursor:end], end, nil
}

// GetEventsAfter 获取指定位置之后的所有 Event（跨 Session）
func (m *SessionChainManager) GetEventsAfter(threadID, sessionID string, afterEventNo int) ([]SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.metas[threadID]; !ok {
		return nil, fmt.Errorf("thread %s 不存在", threadID)
	}

	var result []SessionEvent
	sessions := m.sortedSessionsLocked(threadID)
	startCollecting := false

	for _, sess := range sessions {
		if sess.ID == sessionID {
			startCollecting = true
		}
		if !startCollecting {
			continue
		}
		evts := m.events[threadID][sess.ID]
		for _, e := range evts {
			if e.EventNo > afterEventNo {
				result = append(result, e)
			}
		}
	}

	if !startCollecting {
		for _, sess := range sessions {
			evts := m.events[threadID][sess.ID]
			for _, e := range evts {
				if e.EventNo > afterEventNo {
					result = append(result, e)
				}
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].EventNo < result[j].EventNo
	})
	return result, nil
}

// GetAllEvents 获取 Thread 下所有 Session 的全部 Events（按顺序）
func (m *SessionChainManager) GetAllEvents(threadID string) ([]SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.metas[threadID]; !ok {
		return nil, nil // thread 不存在，返回空
	}

	sessions := m.sortedSessionsLocked(threadID)
	var result []SessionEvent
	for _, sess := range sessions {
		evts := m.events[threadID][sess.ID]
		result = append(result, evts...)
	}
	return result, nil
}

// --- Session 查询 ---

// GetActiveSession 获取当前活跃 Session
func (m *SessionChainManager) GetActiveSession(threadID string) (*SessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta, ok := m.metas[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %s 不存在", threadID)
	}
	session, ok := m.sessions[threadID][meta.ActiveSessionID]
	if !ok {
		return nil, fmt.Errorf("活跃 Session %s 不存在", meta.ActiveSessionID)
	}
	return session, nil
}

// GetSession 获取指定 Session
func (m *SessionChainManager) GetSession(threadID, sessionID string) (*SessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessMap, ok := m.sessions[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %s 不存在", threadID)
	}
	session, ok := sessMap[sessionID]
	if !ok {
		return nil, fmt.Errorf("Session %s 不存在", sessionID)
	}
	return session, nil
}

// ListSessions 列出所有 Session（按 seqNo 排序）
func (m *SessionChainManager) ListSessions(threadID string) ([]*SessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[threadID]; !ok {
		return nil, fmt.Errorf("thread %s 不存在", threadID)
	}
	return m.sortedSessionsLocked(threadID), nil
}

func (m *SessionChainManager) sortedSessionsLocked(threadID string) []*SessionRecord {
	sessMap := m.sessions[threadID]
	result := make([]*SessionRecord, 0, len(sessMap))
	for _, s := range sessMap {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SeqNo < result[j].SeqNo
	})
	return result
}

// --- Cursor 管理 ---

// GetCursor 获取 Agent 的 Cursor
func (m *SessionChainManager) GetCursor(agentName, threadID string) *AgentCursor {
	m.mu.Lock()
	defer m.mu.Unlock()
	cursor, ok := m.cursors[cursorKey(agentName, threadID)]
	if !ok {
		return nil
	}
	return cursor
}

// UpdateCursor 更新 Agent 的 Cursor
func (m *SessionChainManager) UpdateCursor(agentName, threadID, sessionID string, eventNo int, aiSessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cursor := &AgentCursor{
		AgentName:     agentName,
		ThreadID:      threadID,
		LastSessionID: sessionID,
		LastEventNo:   eventNo,
		AISessionID:   aiSessionID,
	}
	m.cursors[cursorKey(agentName, threadID)] = cursor
	return m.writeCursorToDisk(threadID, agentName, cursor)
}

// --- Seal ---

// SealActiveSession 强制 Seal 当前活跃 Session
func (m *SessionChainManager) SealActiveSession(threadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta, ok := m.metas[threadID]
	if !ok {
		return fmt.Errorf("thread %s 不存在", threadID)
	}

	activeID := meta.ActiveSessionID
	session, ok := m.sessions[threadID][activeID]
	if !ok {
		return fmt.Errorf("活跃 Session %s 不存在", activeID)
	}

	now := time.Now()
	session.Status = SCSessionCompressing
	session.SealedAt = &now

	meta.SessionCount++
	newSeq := meta.SessionCount
	newID := sessionIDFromSeq(newSeq)

	newSession := &SessionRecord{
		ID:         newID,
		ThreadID:   threadID,
		SeqNo:      newSeq,
		Status:     SCSessionActive,
		StartEvent: meta.TotalEvents + 1,
		EndEvent:   meta.TotalEvents,
		EventCount: 0,
		TokenCount: 0,
		FilePath:   m.sessionMarkdownPath(threadID, newID),
		CreatedAt:  now,
	}

	meta.ActiveSessionID = newID
	meta.UpdatedAt = now

	m.sessions[threadID][newID] = newSession
	m.events[threadID][newID] = []SessionEvent{}

	if err := m.writeSessionMarkdownToDisk(threadID, session, m.events[threadID][activeID]); err != nil {
		return err
	}
	if err := m.writeSessionMarkdownToDisk(threadID, newSession, nil); err != nil {
		return err
	}
	return m.writeMetaToDisk(threadID, meta)
}

// CheckAndSeal 检查是否需要 Seal
func (m *SessionChainManager) CheckAndSeal(threadID string, config *SessionChainConfig) error {
	m.mu.Lock()

	meta, ok := m.metas[threadID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("thread %s 不存在", threadID)
	}

	session, ok := m.sessions[threadID][meta.ActiveSessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("活跃 Session %s 不存在", meta.ActiveSessionID)
	}

	shouldSeal := false
	tokenThreshold := int(float64(config.MaxTokens) * config.SealThreshold)
	if session.TokenCount >= tokenThreshold {
		shouldSeal = true
	}
	if session.EventCount >= config.MaxEventsPerSession {
		shouldSeal = true
	}

	m.mu.Unlock()

	if shouldSeal {
		return m.SealActiveSession(threadID)
	}
	return nil
}

// DefaultCompressFn 默认压缩函数，通过 InvokeCLI 调用模型
func DefaultCompressFn(prompt string, config *MemoryCompressorConfig) (string, error) {
	model := config.Model
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	options := AgentOptions{Model: model}
	response, _, err := InvokeCLI("claude", prompt, options)
	if err != nil {
		return "", fmt.Errorf("调用压缩模型失败: %w", err)
	}
	return strings.TrimSpace(response), nil
}

// CompressSession 压缩指定 Session
func (m *SessionChainManager) CompressSession(threadID, sessionID string, config *MemoryCompressorConfig) error {
	m.mu.Lock()

	meta, ok := m.metas[threadID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("thread %s 不存在", threadID)
	}

	session, ok := m.sessions[threadID][sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("Session %s 不存在", sessionID)
	}

	if session.Status != SCSessionCompressing {
		m.mu.Unlock()
		return fmt.Errorf("Session %s 状态为 %s，只能压缩 compressing 状态的 Session", sessionID, session.Status)
	}

	// 1. 收集之前所有 sealed/compressing session 的 summary
	sessions := m.sortedSessionsLocked(threadID)
	var previousSummaries strings.Builder
	for _, s := range sessions {
		if s.SeqNo >= session.SeqNo {
			break
		}
		if s.Summary != "" {
			previousSummaries.WriteString(fmt.Sprintf("### Session #%d 摘要\n%s\n\n", s.SeqNo, s.Summary))
		}
	}

	// 2. 收集当前 session 的所有 events
	evts := m.events[threadID][sessionID]
	var eventsText strings.Builder
	for _, e := range evts {
		switch e.Type {
		case SCEventUser:
			eventsText.WriteString(fmt.Sprintf("[用户] %s\n", e.Content))
		case SCEventCat:
			eventsText.WriteString(fmt.Sprintf("[%s] %s\n", e.Sender, e.Content))
		case SCEventSystem:
			eventsText.WriteString(fmt.Sprintf("[系统] %s\n", e.Content))
		}
	}

	m.mu.Unlock()

	// 3. 构建压缩 prompt
	prevSummaryStr := previousSummaries.String()
	if prevSummaryStr == "" {
		prevSummaryStr = "（无）"
	}

	prompt := fmt.Sprintf(`你是一个对话记忆压缩助手。请将以下对话记录压缩为一份结构化摘要。

## 之前的摘要（如有）
%s

## 当前对话记录
%s

## 要求
1. 保留关键决策和结论
2. 保留重要的代码文件路径和技术细节
3. 保留未完成的任务和待办事项
4. 删除寒暄、重复内容和中间过程
5. 摘要长度控制在原文的 20%% 以内
6. 标注未完成的任务和待确认的问题`, prevSummaryStr, eventsText.String())

	// 4. 调用压缩函数
	compressFn := m.CompressFn
	if compressFn == nil {
		compressFn = DefaultCompressFn
	}

	summary, err := compressFn(prompt, config)
	if err != nil {
		return fmt.Errorf("压缩失败: %w", err)
	}

	// 5. 更新 session record
	m.mu.Lock()
	defer m.mu.Unlock()

	session.Summary = summary
	session.Status = SCSessionSealed
	_ = meta

	// 6. 持久化
	return m.writeSessionMarkdownToDisk(threadID, session, m.events[threadID][sessionID])
}

// --- 全文搜索 ---

// SearchEvents 全文搜索 Event
func (m *SessionChainManager) SearchEvents(threadID, query string, limit int) ([]SearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if query == "" {
		return []SearchResult{}, nil
	}

	evtMap, ok := m.events[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %s 不存在", threadID)
	}

	queryLower := strings.ToLower(query)
	var results []SearchResult

	sessions := m.sortedSessionsLocked(threadID)
	for _, sess := range sessions {
		evts := evtMap[sess.ID]
		for _, e := range evts {
			contentLower := strings.ToLower(e.Content)
			if strings.Contains(contentLower, queryLower) {
				score := float64(strings.Count(contentLower, queryLower))
				results = append(results, SearchResult{
					SessionID: sess.ID,
					EventNo:   e.EventNo,
					Snippet:   e.Content,
					Score:     score,
					Timestamp: e.Timestamp,
				})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// --- MCP Server 方法 ---

// MCPListSessionChain MCP: list_session_chain
func (m *SessionChainManager) MCPListSessionChain(threadID, catID string) ([]SessionSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[threadID]; !ok {
		return []SessionSummary{}, nil
	}

	sessions := m.sortedSessionsLocked(threadID)
	summaries := make([]SessionSummary, 0, len(sessions))
	for _, s := range sessions {
		summaries = append(summaries, SessionSummary{
			ID:         s.ID,
			SeqNo:      s.SeqNo,
			Status:     s.Status,
			EventCount: s.EventCount,
			TokenCount: s.TokenCount,
			Summary:    s.Summary,
			CreatedAt:  s.CreatedAt,
			SealedAt:   s.SealedAt,
		})
	}
	return summaries, nil
}

// MCPReadSessionEvents MCP: read_session_events
func (m *SessionChainManager) MCPReadSessionEvents(sessionID string, cursor, limit int, view string) ([]SessionEvent, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, evtMap := range m.events {
		evts, ok := evtMap[sessionID]
		if !ok {
			continue
		}
		if cursor >= len(evts) {
			return []SessionEvent{}, -1, nil
		}
		end := cursor + limit
		if end >= len(evts) {
			return evts[cursor:], -1, nil
		}
		return evts[cursor:end], end, nil
	}
	return nil, -1, fmt.Errorf("Session %s 不存在", sessionID)
}

// MCPReadInvocationDetail MCP: read_invocation_detail
func (m *SessionChainManager) MCPReadInvocationDetail(invocationID string) (*InvocationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		inv, err := m.readInvocationFromDisk(entry.Name(), invocationID)
		if err == nil {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("Invocation %s 不存在", invocationID)
}

// MCPSessionSearch MCP: session_search
func (m *SessionChainManager) MCPSessionSearch(threadID, query string, limit int) ([]SearchResult, error) {
	return m.SearchEvents(threadID, query, limit)
}
