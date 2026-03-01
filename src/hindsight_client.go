package main

import "fmt"

// HindsightConfig Hindsight 长期记忆配置
type HindsightConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"`
	Token   string `yaml:"token,omitempty"`
}

// BankIDForAgent 根据 Agent 名称生成 Bank ID
func BankIDForAgent(agentName string) string {
	return fmt.Sprintf("cat-%s", agentName)
}
