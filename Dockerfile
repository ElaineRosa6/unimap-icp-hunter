# 使用官方Go镜像作为构建环境
FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制go.mod和go.sum文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用（Web）
RUN go build -o unimap-web ./cmd/unimap-web

# 使用alpine作为运行环境
FROM alpine:latest

# 设置工作目录
WORKDIR /app

# 复制构建结果
COPY --from=builder /app/unimap-web /app/

# 复制配置文件
COPY configs /app/configs

# 复制Web文件
COPY web /app/web

# 安装依赖（HTTPS + chromedp 截图需要 Chromium）
RUN apk add --no-cache ca-certificates chromium ttf-freefont

# 暴露端口
EXPOSE 8080

# 启动应用
CMD ["./unimap-web"]
