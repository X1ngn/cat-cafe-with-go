#!/bin/bash

# 停止所有猫猫咖啡屋服务

echo "🛑 停止猫猫咖啡屋服务..."

# 杀掉所有 cat-cafe 进程（API 服务器 + Agent Worker）
PIDS=$(pgrep -f './bin/cat-cafe' 2>/dev/null)
if [ -n "$PIDS" ]; then
    echo "$PIDS" | xargs kill 2>/dev/null
    echo "✓ 已停止所有 cat-cafe 进程"
    # 等待进程退出
    sleep 1
    # 如果还有残留，强制杀掉
    REMAINING=$(pgrep -f './bin/cat-cafe' 2>/dev/null)
    if [ -n "$REMAINING" ]; then
        echo "$REMAINING" | xargs kill -9 2>/dev/null
        echo "✓ 已强制停止残留进程"
    fi
else
    echo "  没有运行中的 cat-cafe 进程"
fi

# 停止 Hindsight Docker 容器
HINDSIGHT_CONTAINER="cat-cafe-hindsight"
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${HINDSIGHT_CONTAINER}$"; then
    docker stop "$HINDSIGHT_CONTAINER" > /dev/null 2>&1
    docker rm "$HINDSIGHT_CONTAINER" > /dev/null 2>&1
    echo "✓ 已停止 Hindsight Docker 容器"
else
    echo "  没有运行中的 Hindsight 容器"
fi

# 杀掉监听端口的进程（兜底）
kill_port() {
    local port=$1
    local name=$2
    local pid=$(lsof -ti:$port 2>/dev/null)
    if [ -n "$pid" ]; then
        kill -9 $pid 2>/dev/null && echo "✓ $name (端口 $port) 已停止"
    fi
}

kill_port 8080 "API 服务器"
kill_port 9001 "API 服务器"

# 清理 PID 文件
rm -f logs/.api.pid logs/.agent1.pid logs/.agent2.pid logs/.agent3.pid
rm -f data/.api.pid data/.agent1.pid data/.agent2.pid data/.agent3.pid
rm -f .api.pid .agent1.pid .agent2.pid .agent3.pid

echo "✅ 所有服务已停止"
