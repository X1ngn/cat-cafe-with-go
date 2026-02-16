#!/bin/bash

# 停止所有猫猫咖啡屋服务

echo "🛑 停止猫猫咖啡屋服务..."

# 从 PID 文件读取并停止
if [ -f .api.pid ]; then
    API_PID=$(cat .api.pid)
    kill $API_PID 2>/dev/null && echo "✓ API 服务器已停止"
    rm -f .api.pid
fi

if [ -f .agent1.pid ]; then
    AGENT1_PID=$(cat .agent1.pid)
    kill $AGENT1_PID 2>/dev/null && echo "✓ 花花已停止"
    rm -f .agent1.pid
fi

if [ -f .agent2.pid ]; then
    AGENT2_PID=$(cat .agent2.pid)
    kill $AGENT2_PID 2>/dev/null && echo "✓ 薇薇已停止"
    rm -f .agent2.pid
fi

if [ -f .agent3.pid ]; then
    AGENT3_PID=$(cat .agent3.pid)
    kill $AGENT3_PID 2>/dev/null && echo "✓ 小乔已停止"
    rm -f .agent3.pid
fi

echo "✅ 所有服务已停止"
