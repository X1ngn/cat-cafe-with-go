# 工作区管理功能

## 概述

工作区管理功能允许猫猫们修改和部署代码，包括修改自己的代码。通过 Nginx 反向代理实现零停机部署。

## 架构

```
用户请求 → Nginx (:8080) → 后端服务
                ↓
         动态切换端口
                ↓
    生产端口 (:9001) ⟷ 测试端口 (:9002)
```

## 工作流程

### 1. 初始设置

```bash
# 安装 Nginx（如果未安装）
# macOS
brew install nginx

# Ubuntu/Debian
sudo apt-get install nginx

# 配置 Nginx
./setup-nginx.sh
```

### 2. 启动服务

```bash
# 使用 Nginx 代理模式启动
./start-with-nginx.sh --nginx

# 或使用直连模式（开发用）
./start.sh
```

### 3. 创建工作区

```bash
# 创建自身工作区
curl -X POST http://localhost:8080/api/workspaces \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/Users/jesuswang/Documents/Project/cat_coffee",
    "type": "self"
  }'

# 返回示例
{
  "id": "ws_abc123",
  "path": "/Users/jesuswang/Documents/Project/cat_coffee",
  "type": "self",
  "build_cmd": "make build",
  "test_cmd": "make test-unit",
  "start_cmd": "./bin/cat-cafe --mode api --port %d",
  "health_check": "http://localhost:%d/api/sessions",
  "state": "idle",
  "created_at": "2026-02-19T10:00:00Z"
}
```

### 4. 部署到测试环境

```bash
# 猫猫修改代码后，部署到测试端口
curl -X POST http://localhost:8080/api/workspaces/ws_abc123/deploy-test

# 返回部署信息
{
  "id": "deploy_xyz789",
  "workspace_id": "ws_abc123",
  "version": "a1b2c3d",
  "test_port": 9002,
  "prod_port": 9001,
  "status": "testing",
  "test_results": [],
  "deployed_at": "2026-02-19T10:05:00Z"
}
```

部署过程会自动执行：
1. 编译代码 (`make build`)
2. 运行测试 (`make test-unit`)
3. 在测试端口启动服务 (`:9002`)
4. 健康检查

### 5. 查看部署状态

```bash
# 查看部署详情
curl http://localhost:8080/api/deployments/deploy_xyz789

# 返回示例
{
  "id": "deploy_xyz789",
  "status": "ready",  # testing | ready | active | failed
  "test_results": [
    "✓ 编译成功",
    "✓ 测试通过",
    "✓ 测试服务已启动 (端口: 9002)",
    "✓ 健康检查通过"
  ]
}
```

### 6. 提升到生产环境

```bash
# 测试通过后，提升到生产
curl -X POST http://localhost:8080/api/deployments/deploy_xyz789/promote

# 此操作会：
# 1. 更新 Nginx 配置，将流量切换到测试端口
# 2. 重载 Nginx（无缝切换）
# 3. 关闭旧的生产服务
# 4. 交换端口角色（9002 变生产，9001 变测试）
```

### 7. 回滚（如果需要）

如果新版本有问题，可以快速回滚：

```bash
# 重新部署上一个版本到测试端口
curl -X POST http://localhost:8080/api/workspaces/ws_abc123/deploy-test

# 提升到生产
curl -X POST http://localhost:8080/api/deployments/deploy_new/promote
```

## API 端点

### 工作区管理

- `GET /api/workspaces` - 列出所有工作区
- `POST /api/workspaces` - 创建工作区
- `GET /api/workspaces/:id` - 获取工作区详情
- `PUT /api/workspaces/:id` - 更新工作区配置
- `DELETE /api/workspaces/:id` - 删除工作区

### 部署管理

- `POST /api/workspaces/:id/deploy-test` - 部署到测试环境
- `POST /api/deployments/:id/promote` - 提升到生产环境
- `GET /api/deployments/:id` - 获取部署详情
- `GET /api/workspaces/:id/deployments` - 列出工作区的所有部署

## 配置说明

### 工作区类型

- `self`: 自身项目（猫猫咖啡屋本身）
- `external`: 外部项目（预留，暂未实现）

### 自定义命令

可以通过 API 更新工作区的命令：

```bash
curl -X PUT http://localhost:8080/api/workspaces/ws_abc123 \
  -H "Content-Type: application/json" \
  -d '{
    "build_cmd": "go build -o bin/custom .",
    "test_cmd": "go test ./...",
    "start_cmd": "./bin/custom --port %d",
    "health_check": "http://localhost:%d/health"
  }'
```

## 端口说明

- `:8080` - Nginx 监听端口（对外服务）
- `:9001` - 初始生产端口
- `:9002` - 初始测试端口

部署后端口角色会交换，确保始终有一个服务在运行。

## 安全注意事项

当前实现为开发版本，生产环境需要考虑：

1. **权限控制**: 添加认证和授权机制
2. **代码审查**: 在部署前进行代码审查
3. **备份机制**: 保留多个历史版本
4. **监控告警**: 部署失败时及时通知
5. **限流保护**: 防止频繁部署

## 故障排查

### Nginx 配置问题

```bash
# 测试配置
sudo nginx -t

# 查看错误日志
tail -f /usr/local/var/log/nginx/error.log  # macOS
tail -f /var/log/nginx/error.log            # Linux
```

### 端口占用

```bash
# 查看端口占用
lsof -i :9001
lsof -i :9002

# 强制关闭
kill -9 <PID>
```

### 部署失败

查看部署日志：

```bash
tail -f logs/test_api.log
```

## 未来扩展

- [ ] 支持外部项目工作区
- [ ] 多版本并存（A/B 测试）
- [ ] 自动回滚机制
- [ ] 部署历史记录
- [ ] 性能监控集成
- [ ] 蓝绿部署支持
