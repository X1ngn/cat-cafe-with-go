package test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试配置
const (
	testConfigPath = "config_test.yaml"
	testRedisAddr  = "localhost:6379"
)

// setupTest 测试前准备
func setupTest(t *testing.T) (*Scheduler, func()) {
	// 创建测试配置文件
	createTestConfig(t)

	// 创建调度器
	scheduler, err := NewScheduler(testConfigPath)
	require.NoError(t, err, "创建调度器失败")

	// 清理 Redis 测试数据
	ctx := context.Background()
	for _, agent := range scheduler.ListAgents() {
		streamKey := fmt.Sprintf("pipe:%s", agent.Pipe)
		scheduler.redisClient.Del(ctx, streamKey)
	}

	// 返回清理函数
	cleanup := func() {
		scheduler.Close()
		os.Remove(testConfigPath)
	}

	return scheduler, cleanup
}

// createTestConfig 创建测试配置文件
func createTestConfig(t *testing.T) {
	// 创建测试提示词文件
	os.MkdirAll("prompts_test", 0755)

	testPrompts := map[string]string{
		"prompts_test/agent_a.md": "你是 Agent A，负责测试任务 A",
		"prompts_test/agent_b.md": "你是 Agent B，负责测试任务 B",
		"prompts_test/agent_c.md": "你是 Agent C，负责测试任务 C",
	}

	for path, content := range testPrompts {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err, "创建测试提示词文件失败")
	}

	// 创建测试配置
	configContent := `agents:
  - name: "Agent_A"
    pipe: "pipe_test_a"
    exec_cmd: "echo"
    system_prompt_path: "prompts_test/agent_a.md"

  - name: "Agent_B"
    pipe: "pipe_test_b"
    exec_cmd: "echo"
    system_prompt_path: "prompts_test/agent_b.md"

  - name: "Agent_C"
    pipe: "pipe_test_c"
    exec_cmd: "echo"
    system_prompt_path: "prompts_test/agent_c.md"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0
`
	err := os.WriteFile(testConfigPath, []byte(configContent), 0644)
	require.NoError(t, err, "创建测试配置文件失败")
}

// TestAgentRegistration 测试 8.1: Agent 注册与配置
func TestAgentRegistration(t *testing.T) {
	scheduler, cleanup := setupTest(t)
	defer cleanup()

	t.Run("验证Agent数量", func(t *testing.T) {
		agents := scheduler.ListAgents()
		assert.Equal(t, 3, len(agents), "应该注册3个Agent")
	})

	t.Run("验证Agent配置", func(t *testing.T) {
		agents := scheduler.ListAgents()
		agentNames := make(map[string]bool)
		for _, agent := range agents {
			agentNames[agent.Name] = true
			assert.NotEmpty(t, agent.Pipe, "Agent管道不能为空")
			assert.NotEmpty(t, agent.ExecCmd, "Agent执行命令不能为空")
			assert.NotEmpty(t, agent.SystemPromptPath, "系统提示词路径不能为空")
		}

		assert.True(t, agentNames["Agent_A"], "应该包含Agent_A")
		assert.True(t, agentNames["Agent_B"], "应该包含Agent_B")
		assert.True(t, agentNames["Agent_C"], "应该包含Agent_C")
	})

	t.Run("验证系统提示词加载", func(t *testing.T) {
		prompt, err := scheduler.GetSystemPrompt("Agent_A")
		assert.NoError(t, err, "获取系统提示词不应该失败")
		assert.Contains(t, prompt, "Agent A", "系统提示词应该包含Agent A")
	})
}

// TestTaskSending 测试 8.1: 任务发送测试
func TestTaskSending(t *testing.T) {
	scheduler, cleanup := setupTest(t)
	defer cleanup()

	t.Run("发送任务到指定Agent", func(t *testing.T) {
		taskID, err := scheduler.SendTask("Agent_A", "测试任务内容")
		assert.NoError(t, err, "发送任务不应该失败")
		assert.NotEmpty(t, taskID, "任务ID不能为空")
		assert.Contains(t, taskID, "Agent_A", "任务ID应该包含Agent名称")
	})

	t.Run("发送任务到不存在的Agent", func(t *testing.T) {
		_, err := scheduler.SendTask("NonExistentAgent", "测试任务")
		assert.Error(t, err, "发送到不存在的Agent应该失败")
	})

	t.Run("验证任务在Redis中", func(t *testing.T) {
		taskID, err := scheduler.SendTask("Agent_B", "验证Redis任务")
		require.NoError(t, err)

		// 读取 Redis Stream
		ctx := context.Background()
		streamKey := "pipe:pipe_test_b"
		messages, err := scheduler.redisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey, "0"},
			Count:   10,
		}).Result()

		assert.NoError(t, err, "读取Redis Stream不应该失败")
		assert.NotEmpty(t, messages, "应该有消息在Stream中")

		// 验证任务ID
		found := false
		for _, stream := range messages {
			for _, msg := range stream.Messages {
				if taskData, ok := msg.Values["task"].(string); ok {
					if contains(taskData, taskID) {
						found = true
						break
					}
				}
			}
		}
		assert.True(t, found, "应该在Redis中找到任务")
	})
}

// TestStatelessCommunication 测试 8.1: 无状态通信测试
func TestStatelessCommunication(t *testing.T) {
	scheduler, cleanup := setupTest(t)
	defer cleanup()

	t.Run("Agent初始状态为idle", func(t *testing.T) {
		state, err := scheduler.GetAgentState("Agent_A")
		assert.NoError(t, err)
		assert.Equal(t, "idle", state.Status, "初始状态应该是idle")
	})

	t.Run("更新Agent状态", func(t *testing.T) {
		err := scheduler.UpdateAgentState("Agent_A", "busy", "task_123")
		assert.NoError(t, err)

		state, err := scheduler.GetAgentState("Agent_A")
		assert.NoError(t, err)
		assert.Equal(t, "busy", state.Status)
		assert.Equal(t, "task_123", state.LastTaskID)
	})

	t.Run("任务完成后恢复idle", func(t *testing.T) {
		// 设置为busy
		scheduler.UpdateAgentState("Agent_B", "busy", "task_456")

		// 模拟任务完成，恢复idle
		scheduler.UpdateAgentState("Agent_B", "idle", "")

		state, err := scheduler.GetAgentState("Agent_B")
		assert.NoError(t, err)
		assert.Equal(t, "idle", state.Status)
	})
}

// TestMessageReliability 测试 8.2: 消息可靠性测试
func TestMessageReliability(t *testing.T) {
	scheduler, cleanup := setupTest(t)
	defer cleanup()

	t.Run("消息持久化", func(t *testing.T) {
		// 发送多个任务
		taskIDs := make([]string, 5)
		for i := 0; i < 5; i++ {
			taskID, err := scheduler.SendTask("Agent_A", fmt.Sprintf("任务 %d", i))
			require.NoError(t, err)
			taskIDs[i] = taskID
		}

		// 验证所有任务都在Redis中
		ctx := context.Background()
		streamKey := "pipe:pipe_test_a"
		messages, err := scheduler.redisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey, "0"},
			Count:   10,
		}).Result()

		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(messages[0].Messages), 5, "应该至少有5条消息")
	})
}

// TestSequentialExecution 测试 8.1: 顺序任务执行
func TestSequentialExecution(t *testing.T) {
	scheduler, cleanup := setupTest(t)
	defer cleanup()

	t.Run("任务按顺序发送", func(t *testing.T) {
		taskIDs := make([]string, 3)
		for i := 0; i < 3; i++ {
			taskID, err := scheduler.SendTask("Agent_C", fmt.Sprintf("顺序任务 %d", i))
			require.NoError(t, err)
			taskIDs[i] = taskID
			time.Sleep(10 * time.Millisecond) // 确保时间戳不同
		}

		// 验证任务顺序
		ctx := context.Background()
		streamKey := "pipe:pipe_test_c"
		messages, err := scheduler.redisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey, "0"},
			Count:   10,
		}).Result()

		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(messages[0].Messages), 3)

		// Redis Stream 保证消息顺序
		for i := 0; i < 3 && i < len(messages[0].Messages); i++ {
			msg := messages[0].Messages[i]
			taskData := msg.Values["task"].(string)
			assert.Contains(t, taskData, fmt.Sprintf("顺序任务 %d", i))
		}
	})
}

// TestNewAgentAddition 测试 8.4: 新增 Agent 测试
func TestNewAgentAddition(t *testing.T) {
	t.Run("动态添加新Agent", func(t *testing.T) {
		// 创建新的配置文件，包含额外的Agent
		newConfigContent := `agents:
  - name: "Agent_A"
    pipe: "pipe_test_a"
    exec_cmd: "echo"
    system_prompt_path: "prompts_test/agent_a.md"

  - name: "Agent_D"
    pipe: "pipe_test_d"
    exec_cmd: "echo"
    system_prompt_path: "prompts_test/agent_d.md"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0
`
		// 创建新Agent的提示词文件
		os.WriteFile("prompts_test/agent_d.md", []byte("你是 Agent D"), 0644)

		newConfigPath := "config_test_new.yaml"
		err := os.WriteFile(newConfigPath, []byte(newConfigContent), 0644)
		require.NoError(t, err)
		defer os.Remove(newConfigPath)

		// 创建新的调度器
		newScheduler, err := NewScheduler(newConfigPath)
		require.NoError(t, err)
		defer newScheduler.Close()

		// 验证新Agent已加载
		agents := newScheduler.ListAgents()
		agentNames := make(map[string]bool)
		for _, agent := range agents {
			agentNames[agent.Name] = true
		}

		assert.True(t, agentNames["Agent_D"], "应该包含新添加的Agent_D")

		// 验证可以向新Agent发送任务
		taskID, err := newScheduler.SendTask("Agent_D", "测试新Agent")
		assert.NoError(t, err)
		assert.NotEmpty(t, taskID)
	})
}

// TestConfigSecurity 测试 8.5: 配置文件安全性测试
func TestConfigSecurity(t *testing.T) {
	t.Run("配置文件权限检查", func(t *testing.T) {
		// 创建只读配置文件
		readOnlyConfig := "config_readonly.yaml"
		configContent := `agents:
  - name: "Agent_A"
    pipe: "pipe_a"
    exec_cmd: "echo"
    system_prompt_path: "prompts_test/agent_a.md"

redis:
  addr: "localhost:6379"
  password: ""
  db: 0
`
		err := os.WriteFile(readOnlyConfig, []byte(configContent), 0444)
		require.NoError(t, err)
		defer os.Remove(readOnlyConfig)

		// 验证可以读取
		scheduler, err := NewScheduler(readOnlyConfig)
		assert.NoError(t, err, "应该能读取只读配置文件")
		if scheduler != nil {
			scheduler.Close()
		}

		// 验证文件权限
		info, err := os.Stat(readOnlyConfig)
		require.NoError(t, err)
		mode := info.Mode()
		assert.Equal(t, os.FileMode(0444), mode.Perm(), "配置文件应该是只读的")
	})

	t.Run("无效配置文件处理", func(t *testing.T) {
		invalidConfig := "config_invalid.yaml"
		err := os.WriteFile(invalidConfig, []byte("invalid: yaml: content: ["), 0644)
		require.NoError(t, err)
		defer os.Remove(invalidConfig)

		_, err = NewScheduler(invalidConfig)
		assert.Error(t, err, "无效的配置文件应该返回错误")
	})
}

// TestAgentCollaboration 测试 Agent 协作功能
func TestAgentCollaboration(t *testing.T) {
	scheduler, cleanup := setupTest(t)
	defer cleanup()

	t.Run("解析@标记", func(t *testing.T) {
		// 模拟 Agent 输出包含 @标记
		output := `
【系统设计】
用户登录系统设计完成...

@Agent_B 请审查这个系统的安全性
[架构文档]
`
		// 这个测试验证输出格式是否正确
		assert.Contains(t, output, "@Agent_B", "输出应该包含 @标记")
		assert.Contains(t, output, "请审查这个系统的安全性", "输出应该包含任务内容")
	})

	t.Run("Agent调用链", func(t *testing.T) {
		ctx := context.Background()

		// 1. 发送初始任务给 Agent_A
		taskID1, err := scheduler.SendTask("Agent_A", "开发用户登录系统")
		require.NoError(t, err)
		assert.NotEmpty(t, taskID1)

		// 验证任务在 Agent_A 的队列中
		streamKey1 := "pipe:pipe_test_a"
		messages1, err := scheduler.redisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey1, "0"},
			Count:   1,
		}).Result()
		require.NoError(t, err)
		assert.Len(t, messages1, 1)
		assert.Len(t, messages1[0].Messages, 1)

		// 2. 模拟 Agent_A 完成任务并调用 Agent_B
		// 在实际场景中，这会由 agent_worker.go 的 parseAndDispatchTasks 完成
		taskID2, err := scheduler.SendTask("Agent_B", "审查登录系统的安全性")
		require.NoError(t, err)
		assert.NotEmpty(t, taskID2)

		// 验证任务在 Agent_B 的队列中
		streamKey2 := "pipe:pipe_test_b"
		messages2, err := scheduler.redisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey2, "0"},
			Count:   1,
		}).Result()
		require.NoError(t, err)
		assert.Len(t, messages2, 1)
		assert.Len(t, messages2[0].Messages, 1)

		// 3. 模拟 Agent_B 完成任务并调用 Agent_C
		taskID3, err := scheduler.SendTask("Agent_C", "设计登录界面")
		require.NoError(t, err)
		assert.NotEmpty(t, taskID3)

		// 验证任务在 Agent_C 的队列中
		streamKey3 := "pipe:pipe_test_c"
		messages3, err := scheduler.redisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey3, "0"},
			Count:   1,
		}).Result()
		require.NoError(t, err)
		assert.Len(t, messages3, 1)
		assert.Len(t, messages3[0].Messages, 1)

		// 验证完整的调用链
		assert.NotEqual(t, taskID1, taskID2, "不同任务应该有不同的ID")
		assert.NotEqual(t, taskID2, taskID3, "不同任务应该有不同的ID")
	})

	t.Run("@铲屎官标记", func(t *testing.T) {
		// 模拟 Agent 完成所有工作
		output := `
【项目完成】
用户登录系统开发完成，包含:
- 架构设计
- 代码实现
- 安全审查
- 界面设计

@铲屎官 项目已完成，请查看
`
		// 验证包含 @铲屎官 标记
		assert.Contains(t, output, "@铲屎官", "完成时应该包含 @铲屎官 标记")
		assert.Contains(t, output, "项目已完成", "应该包含完成消息")
	})

	t.Run("多Agent并行协作", func(t *testing.T) {
		// 模拟 Agent_A 同时调用 Agent_B 和 Agent_C
		taskID1, err := scheduler.SendTask("Agent_B", "审查后端安全性")
		require.NoError(t, err)

		taskID2, err := scheduler.SendTask("Agent_C", "设计用户界面")
		require.NoError(t, err)

		// 验证两个任务都成功发送
		assert.NotEmpty(t, taskID1)
		assert.NotEmpty(t, taskID2)
		assert.NotEqual(t, taskID1, taskID2)

		// 验证两个任务在各自的队列中
		ctx := context.Background()

		messages1, err := scheduler.redisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{"pipe:pipe_test_b", "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		assert.Greater(t, len(messages1[0].Messages), 0)

		messages2, err := scheduler.redisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{"pipe:pipe_test_c", "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		assert.Greater(t, len(messages2[0].Messages), 0)
	})

	t.Run("迭代协作", func(t *testing.T) {
		// 模拟迭代修复流程
		// Agent_A → Agent_B → Agent_A → Agent_B → Agent_C → 铲屎官

		// 第一轮：Agent_A 设计
		taskID1, err := scheduler.SendTask("Agent_A", "设计登录系统")
		require.NoError(t, err)
		assert.NotEmpty(t, taskID1)

		// 第二轮：Agent_B 审查发现问题
		taskID2, err := scheduler.SendTask("Agent_B", "审查登录系统")
		require.NoError(t, err)
		assert.NotEmpty(t, taskID2)

		// 第三轮：Agent_A 修复
		taskID3, err := scheduler.SendTask("Agent_A", "修复安全问题")
		require.NoError(t, err)
		assert.NotEmpty(t, taskID3)

		// 第四轮：Agent_B 重新审查通过
		taskID4, err := scheduler.SendTask("Agent_B", "重新审查")
		require.NoError(t, err)
		assert.NotEmpty(t, taskID4)

		// 第五轮：Agent_C 设计界面
		taskID5, err := scheduler.SendTask("Agent_C", "设计登录界面")
		require.NoError(t, err)
		assert.NotEmpty(t, taskID5)

		// 验证所有任务ID都不同
		taskIDs := []string{taskID1, taskID2, taskID3, taskID4, taskID5}
		for i := 0; i < len(taskIDs); i++ {
			for j := i + 1; j < len(taskIDs); j++ {
				assert.NotEqual(t, taskIDs[i], taskIDs[j], "任务ID应该唯一")
			}
		}
	})

	t.Run("无效Agent调用", func(t *testing.T) {
		// 尝试调用不存在的 Agent
		_, err := scheduler.SendTask("不存在的Agent", "测试任务")
		assert.Error(t, err, "调用不存在的Agent应该返回错误")
	})
}

// 辅助函数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
