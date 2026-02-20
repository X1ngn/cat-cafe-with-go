#!/bin/bash

# 停止所有猫猫咖啡屋服务

echo "🛑 停止猫猫咖啡屋服务..."

# 杀掉监听端口的进程
kill_port() {
    local port=$1
    local name=$2
    local pid=$(lsof -ti:$port 2>/dev/null)
    if [ -n "$pid" ]; then
        kill -9 $pid 2>/dev/null && echo "✓ $name (端口 $port) 已停止"
    fi
}

# 停止 API 服务器 (端口 8081)
kill_port 8080 "API 服务器"
kill_port 9001 "API 服务器"

# 从 PID 文件读取并停止 Agent
if [ -f data/.agent1.pid ]; then
    AGENT1_PID=$(cat data/.agent1.pid)
    kill $AGENT1_PID 2>/dev/null && echo "✓ 花花已停止"
    rm -f data/.agent1.pid
fi

if [ -f data/.agent2.pid ]; then
    AGENT2_PID=$(cat data/.agent2.pid)
    kill $AGENT2_PID 2>/dev/null && echo "✓ 薇薇已停止"
    rm -f data/.agent2.pid
fi

if [ -f data/.agent3.pid ]; then
    AGENT3_PID=$(cat data/.agent3.pid)
    kill $AGENT3_PID 2>/dev/null && echo "✓ 小乔已停止"
    rm -f data/.agent3.pid
fi

# 清理旧的 PID 文件（如果存在）
rm -f .api.pid .agent1.pid .agent2.pid .agent3.pid

echo "✅ 所有服务已停止"
