#!/bin/bash

echo "=========================================="
echo "New API 一键部署脚本"
echo "=========================================="

# 创建部署目录
mkdir -p ~/new-api
cd ~/new-api

echo "📝 创建 docker-compose.yml..."

# 生成随机密钥
SESSION_SECRET=$(openssl rand -base64 32)

cat > docker-compose.yml <<EOF
version: '3.8'

services:
  new-api:
    image: calciumion/new-api:latest
    container_name: new-api
    restart: always
    ports:
      - "3000:3000"
    environment:
      - SQL_DSN=/data/new-api.db
      - SESSION_SECRET=${SESSION_SECRET}
      - TZ=Asia/Shanghai
      - POLLING_INTERVAL=60
    volumes:
      - ./data:/data
    extra_hosts:
      - "host.docker.internal:host-gateway"
    command: --log-dir /data/logs
EOF

echo "✅ 配置文件创建完成"

# 创建数据目录
mkdir -p ./data/logs

echo ""
echo "🚀 启动 New API..."
sudo docker-compose up -d

echo ""
echo "⏳ 等待服务启动..."
sleep 10

# 检查服务状态
if sudo docker ps | grep -q new-api; then
    echo "✅ New API 启动成功！"
else
    echo "❌ New API 启动失败，查看日志..."
    sudo docker logs new-api
    exit 1
fi

# 获取服务器 IP
SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || curl -s icanhazip.com 2>/dev/null || echo "your-server-ip")

echo ""
echo "=========================================="
echo "✅ 部署完成！"
echo "=========================================="
echo ""
echo "🌐 访问地址："
echo "  管理面板: http://${SERVER_IP}:3000"
echo "  API 端点: http://${SERVER_IP}:3000/v1"
echo ""
echo "🔑 默认账号："
echo "  用户名: root"
echo "  密码: 123456"
echo "  ⚠️  请立即登录并修改密码！"
echo ""
echo "📝 管理命令："
echo "  查看日志: sudo docker logs -f new-api"
echo "  重启服务: cd ~/new-api && sudo docker-compose restart"
echo "  停止服务: cd ~/new-api && sudo docker-compose down"
echo ""
echo "🔗 集成 Chat Gateway："
echo "  1. 登录管理面板"
echo "  2. 渠道管理 → 添加渠道"
echo "  3. 类型: OpenAI"
echo "  4. Base URL: http://host.docker.internal:8080/v1"
echo "  5. 密钥: sk-test (随意)"
echo "  6. 模型: gpt-5.2,claude-opus-4.5,claude-sonnet-4.5,gemini-3-pro-preview"
echo ""
echo "=========================================="
