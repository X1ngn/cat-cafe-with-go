package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// --- 路径辅助 ---

func (m *SessionChainManager) threadPath(threadID string) string {
	return filepath.Join(m.dataDir, threadID)
}

func (m *SessionChainManager) metaPath(threadID string) string {
	return filepath.Join(m.threadPath(threadID), "meta.json")
}

func (m *SessionChainManager) sessionMarkdownPath(threadID, sessionID string) string {
	return filepath.Join(m.threadPath(threadID), sessionID+".md")
}

func (m *SessionChainManager) invocationPath(threadID, invocationID string) string {
	return filepath.Join(m.threadPath(threadID), "invocations", invocationID+".json")
}

func (m *SessionChainManager) cursorPath(threadID, agentName string) string {
	return filepath.Join(m.threadPath(threadID), "cursors", agentName+".json")
}

// --- 磁盘加载 ---

func (m *SessionChainManager) loadFromDisk() error {
	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return nil
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

		for seq := 1; seq <= meta.SessionCount; seq++ {
			sid := sessionIDFromSeq(seq)
			sess, evts, err := m.readSessionMarkdownFromDisk(threadID, sid)
			if err != nil {
				continue
			}
			m.sessions[threadID][sid] = sess
			m.events[threadID][sid] = evts
		}

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

// --- meta.json ---

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

// WriteMeta 写入 meta.json（公开方法）
func (m *SessionChainManager) WriteMeta(threadID string, meta *SessionChainMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metas[threadID] = meta
	return m.writeMetaToDisk(threadID, meta)
}

// ReadMeta 读取 meta.json（公开方法）
func (m *SessionChainManager) ReadMeta(threadID string) (*SessionChainMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if meta, ok := m.metas[threadID]; ok {
		return meta, nil
	}
	return m.readMetaFromDisk(threadID)
}

// --- Session Markdown ---

func (m *SessionChainManager) writeSessionMarkdownToDisk(threadID string, session *SessionRecord, events []SessionEvent) error {
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

	for _, e := range events {
		ts := e.Timestamp.Format("15:04:05")
		switch e.Type {
		case SCEventUser:
			sb.WriteString(fmt.Sprintf("### #%d [%s] **[用户]**\n\n%s\n\n", e.EventNo, ts, e.Content))
		case SCEventCat:
			sb.WriteString(fmt.Sprintf("### #%d [%s] **[%s]**\n\n%s\n\n", e.EventNo, ts, e.Sender, e.Content))
		case SCEventSystem:
			sb.WriteString(fmt.Sprintf("### #%d [%s] **[系统]**\n\n%s\n\n", e.EventNo, ts, e.Content))
		case SCEventInvocation:
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

	if !strings.HasPrefix(content, "---\n") {
		return nil, nil, fmt.Errorf("无效的 Markdown 格式：缺少 frontmatter")
	}
	endIdx := strings.Index(content[4:], "\n---\n")
	if endIdx == -1 {
		return nil, nil, fmt.Errorf("无效的 Markdown 格式：frontmatter 未闭合")
	}
	fmStr := content[4 : 4+endIdx]
	body := content[4+endIdx+5:]

	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(fmStr), &fm); err != nil {
		return nil, nil, fmt.Errorf("解析 frontmatter 失败: %w", err)
	}

	session := &SessionRecord{
		ID:         yamlGetString(fm, "id"),
		ThreadID:   yamlGetString(fm, "threadId"),
		SeqNo:      yamlGetInt(fm, "seqNo"),
		Status:     SessionChainStatus(yamlGetString(fm, "status")),
		StartEvent: yamlGetInt(fm, "startEvent"),
		EndEvent:   yamlGetInt(fm, "endEvent"),
		EventCount: yamlGetInt(fm, "eventCount"),
		TokenCount: yamlGetInt(fm, "tokenCount"),
		Summary:    yamlGetString(fm, "summary"),
		FilePath:   mdPath,
	}
	if t, err := time.Parse(time.RFC3339, yamlGetString(fm, "createdAt")); err == nil {
		session.CreatedAt = t
	}
	if sealedStr := yamlGetString(fm, "sealedAt"); sealedStr != "" {
		if t, err := time.Parse(time.RFC3339, sealedStr); err == nil {
			session.SealedAt = &t
		}
	}

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
			rest := line[5:]
			spaceIdx := strings.Index(rest, " ")
			if spaceIdx == -1 {
				continue
			}
			fmt.Sscanf(rest[:spaceIdx], "%d", &e.EventNo)
			rest = rest[spaceIdx+1:]

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

			if strings.Contains(rest, "**[用户]**") {
				e.Type = SCEventUser
				e.Sender = "用户"
			} else if strings.Contains(rest, "**[系统]**") {
				e.Type = SCEventSystem
				e.Sender = "系统"
			} else if strings.Contains(rest, "**[调用:") {
				e.Type = SCEventInvocation
				start := strings.Index(rest, "**[调用:") + len("**[调用:")
				end := strings.Index(rest[start:], "]**")
				if end > 0 {
					e.Sender = rest[start : start+end]
				}
				if idIdx := strings.Index(rest, "invocation_id="); idIdx >= 0 {
					e.InvocationID = strings.TrimSpace(rest[idIdx+len("invocation_id="):])
				}
			} else if strings.Contains(rest, "**[") {
				e.Type = SCEventCat
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

// --- Invocation JSON ---

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

// --- Cursor JSON ---

func (m *SessionChainManager) writeCursorToDisk(threadID, agentName string, cursor *AgentCursor) error {
	data, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 Cursor 失败: %w", err)
	}
	path := m.cursorPath(threadID, agentName)
	return os.WriteFile(path, data, 0644)
}

// ReloadThread 从磁盘重新加载指定 Thread 的全部数据到内存
// 用于跨进程场景：API Server 写入后，Agent Worker 需要读取最新数据
func (m *SessionChainManager) ReloadThread(threadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	metaPath := m.metaPath(threadID)
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		return fmt.Errorf("thread %s 不存在", threadID)
	}

	meta, err := m.readMetaFromDisk(threadID)
	if err != nil {
		return err
	}

	m.metas[threadID] = meta
	m.sessions[threadID] = make(map[string]*SessionRecord)
	m.events[threadID] = make(map[string][]SessionEvent)

	for seq := 1; seq <= meta.SessionCount; seq++ {
		sid := sessionIDFromSeq(seq)
		sess, evts, err := m.readSessionMarkdownFromDisk(threadID, sid)
		if err != nil {
			continue
		}
		m.sessions[threadID][sid] = sess
		m.events[threadID][sid] = evts
	}

	// 重新加载 cursors
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

	return nil
}

// --- YAML 辅助函数 ---

func yamlGetString(m map[string]interface{}, key string) string {
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

func yamlGetInt(m map[string]interface{}, key string) int {
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
