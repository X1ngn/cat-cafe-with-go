#!/bin/bash

# Nginx 配置部署脚本

echo "🔧 配置 Nginx 用于猫猫咖啡屋"
echo ""

# 检测操作系统
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    NGINX_CONF_DIR="/usr/local/etc/nginx/servers"
    NGINX_BIN="nginx"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    # Linux
    NGINX_CONF_DIR="/etc/nginx/sites-available"
    NGINX_LINK_DIR="/etc/nginx/sites-enabled"
    NGINX_BIN="nginx"
else
    echo "❌ 不支持的操作系统: $OSTYPE"
    exit 1
fi

# 检查 Nginx 是否安装
if ! command -v nginx &> /dev/null; then
    echo "❌ Nginx 未安装"
    echo ""
    echo "请先安装 Nginx:"
    echo "  macOS: brew install nginx"
    echo "  Ubuntu/Debian: sudo apt-get install nginx"
    echo "  CentOS/RHEL: sudo yum install nginx"
    exit 1
fi

echo "✓ Nginx 已安装"

# 创建配置目录（如果不存在）
if [[ "$OSTYPE" == "darwin"* ]]; then
    sudo mkdir -p "$NGINX_CONF_DIR"
else
    sudo mkdir -p "$NGINX_CONF_DIR"
    sudo mkdir -p "$NGINX_LINK_DIR"
fi

# 复制配置文件
CONF_FILE="$NGINX_CONF_DIR/cat-cafe.conf"
echo ""
echo "📝 复制配置文件到: $CONF_FILE"
sudo cp nginx/cat-cafe.conf.template "$CONF_FILE"

# Linux 需要创建符号链接
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    sudo ln -sf "$NGINX_CONF_DIR/cat-cafe.conf" "$NGINX_LINK_DIR/cat-cafe.conf"
    echo "✓ 已创建符号链接"
fi

# 测试配置
echo ""
echo "🧪 测试 Nginx 配置..."
if sudo nginx -t; then
    echo "✓ 配置文件语法正确"
else
    echo "❌ 配置文件语法错误"
    exit 1
fi

# 重载 Nginx
echo ""
echo "🔄 重载 Nginx..."
if sudo nginx -s reload 2>/dev/null || sudo systemctl reload nginx 2>/dev/null; then
    echo "✓ Nginx 已重载"
else
    echo "⚠️  Nginx 未运行，尝试启动..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sudo nginx
    else
        sudo systemctl start nginx
    fi
    echo "✓ Nginx 已启动"
fi

echo ""
echo "✅ Nginx 配置完成！"
echo ""
echo "📝 配置信息:"
echo "   监听端口: 8080"
echo "   后端地址: http://127.0.0.1:9001 (生产)"
echo "   配置文件: $CONF_FILE"
echo ""
echo "💡 提示:"
echo "   - 工作区管理器会自动更新后端端口"
echo "   - 使用 'sudo nginx -s reload' 手动重载配置"
echo "   - 查看日志: tail -f /usr/local/var/log/nginx/error.log (macOS)"
echo "            tail -f /var/log/nginx/error.log (Linux)"
