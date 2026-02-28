package main

import (
	"fmt"
	"strings"
)

// buildOrchestratedPrompt 策略 A：调度系统管理
// 每次调用传入活跃 Session 的全部 Event，不使用 --resume
func (w *AgentWorker) buildOrchestratedPrompt(task *TaskMessage) string {
	threadID := task.SessionID
	if threadID == "" || w.chainManager == nil {
		return w.buildLegacyPrompt(w.getSessionHistory(task.SessionID), task)
	}

	// 确保 chain 存在
	_, err := w.chainManager.GetOrCreateChain(threadID)
	if err != nil {
		LogWarn("[Agent-%s] 获取 Session Chain 失败: %v，回退到旧逻辑", w.config.Name, err)
		return w.buildLegacyPrompt(w.getSessionHistory(task.SessionID), task)
	}

	// 读取活跃 Session 的所有 Event
	activeSession, err := w.chainManager.GetActiveSession(threadID)
	if err != nil {
		LogWarn("[Agent-%s] 获取活跃 Session 失败: %v", w.config.Name, err)
		return w.buildLegacyPrompt("", task)
	}

	events, _, err := w.chainManager.GetEvents(threadID, activeSession.ID, 0, 10000)
	if err != nil {
		LogWarn("[Agent-%s] 读取 Event 失败: %v", w.config.Name, err)
		return w.buildLegacyPrompt("", task)
	}

	// 收集已 seal 的 Session 的 Summary
	summaries := w.collectSealedSummaries(threadID)

	// 格式化
	history := formatEventsAsHistory(events)

	var sb strings.Builder
	sb.WriteString(w.systemPrompt)
	sb.WriteString("\n\n========================================\n\n")

	if summaries != "" {
		sb.WriteString("【历史摘要】\n")
		sb.WriteString(summaries)
		sb.WriteString("\n========================================\n\n")
	}

	if history != "" {
		sb.WriteString("【对话历史】\n")
		sb.WriteString(history)
		sb.WriteString("\n========================================\n\n")
	}

	sb.WriteString(fmt.Sprintf("🎯 你是%s，请回应以下消息：\n%s\n\n请结合上面的对话历史来完成任务。",
		w.config.Name, task.Content))

	return sb.String()
}

// buildCLIManagedPrompt 策略 B：CLI 自动管理
// 使用 --resume + AI session ID，只传增量 Event
// 返回: (prompt, aiSessionID)
func (w *AgentWorker) buildCLIManagedPrompt(task *TaskMessage) (string, string) {
	threadID := task.SessionID
	if threadID == "" || w.chainManager == nil {
		chatHistory := w.getSessionHistory(task.SessionID)
		return w.buildLegacyPrompt(chatHistory, task), ""
	}

	_, err := w.chainManager.GetOrCreateChain(threadID)
	if err != nil {
		LogWarn("[Agent-%s] 获取 Session Chain 失败: %v，回退到旧逻辑", w.config.Name, err)
		return w.buildLegacyPrompt("", task), ""
	}

	cursor := w.chainManager.GetCursor(w.config.Name, threadID)

	var incrementalEvents []SessionEvent
	var aiSessionID string

	if cursor == nil {
		// 首次调用：传入活跃 Session 的所有 Event
		activeSession, err := w.chainManager.GetActiveSession(threadID)
		if err == nil {
			incrementalEvents, _, _ = w.chainManager.GetEvents(threadID, activeSession.ID, 0, 10000)
		}
		aiSessionID = ""
	} else {
		aiSessionID = cursor.AISessionID

		// 检查 cursor 指向的 Session 是否已 seal
		cursorSession, err := w.chainManager.GetSession(threadID, cursor.LastSessionID)
		if err == nil && cursorSession.Status != SCSessionActive {
			// Cursor 指向已 seal 的 Session，需要包含 Summary + 增量
			summaries := w.collectSummariesAfter(threadID, cursor.LastSessionID)
			incrementalEvents, _ = w.chainManager.GetEventsAfter(threadID, cursor.LastSessionID, cursor.LastEventNo)

			// 如果有 summary，需要重置 AI session（上下文已断裂）
			if summaries != "" {
				aiSessionID = "" // 强制新建 session
				// 构建包含 summary 的 prompt
				history := formatEventsAsHistory(incrementalEvents)
				var sb strings.Builder
				sb.WriteString(w.systemPrompt)
				sb.WriteString("\n\n========================================\n\n")
				sb.WriteString("【历史摘要（已压缩）】\n")
				sb.WriteString(summaries)
				sb.WriteString("\n========================================\n\n")
				if history != "" {
					sb.WriteString("【最近对话】\n")
					sb.WriteString(history)
					sb.WriteString("\n========================================\n\n")
				}
				sb.WriteString(fmt.Sprintf("🎯 你是%s，请回应以下消息：\n%s",
					w.config.Name, task.Content))
				return sb.String(), aiSessionID
			}
		} else {
			// 正常增量读取
			incrementalEvents, _ = w.chainManager.GetEventsAfter(threadID, cursor.LastSessionID, cursor.LastEventNo)
		}
	}

	// 构建增量 prompt
	history := formatEventsAsHistory(incrementalEvents)

	var sb strings.Builder
	if aiSessionID == "" {
		// 首次调用，需要完整 prompt
		sb.WriteString(w.systemPrompt)
		sb.WriteString("\n\n========================================\n\n")
	}

	if history != "" {
		if aiSessionID != "" {
			// 有 AI session，只传增量内容
			sb.WriteString("【新消息】\n")
		} else {
			sb.WriteString("【对话历史】\n")
		}
		sb.WriteString(history)
		sb.WriteString("\n========================================\n\n")
	}

	sb.WriteString(fmt.Sprintf("🎯 你是%s，请回应以下消息：\n%s",
		w.config.Name, task.Content))

	return sb.String(), aiSessionID
}

// buildLegacyPrompt 旧逻辑兼容：无 context_mode 配置时使用
func (w *AgentWorker) buildLegacyPrompt(chatHistory string, task *TaskMessage) string {
	if chatHistory != "" {
		return fmt.Sprintf("%s\n\n========================================\n\n【对话历史】\n%s\n========================================\n\n🎯 你是%s，请回应以下消息：\n%s\n\n请结合上面的对话历史来完成任务。",
			w.systemPrompt, chatHistory, w.config.Name, task.Content)
	}
	return fmt.Sprintf("%s\n\n========================================\n\n用户需求：\n%s",
		w.systemPrompt, task.Content)
}

// collectSealedSummaries 收集所有已 seal 的 Session 的 Summary
func (w *AgentWorker) collectSealedSummaries(threadID string) string {
	sessions, err := w.chainManager.ListSessions(threadID)
	if err != nil {
		return ""
	}

	var summaries []string
	for _, s := range sessions {
		if s.Status == SCSessionActive {
			continue
		}
		if s.Summary != "" {
			summaries = append(summaries, fmt.Sprintf("[Session #%d] %s", s.SeqNo, s.Summary))
		}
	}

	return strings.Join(summaries, "\n")
}

// collectSummariesAfter 收集指定 Session 之后的所有 sealed Session 的 Summary
func (w *AgentWorker) collectSummariesAfter(threadID, afterSessionID string) string {
	sessions, err := w.chainManager.ListSessions(threadID)
	if err != nil {
		return ""
	}

	var summaries []string
	found := false
	for _, s := range sessions {
		if s.ID == afterSessionID {
			found = true
			// 包含当前 session 的 summary（如果有）
			if s.Summary != "" {
				summaries = append(summaries, fmt.Sprintf("[Session #%d] %s", s.SeqNo, s.Summary))
			}
			continue
		}
		if found && s.Status != SCSessionActive && s.Summary != "" {
			summaries = append(summaries, fmt.Sprintf("[Session #%d] %s", s.SeqNo, s.Summary))
		}
	}

	return strings.Join(summaries, "\n")
}

// formatEventsAsHistory 将 Event 列表格式化为对话历史文本
func formatEventsAsHistory(events []SessionEvent) string {
	if len(events) == 0 {
		return ""
	}

	const maxContentLen = 500
	var sb strings.Builder

	for _, e := range events {
		content := e.Content
		switch e.Type {
		case SCEventUser:
			sb.WriteString(fmt.Sprintf("[用户] %s\n", content))
		case SCEventCat:
			if len(content) > maxContentLen {
				content = content[:maxContentLen] + "...(已截断)"
			}
			sb.WriteString(fmt.Sprintf("[%s] %s\n", e.Sender, content))
		case SCEventSystem:
			sb.WriteString(fmt.Sprintf("[系统] %s\n", content))
		case SCEventInvocation:
			// 简化 invocation 信息
			if len(content) > maxContentLen {
				content = content[:maxContentLen] + "...(已截断)"
			}
			sb.WriteString(fmt.Sprintf("[%s:调用] %s\n", e.Sender, content))
		}
	}

	return strings.TrimSpace(sb.String())
}
