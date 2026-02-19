package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// WorkspaceType 工作区类型
type WorkspaceType string

const (
	WorkspaceTypeSelf     WorkspaceType = "self"     // 自身项目
	WorkspaceTypeExternal WorkspaceType = "external" // 外部项目
)

// DeploymentStatus 部署状态
type DeploymentStatus string

const (
	DeploymentStatusTesting DeploymentStatus = "testing" // 测试中
	DeploymentStatusReady   DeploymentStatus = "ready"   // 就绪
	DeploymentStatusActive  DeploymentStatus = "active"  // 生产中
	DeploymentStatusFailed  DeploymentStatus = "failed"  // 失败
)

// Workspace 工作区
type Workspace struct {
	ID          string        `json:"id"`
	Path        string        `json:"path"`
	Type        WorkspaceType `json:"type"`
	BuildCmd    string        `json:"build_cmd"`
	TestCmd     string        `json:"test_cmd"`
	StartCmd    string        `json:"start_cmd"`
	HealthCheck string        `json:"health_check"`
	State       string        `json:"state"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// Deployment 部署信息
type Deployment struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspace_id"`
	Version     string           `json:"version"`      // git commit hash
	TestPort    int              `json:"test_port"`    // 测试端口
	ProdPort    int              `json:"prod_port"`    // 生产端口
	Status      DeploymentStatus `json:"status"`       // 部署状态
	TestResults []string         `json:"test_results"` // 测试结果
	DeployedAt  time.Time        `json:"deployed_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// WorkspaceManager 工作区管理器
type WorkspaceManager struct {
	workspaces  map[string]*Workspace
	deployments map[string]*Deployment
	mu          sync.RWMutex
	redisClient *redis.Client
	ctx         context.Context

	// 端口管理
	testPort int // 当前测试端口
	prodPort int // 当前生产端口
}

// NewWorkspaceManager 创建工作区管理器
func NewWorkspaceManager(redisClient *redis.Client, ctx context.Context) *WorkspaceManager {
	wm := &WorkspaceManager{
		workspaces:  make(map[string]*Workspace),
		deployments: make(map[string]*Deployment),
		redisClient: redisClient,
		ctx:         ctx,
		testPort:    9002, // 初始测试端口
		prodPort:    9001, // 初始生产端口
	}

	// 从 Redis 加载工作区
	wm.loadWorkspacesFromRedis()

	return wm
}

// CreateWorkspace 创建工作区
func (wm *WorkspaceManager) CreateWorkspace(path string, wsType WorkspaceType) (*Workspace, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// 验证路径
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	workspaceID := fmt.Sprintf("ws_%s", uuid.New().String()[:8])

	// 根据类型设置默认命令
	var buildCmd, testCmd, startCmd, healthCheck string
	if wsType == WorkspaceTypeSelf {
		buildCmd = "make build"
		testCmd = "make test-unit"
		startCmd = "./bin/cat-cafe --mode api --port %d"
		healthCheck = "http://localhost:%d/api/sessions"
	} else {
		// 外部项目的默认命令（可以后续自定义）
		buildCmd = "make build"
		testCmd = "make test"
		startCmd = ""
		healthCheck = ""
	}

	workspace := &Workspace{
		ID:          workspaceID,
		Path:        path,
		Type:        wsType,
		BuildCmd:    buildCmd,
		TestCmd:     testCmd,
		StartCmd:    startCmd,
		HealthCheck: healthCheck,
		State:       "idle",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	wm.workspaces[workspaceID] = workspace

	// 保存到 Redis
	if err := wm.saveWorkspaceToRedis(workspace); err != nil {
		LogError("[Workspace] 保存工作区到 Redis 失败: %v", err)
	}

	LogInfo("[Workspace] 工作区已创建: %s (%s)", workspaceID, path)
	return workspace, nil
}

// GetWorkspace 获取工作区
func (wm *WorkspaceManager) GetWorkspace(workspaceID string) (*Workspace, error) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	workspace, exists := wm.workspaces[workspaceID]
	if !exists {
		return nil, fmt.Errorf("工作区不存在: %s", workspaceID)
	}

	return workspace, nil
}

// ListWorkspaces 列出所有工作区
func (wm *WorkspaceManager) ListWorkspaces() []*Workspace {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	workspaces := make([]*Workspace, 0, len(wm.workspaces))
	for _, ws := range wm.workspaces {
		workspaces = append(workspaces, ws)
	}

	return workspaces
}

// UpdateWorkspace 更新工作区配置
func (wm *WorkspaceManager) UpdateWorkspace(workspaceID string, updates map[string]interface{}) (*Workspace, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	workspace, exists := wm.workspaces[workspaceID]
	if !exists {
		return nil, fmt.Errorf("工作区不存在: %s", workspaceID)
	}

	// 更新字段
	if buildCmd, ok := updates["build_cmd"].(string); ok {
		workspace.BuildCmd = buildCmd
	}
	if testCmd, ok := updates["test_cmd"].(string); ok {
		workspace.TestCmd = testCmd
	}
	if startCmd, ok := updates["start_cmd"].(string); ok {
		workspace.StartCmd = startCmd
	}
	if healthCheck, ok := updates["health_check"].(string); ok {
		workspace.HealthCheck = healthCheck
	}

	workspace.UpdatedAt = time.Now()

	// 保存到 Redis
	if err := wm.saveWorkspaceToRedis(workspace); err != nil {
		LogError("[Workspace] 更新工作区到 Redis 失败: %v", err)
	}

	return workspace, nil
}

// DeleteWorkspace 删除工作区
func (wm *WorkspaceManager) DeleteWorkspace(workspaceID string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.workspaces[workspaceID]; !exists {
		return fmt.Errorf("工作区不存在: %s", workspaceID)
	}

	delete(wm.workspaces, workspaceID)

	// 从 Redis 删除
	key := fmt.Sprintf("workspace:%s", workspaceID)
	if err := wm.redisClient.Del(wm.ctx, key).Err(); err != nil {
		LogError("[Workspace] 从 Redis 删除工作区失败: %v", err)
	}

	LogInfo("[Workspace] 工作区已删除: %s", workspaceID)
	return nil
}

// DeployToTest 部署到测试环境
func (wm *WorkspaceManager) DeployToTest(workspaceID string) (*Deployment, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	workspace, exists := wm.workspaces[workspaceID]
	if !exists {
		return nil, fmt.Errorf("工作区不存在: %s", workspaceID)
	}

	// 创建部署记录
	deploymentID := fmt.Sprintf("deploy_%s", uuid.New().String()[:8])
	deployment := &Deployment{
		ID:          deploymentID,
		WorkspaceID: workspaceID,
		Version:     wm.getGitCommitHash(workspace.Path),
		TestPort:    wm.testPort,
		ProdPort:    wm.prodPort,
		Status:      DeploymentStatusTesting,
		TestResults: make([]string, 0),
		DeployedAt:  time.Now(),
		UpdatedAt:   time.Now(),
	}

	wm.deployments[deploymentID] = deployment

	// 异步执行部署
	go wm.executeTestDeployment(deployment, workspace)

	LogInfo("[Workspace] 开始测试部署: %s", deploymentID)
	return deployment, nil
}

// executeTestDeployment 执行测试部署
func (wm *WorkspaceManager) executeTestDeployment(deployment *Deployment, workspace *Workspace) {
	LogInfo("[Workspace] 执行测试部署 - DeploymentID: %s", deployment.ID)

	// 1. 编译代码
	LogInfo("[Workspace] 步骤 1/4: 编译代码")
	if err := wm.runCommand(workspace.Path, workspace.BuildCmd); err != nil {
		wm.updateDeploymentStatus(deployment.ID, DeploymentStatusFailed, []string{fmt.Sprintf("编译失败: %v", err)})
		return
	}
	deployment.TestResults = append(deployment.TestResults, "✓ 编译成功")

	// 2. 运行测试
	LogInfo("[Workspace] 步骤 2/4: 运行测试")
	if workspace.TestCmd != "" {
		if err := wm.runCommand(workspace.Path, workspace.TestCmd); err != nil {
			wm.updateDeploymentStatus(deployment.ID, DeploymentStatusFailed, append(deployment.TestResults, fmt.Sprintf("测试失败: %v", err)))
			return
		}
		deployment.TestResults = append(deployment.TestResults, "✓ 测试通过")
	}

	// 3. 启动测试服务
	LogInfo("[Workspace] 步骤 3/4: 启动测试服务 (端口: %d)", deployment.TestPort)
	startCmd := fmt.Sprintf(workspace.StartCmd, deployment.TestPort)
	if err := wm.startService(workspace.Path, startCmd, deployment.TestPort); err != nil {
		wm.updateDeploymentStatus(deployment.ID, DeploymentStatusFailed, append(deployment.TestResults, fmt.Sprintf("启动服务失败: %v", err)))
		return
	}
	deployment.TestResults = append(deployment.TestResults, fmt.Sprintf("✓ 测试服务已启动 (端口: %d)", deployment.TestPort))

	// 4. 健康检查
	LogInfo("[Workspace] 步骤 4/4: 健康检查")
	healthCheckURL := fmt.Sprintf(workspace.HealthCheck, deployment.TestPort)
	if err := wm.healthCheck(healthCheckURL); err != nil {
		wm.updateDeploymentStatus(deployment.ID, DeploymentStatusFailed, append(deployment.TestResults, fmt.Sprintf("健康检查失败: %v", err)))
		return
	}
	deployment.TestResults = append(deployment.TestResults, "✓ 健康检查通过")

	// 部署成功
	wm.updateDeploymentStatus(deployment.ID, DeploymentStatusReady, deployment.TestResults)
	LogInfo("[Workspace] 测试部署完成: %s", deployment.ID)
}

// PromoteToProduction 提升到生产环境
func (wm *WorkspaceManager) PromoteToProduction(deploymentID string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	deployment, exists := wm.deployments[deploymentID]
	if !exists {
		return fmt.Errorf("部署不存在: %s", deploymentID)
	}

	if deployment.Status != DeploymentStatusReady {
		return fmt.Errorf("部署状态不是 ready，无法提升到生产: %s", deployment.Status)
	}

	// 更新 Nginx 配置
	if err := wm.updateNginxConfig(deployment.TestPort); err != nil {
		return fmt.Errorf("更新 Nginx 配置失败: %w", err)
	}

	// 重载 Nginx
	if err := wm.reloadNginx(); err != nil {
		return fmt.Errorf("重载 Nginx 失败: %w", err)
	}

	// 等待流量切换完成
	time.Sleep(2 * time.Second)

	// 关闭旧的生产服务
	wm.stopService(deployment.ProdPort)

	// 交换端口角色
	deployment.ProdPort, deployment.TestPort = deployment.TestPort, deployment.ProdPort
	deployment.Status = DeploymentStatusActive
	deployment.UpdatedAt = time.Now()

	LogInfo("[Workspace] 部署已提升到生产: %s", deploymentID)
	return nil
}

// GetDeployment 获取部署信息
func (wm *WorkspaceManager) GetDeployment(deploymentID string) (*Deployment, error) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	deployment, exists := wm.deployments[deploymentID]
	if !exists {
		return nil, fmt.Errorf("部署不存在: %s", deploymentID)
	}

	return deployment, nil
}

// ListDeployments 列出工作区的所有部署
func (wm *WorkspaceManager) ListDeployments(workspaceID string) []*Deployment {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	deployments := make([]*Deployment, 0)
	for _, deployment := range wm.deployments {
		if deployment.WorkspaceID == workspaceID {
			deployments = append(deployments, deployment)
		}
	}

	return deployments
}

// 辅助方法

func (wm *WorkspaceManager) runCommand(workDir, command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		LogError("[Workspace] 命令执行失败: %s\n输出: %s", command, string(output))
		return fmt.Errorf("%w: %s", err, string(output))
	}
	LogDebug("[Workspace] 命令执行成功: %s", command)
	return nil
}

func (wm *WorkspaceManager) startService(workDir, command string, port int) error {
	// 启动服务（后台运行）
	cmd := exec.Command("sh", "-c", command+" > logs/test_api.log 2>&1 &")
	cmd.Dir = workDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}

	// 保存 PID
	pidFile := filepath.Join(workDir, "logs", fmt.Sprintf(".test_%d.pid", port))
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		LogWarn("[Workspace] 保存 PID 文件失败: %v", err)
	}

	// 等待服务启动
	time.Sleep(3 * time.Second)

	return nil
}

func (wm *WorkspaceManager) stopService(port int) error {
	// 通过端口查找进程并关闭
	cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti:%d | xargs kill -9", port))
	if err := cmd.Run(); err != nil {
		LogWarn("[Workspace] 停止服务失败 (端口: %d): %v", port, err)
	}
	return nil
}

func (wm *WorkspaceManager) healthCheck(url string) error {
	// 简单的 HTTP 健康检查
	cmd := exec.Command("curl", "-f", "-s", url)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("健康检查失败: %w", err)
	}
	return nil
}

func (wm *WorkspaceManager) getGitCommitHash(workDir string) string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return string(output[:len(output)-1]) // 去掉换行符
}

func (wm *WorkspaceManager) updateNginxConfig(newPort int) error {
	// 读取 Nginx 配置模板
	configPath := "/usr/local/etc/nginx/servers/cat-cafe.conf"
	template := fmt.Sprintf(`upstream backend {
    server 127.0.0.1:%d;
}

server {
    listen 8080;
    server_name localhost;

    location / {
        proxy_pass http://backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
`, newPort)

	// 写入配置文件
	if err := os.WriteFile(configPath, []byte(template), 0644); err != nil {
		return fmt.Errorf("写入 Nginx 配置失败: %w", err)
	}

	LogInfo("[Workspace] Nginx 配置已更新 (端口: %d)", newPort)
	return nil
}

func (wm *WorkspaceManager) reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("重载 Nginx 失败: %w", err)
	}
	LogInfo("[Workspace] Nginx 已重载")
	return nil
}

func (wm *WorkspaceManager) updateDeploymentStatus(deploymentID string, status DeploymentStatus, testResults []string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if deployment, exists := wm.deployments[deploymentID]; exists {
		deployment.Status = status
		deployment.TestResults = testResults
		deployment.UpdatedAt = time.Now()
	}
}

// Redis 持久化

func (wm *WorkspaceManager) saveWorkspaceToRedis(workspace *Workspace) error {
	key := fmt.Sprintf("workspace:%s", workspace.ID)
	data, err := json.Marshal(workspace)
	if err != nil {
		return err
	}
	return wm.redisClient.Set(wm.ctx, key, data, 0).Err()
}

func (wm *WorkspaceManager) loadWorkspacesFromRedis() error {
	keys, err := wm.redisClient.Keys(wm.ctx, "workspace:*").Result()
	if err != nil {
		return err
	}

	for _, key := range keys {
		data, err := wm.redisClient.Get(wm.ctx, key).Result()
		if err != nil {
			continue
		}

		var workspace Workspace
		if err := json.Unmarshal([]byte(data), &workspace); err != nil {
			continue
		}

		wm.workspaces[workspace.ID] = &workspace
	}

	LogInfo("[Workspace] 从 Redis 加载了 %d 个工作区", len(wm.workspaces))
	return nil
}
