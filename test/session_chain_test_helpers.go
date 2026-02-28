package test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ============================================================
// Session Chain 实现 + 测试辅助函数
// Phase 1: 基础设施（Chain 生命周期、Session 管理、存储、Token 估算）
// ============================================================

// --- 常量 ---

const (
	SessionActive      SessionStatus = "active"
	SessionSealed      SessionStatus = "sealed"
	SessionCompressing SessionStatus = "compressing"

	EventUser       EventType = "user"
	EventCat        EventType = "cat"
	EventSystem     EventType = "system"
	EventInvocation EventType = "invocation"
)

// --- 类型定义 ---

type SessionStatus string
type EventType string

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
	ID         string        `json:"id"         yaml:"id"`
	ThreadID   string        `json:"threadId"   yaml:"threadId"`
	SeqNo      int           `json:"seqNo"      yaml:"seqNo"`
	Status     SessionStatus `json:"status"     yaml:"status"`
	StartEvent int           `json:"startEvent" yaml:"startEvent"`
	EndEvent   int           `json:"endEvent"   yaml:"endEvent"`
	EventCount int           `json:"eventCount" yaml:"eventCount"`
	TokenCount int           `json:"tokenCount" yaml:"tokenCount"`
	Summary    string        `json:"summary"    yaml:"summary"`
	FilePath   string        `json:"filePath"   yaml:"filePath"`
	CreatedAt  time.Time     `json:"createdAt"  yaml:"createdAt"`
	SealedAt   *time.Time    `json:"sealedAt,omitempty" yaml:"sealedAt,omitempty"`
}

// SessionEvent Session 内的一条事件
type SessionEvent struct {
	EventNo      int       `json:"eventNo"`
	Type         EventType `json:"type"`
	Sender       string    `json:"sender"`
	Content      string    `json:"content"`
	InvocationID string    `json:"invocationId,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	TokenCount   int       `json:"tokenCount"`
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
	ID         string        `json:"id"`
	SeqNo      int           `json:"seqNo"`
	Status     SessionStatus `json:"status"`
	EventCount int           `json:"eventCount"`
	TokenCount int           `json:"tokenCount"`
	Summary    string        `json:"summary,omitempty"`
	CreatedAt  time.Time     `json:"createdAt"`
	SealedAt   *time.Time    `json:"sealedAt,omitempty"`
}

// --- SessionChainManager ---

// CompressFunc 压缩函数签名（与 src 保持一致）
type CompressFunc func(prompt string, config *MemoryCompressorConfig) (string, error)

// SessionChainManager 管理所有 Thread 的 Session Chain
type SessionChainManager struct {
	dataDir string
	mu      sync.Mutex
	// 内存缓存
	metas    map[string]*SessionChainMeta            // threadID -> meta
	sessions map[string]map[string]*SessionRecord     // threadID -> sessionID -> record
	events   map[string]map[string][]SessionEvent     // threadID -> sessionID -> events
	cursors  map[string]*AgentCursor                  // "agentName:threadID" -> cursor
	CompressFn CompressFunc // 可注入的压缩函数
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
	// 从磁盘恢复已有数据
	if err := mgr.loadFromDisk(); err != nil {
		return nil, err
	}
	return mgr, nil
}

// cursorKey 生成 cursor 的 map key
func cursorKey(agentName, threadID string) string {
	return agentName + ":" + threadID
}

// sessionIDFromSeq 根据序号生成 Session ID
func sessionIDFromSeq(seq int) string {
	return fmt.Sprintf("S%03d", seq)
}

// threadPath 获取 thread 数据目录路径
func (m *SessionChainManager) threadPath(threadID string) string {
	return filepath.Join(m.dataDir, threadID)
}

// metaPath 获取 meta.json 路径
func (m *SessionChainManager) metaPath(threadID string) string {
	return filepath.Join(m.threadPath(threadID), "meta.json")
}

// sessionMarkdownPath 获取 session markdown 路径
func (m *SessionChainManager) sessionMarkdownPath(threadID, sessionID string) string {
	return filepath.Join(m.threadPath(threadID), sessionID+".md")
}

// invocationPath 获取 invocation JSON 路径
func (m *SessionChainManager) invocationPath(threadID, invocationID string) string {
	return filepath.Join(m.threadPath(threadID), "invocations", invocationID+".json")
}

// cursorPath 获取 cursor JSON 路径
func (m *SessionChainManager) cursorPath(threadID, agentName string) string {
	return filepath.Join(m.threadPath(threadID), "cursors", agentName+".json")
}

// --- 磁盘加载 ---

func (m *SessionChainManager) loadFromDisk() error {
	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return nil // 空目录，正常
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		threadID := entry.Name()
		metaPath := m.metaPath(threadID)
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			continue
		}
		meta, err := m.readMetaFromDisk(threadID)
		if err != nil {
			continue
		}
		m.metas[threadID] = meta
		m.sessions[threadID] = make(map[string]*SessionRecord)
		m.events[threadID] = make(map[string][]SessionEvent)

		// 加载所有 session markdown
		for seq := 1; seq <= meta.SessionCount; seq++ {
			sid := sessionIDFromSeq(seq)
			sess, evts, err := m.readSessionMarkdownFromDisk(threadID, sid)
			if err != nil {
				continue
			}
			m.sessions[threadID][sid] = sess
			m.events[threadID][sid] = evts
		}

		// 加载 cursors
		cursorsDir := filepath.Join(m.threadPath(threadID), "cursors")
		cursorEntries, err := os.ReadDir(cursorsDir)
		if err == nil {
			for _, ce := range cursorEntries {
				if !strings.HasSuffix(ce.Name(), ".json") {
					continue
				}
				agentName := strings.TrimSuffix(ce.Name(), ".json")
				data, err := os.ReadFile(filepath.Join(cursorsDir, ce.Name()))
				if err != nil {
					continue
				}
				var cursor AgentCursor
				if err := json.Unmarshal(data, &cursor); err != nil {
					continue
				}
				m.cursors[cursorKey(agentName, threadID)] = &cursor
			}
		}
	}
	return nil
}

// --- GetOrCreateChain ---

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
		Status:     SessionActive,
		StartEvent: 1,
		EndEvent:   0,
		EventCount: 0,
		TokenCount: 0,
		FilePath:   m.sessionMarkdownPath(threadID, firstSessionID),
		CreatedAt:  now,
	}

	// 创建目录
	tDir := m.threadPath(threadID)
	if err := os.MkdirAll(filepath.Join(tDir, "invocations"), 0755); err != nil {
		return nil, fmt.Errorf("创建 thread 目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(tDir, "cursors"), 0755); err != nil {
		return nil, fmt.Errorf("创建 cursors 目录失败: %w", err)
	}

	// 写入磁盘
	if err := m.writeMetaToDisk(threadID, meta); err != nil {
		return nil, err
	}
	if err := m.writeSessionMarkdownToDisk(threadID, session, nil); err != nil {
		return nil, err
	}

	// 更新内存
	m.metas[threadID] = meta
	m.sessions[threadID] = map[string]*SessionRecord{firstSessionID: session}
	m.events[threadID] = map[string][]SessionEvent{firstSessionID: {}}

	return meta, nil
}

// --- AppendEvent ---

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
	if session.Status != SessionActive {
		return fmt.Errorf("Session %s 不是 active 状态", activeID)
	}

	// 分配 eventNo（全局递增）
	meta.TotalEvents++
	event.EventNo = meta.TotalEvents
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.TokenCount = EstimateTokens(event.Content)

	// 更新 session
	session.EventCount++
	session.EndEvent = event.EventNo
	session.TokenCount += event.TokenCount

	// 更新 meta
	meta.UpdatedAt = time.Now()

	// 追加到内存
	m.events[threadID][activeID] = append(m.events[threadID][activeID], event)

	// 持久化
	evts := m.events[threadID][activeID]
	if err := m.writeSessionMarkdownToDisk(threadID, session, evts); err != nil {
		return err
	}
	if err := m.writeMetaToDisk(threadID, meta); err != nil {
		return err
	}

	return nil
}

// --- RecordInvocation ---

func (m *SessionChainManager) RecordInvocation(threadID string, inv InvocationRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.metas[threadID]; !ok {
		return fmt.Errorf("thread %s 不存在", threadID)
	}

	return m.writeInvocationToDisk(threadID, &inv)
}

// --- GetInvocation ---

func (m *SessionChainManager) GetInvocation(threadID, invocationID string) (*InvocationRecord, error) {
	return m.readInvocationFromDisk(threadID, invocationID)
}

// --- Session 查询 ---

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

func (m *SessionChainManager) ListSessions(threadID string) ([]*SessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessMap, ok := m.sessions[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %s 不存在", threadID)
	}

	result := make([]*SessionRecord, 0, len(sessMap))
	for _, s := range sessMap {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SeqNo < result[j].SeqNo
	})
	return result, nil
}

// --- Event 读取 ---

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

func (m *SessionChainManager) GetEventsAfter(threadID, sessionID string, afterEventNo int) ([]SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta, ok := m.metas[threadID]
	if !ok {
		return nil, fmt.Errorf("thread %s 不存在", threadID)
	}

	var result []SessionEvent

	// 收集从指定 session 开始的所有 session（按 seqNo 排序）
	sessions := m.sortedSessions(threadID)
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

	// 如果没找到指定 session，可能 cursor 指向已 seal 的旧 session
	// 需要收集后续所有 session 的 event
	if !startCollecting {
		// 尝试从所有 session 中收集 eventNo > afterEventNo 的
		for _, sess := range sessions {
			evts := m.events[threadID][sess.ID]
			for _, e := range evts {
				if e.EventNo > afterEventNo {
					result = append(result, e)
				}
			}
		}
	}

	_ = meta
	sort.Slice(result, func(i, j int) bool {
		return result[i].EventNo < result[j].EventNo
	})
	return result, nil
}

// sortedSessions 返回按 seqNo 排序的 session 列表（需要在持有锁时调用）
func (m *SessionChainManager) sortedSessions(threadID string) []*SessionRecord {
	sessMap := m.sessions[threadID]
	sessions := make([]*SessionRecord, 0, len(sessMap))
	for _, s := range sessMap {
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].SeqNo < sessions[j].SeqNo
	})
	return sessions
}

// --- Cursor 管理 ---

func (m *SessionChainManager) GetCursor(agentName, threadID string) *AgentCursor {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := cursorKey(agentName, threadID)
	cursor, ok := m.cursors[key]
	if !ok {
		return nil
	}
	return cursor
}

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

	key := cursorKey(agentName, threadID)
	m.cursors[key] = cursor

	// 持久化
	return m.writeCursorToDisk(threadID, agentName, cursor)
}

// --- Seal ---

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

	// 标记为 compressing
	now := time.Now()
	session.Status = SessionCompressing
	session.SealedAt = &now

	// 创建新 session
	meta.SessionCount++
	newSeq := meta.SessionCount
	newID := sessionIDFromSeq(newSeq)

	newSession := &SessionRecord{
		ID:         newID,
		ThreadID:   threadID,
		SeqNo:      newSeq,
		Status:     SessionActive,
		StartEvent: meta.TotalEvents + 1,
		EndEvent:   meta.TotalEvents,
		EventCount: 0,
		TokenCount: 0,
		FilePath:   m.sessionMarkdownPath(threadID, newID),
		CreatedAt:  now,
	}

	meta.ActiveSessionID = newID
	meta.UpdatedAt = now

	// 更新内存
	m.sessions[threadID][newID] = newSession
	m.events[threadID][newID] = []SessionEvent{}

	// 持久化
	if err := m.writeSessionMarkdownToDisk(threadID, session, m.events[threadID][activeID]); err != nil {
		return err
	}
	if err := m.writeSessionMarkdownToDisk(threadID, newSession, nil); err != nil {
		return err
	}
	if err := m.writeMetaToDisk(threadID, meta); err != nil {
		return err
	}

	return nil
}

func (m *SessionChainManager) CheckAndSeal(threadID string, config *SessionChainConfig) error {
	m.mu.Lock()

	meta, ok := m.metas[threadID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("thread %s 不存在", threadID)
	}

	activeID := meta.ActiveSessionID
	session, ok := m.sessions[threadID][activeID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("活跃 Session %s 不存在", activeID)
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

// --- CompressSession ---

func (m *SessionChainManager) CompressSession(threadID, sessionID string, config *MemoryCompressorConfig) error {
	m.mu.Lock()

	if _, ok := m.metas[threadID]; !ok {
		m.mu.Unlock()
		return fmt.Errorf("thread %s 不存在", threadID)
	}

	session, ok := m.sessions[threadID][sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("Session %s 不存在", sessionID)
	}

	if session.Status != SessionCompressing {
		m.mu.Unlock()
		return fmt.Errorf("Session %s 状态为 %s，只能压缩 compressing 状态的 Session", sessionID, session.Status)
	}

	// 收集之前所有 sealed session 的 summary
	sessions := m.sortedSessions(threadID)
	var previousSummaries strings.Builder
	for _, s := range sessions {
		if s.SeqNo >= session.SeqNo {
			break
		}
		if s.Summary != "" {
			previousSummaries.WriteString(fmt.Sprintf("### Session #%d 摘要\n%s\n\n", s.SeqNo, s.Summary))
		}
	}

	// 收集当前 session 的所有 events
	evts := m.events[threadID][sessionID]
	var eventsText strings.Builder
	for _, e := range evts {
		switch e.Type {
		case EventUser:
			eventsText.WriteString(fmt.Sprintf("[用户] %s\n", e.Content))
		case EventCat:
			eventsText.WriteString(fmt.Sprintf("[%s] %s\n", e.Sender, e.Content))
		case EventSystem:
			eventsText.WriteString(fmt.Sprintf("[系统] %s\n", e.Content))
		}
	}

	m.mu.Unlock()

	// 构建压缩 prompt
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

	// 调用压缩函数
	if m.CompressFn == nil {
		return fmt.Errorf("压缩模型不可用（未注入 CompressFn）")
	}

	summary, err := m.CompressFn(prompt, config)
	if err != nil {
		return fmt.Errorf("压缩失败: %w", err)
	}

	// 更新 session record
	m.mu.Lock()
	defer m.mu.Unlock()

	session.Summary = summary
	session.Status = SessionSealed

	return m.writeSessionMarkdownToDisk(threadID, session, m.events[threadID][sessionID])
}

// --- Token 估算 ---

func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	tokens := 0
	for _, r := range text {
		if utf8.RuneLen(r) > 1 {
			// 中文等多字节字符：每个约 2 token
			tokens += 2
		} else {
			// ASCII 字符累计
			tokens++
		}
	}
	// ASCII 部分按 4 字符 ≈ 1 token 折算
	// 上面已经按字符计数了，需要重新算
	// 重新实现：分别统计
	tokens = 0
	asciiCount := 0
	for _, r := range text {
		if utf8.RuneLen(r) > 1 {
			tokens += 2
		} else {
			asciiCount++
		}
	}
	if asciiCount > 0 {
		tokens += (asciiCount + 3) / 4 // 向上取整
	}
	return tokens
}

// --- 磁盘存储：meta.json ---

func (m *SessionChainManager) writeMetaToDisk(threadID string, meta *SessionChainMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 meta 失败: %w", err)
	}
	return os.WriteFile(m.metaPath(threadID), data, 0644)
}

func (m *SessionChainManager) readMetaFromDisk(threadID string) (*SessionChainMeta, error) {
	data, err := os.ReadFile(m.metaPath(threadID))
	if err != nil {
		return nil, fmt.Errorf("读取 meta.json 失败: %w", err)
	}
	var meta SessionChainMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("解析 meta.json 失败: %w", err)
	}
	return &meta, nil
}

// WriteMeta 公开方法
func (m *SessionChainManager) WriteMeta(threadID string, meta *SessionChainMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metas[threadID] = meta
	return m.writeMetaToDisk(threadID, meta)
}

// ReadMeta 公开方法
func (m *SessionChainManager) ReadMeta(threadID string) (*SessionChainMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if meta, ok := m.metas[threadID]; ok {
		return meta, nil
	}
	return m.readMetaFromDisk(threadID)
}

// --- 磁盘存储：Session Markdown ---

func (m *SessionChainManager) writeSessionMarkdownToDisk(threadID string, session *SessionRecord, events []SessionEvent) error {
	// YAML frontmatter
	fm := map[string]interface{}{
		"id":         session.ID,
		"threadId":   session.ThreadID,
		"seqNo":      session.SeqNo,
		"status":     string(session.Status),
		"startEvent": session.StartEvent,
		"endEvent":   session.EndEvent,
		"eventCount": session.EventCount,
		"tokenCount": session.TokenCount,
		"createdAt":  session.CreatedAt.Format(time.RFC3339),
	}
	if session.SealedAt != nil {
		fm["sealedAt"] = session.SealedAt.Format(time.RFC3339)
	}
	if session.Summary != "" {
		fm["summary"] = session.Summary
	}

	fmData, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("序列化 frontmatter 失败: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(fmData)
	sb.WriteString("---\n\n")

	// Event 内容
	for _, e := range events {
		ts := e.Timestamp.Format("15:04:05")
		switch e.Type {
		case EventUser:
			sb.WriteString(fmt.Sprintf("### #%d [%s] **[用户]**\n\n%s\n\n", e.EventNo, ts, e.Content))
		case EventCat:
			sb.WriteString(fmt.Sprintf("### #%d [%s] **[%s]**\n\n%s\n\n", e.EventNo, ts, e.Sender, e.Content))
		case EventSystem:
			sb.WriteString(fmt.Sprintf("### #%d [%s] **[系统]**\n\n%s\n\n", e.EventNo, ts, e.Content))
		case EventInvocation:
			sb.WriteString(fmt.Sprintf("### #%d [%s] **[调用:%s]** invocation_id=%s\n\n%s\n\n",
				e.EventNo, ts, e.Sender, e.InvocationID, e.Content))
		}
	}

	mdPath := m.sessionMarkdownPath(threadID, session.ID)
	return os.WriteFile(mdPath, []byte(sb.String()), 0644)
}

func (m *SessionChainManager) readSessionMarkdownFromDisk(threadID, sessionID string) (*SessionRecord, []SessionEvent, error) {
	mdPath := m.sessionMarkdownPath(threadID, sessionID)
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, nil, fmt.Errorf("读取 Session Markdown 失败: %w", err)
	}

	content := string(data)

	// 解析 frontmatter
	if !strings.HasPrefix(content, "---\n") {
		return nil, nil, fmt.Errorf("无效的 Markdown 格式：缺少 frontmatter")
	}
	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return nil, nil, fmt.Errorf("无效的 Markdown 格式：frontmatter 未闭合")
	}
	fmStr := content[4 : 4+endIdx]
	body := content[4+endIdx+5:] // skip "\n---\n"

	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(fmStr), &fm); err != nil {
		return nil, nil, fmt.Errorf("解析 frontmatter 失败: %w", err)
	}

	session := &SessionRecord{
		ID:       getString(fm, "id"),
		ThreadID: getString(fm, "threadId"),
		SeqNo:    getInt(fm, "seqNo"),
		Status:   SessionStatus(getString(fm, "status")),
		StartEvent: getInt(fm, "startEvent"),
		EndEvent:   getInt(fm, "endEvent"),
		EventCount: getInt(fm, "eventCount"),
		TokenCount: getInt(fm, "tokenCount"),
		Summary:    getString(fm, "summary"),
		FilePath:   mdPath,
	}
	if t, err := time.Parse(time.RFC3339, getString(fm, "createdAt")); err == nil {
		session.CreatedAt = t
	}
	if sealedStr := getString(fm, "sealedAt"); sealedStr != "" {
		if t, err := time.Parse(time.RFC3339, sealedStr); err == nil {
			session.SealedAt = &t
		}
	}

	// 解析 events from body
	events := parseEventsFromMarkdown(body)

	return session, events, nil
}

// parseEventsFromMarkdown 从 markdown body 解析 events
func parseEventsFromMarkdown(body string) []SessionEvent {
	var events []SessionEvent
	lines := strings.Split(body, "\n")
	var currentEvent *SessionEvent
	var contentLines []string

	flushEvent := func() {
		if currentEvent != nil {
			currentEvent.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
			currentEvent.TokenCount = EstimateTokens(currentEvent.Content)
			events = append(events, *currentEvent)
			currentEvent = nil
			contentLines = nil
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "### #") {
			flushEvent()
			e := SessionEvent{}
			// 解析 "### #1 [15:04:05] **[用户]**"
			rest := line[5:] // after "### #"
			// 提取 eventNo
			spaceIdx := strings.Index(rest, " ")
			if spaceIdx == -1 {
				continue
			}
			fmt.Sscanf(rest[:spaceIdx], "%d", &e.EventNo)
			rest = rest[spaceIdx+1:]

			// 提取时间
			if strings.HasPrefix(rest, "[") {
				endBracket := strings.Index(rest, "]")
				if endBracket > 0 {
					tsStr := rest[1:endBracket]
					today := time.Now().Format("2006-01-02")
					if t, err := time.Parse("2006-01-02 15:04:05", today+" "+tsStr); err == nil {
						e.Timestamp = t
					}
					rest = strings.TrimSpace(rest[endBracket+1:])
				}
			}

			// 提取类型和发送者
			if strings.Contains(rest, "**[用户]**") {
				e.Type = EventUser
				e.Sender = "用户"
			} else if strings.Contains(rest, "**[系统]**") {
				e.Type = EventSystem
				e.Sender = "系统"
			} else if strings.Contains(rest, "**[调用:") {
				e.Type = EventInvocation
				// 提取 sender
				start := strings.Index(rest, "**[调用:") + len("**[调用:")
				end := strings.Index(rest[start:], "]**")
				if end > 0 {
					e.Sender = rest[start : start+end]
				}
				// 提取 invocation_id
				if idIdx := strings.Index(rest, "invocation_id="); idIdx >= 0 {
					e.InvocationID = strings.TrimSpace(rest[idIdx+len("invocation_id="):])
				}
			} else if strings.Contains(rest, "**[") {
				e.Type = EventCat
				start := strings.Index(rest, "**[") + 3
				end := strings.Index(rest[start:], "]**")
				if end > 0 {
					e.Sender = rest[start : start+end]
				}
			}

			currentEvent = &e
			contentLines = nil
		} else if currentEvent != nil {
			contentLines = append(contentLines, line)
		}
	}
	flushEvent()

	return events
}

// YAML helper functions
func getString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

func getInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case int64:
		return int(val)
	default:
		var i int
		fmt.Sscanf(fmt.Sprintf("%v", val), "%d", &i)
		return i
	}
}

// WriteSessionMarkdown 公开方法
func (m *SessionChainManager) WriteSessionMarkdown(threadID string, session *SessionRecord, events []SessionEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeSessionMarkdownToDisk(threadID, session, events)
}

// ReadSessionMarkdown 公开方法
func (m *SessionChainManager) ReadSessionMarkdown(threadID, sessionID string) (*SessionRecord, []SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readSessionMarkdownFromDisk(threadID, sessionID)
}

// --- 磁盘存储：Invocation JSON ---

func (m *SessionChainManager) writeInvocationToDisk(threadID string, inv *InvocationRecord) error {
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 Invocation 失败: %w", err)
	}
	path := m.invocationPath(threadID, inv.ID)
	return os.WriteFile(path, data, 0644)
}

func (m *SessionChainManager) readInvocationFromDisk(threadID, invocationID string) (*InvocationRecord, error) {
	path := m.invocationPath(threadID, invocationID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 Invocation 失败: %w", err)
	}
	var inv InvocationRecord
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("解析 Invocation 失败: %w", err)
	}
	return &inv, nil
}

// WriteInvocation 公开方法
func (m *SessionChainManager) WriteInvocation(threadID string, inv *InvocationRecord) error {
	return m.writeInvocationToDisk(threadID, inv)
}

// ReadInvocation 公开方法
func (m *SessionChainManager) ReadInvocation(threadID, invocationID string) (*InvocationRecord, error) {
	return m.readInvocationFromDisk(threadID, invocationID)
}

// --- Cursor 磁盘存储 ---

func (m *SessionChainManager) writeCursorToDisk(threadID, agentName string, cursor *AgentCursor) error {
	data, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 Cursor 失败: %w", err)
	}
	path := m.cursorPath(threadID, agentName)
	return os.WriteFile(path, data, 0644)
}

// --- 全文搜索 ---

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

	sessions := m.sortedSessions(threadID)
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

	// 按 score 降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// --- MCP Server 方法 ---

func (m *SessionChainManager) MCPListSessionChain(threadID, catID string) ([]SessionSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessMap, ok := m.sessions[threadID]
	if !ok {
		return []SessionSummary{}, nil
	}

	sessions := m.sortedSessions(threadID)
	summaries := make([]SessionSummary, 0, len(sessMap))
	for _, s := range sessions {
		summary := SessionSummary{
			ID:         s.ID,
			SeqNo:      s.SeqNo,
			Status:     s.Status,
			EventCount: s.EventCount,
			TokenCount: s.TokenCount,
			Summary:    s.Summary,
			CreatedAt:  s.CreatedAt,
			SealedAt:   s.SealedAt,
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func (m *SessionChainManager) MCPReadSessionEvents(sessionID string, cursor, limit int, view string) ([]SessionEvent, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 查找 sessionID 所属的 thread
	for threadID, evtMap := range m.events {
		evts, ok := evtMap[sessionID]
		if !ok {
			continue
		}
		_ = threadID

		if view == "chat" {
			// chat 模式：过滤掉 invocation 细节（但保留基本信息）
			// 当前实现返回所有 event
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

func (m *SessionChainManager) MCPReadInvocationDetail(invocationID string) (*InvocationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 遍历所有 thread 查找 invocation
	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		threadID := entry.Name()
		inv, err := m.readInvocationFromDisk(threadID, invocationID)
		if err == nil {
			return inv, nil
		}
	}
	return nil, fmt.Errorf("Invocation %s 不存在", invocationID)
}

func (m *SessionChainManager) MCPSessionSearch(threadID, query string, limit int) ([]SearchResult, error) {
	return m.SearchEvents(threadID, query, limit)
}

// --- 测试辅助函数 ---

func setupSessionChainTest(t *testing.T) (*SessionChainManager, string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "session_chain_test_*")
	require.NoError(t, err)

	mgr, err := NewSessionChainManager(tmpDir)
	require.NoError(t, err)

	// 注入 mock 压缩函数
	mgr.CompressFn = func(prompt string, config *MemoryCompressorConfig) (string, error) {
		return fmt.Sprintf("测试摘要：对话包含多条消息，涉及测试场景。模型: %s", config.Model), nil
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return mgr, tmpDir, cleanup
}

func makeEvent(eventType EventType, sender, content string) SessionEvent {
	return SessionEvent{
		Type:      eventType,
		Sender:    sender,
		Content:   content,
		Timestamp: time.Now(),
	}
}

func makeUserEvent(content string) SessionEvent {
	return makeEvent(EventUser, "用户", content)
}

func makeCatEvent(catName, content string) SessionEvent {
	return makeEvent(EventCat, catName, content)
}

func makeInvocation(agentName, prompt, response string) InvocationRecord {
	return InvocationRecord{
		ID:        fmt.Sprintf("inv_%d", time.Now().UnixNano()),
		AgentName: agentName,
		Prompt:    prompt,
		Response:  response,
		Timestamp: time.Now(),
	}
}

func appendNEvents(t *testing.T, mgr *SessionChainManager, threadID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		err := mgr.AppendEvent(threadID, makeUserEvent(fmt.Sprintf("测试消息 #%d", i+1)))
		require.NoError(t, err, "追加第 %d 个 Event 失败", i+1)
	}
}

func readFileContent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "读取文件失败: %s", path)
	return string(data)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readJSON(t *testing.T, path string, target interface{}) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	err = json.Unmarshal(data, target)
	require.NoError(t, err)
}

func threadDir(baseDir, threadID string) string {
	return filepath.Join(baseDir, threadID)
}
