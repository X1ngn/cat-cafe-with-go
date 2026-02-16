#!/bin/bash

# 测试 API 功能

echo "🧪 测试猫猫咖啡屋 API"
echo ""

# 1. 创建会话
echo "1️⃣ 创建会话..."
SESSION_RESPONSE=$(curl -s -X POST http://localhost:8080/api/sessions)
SESSION_ID=$(echo $SESSION_RESPONSE | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

if [ -z "$SESSION_ID" ]; then
    echo "❌ 创建会话失败"
    exit 1
fi

echo "✓ 会话已创建: $SESSION_ID"
echo ""

# 2. 发送消息（提及花花）
echo "2️⃣ 发送消息: @花花 你好"
MESSAGE_RESPONSE=$(curl -s -X POST http://localhost:8080/api/sessions/$SESSION_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "content": "@花花 你好",
    "mentionedCats": ["cat_001"]
  }')

echo "✓ 消息已发送"
echo ""

# 3. 等待处理
echo "3️⃣ 等待 Agent 处理..."
sleep 2
echo ""

# 4. 获取消息列表
echo "4️⃣ 获取消息列表..."
MESSAGES=$(curl -s http://localhost:8080/api/sessions/$SESSION_ID/messages)
echo "$MESSAGES" | jq '.'
echo ""

# 5. 获取调用历史
echo "5️⃣ 获取调用历史..."
HISTORY=$(curl -s http://localhost:8080/api/sessions/$SESSION_ID/history)
echo "$HISTORY" | jq '.'
echo ""

# 6. 获取猫猫列表
echo "6️⃣ 获取猫猫列表..."
CATS=$(curl -s http://localhost:8080/api/cats)
echo "$CATS" | jq '.'
echo ""

echo "✅ 测试完成！"
echo ""
echo "💡 提示: 查看后台日志以了解详细的任务处理过程"
