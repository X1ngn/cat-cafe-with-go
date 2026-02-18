.PHONY: build clean install test test-unit help redis-start redis-stop

# 编译主程序
build:
	@echo "🔨 编译猫猫咖啡屋..."
	go mod download
	go build -o bin/cat-cafe src/main.go src/scheduler.go src/agent_worker.go src/user_interface.go src/api_server.go src/logger.go src/mode_interface.go src/mode_registry.go src/mode_free_discussion.go src/orchestrator.go src/invoke.go src/websocket.go src/session_persistence.go
	go build -o bin/minimal-claude src/minimal-claude.go src/invoke.go
	go build -o bin/minimal-codex src/minimal-codex.go src/invoke.go
	go build -o bin/minimal-gemini src/minimal-gemini.go src/invoke.go
	@echo "✓ 编译完成！"

# 运行单元测试
test-unit:
	@echo "🧪 运行单元测试..."
	go test -v -run "Test" ./test/...
	@echo "✓ 单元测试完成！"

# 运行所有测试
test: redis-start
	@echo "🧪 运行所有测试..."
	@sleep 2
	go test -v -count=1 ./test/...
	@echo "✓ 所有测试完成！"

# 启动 Redis (用于测试)
redis-start:
	@echo "🚀 启动 Redis..."
	@if ! pgrep -x redis-server > /dev/null; then \
		redis-server --daemonize yes --port 6379; \
		echo "✓ Redis 已启动"; \
	else \
		echo "✓ Redis 已在运行"; \
	fi

# 停止 Redis
redis-stop:
	@echo "🛑 停止 Redis..."
	@redis-cli shutdown || true
	@echo "✓ Redis 已停止"

# 安装到系统路径
install: build
	@echo "📦 安装到 /usr/local/bin..."
	sudo cp bin/cat-cafe /usr/local/bin/
	sudo cp bin/minimal-claude /usr/local/bin/
	sudo cp bin/minimal-codex /usr/local/bin/
	sudo cp bin/minimal-gemini /usr/local/bin/
	@echo "✓ 安装完成！"

# 清理编译产物
clean:
	@echo "🧹 清理编译产物..."
	rm -f bin/cat-cafe bin/minimal-claude bin/minimal-codex bin/minimal-gemini
	rm -rf prompts_test config_test*.yaml
	@echo "✓ 清理完成！"

# 帮助信息
help:
	@echo "猫猫咖啡屋 - Multi-Agent 调度器"
	@echo ""
	@echo "可用命令:"
	@echo "  make build        - 编译程序"
	@echo "  make test         - 运行所有测试"
	@echo "  make test-unit    - 运行单元测试"
	@echo "  make redis-start  - 启动 Redis"
	@echo "  make redis-stop   - 停止 Redis"
	@echo "  make install      - 编译并安装到系统"
	@echo "  make clean        - 清理编译产物"
	@echo "  make help         - 显示此帮助信息"
	@echo ""
	@echo "使用示例:"
	@echo "  ./bin/cat-cafe --list                              # 列出所有 Agent"
	@echo "  ./bin/cat-cafe --send --to 花花 --task \"实现HTTP\"   # 发送任务"
	@echo "  ./bin/cat-cafe --mode agent --agent 花花            # 启动 Agent"
