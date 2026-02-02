# Chat Gateway 快速部署指南（使用你的 Clash 订阅）

## 🎯 部署流程（3 步完成）

### 第 1 步：上传文件到服务器

**在 Windows 上双击运行：**
```
E:\demo\upload.bat
```

按提示输入：
- 服务器 IP 地址
- 用户名（默认 root）

脚本会自动上传所有文件到服务器。

---

### 第 2 步：SSH 连接到服务器

```bash
ssh root@your-server-ip
```

---

### 第 3 步：运行一键部署脚本

```bash
cd /opt/chat-gateway
./auto-deploy.sh
```

脚本会自动：
- ✅ 安装 Docker 和 Docker Compose
- ✅ 下载并配置 Clash（使用你的订阅）
- ✅ 启动 Clash 代理服务
- ✅ 构建并启动 Chat Gateway
- ✅ 测试所有服务

**等待 5-10 分钟，部署完成！**

---

## 🧪 测试服务

### 1. 健康检查
```bash
curl http://localhost:8080/health
# 应该返回: OK
```

### 2. 查看可用模型
```bash
curl http://localhost:8080/v1/models
```

### 3. 测试聊天
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.2",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": false
  }'
```

### 4. 检查代理状态
```bash
# 查看 Clash 状态
systemctl status clash

# 查看当前代理 IP
curl -x http://127.0.0.1:7890 https://api.ip.sb/ip

# 查看 Gateway 日志
docker logs -f chat-gateway
```

---

## 📊 服务管理

### Clash 管理
```bash
# 查看状态
systemctl status clash

# 重启 Clash
systemctl restart clash

# 查看日志
journalctl -u clash -f

# 更新订阅配置
cd /opt/clash
wget -O config.yaml "https://dash.pqjc.site/api/v1/client/subscribe?token=0b98777d0a5c462a144b89588db6d49d"
systemctl restart clash
```

### Gateway 管理
```bash
cd /opt/chat-gateway

# 查看日志
docker logs -f chat-gateway

# 重启服务
docker-compose restart

# 停止服务
docker-compose down

# 重新构建
docker-compose build
docker-compose up -d
```

---

## 🌐 外网访问

### 方法1：直接访问（需开放端口）

1. **开放防火墙端口**
```bash
# Ubuntu/Debian
ufw allow 8080

# CentOS/RHEL
firewall-cmd --permanent --add-port=8080/tcp
firewall-cmd --reload
```

2. **云服务器安全组**
   - 登录云服务商控制台
   - 添加安全组规则：允许 TCP 8080 端口

3. **访问地址**
```
http://your-server-ip:8080/v1
```

### 方法2：使用 Nginx 反向代理（推荐）

```bash
# 安装 Nginx
apt install nginx -y  # Ubuntu/Debian
# 或
yum install nginx -y  # CentOS

# 创建配置
cat > /etc/nginx/sites-available/chat-gateway <<EOF
server {
    listen 80;
    server_name your-domain.com;  # 改成你的域名

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }
}
EOF

# 启用配置
ln -s /etc/nginx/sites-available/chat-gateway /etc/nginx/sites-enabled/
nginx -t
systemctl restart nginx
```

### 方法3：配置 HTTPS（推荐）

```bash
# 安装 Certbot
apt install certbot python3-certbot-nginx -y

# 申请证书
certbot --nginx -d your-domain.com

# 自动续期
certbot renew --dry-run
```

---

## 🔧 配置优化

### 启用调试模式
```bash
cd /opt/chat-gateway
nano docker-compose.yml

# 修改环境变量
environment:
  - DEBUG=true  # 改为 true

# 重启
docker-compose restart
docker logs -f chat-gateway
```

### 修改监听端口
```bash
nano docker-compose.yml

# 修改端口映射
environment:
  - PORT=8080  # 改成其他端口如 3000

# 重启
docker-compose restart
```

### 禁用账户模式（使用游客模式）
```bash
nano docker-compose.yml

environment:
  - USE_AUTH=false  # 改为 false

docker-compose restart
```

---

## 🐛 故障排查

### 问题1：Clash 无法启动
```bash
# 查看详细日志
journalctl -u clash -n 50

# 检查配置文件
cat /opt/clash/config.yaml

# 手动测试
cd /opt/clash
./clash -d /opt/clash

# 重新下载订阅
wget -O config.yaml "https://dash.pqjc.site/api/v1/client/subscribe?token=0b98777d0a5c462a144b89588db6d49d"
systemctl restart clash
```

### 问题2：代理连接失败
```bash
# 测试代理
curl -v -x http://127.0.0.1:7890 https://www.google.com

# 检查端口
netstat -tlnp | grep 7890

# 查看 Clash 日志
journalctl -u clash -f
```

### 问题3：Gateway 无法访问上游
```bash
# 查看 Gateway 日志
docker logs chat-gateway

# 进入容器测试
docker exec -it chat-gateway sh
curl -x http://127.0.0.1:7890 https://demo.chat-sdk.dev

# 检查网络模式
docker inspect chat-gateway | grep NetworkMode
# 应该是 "host"
```

### 问题4：429 限流
```bash
# 查看日志中的限流信息
docker logs chat-gateway | grep 429

# 检查代理 IP
curl -x http://127.0.0.1:7890 https://api.ip.sb/ip

# 重启 Clash 刷新 IP
systemctl restart clash
sleep 10
docker-compose restart
```

---

## 📱 在应用中使用

### 配置示例

**API 地址：** `http://your-server-ip:8080/v1`
**API Key：** 不需要（或随意填写）

**支持的模型：**
- `gpt-5.2`
- `claude-opus-4.5`
- `claude-sonnet-4.5`
- `gemini-3-pro-preview`

### 客户端配置示例

**ChatGPT Next Web:**
```
API 地址: http://your-server-ip:8080
API Key: sk-any-key-works
模型: gpt-5.2
```

**OpenAI SDK (Python):**
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://your-server-ip:8080/v1",
    api_key="any-key"
)

response = client.chat.completions.create(
    model="gpt-5.2",
    messages=[{"role": "user", "content": "你好"}]
)
```

**Curl:**
```bash
curl -X POST http://your-server-ip:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer any-key" \
  -d '{
    "model": "gpt-5.2",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

---

## 🎉 完成！

现在你的服务器已经：
- ✅ 运行 Clash 代理（使用你的订阅）
- ✅ 运行 Chat Gateway（通过 Clash 访问上游）
- ✅ 自动注册账号绕过限制
- ✅ 提供 OpenAI 兼容 API

**享受你的 AI 服务吧！** 🚀
