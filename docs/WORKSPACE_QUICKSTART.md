# 工作区管理 - 快速开始

## 场景：让猫猫修改自己的代码并重启服务

### 步骤 1: 配置 Nginx（首次使用）

```bash
# 安装 Nginx（如果未安装）
brew install nginx  # macOS

# 配置 Nginx
./setup-nginx.sh
```

### 步骤 2: 启动服务（Nginx 模式）

```bash
./start-with-nginx.sh --nginx
```

此时：
- API 服务器运行在 `:9001`（生产端口）
- Nginx 监听 `:8080`，代理到 `:9001`
- 用户通过 `http://localhost:8080` 访问

### 步骤 3: 创建工作区

```bash
curl -X POST http://localhost:8080/api/workspaces \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/Users/jesuswang/Documents/Project/cat_coffee",
    "type": "self"
  }'
```

保存返回的 `workspace_id`，例如：`ws_abc123`

### 步骤 4: 猫猫修改代码

猫猫可以通过现有的文件操作 API 修改代码文件，例如：
- 修改 `src/api_server.go`
- 添加新功能
- 修复 bug

### 步骤 5: 部署到测试环境

```bash
curl -X POST http://localhost:8080/api/workspaces/ws_abc123/deploy-test
```

这会：
1. ✓ 编译代码
2. ✓ 运行测试
3. ✓ 在 `:9002` 启动新服务
4. ✓ 健康检查

保存返回的 `deployment_id`，例如：`deploy_xyz789`

### 步骤 6: 检查部署状态

```bash
# 查看部署状态
curl http://localhost:8080/api/deployments/deploy_xyz789

# 如果 status 为 "ready"，说明测试通过
```

### 步骤 7: 手动验证（可选）

```bash
# 直接访问测试端口验证新功能
curl http://localhost:9002/api/sessions
```

### 步骤 8: 提升到生产

确认测试通过后：

```bash
curl -X POST http://localhost:8080/api/deployments/deploy_xyz789/promote
```

这会：
1. 更新 Nginx 配置（`:9001` → `:9002`）
2. 重载 Nginx（零停机）
3. 关闭旧服务（`:9001`）
4. 交换端口角色

现在新代码已经在生产环境运行！

### 步骤 9: 验证

```bash
# 通过 Nginx 访问，应该看到新功能
curl http://localhost:8080/api/sessions
```

## 完整示例

```bash
# 1. 创建工作区
WS_ID=$(curl -s -X POST http://localhost:8080/api/workspaces \
  -H "Content-Type: application/json" \
  -d '{"path": "/Users/jesuswang/Documents/Project/cat_coffee", "type": "self"}' \
  | jq -r '.id')

echo "工作区 ID: $WS_ID"

# 2. 猫猫修改代码（这里省略实际修改步骤）

# 3. 部署到测试
DEPLOY_ID=$(curl -s -X POST http://localhost:8080/api/workspaces/$WS_ID/deploy-test \
  | jq -r '.id')

echo "部署 ID: $DEPLOY_ID"

# 4. 等待部署完成（轮询状态）
while true; do
  STATUS=$(curl -s http://localhost:8080/api/deployments/$DEPLOY_ID | jq -r '.status')
  echo "部署状态: $STATUS"

  if [ "$STATUS" = "ready" ]; then
    echo "✓ 部署成功，可以提升到生产"
    break
  elif [ "$STATUS" = "failed" ]; then
    echo "✗ 部署失败"
    exit 1
  fi

  sleep 2
done

# 5. 提升到生产
curl -X POST http://localhost:8080/api/deployments/$DEPLOY_ID/promote

echo "✓ 已提升到生产环境"
```

## 注意事项

1. **Session 状态**: 重启不影响 Session，因为状态存储在 Redis 中
2. **Agent 进程**: 当前实现只重启 API 服务器，Agent 进程继续运行
3. **测试端口**: 确保 `:9002` 端口未被占用
4. **权限**: 需要 sudo 权限来重载 Nginx

## 故障处理

如果部署失败，查看测试结果：

```bash
curl http://localhost:8080/api/deployments/deploy_xyz789 | jq '.test_results'
```

常见问题：
- 编译失败：检查代码语法
- 测试失败：修复测试用例
- 健康检查失败：检查服务是否正常启动

## 下一步

- 集成到前端 UI，让用户可视化操作
- 添加自动化测试流程
- 实现回滚机制
- 添加部署历史记录
