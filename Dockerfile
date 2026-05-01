# ============================================
# bookget-web Dockerfile
# Multi-stage build for minimal image size
# ============================================

# Stage 1: Build
FROM golang:1.23-alpine AS builder

# 使用阿里云 Alpine 镜像源
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk add --no-cache git ca-certificates tzdata

# 使用七牛云 Go 模块代理
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .

# Build webapi binary (static linking for Alpine)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X bookget/config.Version=25.0701-web" \
    -o bookget-web \
    ./cmd/webapi/

# Stage 2: Runtime (minimal image)
FROM alpine:3.19

# 使用阿里云 Alpine 镜像源
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk add --no-cache ca-certificates tzdata curl

# Set timezone
ENV TZ=Asia/Shanghai

# Create non-root user
RUN adduser -D -h /app -s /sbin/nologin bookget

WORKDIR /app

# Copy binary
COPY --from=builder /build/bookget-web /app/bookget-web
RUN chmod +x /app/bookget-web

# Create download directory
RUN mkdir -p /app/downloads && chown -R bookget:bookget /app

USER bookget

# Expose port
EXPOSE 8088

# Health check
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8088/api/status || exit 1

ENTRYPOINT ["/app/bookget-web"]
