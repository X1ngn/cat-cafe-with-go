#!/bin/bash

# 猫猫咖啡屋启动脚本（支持 Nginx 代理）

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

# 检查是否使用 Nginx 模式
USE_NGINX=false
if [ "$1" = "--nginx" ]; then
    USE_NGINX=true
    echo "✓ 使用 Nginx 代理模式"
fi

# 编译项目
echo ""
echo "📦 编译项目..."
make build

if [ $? -ne 0 ]; then
    echo "❌ 编译失败"
    exit 1
fi

echo "✓ 编译成功"

# 确定 API 服务器端口
if [ "$USE_NGINX" = true ]; then
    API_PORT=9001  # Nginx 代理模式使用 9001
    echo ""
    echo "🌐 Nginx 模式: API 服务器将在 :9001 启动，通过 :8080 访问"
else
    API_PORT=8080  # 直连模式
    echo ""
    echo "🌐 直连模式: API 服务器将在 :8080 启动"
fi

# 启动 Hindsight 长期记忆服务（Docker）
echo ""
echo "🧠 启动 Hindsight 长期记忆服务..."
HINDSIGHT_ENV="$(cd "$(dirname "$0")/hindsight" && pwd)/.env"
HINDSIGHT_CONTAINER="cat-cafe-hindsight"

# 检查 Docker 是否可用
if ! command -v docker &> /dev/null; then
    echo "⚠️  Docker 未安装，跳过 Hindsight 启动"
    HINDSIGHT_STARTED=false
elif [ ! -f "$HINDSIGHT_ENV" ]; then
    echo "⚠️  Hindsight .env 不存在，跳过启动（请先配置 hindsight/.env）"
    HINDSIGHT_STARTED=false
else
    # 读取 .env 中的 API key
    OPENAI_API_KEY=$(grep '^HINDSIGHT_API_LLM_API_KEY=' "$HINDSIGHT_ENV" | cut -d'=' -f2-)

    # 如果已有同名容器在跑，跳过启动
    if docker ps --format '{{.Names}}' | grep -q "^${HINDSIGHT_CONTAINER}$"; then
        echo "✓ Hindsight 容器已在运行"
        HINDSIGHT_STARTED=true
    else
        # 清理同名的已停止容器
        docker rm -f "$HINDSIGHT_CONTAINER" 2>/dev/null

        docker run -d --name "$HINDSIGHT_CONTAINER" \
            -p 8888:8888 -p 9999:9999 \
            -e HINDSIGHT_API_LLM_API_KEY="$OPENAI_API_KEY" \
            -v "$HOME/.hindsight-docker:/home/hindsight/.pg0" \
            ghcr.io/vectorize-io/hindsight:latest > /dev/null 2>&1

        if [ $? -eq 0 ]; then
            echo "✓ Hindsight Docker 容器已启动 (Port: 8888)"
            HINDSIGHT_STARTED=true
            # 等待服务就绪
            echo "  等待 Hindsight 就绪..."
            for i in $(seq 1 15); do
                if curl -s http://localhost:8888/health > /dev/null 2>&1; then
                    echo "  ✓ Hindsight 已就绪"
                    break
                fi
                sleep 2
            done
        else
            echo "⚠️  Hindsight Docker 启动失败，查看: docker logs $HINDSIGHT_CONTAINER"
            HINDSIGHT_STARTED=false
        fi
    fi
fi

# 启动 API 服务器
echo ""
echo "🚀 启动 API 服务器..."
./bin/cat-cafe --mode api --port $API_PORT > logs/api.log 2>&1 &
API_PID=$!

echo "✓ API 服务器已启动 (PID: $API_PID, Port: $API_PORT)"

# 启动 Agent 工作进程
echo ""
echo "🐱 启动猫猫 Agent..."

# 取消 CLAUDECODE 环境变量，避免嵌套会话冲突
env -u CLAUDECODE ./bin/cat-cafe --mode agent --agent 花花 > logs/agent_huahua.log 2>&1 &
AGENT1_PID=$!
echo "✓ 花花已启动 (PID: $AGENT1_PID)"

env -u CLAUDECODE ./bin/cat-cafe --mode agent --agent 薇薇 > logs/agent_weiwei.log 2>&1 &
AGENT2_PID=$!
echo "✓ 薇薇已启动 (PID: $AGENT2_PID)"

env -u CLAUDECODE ./bin/cat-cafe --mode agent --agent 小乔 > logs/agent_xiaoqiao.log 2>&1 &
AGENT3_PID=$!
echo "✓ 小乔已启动 (PID: $AGENT3_PID)"

echo ""
echo "✅ 所有服务已启动！"
echo ""
echo "📝 进程信息:"
echo "   API 服务器: $API_PID (端口: $API_PORT)"
if [ "$HINDSIGHT_STARTED" = true ]; then
    echo "   Hindsight:  Docker 容器 $HINDSIGHT_CONTAINER (端口: 8888)"
fi
echo "   花花: $AGENT1_PID"
echo "   薇薇: $AGENT2_PID"
echo "   小乔: $AGENT3_PID"
echo ""

if [ "$USE_NGINX" = true ]; then
    echo "🌐 访问地址:"
    echo "   通过 Nginx: http://localhost:8080"
    echo "   直接访问: http://localhost:$API_PORT"
else
    echo "🌐 API 地址: http://localhost:$API_PORT"
fi

if [ "$HINDSIGHT_STARTED" = true ]; then
    echo "🧠 Hindsight: http://localhost:8888"
fi

echo "📖 日志目录: logs/"
echo ""
echo "💡 工作区管理 API:"
echo "   GET    /api/workspaces              - 列出所有工作区"
echo "   POST   /api/workspaces              - 创建工作区"
echo "   POST   /api/workspaces/:id/deploy-test  - 部署到测试环境"
echo "   POST   /api/deployments/:id/promote     - 提升到生产环境"
echo ""
echo "按 Ctrl+C 停止所有服务"

# 保存 PID 到文件
echo "$API_PID" > logs/.api.pid
echo "$AGENT1_PID" > logs/.agent1.pid
echo "$AGENT2_PID" > logs/.agent2.pid
echo "$AGENT3_PID" > logs/.agent3.pid

# 收集所有需要停止的 PID
ALL_PIDS="$API_PID $AGENT1_PID $AGENT2_PID $AGENT3_PID"

# 等待中断信号
trap "echo ''; echo '🛑 停止所有服务...'; kill $ALL_PIDS 2>/dev/null; if [ '$HINDSIGHT_STARTED' = true ]; then docker stop $HINDSIGHT_CONTAINER > /dev/null 2>&1; fi; rm -f logs/.api.pid logs/.agent1.pid logs/.agent2.pid logs/.agent3.pid; echo '✓ 已停止'; exit 0" INT TERM

wait
