#!/bin/bash

echo "=========================================="
echo "Chat Gateway + Clash 一键部署脚本 v2"
echo "=========================================="

# 获取当前目录（应该是 ~/chat-gateway）
DEPLOY_DIR=$(pwd)
echo "📁 部署目录: $DEPLOY_DIR"

# 检查文件是否存在
if [ ! -f "main.go" ]; then
    echo "❌ 错误：找不到 main.go 文件"
    echo "请确保在正确的目录下运行此脚本"
    echo "当前目录: $DEPLOY_DIR"
    exit 1
fi

echo "✅ 文件检查通过"

# 1. 安装 Docker
echo ""
echo "📦 检查 Docker..."
if ! command -v docker &> /dev/null; then
    echo "正在安装 Docker..."
    curl -fsSL https://get.docker.com | sh
    sudo systemctl start docker
    sudo systemctl enable docker
fi

if ! command -v docker-compose &> /dev/null; then
    echo "正在安装 Docker Compose..."
    sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    sudo chmod +x /usr/local/bin/docker-compose
fi

echo "✅ Docker 环境就绪"

# 2. 安装 Clash
echo ""
echo "📥 安装 Clash..."
sudo mkdir -p /opt/clash
cd /opt/clash

# 下载 Clash Premium（使用新的下载链接）
if [ ! -f "clash" ]; then
    echo "下载 Clash Premium..."
    # 使用 Meta 版本（更新维护的版本）
    sudo wget -O clash.gz https://github.com/MetaCubeX/mihomo/releases/download/v1.18.0/mihomo-linux-amd64-v1.18.0.gz 2>/dev/null || \
    sudo wget -O clash.gz https://github.com/MetaCubeX/Clash.Meta/releases/download/v1.15.1/clash.meta-linux-amd64-v1.15.1.gz 2>/dev/null || \
    {
        echo "⚠️  Clash 下载失败，尝试备用方案..."
        # 如果都失败，使用预编译的二进制
        sudo wget -O clash https://raw.githubusercontent.com/Kuingsmile/clash-core/master/premium/clash-linux-amd64 2>/dev/null
    }

    if [ -f "clash.gz" ]; then
        sudo gunzip clash.gz 2>/dev/null || sudo mv clash.gz clash
    fi
    sudo chmod +x clash
fi

# 下载 GeoIP 数据库
if [ ! -f "Country.mmdb" ]; then
    echo "下载 GeoIP 数据库..."
    sudo wget https://github.com/Dreamacro/maxmind-geoip/releases/latest/download/Country.mmdb
fi

# 下载订阅配置
echo "📥 下载 Clash 配置..."
sudo wget -O config.yaml "https://dash.pqjc.site/api/v1/client/subscribe?token=0b98777d0a5c462a144b89588db6d49d"

# 检查配置文件
if [ ! -s "config.yaml" ]; then
    echo "❌ 订阅配置下载失败"
    exit 1
fi

echo "✅ Clash 配置完成"

# 3. 创建 Clash systemd 服务
echo ""
echo "⚙️  配置 Clash 服务..."
sudo tee /etc/systemd/system/clash.service > /dev/null <<EOF
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
sudo systemctl daemon-reload
sudo systemctl start clash
sudo systemctl enable clash

echo "⏳ 等待 Clash 启动..."
sleep 8

# 测试 Clash
echo "🔍 测试 Clash 代理..."
if curl -x http://127.0.0.1:7890 -s --connect-timeout 5 https://www.google.com > /dev/null 2>&1; then
    echo "✅ Clash 代理工作正常"
    PROXY_IP=$(curl -x http://127.0.0.1:7890 -s https://api.ip.sb/ip 2>/dev/null)
    echo "当前代理 IP: $PROXY_IP"
else
    echo "⚠️  Clash 代理测试失败，检查状态..."
    sudo systemctl status clash --no-pager
    echo "继续部署，稍后可手动检查..."
fi

# 4. 部署 Chat Gateway
echo ""
echo "🚀 部署 Chat Gateway..."
cd $DEPLOY_DIR

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
sudo docker-compose build

echo "🚀 启动服务..."
sudo docker-compose up -d

# 等待服务启动
echo "⏳ 等待服务启动..."
sleep 10

# 5. 测试服务
echo ""
echo "🔍 测试服务..."
if curl -s http://localhost:8080/health > /dev/null; then
    echo "✅ 健康检查通过"
else
    echo "⚠️  健康检查失败，查看日志..."
    sudo docker logs chat-gateway --tail 20
fi

# 6. 显示状态
echo ""
echo "=========================================="
echo "✅ 部署完成！"
echo "=========================================="
echo ""
echo "📊 服务状态："
echo "  Clash:        $(sudo systemctl is-active clash)"
echo "  Chat Gateway: $(sudo docker ps --filter name=chat-gateway --format '{{.Status}}' 2>/dev/null || echo '未运行')"
echo ""
SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || curl -s icanhazip.com 2>/dev/null || echo "your-server-ip")
echo "🌐 访问地址："
echo "  API 端点:     http://$SERVER_IP:8080/v1"
echo "  健康检查:     http://$SERVER_IP:8080/health"
echo "  Clash 面板:   http://$SERVER_IP:9090/ui"
echo ""
echo "📝 管理命令："
echo "  查看日志:     sudo docker logs -f chat-gateway"
echo "  重启服务:     cd $DEPLOY_DIR && sudo docker-compose restart"
echo "  停止服务:     cd $DEPLOY_DIR && sudo docker-compose down"
echo "  Clash 状态:   sudo systemctl status clash"
echo ""
echo "🧪 测试命令："
echo "  curl http://localhost:8080/v1/models"
echo "  curl -X POST http://localhost:8080/v1/chat/completions \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"model\":\"gpt-5.2\",\"messages\":[{\"role\":\"user\",\"content\":\"你好\"}]}'"
echo ""
echo "=========================================="
