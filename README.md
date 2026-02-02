# Chat SDK Gateway 部署指南

## 📋 部署方案

### 方案1：Docker Compose 部署（推荐）

#### 1. 准备服务器
- 系统：Ubuntu 20.04+ / CentOS 7+ / Debian 10+
- 配置：1核2G 起步，建议 2核4G
- 端口：开放 8080 端口

#### 2. 上传文件到服务器
```bash
# 将以下文件上传到服务器 /opt/chat-gateway 目录
- main.go
- go.mod
- go.sum
- Dockerfile
- docker-compose.yml
- deploy.sh
```

#### 3. 一键部署
```bash
cd /opt/chat-gateway
chmod +x deploy.sh
./deploy.sh
```

#### 4. 验证服务
```bash
# 健康检查
curl http://localhost:8080/health

# 查看模型列表
curl http://localhost:8080/v1/models

# 测试聊天
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.2","messages":[{"role":"user","content":"你好"}],"stream":false}'
```

---

### 方案2：直接编译部署（无 Docker）

#### 1. 安装 Go
```bash
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

#### 2. 编译程序
```bash
cd /opt/chat-gateway
go mod tidy
go build -o chat-gateway main.go
```

#### 3. 创建 systemd 服务
```bash
cat > /etc/systemd/system/chat-gateway.service <<EOF
[Unit]
Description=Chat SDK Gateway
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/chat-gateway
ExecStart=/opt/chat-gateway/chat-gateway
Environment="BASE_URL=https://demo.chat-sdk.dev"
Environment="PORT=8080"
Environment="USE_AUTH=true"
Environment="WARP_PROXIES=socks5://127.0.0.1:1080"
Restart=always

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl start chat-gateway
systemctl enable chat-gateway
```

#### 4. 查看状态
```bash
systemctl status chat-gateway
journalctl -u chat-gateway -f
```

---

### 方案3：使用现有 Clash 代理

如果服务器上已有 Clash：

#### 修改 docker-compose.yml
```yaml
services:
  chat-gateway:
    build: .
    container_name: chat-gateway
    restart: always
    ports:
      - "8080:8080"
    environment:
      - BASE_URL=https://demo.chat-sdk.dev
      - WARP_PROXIES=http://host.docker.internal:7890  # Clash 端口
      - WARP_CONTAINERS=clash
      - PORT=8080
      - USE_AUTH=true
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

---

## 🔧 配置说明

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| BASE_URL | 上游 Chat SDK 地址 | https://demo.chat-sdk.dev |
| PORT | 监听端口 | 8080 |
| WARP_PROXIES | 代理列表（逗号分隔） | 空（不使用代理） |
| WARP_CONTAINERS | Docker 容器名（逗号分隔） | 空 |
| USE_AUTH | 是否使用账户模式 | false |
| DEBUG | 调试模式 | false |

### 代理配置示例

**单代理：**
```bash
WARP_PROXIES=socks5://127.0.0.1:1080
```

**多代理（轮换）：**
```bash
WARP_PROXIES=socks5://warp1:1080,socks5://warp2:1080,socks5://warp3:1080
WARP_CONTAINERS=warp1,warp2,warp3
```

**HTTP 代理：**
```bash
WARP_PROXIES=http://127.0.0.1:7890
```

---

## 📊 管理命令

### Docker Compose 方式

```bash
# 查看日志
docker-compose logs -f chat-gateway

# 重启服务
docker-compose restart

# 停止服务
docker-compose down

# 更新代码后重新部署
docker-compose down
docker-compose build
docker-compose up -d

# 查看容器状态
docker-compose ps
```

### Systemd 方式

```bash
# 查看状态
systemctl status chat-gateway

# 查看日志
journalctl -u chat-gateway -f

# 重启服务
systemctl restart chat-gateway

# 停止服务
systemctl stop chat-gateway
```

---

## 🔒 安全建议

1. **使用反向代理（Nginx）**
```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

2. **配置 SSL 证书**
```bash
certbot --nginx -d your-domain.com
```

3. **限制访问（可选）**
```nginx
# 添加 IP 白名单
allow 1.2.3.4;
deny all;

# 或添加认证
auth_basic "Restricted";
auth_basic_user_file /etc/nginx/.htpasswd;
```

---

## 🐛 故障排查

### 服务无法启动
```bash
# 查看详细日志
docker-compose logs chat-gateway

# 检查端口占用
netstat -tlnp | grep 8080

# 检查 Docker 网络
docker network ls
```

### 代理连接失败
```bash
# 测试代理连接
curl -x socks5://127.0.0.1:1080 https://www.google.com

# 检查 WARP 容器状态
docker ps | grep warp
docker logs warp1
```

### 429 限流问题
- 增加代理数量
- 启用账户模式（USE_AUTH=true）
- 检查代理 IP 是否被封

---

## 📈 性能优化

1. **增加代理数量**：减少单个代理的请求压力
2. **启用账户模式**：每个代理独立账户，提高并发
3. **调整超时时间**：修改 main.go 中的 `Timeout: 60 * time.Second`
4. **使用 CDN**：如果对外提供服务，建议使用 Cloudflare

---

## 📞 使用示例

### OpenAI 格式
```bash
curl -X POST http://your-server:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.2",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

### Anthropic 格式
```bash
curl -X POST http://your-server:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus-4.5",
    "messages": [{"role": "user", "content": "Hello"}],
    "max_tokens": 1024
  }'
```

### 在应用中使用
- **API 地址**：`http://your-server:8080/v1`
- **API Key**：不需要（或随意填写）
- **支持模型**：gpt-5.2, claude-opus-4.5, claude-sonnet-4.5, gemini-3-pro-preview
