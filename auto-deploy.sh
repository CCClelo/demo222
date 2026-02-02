#!/bin/bash

echo "=========================================="
echo "Chat Gateway + Clash 一键部署脚本"
echo "=========================================="

# 检查是否为 root 用户
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 root 用户运行此脚本"
    exit 1
fi

# 1. 安装 Docker
echo "📦 检查 Docker..."
if ! command -v docker &> /dev/null; then
    echo "正在安装 Docker..."
    curl -fsSL https://get.docker.com | sh
    systemctl start docker
    systemctl enable docker
fi

if ! command -v docker-compose &> /dev/null; then
    echo "正在安装 Docker Compose..."
    curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose
fi

echo "✅ Docker 环境就绪"

# 2. 安装 Clash
echo ""
echo "📥 安装 Clash..."
mkdir -p /opt/clash
cd /opt/clash

# 下载 Clash
if [ ! -f "clash" ]; then
    wget -O clash.gz https://github.com/Dreamacro/clash/releases/download/premium/clash-linux-amd64-v3.gz
    gunzip clash.gz
    chmod +x clash
fi

# 下载 GeoIP 数据库
if [ ! -f "Country.mmdb" ]; then
    wget https://github.com/Dreamacro/maxmind-geoip/releases/latest/download/Country.mmdb
fi

# 下载订阅配置
echo "📥 下载 Clash 配置..."
wget -O config.yaml "https://dash.pqjc.site/api/v1/client/subscribe?token=0b98777d0a5c462a144b89588db6d49d"

# 修改配置以允许局域网访问
cat > config-override.yaml <<EOF
# 覆盖配置
mixed-port: 7890
allow-lan: true
bind-address: "*"
mode: rule
log-level: info
external-controller: 0.0.0.0:9090
EOF

# 合并配置（如果订阅配置没有这些选项）
echo "✅ Clash 配置完成"

# 3. 创建 Clash systemd 服务
echo ""
echo "⚙️  配置 Clash 服务..."
cat > /etc/systemd/system/clash.service <<EOF
[Unit]
Description=Clash Daemon
After=network.target

[Service]
Type=simple
User=root
ExecStart=/opt/clash/clash -d /opt/clash
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

# 启动 Clash
systemctl daemon-reload
systemctl start clash
systemctl enable clash

echo "⏳ 等待 Clash 启动..."
sleep 5

# 测试 Clash
echo "🔍 测试 Clash 代理..."
if curl -x http://127.0.0.1:7890 -s --connect-timeout 5 https://www.google.com > /dev/null; then
    echo "✅ Clash 代理工作正常"
    echo "当前 IP: $(curl -x http://127.0.0.1:7890 -s https://api.ip.sb/ip)"
else
    echo "⚠️  Clash 代理测试失败，但继续部署..."
fi

# 4. 部署 Chat Gateway
echo ""
echo "🚀 部署 Chat Gateway..."
mkdir -p /opt/chat-gateway
cd /opt/chat-gateway

# 检查文件是否存在
if [ ! -f "main.go" ]; then
    echo "❌ 错误：找不到 main.go 文件"
    echo "请先上传以下文件到 /opt/chat-gateway/："
    echo "  - main.go"
    echo "  - go.mod"
    echo "  - go.sum"
    echo "  - Dockerfile"
    echo ""
    echo "上传命令示例："
    echo "  scp main.go go.mod go.sum Dockerfile root@your-server:/opt/chat-gateway/"
    exit 1
fi

# 创建 docker-compose.yml
cat > docker-compose.yml <<EOF
version: '3.8'

services:
  chat-gateway:
    build: .
    container_name: chat-gateway
    restart: always
    network_mode: "host"
    environment:
      - BASE_URL=https://demo.chat-sdk.dev
      - WARP_PROXIES=http://127.0.0.1:7890
      - WARP_CONTAINERS=clash
      - PORT=8080
      - USE_AUTH=true
      - DEBUG=false
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
EOF

# 构建并启动
echo "🔨 构建 Docker 镜像..."
docker-compose build

echo "🚀 启动服务..."
docker-compose up -d

# 等待服务启动
echo "⏳ 等待服务启动..."
sleep 10

# 5. 测试服务
echo ""
echo "🔍 测试服务..."
if curl -s http://localhost:8080/health > /dev/null; then
    echo "✅ 健康检查通过"
else
    echo "⚠️  健康检查失败"
fi

# 6. 显示状态
echo ""
echo "=========================================="
echo "✅ 部署完成！"
echo "=========================================="
echo ""
echo "📊 服务状态："
echo "  Clash:        $(systemctl is-active clash)"
echo "  Chat Gateway: $(docker ps --filter name=chat-gateway --format '{{.Status}}')"
echo ""
echo "🌐 访问地址："
echo "  API 端点:     http://$(curl -s ifconfig.me):8080/v1"
echo "  健康检查:     http://$(curl -s ifconfig.me):8080/health"
echo "  Clash 面板:   http://$(curl -s ifconfig.me):9090/ui"
echo ""
echo "📝 管理命令："
echo "  查看日志:     docker logs -f chat-gateway"
echo "  重启服务:     docker-compose restart"
echo "  停止服务:     docker-compose down"
echo "  Clash 状态:   systemctl status clash"
echo ""
echo "🧪 测试命令："
echo "  curl http://localhost:8080/v1/models"
echo "  curl -X POST http://localhost:8080/v1/chat/completions \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"model\":\"gpt-5.2\",\"messages\":[{\"role\":\"user\",\"content\":\"你好\"}]}'"
echo ""
echo "=========================================="
