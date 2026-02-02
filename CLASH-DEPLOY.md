# 服务器使用 Clash 代理部署指南

## 🎯 三种方案对比

| 方案 | 优点 | 缺点 | 适用场景 |
|------|------|------|----------|
| **方案1：服务器安装 Clash** | 性能最好，配置简单 | 需要手动管理 Clash | 推荐，适合长期使用 |
| **方案2：Clash Docker 容器** | 统一管理，易于迁移 | 配置稍复杂 | 适合容器化部署 |
| **方案3：使用本地 Clash 转发** | 无需服务器配置 | 网络延迟高，不稳定 | 仅测试用 |

---

## 方案1：在服务器上安装 Clash（推荐）⭐

### 步骤1：安装 Clash

```bash
# 上传安装脚本到服务器
scp install-clash.sh root@your-server:/root/

# 在服务器上执行
ssh root@your-server
chmod +x install-clash.sh
./install-clash.sh
```

### 步骤2：配置 Clash

**方法A：使用订阅链接**
```bash
cd /opt/clash

# 下载订阅配置
wget -O config.yaml "你的Clash订阅链接"

# 或者手动编辑
nano config.yaml
```

**方法B：从本地上传配置**
```bash
# 在你的电脑上，找到 Clash 配置文件
# Windows: C:\Users\你的用户名\.config\clash\config.yaml
# 上传到服务器
scp "C:\Users\你的用户名\.config\clash\config.yaml" root@your-server:/opt/clash/
```

### 步骤3：启动 Clash

```bash
# 测试运行
cd /opt/clash
./clash -d /opt/clash

# 看到 "HTTP proxy listening at: :7890" 表示成功
# 按 Ctrl+C 停止
```

### 步骤4：设置开机自启

```bash
# 上传 systemd 服务文件
scp clash.service root@your-server:/etc/systemd/system/

# 启动服务
systemctl daemon-reload
systemctl start clash
systemctl enable clash

# 查看状态
systemctl status clash
```

### 步骤5：测试 Clash 代理

```bash
# 测试 HTTP 代理
curl -x http://127.0.0.1:7890 https://www.google.com

# 测试 SOCKS5 代理
curl -x socks5://127.0.0.1:7891 https://www.google.com

# 查看当前 IP
curl -x http://127.0.0.1:7890 https://api.ip.sb/ip
```

### 步骤6：部署 Chat Gateway

```bash
# 上传文件
scp main.go go.mod go.sum Dockerfile docker-compose-clash.yml root@your-server:/opt/chat-gateway/

# 部署
cd /opt/chat-gateway
mv docker-compose-clash.yml docker-compose.yml

# 编辑配置（使用方案A）
nano docker-compose.yml
# 确保使用 network_mode: "host" 和 WARP_PROXIES=http://127.0.0.1:7890

# 启动
docker-compose build
docker-compose up -d

# 查看日志
docker-compose logs -f
```

---

## 方案2：Clash 也在 Docker 中运行

### 步骤1：准备 Clash 配置

```bash
# 在服务器上创建目录
mkdir -p /opt/chat-gateway
cd /opt/chat-gateway

# 上传你的 Clash 配置文件
scp "C:\Users\你的用户名\.config\clash\config.yaml" root@your-server:/opt/chat-gateway/clash-config.yaml

# 下载 GeoIP 数据库
wget https://github.com/Dreamacro/maxmind-geoip/releases/latest/download/Country.mmdb
```

### 步骤2：修改 Clash 配置

```bash
nano clash-config.yaml
```

确保包含以下配置：
```yaml
port: 7890
socks-port: 7891
allow-lan: true  # 重要：允许局域网访问
bind-address: "*"  # 监听所有接口
external-controller: 0.0.0.0:9090
```

### 步骤3：部署

```bash
# 上传文件
scp main.go go.mod go.sum Dockerfile docker-compose-clash.yml root@your-server:/opt/chat-gateway/

cd /opt/chat-gateway
mv docker-compose-clash.yml docker-compose.yml

# 使用方案B的配置（Clash 在 Docker 中）
docker-compose up -d

# 查看日志
docker-compose logs -f clash
docker-compose logs -f chat-gateway
```

---

## 方案3：使用本地 Clash 转发（仅测试）

### 在你的 Windows 电脑上：

1. **开启 Clash 的局域网访问**
   - 打开 Clash
   - 设置 → 允许局域网连接

2. **配置防火墙**
   ```powershell
   # 以管理员身份运行 PowerShell
   New-NetFirewallRule -DisplayName "Clash Proxy" -Direction Inbound -LocalPort 7890 -Protocol TCP -Action Allow
   ```

3. **获取本地 IP**
   ```cmd
   ipconfig
   # 找到你的局域网 IP，如 192.168.1.100
   ```

### 在服务器上：

```bash
# 修改 docker-compose.yml
nano docker-compose.yml

# 设置代理为你的电脑 IP
environment:
  - WARP_PROXIES=http://192.168.1.100:7890

# 启动
docker-compose up -d
```

**注意**：这种方式仅适合测试，不适合生产环境！

---

## 🔍 验证部署

### 1. 检查 Clash 状态
```bash
# 方案1（系统服务）
systemctl status clash
curl -x http://127.0.0.1:7890 https://api.ip.sb/ip

# 方案2（Docker）
docker logs clash
curl -x http://localhost:7890 https://api.ip.sb/ip
```

### 2. 检查 Gateway 状态
```bash
docker logs chat-gateway

# 应该看到类似输出：
# [INFO] Chat SDK 2API 网关启动
# [INFO] 监听端口: 8080
# [INFO] WARP 代理: http://127.0.0.1:7890
# [INFO] 服务就绪，等待请求...
```

### 3. 测试 API
```bash
# 健康检查
curl http://localhost:8080/health

# 测试聊天（会通过 Clash 代理）
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.2","messages":[{"role":"user","content":"你好"}],"stream":false}'
```

---

## 🐛 常见问题

### Q1: Clash 无法连接
```bash
# 检查 Clash 是否运行
systemctl status clash
# 或
docker ps | grep clash

# 检查端口是否监听
netstat -tlnp | grep 7890

# 测试代理
curl -v -x http://127.0.0.1:7890 https://www.google.com
```

### Q2: Gateway 无法使用代理
```bash
# 查看 Gateway 日志
docker logs chat-gateway

# 检查网络连接
docker exec chat-gateway curl -x http://127.0.0.1:7890 https://www.google.com
```

### Q3: 订阅链接无法下载
```bash
# 手动下载配置
wget -O config.yaml "订阅链接"

# 或使用代理下载
curl -x http://existing-proxy:port -o config.yaml "订阅链接"
```

---

## 📊 推荐配置

### 最佳实践（方案1）

```yaml
# /opt/clash/config.yaml
port: 7890
socks-port: 7891
allow-lan: true
mode: rule
log-level: info
external-controller: 0.0.0.0:9090

# 你的节点配置...
proxies:
  - name: "节点1"
    type: vmess
    server: xxx.com
    port: 443
    # ...

proxy-groups:
  - name: "PROXY"
    type: select
    proxies:
      - 节点1
      - 节点2

rules:
  - DOMAIN-SUFFIX,chat-sdk.dev,PROXY
  - MATCH,DIRECT
```

### Gateway 配置

```yaml
# docker-compose.yml
services:
  chat-gateway:
    build: .
    container_name: chat-gateway
    restart: always
    network_mode: "host"
    environment:
      - BASE_URL=https://demo.chat-sdk.dev
      - WARP_PROXIES=http://127.0.0.1:7890
      - PORT=8080
      - USE_AUTH=true
      - DEBUG=false
```

---

## 🎉 完成！

部署完成后，你的服务器将：
- ✅ 通过 Clash 代理访问上游服务
- ✅ 自动注册账号绕过限制
- ✅ 提供 OpenAI 兼容的 API 接口
- ✅ 支持多种 AI 模型

**API 地址**: `http://your-server-ip:8080/v1`
