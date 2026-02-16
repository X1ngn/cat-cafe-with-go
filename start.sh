#!/bin/bash

# 猫猫咖啡屋启动脚本

echo "🐱 猫猫咖啡屋启动脚本"
echo ""

# 检查 Redis 是否运行
if ! redis-cli ping > /dev/null 2>&1; then
    echo "❌ Redis 未运行，请先启动 Redis"
    echo "   macOS: brew services start redis"
    echo "   Linux: sudo systemctl start redis"
    exit 1
fi

echo "✓ Redis 已运行"

# 编译项目
echo ""
echo "📦 编译项目..."
make build

if [ $? -ne 0 ]; then
    echo "❌ 编译失败"
    exit 1
fi

echo "✓ 编译成功"

# 启动 API 服务器
echo ""
echo "🚀 启动 API 服务器..."
./bin/cat-cafe --mode api --port 8080 &
API_PID=$!

echo "✓ API 服务器已启动 (PID: $API_PID)"

# 启动 Agent 工作进程
echo ""
echo "🐱 启动猫猫 Agent..."

# 取消 CLAUDECODE 环境变量，避免嵌套会话冲突
env -u CLAUDECODE ./bin/cat-cafe --mode agent --agent 花花 &
AGENT1_PID=$!
echo "✓ 花花已启动 (PID: $AGENT1_PID)"

env -u CLAUDECODE ./bin/cat-cafe --mode agent --agent 薇薇 &
AGENT2_PID=$!
echo "✓ 薇薇已启动 (PID: $AGENT2_PID)"

env -u CLAUDECODE ./bin/cat-cafe --mode agent --agent 小乔 &
AGENT3_PID=$!
echo "✓ 小乔已启动 (PID: $AGENT3_PID)"

echo ""
echo "✅ 所有服务已启动！"
echo ""
echo "📝 进程信息:"
echo "   API 服务器: $API_PID"
echo "   花花: $AGENT1_PID"
echo "   薇薇: $AGENT2_PID"
echo "   小乔: $AGENT3_PID"
echo ""
echo "🌐 API 地址: http://localhost:8080"
echo "📖 API 文档: frontend/docs/API.md"
echo ""
echo "按 Ctrl+C 停止所有服务"

# 保存 PID 到文件
echo "$API_PID" > .api.pid
echo "$AGENT1_PID" > .agent1.pid
echo "$AGENT2_PID" > .agent2.pid
echo "$AGENT3_PID" > .agent3.pid

# 等待中断信号
trap "echo ''; echo '🛑 停止所有服务...'; kill $API_PID $AGENT1_PID $AGENT2_PID $AGENT3_PID 2>/dev/null; rm -f .api.pid .agent1.pid .agent2.pid .agent3.pid; echo '✓ 已停止'; exit 0" INT TERM

wait
