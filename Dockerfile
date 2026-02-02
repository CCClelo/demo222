# 使用官方 Go 镜像作为构建环境
FROM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY main.go .

# 编译程序
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o chat-gateway main.go

# 使用轻量级镜像运行
FROM alpine:latest

# 安装 ca-certificates（HTTPS 需要）和 docker cli（重启容器需要）
RUN apk --no-cache add ca-certificates docker-cli

WORKDIR /root/

# 从构建阶段复制编译好的程序
COPY --from=builder /app/chat-gateway .

# 暴露端口
EXPOSE 8080

# 运行程序
CMD ["./chat-gateway"]
