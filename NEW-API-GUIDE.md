# New API 部署指南

## 📖 什么是 New API？

New API 是一个 **API 管理和中转系统**，功能包括：
- ✅ 统一管理多个 API 渠道
- ✅ 令牌管理和额度控制
- ✅ 使用统计和监控
- ✅ 支持多用户
- ✅ 兼容 OpenAI API 格式

---

## 🚀 快速部署

### 方法1：使用一键脚本（推荐）

```bash
# 1. 上传脚本到服务器
cd ~
wget https://raw.githubusercontent.com/CCClelo/demo222/main/deploy-newapi.sh

# 或者手动创建
nano deploy-newapi.sh
# 粘贴脚本内容

# 2. 运行部署
chmod +x deploy-newapi.sh
./deploy-newapi.sh
```

### 方法2：手动部署

```bash
# 1. 创建目录
mkdir -p ~/new-api
cd ~/new-api

# 2. 创建 docker-compose.yml
cat > docker-compose.yml <<'EOF'
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
      - SESSION_SECRET=your-random-secret-key-here
      - TZ=Asia/Shanghai
    volumes:
      - ./data:/data
    extra_hosts:
      - "host.docker.internal:host-gateway"
    command: --log-dir /data/logs
EOF

# 3. 创建数据目录
mkdir -p ./data/logs

# 4. 启动服务
sudo docker-compose up -d

# 5. 查看日志
sudo docker logs -f new-api
```

---

## 🔧 配置开机自启

```bash
# 1. 上传服务文件
sudo nano /etc/systemd/system/new-api.service

# 粘贴以下内容：
[Unit]
Description=New API Service
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/home/azureuser/new-api
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down
User=root

[Install]
WantedBy=multi-user.target

# 2. 启用服务
sudo systemctl daemon-reload
sudo systemctl enable new-api
sudo systemctl status new-api
```

---

## 🌐 访问管理面板

### 1. 开放端口

```bash
# Ubuntu/Debian
sudo ufw allow 3000

# 或在云服务器控制台添加安全组规则：TCP 3000
```

### 2. 访问地址

浏览器打开：`http://your-server-ip:3000`

### 3. 默认账号

- **用户名**：`root`
- **密码**：`123456`

**⚠️ 重要：登录后立即修改密码！**

---

## 🔗 集成 Chat Gateway

### 步骤1：添加渠道

1. 登录 New API 管理面板
2. 点击 **渠道管理** → **添加渠道**
3. 填写配置：

| 字段 | 值 |
|------|-----|
| 类型 | OpenAI |
| 名称 | Chat Gateway |
| Base URL | `http://host.docker.internal:8080/v1` |
| 密钥 | `sk-test`（随意填写） |
| 模型 | `gpt-5.2,claude-opus-4.5,claude-sonnet-4.5,gemini-3-pro-preview` |
| 优先级 | 0 |

4. 点击 **提交**

### 步骤2：测试渠道

在渠道列表中点击 **测试** 按钮，确保连接正常。

### 步骤3：创建令牌

1. 点击 **令牌管理** → **添加令牌**
2. 配置：
   - 名称：`测试令牌`
   - 额度：`1000000`（100万tokens）
   - 过期时间：永不过期
   - 模型：选择所有模型
3. 点击 **提交**
4. **复制生成的 API Key**（只显示一次！）

---

## 🧪 测试 API

```bash
# 使用 New API 的令牌测试
curl -X POST http://your-server-ip:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxxxx" \
  -d '{
    "model": "gpt-5.2",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

---

## 📊 功能说明

### 1. 渠道管理
- 添加多个 API 渠道（OpenAI、Claude、自建等）
- 设置优先级和权重
- 自动故障转移

### 2. 令牌管理
- 创建多个 API Key
- 设置额度限制
- 设置过期时间
- 绑定特定模型

### 3. 用户管理
- 多用户支持
- 用户组管理
- 额度分配

### 4. 统计监控
- 实时使用统计
- 费用统计
- 日志查询

---

## 🔒 安全建议

### 1. 修改默认密码
```
设置 → 个人设置 → 修改密码
```

### 2. 配置 Nginx 反向代理

```nginx
server {
    listen 80;
    server_name api.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

### 3. 配置 HTTPS

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d api.yourdomain.com
```

---

## 📝 管理命令

```bash
# 查看日志
sudo docker logs -f new-api

# 重启服务
cd ~/new-api
sudo docker-compose restart

# 停止服务
sudo docker-compose down

# 更新到最新版本
sudo docker-compose pull
sudo docker-compose up -d

# 备份数据
cp -r ~/new-api/data ~/new-api-backup-$(date +%Y%m%d)
```

---

## 🐛 故障排查

### 问题1：无法访问管理面板

```bash
# 检查服务状态
sudo docker ps | grep new-api

# 查看日志
sudo docker logs new-api

# 检查端口
sudo netstat -tlnp | grep 3000
```

### 问题2：无法连接 Chat Gateway

确保：
1. Chat Gateway 正在运行：`sudo docker ps | grep chat-gateway`
2. 使用 `host.docker.internal` 而不是 `localhost`
3. 两个容器都在运行

### 问题3：渠道测试失败

```bash
# 进入 New API 容器测试
sudo docker exec -it new-api sh
curl http://host.docker.internal:8080/health
```

---

## 🎯 完整架构

```
用户请求
    ↓
New API (端口 3000)
    ↓
Chat Gateway (端口 8080)
    ↓
Clash 代理 (端口 7890)
    ↓
上游 API (demo.chat-sdk.dev)
```

---

## 📞 使用示例

### Python

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://your-server-ip:3000/v1",
    api_key="sk-xxxxxx"  # New API 生成的令牌
)

response = client.chat.completions.create(
    model="gpt-5.2",
    messages=[{"role": "user", "content": "你好"}]
)

print(response.choices[0].message.content)
```

### Curl

```bash
curl -X POST http://your-server-ip:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxxxx" \
  -d '{
    "model": "gpt-5.2",
    "messages": [{"role": "user", "content": "你好"}],
    "stream": false
  }'
```

---

## 🎉 部署完成！

现在你有了一个完整的 API 管理系统：
- ✅ New API：统一管理和分发
- ✅ Chat Gateway：自动注册和代理
- ✅ Clash：网络代理
- ✅ 全部开机自启

享受你的 AI 服务吧！🚀
