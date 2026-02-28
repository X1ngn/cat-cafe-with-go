package main

import "unicode/utf8"

// EstimateTokens 估算文本的 token 数量
// 中文等多字节字符：每个约 2 token
// ASCII 字符：约 4 字符 ≈ 1 token
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	tokens := 0
	asciiCount := 0
	for _, r := range text {
		if utf8.RuneLen(r) > 1 {
			tokens += 2
		} else {
			asciiCount++
		}
	}
	if asciiCount > 0 {
		tokens += (asciiCount + 3) / 4
	}
	return tokens
}

// ContextTokenReport 上下文 token 评估报告
type ContextTokenReport struct {
	SystemPromptTokens  int     `json:"systemPromptTokens"`
	SummaryTokens       int     `json:"summaryTokens"`
	EventTokens         int     `json:"eventTokens"`
	CurrentTaskTokens   int     `json:"currentTaskTokens"`
	TotalTokens         int     `json:"totalTokens"`
	MaxTokens           int     `json:"maxTokens"`
	UsagePercent        float64 `json:"usagePercent"`
	RemainingTokens     int     `json:"remainingTokens"`
	EventCount          int     `json:"eventCount"`
	MaxEventsPerSession int     `json:"maxEventsPerSession"`
}

// ContextTokenEstimator 上下文长度评估器
type ContextTokenEstimator struct {
	maxTokens           int
	maxEventsPerSession int
}

// NewContextTokenEstimator 创建评估器
func NewContextTokenEstimator(maxTokens, maxEventsPerSession int) *ContextTokenEstimator {
	return &ContextTokenEstimator{
		maxTokens:           maxTokens,
		maxEventsPerSession: maxEventsPerSession,
	}
}

// EstimateContext 评估完整上下文的 token 消耗
func (e *ContextTokenEstimator) EstimateContext(
	systemPrompt string,
	sealedSummaries []string,
	events []SessionEvent,
	currentTask string,
) *ContextTokenReport {
	report := &ContextTokenReport{
		MaxTokens:           e.maxTokens,
		MaxEventsPerSession: e.maxEventsPerSession,
	}

	report.SystemPromptTokens = EstimateTokens(systemPrompt)

	for _, s := range sealedSummaries {
		report.SummaryTokens += EstimateTokens(s)
	}

	for _, ev := range events {
		report.EventTokens += ev.TokenCount
	}
	report.EventCount = len(events)

	report.CurrentTaskTokens = EstimateTokens(currentTask)

	report.TotalTokens = report.SystemPromptTokens + report.SummaryTokens + report.EventTokens + report.CurrentTaskTokens

	if e.maxTokens > 0 {
		report.UsagePercent = float64(report.TotalTokens) / float64(e.maxTokens)
		if report.UsagePercent > 1.0 {
			report.UsagePercent = 1.0
		}
		report.RemainingTokens = e.maxTokens - report.TotalTokens
		if report.RemainingTokens < 0 {
			report.RemainingTokens = 0
		}
	}

	return report
}

// EstimateSessionUsage 仅评估当前 Session 的 token 使用情况（轻量版）
func (e *ContextTokenEstimator) EstimateSessionUsage(
	session *SessionRecord,
	config *SessionChainConfig,
) *ContextTokenReport {
	maxTokens := e.maxTokens
	maxEvents := e.maxEventsPerSession
	if config != nil {
		if config.MaxTokens > 0 {
			maxTokens = config.MaxTokens
		}
		if config.MaxEventsPerSession > 0 {
			maxEvents = config.MaxEventsPerSession
		}
	}

	report := &ContextTokenReport{
		EventTokens:         session.TokenCount,
		TotalTokens:         session.TokenCount,
		MaxTokens:           maxTokens,
		EventCount:          session.EventCount,
		MaxEventsPerSession: maxEvents,
	}

	if maxTokens > 0 {
		report.UsagePercent = float64(session.TokenCount) / float64(maxTokens)
		if report.UsagePercent > 1.0 {
			report.UsagePercent = 1.0
		}
		report.RemainingTokens = maxTokens - session.TokenCount
		if report.RemainingTokens < 0 {
			report.RemainingTokens = 0
		}
	}

	return report
}
