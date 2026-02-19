#!/bin/bash

# 修复 Nginx 配置冲突

echo "🔧 修复 Nginx 8080 端口冲突"
echo ""

NGINX_CONF="/usr/local/etc/nginx/nginx.conf"
BACKUP_CONF="/usr/local/etc/nginx/nginx.conf.backup"

# 备份原配置
echo "📦 备份原配置..."
sudo cp "$NGINX_CONF" "$BACKUP_CONF"
echo "✓ 已备份到: $BACKUP_CONF"
echo ""

# 注释掉默认的 server 块
echo "✏️  注释掉默认 server 块..."
sudo sed -i '' '35,77s/^/    #/' "$NGINX_CONF"

echo "✓ 配置已修改"
echo ""

# 测试配置
echo "🧪 测试 Nginx 配置..."
if sudo nginx -t; then
    echo "✓ 配置文件语法正确"
else
    echo "❌ 配置文件语法错误，恢复备份..."
    sudo cp "$BACKUP_CONF" "$NGINX_CONF"
    exit 1
fi

# 重载 Nginx
echo ""
echo "🔄 重载 Nginx..."
sudo nginx -s reload

echo "✓ Nginx 已重载"
echo ""
echo "✅ 修复完成！"
echo ""
echo "现在可以通过 http://localhost:8080 访问 API"
