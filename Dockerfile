# 使用官方Go镜像作为构建环境
FROM golang:1.26-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制go.mod和go.sum文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用（Web）
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o unimap-web ./cmd/unimap-web

# 使用alpine作为运行环境（固定版本）
FROM alpine:3.21

# 设置工作目录
WORKDIR /app

# 安装依赖（HTTPS + chromedp 截图需要 Chromium）
RUN apk add --no-cache ca-certificates chromium ttf-freefont

# 复制构建结果
COPY --from=builder /app/unimap-web /app/

# 复制配置文件
COPY configs /app/configs

# 复制Web文件
COPY web /app/web

# 创建非root用户
RUN addgroup -S unimap && adduser -S -G unimap -h /app unimap

# 设置目录所有权
RUN chown -R unimap:unimap /app

# 切换到非root用户
USER unimap:unimap

# 暴露端口
EXPOSE 8448

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8448/health || exit 1

# 启动应用
CMD ["./unimap-web"]
