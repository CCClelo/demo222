@echo off
chcp 65001 >nul
echo ==========================================
echo Chat Gateway 文件上传脚本（密钥登录版）
echo ==========================================
echo.

set /p SERVER_IP="请输入服务器 IP 地址: "
set /p SERVER_USER="请输入服务器用户名 (默认 root): "
if "%SERVER_USER%"=="" set SERVER_USER=root

set /p KEY_FILE="请输入密钥文件路径 (例如: C:\Users\xxx\.ssh\id_rsa): "

echo.
echo 📤 正在上传文件到 %SERVER_USER%@%SERVER_IP%...
echo 🔑 使用密钥: %KEY_FILE%
echo.

REM 创建远程目录
ssh -i "%KEY_FILE%" %SERVER_USER%@%SERVER_IP% "mkdir -p /opt/chat-gateway"

REM 上传主要文件
echo [1/5] 上传 main.go...
scp -i "%KEY_FILE%" main.go %SERVER_USER%@%SERVER_IP%:/opt/chat-gateway/

echo [2/5] 上传 Go 依赖文件...
scp -i "%KEY_FILE%" go.mod go.sum %SERVER_USER%@%SERVER_IP%:/opt/chat-gateway/

echo [3/5] 上传 Dockerfile...
scp -i "%KEY_FILE%" Dockerfile %SERVER_USER%@%SERVER_IP%:/opt/chat-gateway/

echo [4/5] 上传部署脚本...
scp -i "%KEY_FILE%" auto-deploy.sh %SERVER_USER%@%SERVER_IP%:/opt/chat-gateway/

echo [5/5] 设置执行权限...
ssh -i "%KEY_FILE%" %SERVER_USER%@%SERVER_IP% "chmod +x /opt/chat-gateway/auto-deploy.sh"

echo.
echo ✅ 文件上传完成！
echo.
echo ==========================================
echo 下一步操作：
echo ==========================================
echo 1. 连接到服务器：
echo    ssh -i "%KEY_FILE%" %SERVER_USER%@%SERVER_IP%
echo.
echo 2. 运行部署脚本：
echo    cd /opt/chat-gateway
echo    ./auto-deploy.sh
echo.
echo 3. 等待部署完成后测试：
echo    curl http://%SERVER_IP%:8080/health
echo ==========================================
echo.
pause
