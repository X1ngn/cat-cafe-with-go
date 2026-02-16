# 文件结构说明

## 📁 项目结构

### 主程序文件（根目录）
基于 Redis Streams 的完整实现。

```
主程序:
- main.go              # 主程序入口
- scheduler.go         # 调度器核心
- agent_worker.go      # Agent 工作进程
- user_interface.go    # 交互式用户界面

CLI 工具:
- minimal-claude.go    # Claude CLI 包装器
- minimal-codex.go     # Codex CLI 包装器
- minimal-gemini.go    # Gemini CLI 包装器
- invoke.go            # CLI 调用核心逻辑

配置:
- config.yaml          # 系统配置文件
- go.mod               # Go 模块依赖
- Makefile             # 编译脚本
```

### 测试文件（test/ 目录）
所有测试相关文件。

```
test/
├── scheduler_test.go     # 单元测试
└── scheduler_wrapper.go  # 测试包装器
```

### 提示词文件（prompts/ 目录）
Agent 的系统提示词。

```
prompts/
├── calico_cat.md      # 花花（Claude）的提示词
├── lihua_cat.md       # 薇薇（Codex）的提示词
└── silver_cat.md      # 小乔（Gemini）的提示词
```

### 文档文件
```
- README.md                    # 项目说明
- SPEC.md                      # 系统设计规范
- TEST_SPEC.md                 # 测试规范
- TEST_REPORT.md               # 测试报告
- USAGE_GUIDE.md               # 使用指南
- IMPLEMENTATION_SUMMARY.md    # 实现总结
- FILE_STRUCTURE.md            # 本文件
```

## 🔨 编译产物

```
编译后生成:
- cat-cafe             # 主程序可执行文件
- minimal-claude       # Claude CLI 工具
- minimal-codex        # Codex CLI 工具
- minimal-gemini       # Gemini CLI 工具
```

## 📝 命名约定

- 所有文件使用清晰的功能名称
- 测试文件使用 `_test.go` 后缀
- 配置文件使用 `.yaml` 扩展名
- 文档文件使用大写 `.md` 扩展名

## 🎯 使用建议

### 日常开发
```bash
make build
./cat-cafe --list
```

### 运行测试
```bash
cd test && go test -v
```

## 🏗️ 系统架构

### 核心特性
1. **架构**: Redis Streams 异步消息队列
2. **配置**: YAML 配置文件
3. **通信**: 无状态消息传递
4. **可靠性**: 自动重试机制
5. **扩展性**: 动态配置 Agent

## ✅ 文件整理完成

所有文件已按照规范整理:
- ✅ 主版本文件在根目录，命名清晰
- ✅ 测试文件在 test/ 目录
- ✅ 文档完整且更新
- ✅ 编译脚本已更新
- ✅ 所有引用已修正
