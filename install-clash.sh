#!/bin/bash

echo "=========================================="
echo "Clash 安装脚本"
echo "=========================================="

# 创建目录
mkdir -p /opt/clash
cd /opt/clash

# 下载 Clash Premium
echo "📥 下载 Clash..."
wget https://github.com/Dreamacro/clash/releases/download/premium/clash-linux-amd64-v3.gz
gunzip clash-linux-amd64-v3.gz
mv clash-linux-amd64-v3 clash
chmod +x clash

# 下载 Country.mmdb（GeoIP 数据库）
echo "📥 下载 GeoIP 数据库..."
wget https://github.com/Dreamacro/maxmind-geoip/releases/latest/download/Country.mmdb

# 创建配置文件模板
cat > config.yaml <<EOF
# Clash 配置文件
# 请替换为你自己的订阅链接或节点配置

port: 7890
socks-port: 7891
allow-lan: true
mode: rule
log-level: info
external-controller: 0.0.0.0:9090

# 代理配置
proxies:
  # 示例节点（请替换为你的实际节点）
  - name: "节点1"
    type: ss
    server: server.com
    port: 443
    cipher: aes-256-gcm
    password: password

proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - 节点1

rules:
  - MATCH,PROXY
EOF

echo ""
echo "✅ Clash 安装完成！"
echo ""
echo "=========================================="
echo "下一步操作："
echo "=========================================="
echo "1. 编辑配置文件："
echo "   nano /opt/clash/config.yaml"
echo ""
echo "2. 粘贴你的 Clash 配置或订阅链接"
echo ""
echo "3. 启动 Clash："
echo "   /opt/clash/clash -d /opt/clash"
echo ""
echo "4. 设置开机自启（见下方）"
echo "=========================================="
