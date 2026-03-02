# UTF-8 编码错误修复说明

## 问题描述

薇薇 Agent 在调用 `codex` CLI 时遇到 UTF-8 编码错误：

```
Failed to read prompt from stdin: input is not valid UTF-8 (invalid byte at offset 5819).
Convert it to UTF-8 and retry (e.g., `iconv -f <ENC> -t UTF-8 prompt.txt`).
```

**错误日志**: `logs/agent_weiwei.log` (第 17-22 行)

**影响**:
- 任务执行失败
- 重试 3 次后放弃
- 用户无法收到回复

## 根本原因

在 `src/invoke.go` 第 108 行，代码直接将 prompt 写入 codex CLI 的 stdin：

```go
stdin.Write([]byte(prompt))
```

**问题**：
- 如果 prompt 中包含非 UTF-8 字符（如损坏的 emoji、特殊字符、二进制数据等），转换为字节数组时会产生无效的 UTF-8 序列
- codex CLI 严格验证 stdin 输入必须是有效的 UTF-8
- 无效字符可能来自：
  - Session Chain 中存储的历史消息
  - System Prompt 文件
  - 用户输入的特殊字符

## 解决方案

### 1. 添加 UTF-8 验证和清理

**修改文件**: `src/invoke.go`

**新增导入**:
```go
import (
    // ...
    "unicode/utf8"
)
```

**修改 stdin 写入逻辑** (第 100-111 行):
```go
// 对于 codex，通过 stdin 传递 prompt
if cliName == "codex" {
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return "", "", fmt.Errorf("无法获取 stdin: %w", err)
    }
    go func() {
        defer stdin.Close()
        // 确保 prompt 是有效的 UTF-8
        validPrompt := ensureValidUTF8(prompt)
        stdin.Write([]byte(validPrompt))
    }()
}
```

**新增辅助函数** (第 234-243 行):
```go
// ensureValidUTF8 确保字符串是有效的 UTF-8 编码
// 将所有无效的 UTF-8 字节序列替换为 Unicode 替换字符 (U+FFFD)
func ensureValidUTF8(s string) string {
    if utf8.ValidString(s) {
        return s
    }

    // 使用 strings.ToValidUTF8 替换无效字符
    // 替换字符使用 � (U+FFFD, Unicode replacement character)
    return strings.ToValidUTF8(s, "�")
}
```

### 2. 工作原理

1. **快速路径**: 如果字符串已经是有效的 UTF-8，直接返回（零开销）
2. **清理路径**: 如果包含无效字节，使用 `strings.ToValidUTF8()` 替换：
   - 无效的 UTF-8 字节序列 → `�` (U+FFFD, Unicode 替换字符)
   - 保留所有有效的 UTF-8 字符
   - 返回清理后的字符串

### 3. 示例

```go
// 有效的 UTF-8 - 不做修改
ensureValidUTF8("Hello 世界 🌍")
// → "Hello 世界 🌍"

// 包含无效字节 - 替换为 �
ensureValidUTF8("Hello \xFF\xFE World")
// → "Hello �� World"

// 部分有效 - 只替换无效部分
ensureValidUTF8("正常文本\x80\x81异常字节")
// → "正常文本��异常字节"
```

## 为什么只修复 codex？

**当前实现**：
- `claude` CLI: 使用 `-p` 参数传递 prompt（命令行参数）
- `gemini` CLI: 使用 `-p` 参数传递 prompt（命令行参数）
- `codex` CLI: 使用 stdin 传递 prompt（管道输入）

**原因**：
- 命令行参数由 shell 处理，会自动处理编码问题
- stdin 是原始字节流，需要严格的 UTF-8 验证
- codex CLI 对 stdin 输入有严格的 UTF-8 校验

**未来优化**：
- 可以考虑对所有 CLI 都添加 UTF-8 验证（防御性编程）
- 但目前只有 codex 遇到了这个问题

## 测试建议

### 单元测试

```go
func TestEnsureValidUTF8(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "valid UTF-8",
            input:    "Hello 世界 🌍",
            expected: "Hello 世界 🌍",
        },
        {
            name:     "invalid bytes",
            input:    "Hello \xFF\xFE World",
            expected: "Hello �� World",
        },
        {
            name:     "empty string",
            input:    "",
            expected: "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := ensureValidUTF8(tt.input)
            if result != tt.expected {
                t.Errorf("expected %q, got %q", tt.expected, result)
            }
        })
    }
}
```

### 集成测试

1. **正常场景**：发送包含中文、emoji 的消息，验证正常工作
2. **异常场景**：构造包含无效 UTF-8 的 prompt，验证能够正常处理
3. **性能测试**：验证 UTF-8 验证不会显著影响性能

### 手动测试

```bash
# 1. 启动系统
go run src/*.go

# 2. 发送测试消息
curl -X POST http://localhost:8080/api/send \
  -H "Content-Type: application/json" \
  -d '{
    "agent_name": "薇薇",
    "content": "测试中文和 emoji 🎉",
    "session_id": "test_session"
  }'

# 3. 检查日志
tail -f logs/agent_weiwei.log
```

## 性能影响

- **有效 UTF-8 字符串**（99% 的情况）：
  - `utf8.ValidString()` 是 O(n) 但非常快
  - 直接返回，零额外开销

- **无效 UTF-8 字符串**（罕见）：
  - `strings.ToValidUTF8()` 需要遍历并替换
  - 开销可接受（只在出错时触发）

## 相关资源

- Go 文档: [unicode/utf8](https://pkg.go.dev/unicode/utf8)
- Go 文档: [strings.ToValidUTF8](https://pkg.go.dev/strings#ToValidUTF8)
- Unicode 替换字符: [U+FFFD](https://www.fileformat.info/info/unicode/char/fffd/index.htm)

## 提交信息

```
commit 2cd174d
Author: [Your Name]
Date:   2026-03-02

fix: 修复 codex CLI 的 UTF-8 编码错误

- 在写入 stdin 之前验证并清理 prompt 中的无效 UTF-8 字符
- 添加 ensureValidUTF8 函数，使用 strings.ToValidUTF8 替换无效字节
- 修复错误: Failed to read prompt from stdin: input is not valid UTF-8
```

## 后续工作

- [ ] 添加单元测试
- [ ] 进行集成测试验证修复有效
- [ ] 考虑是否需要对 claude 和 gemini CLI 也添加类似保护
- [ ] 监控日志，确认不再出现 UTF-8 错误
- [ ] 如果问题仍然存在，需要排查 prompt 来源（Session Chain、System Prompt 等）
