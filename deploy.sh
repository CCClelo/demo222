#!/bin/bash

# Chat SDK Gateway 一键部署脚本

echo "=========================================="
echo "Chat SDK Gateway 部署脚本"
echo "=========================================="

# 检查 Docker 是否安装
if ! command -v docker &> /dev/null; then
    echo "❌ Docker 未安装，正在安装..."
    curl -fsSL https://get.docker.com | sh
    systemctl start docker
    systemctl enable docker
fi

# 检查 Docker Compose 是否安装
if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose 未安装，正在安装..."
    curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose
fi

echo "✅ Docker 环境检查完成"

# 创建部署目录
DEPLOY_DIR="/opt/chat-gateway"
mkdir -p $DEPLOY_DIR
cd $DEPLOY_DIR

echo "📁 部署目录: $DEPLOY_DIR"

# 下载或复制文件
echo "📥 准备部署文件..."

# 如果文件已存在则跳过
if [ ! -f "main.go" ]; then
    echo "请将 main.go, Dockerfile, docker-compose.yml 复制到 $DEPLOY_DIR"
    exit 1
fi

# 构建并启动服务
echo "🚀 构建并启动服务..."
docker-compose down
docker-compose build
docker-compose up -d

# 等待服务启动
echo "⏳ 等待服务启动..."
sleep 10

# 检查服务状态
echo "📊 服务状态:"
docker-compose ps

# 测试健康检查
echo ""
echo "🔍 测试服务..."
curl -s http://localhost:8080/health

echo ""
echo "=========================================="
echo "✅ 部署完成！"
echo "=========================================="
echo "服务地址: http://YOUR_SERVER_IP:8080"
echo "API 端点: http://YOUR_SERVER_IP:8080/v1"
echo ""
echo "查看日志: docker-compose logs -f chat-gateway"
echo "停止服务: docker-compose down"
echo "重启服务: docker-compose restart"
echo "=========================================="
