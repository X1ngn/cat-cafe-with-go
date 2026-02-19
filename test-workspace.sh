#!/bin/bash

# 工作区功能测试脚本

set -e

API_BASE="http://localhost:8080/api"
PROJECT_PATH="/Users/jesuswang/Documents/Project/cat_coffee"

echo "🧪 工作区功能测试"
echo ""

# 检查服务是否运行
echo "1️⃣ 检查服务状态..."
if ! curl -s -f "$API_BASE/sessions" > /dev/null; then
    echo "❌ API 服务器未运行"
    echo "请先运行: ./start-with-nginx.sh --nginx"
    exit 1
fi
echo "✓ API 服务器正常运行"
echo ""

# 创建工作区
echo "2️⃣ 创建工作区..."
WS_RESPONSE=$(curl -s -X POST "$API_BASE/workspaces" \
  -H "Content-Type: application/json" \
  -d "{\"path\": \"$PROJECT_PATH\", \"type\": \"self\"}")

WS_ID=$(echo "$WS_RESPONSE" | jq -r '.id')

if [ "$WS_ID" = "null" ] || [ -z "$WS_ID" ]; then
    echo "❌ 创建工作区失败"
    echo "$WS_RESPONSE"
    exit 1
fi

echo "✓ 工作区已创建: $WS_ID"
echo ""

# 查看工作区详情
echo "3️⃣ 查看工作区详情..."
curl -s "$API_BASE/workspaces/$WS_ID" | jq '.'
echo ""

# 列出所有工作区
echo "4️⃣ 列出所有工作区..."
curl -s "$API_BASE/workspaces" | jq '.'
echo ""

# 更新工作区配置
echo "5️⃣ 更新工作区配置..."
curl -s -X PUT "$API_BASE/workspaces/$WS_ID" \
  -H "Content-Type: application/json" \
  -d '{"build_cmd": "make build"}' | jq '.'
echo ""

# 部署到测试环境
echo "6️⃣ 部署到测试环境..."
echo "⚠️  这将编译代码并在端口 9002 启动测试服务"
read -p "是否继续? (y/n) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "已取消部署测试"
    echo ""
    echo "清理工作区..."
    curl -s -X DELETE "$API_BASE/workspaces/$WS_ID"
    echo "✓ 工作区已删除"
    exit 0
fi

DEPLOY_RESPONSE=$(curl -s -X POST "$API_BASE/workspaces/$WS_ID/deploy-test")
DEPLOY_ID=$(echo "$DEPLOY_RESPONSE" | jq -r '.id')

if [ "$DEPLOY_ID" = "null" ] || [ -z "$DEPLOY_ID" ]; then
    echo "❌ 部署失败"
    echo "$DEPLOY_RESPONSE"
    exit 1
fi

echo "✓ 部署已启动: $DEPLOY_ID"
echo ""

# 轮询部署状态
echo "7️⃣ 等待部署完成..."
MAX_WAIT=60
WAITED=0

while [ $WAITED -lt $MAX_WAIT ]; do
    DEPLOY_STATUS=$(curl -s "$API_BASE/deployments/$DEPLOY_ID")
    STATUS=$(echo "$DEPLOY_STATUS" | jq -r '.status')

    echo "   状态: $STATUS (${WAITED}s)"

    if [ "$STATUS" = "ready" ]; then
        echo ""
        echo "✓ 部署成功！"
        echo ""
        echo "测试结果:"
        echo "$DEPLOY_STATUS" | jq -r '.test_results[]'
        echo ""
        break
    elif [ "$STATUS" = "failed" ]; then
        echo ""
        echo "❌ 部署失败"
        echo ""
        echo "失败原因:"
        echo "$DEPLOY_STATUS" | jq -r '.test_results[]'
        echo ""
        exit 1
    fi

    sleep 3
    WAITED=$((WAITED + 3))
done

if [ $WAITED -ge $MAX_WAIT ]; then
    echo "❌ 部署超时"
    exit 1
fi

# 测试新服务
echo "8️⃣ 测试新服务 (端口 9002)..."
if curl -s -f "http://localhost:9002/api/sessions" > /dev/null; then
    echo "✓ 测试服务正常运行"
else
    echo "❌ 测试服务无响应"
    exit 1
fi
echo ""

# 提升到生产
echo "9️⃣ 提升到生产环境..."
echo "⚠️  这将更新 Nginx 配置并切换流量"
read -p "是否继续? (y/n) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "已取消提升到生产"
    echo ""
    echo "停止测试服务..."
    lsof -ti:9002 | xargs kill -9 2>/dev/null || true
    echo "✓ 测试服务已停止"
    exit 0
fi

PROMOTE_RESPONSE=$(curl -s -X POST "$API_BASE/deployments/$DEPLOY_ID/promote")
PROMOTE_STATUS=$(echo "$PROMOTE_RESPONSE" | jq -r '.status')

if [ "$PROMOTE_STATUS" = "active" ]; then
    echo "✓ 已提升到生产环境"
else
    echo "❌ 提升失败"
    echo "$PROMOTE_RESPONSE"
    exit 1
fi
echo ""

# 验证生产服务
echo "🔟 验证生产服务..."
if curl -s -f "http://localhost:8080/api/sessions" > /dev/null; then
    echo "✓ 生产服务正常运行"
else
    echo "❌ 生产服务无响应"
    exit 1
fi
echo ""

echo "✅ 所有测试通过！"
echo ""
echo "📝 总结:"
echo "   工作区 ID: $WS_ID"
echo "   部署 ID: $DEPLOY_ID"
echo "   当前生产端口: 9002"
echo "   下次测试端口: 9001"
echo ""
echo "💡 提示:"
echo "   - 查看部署历史: curl $API_BASE/workspaces/$WS_ID/deployments | jq"
echo "   - 删除工作区: curl -X DELETE $API_BASE/workspaces/$WS_ID"
