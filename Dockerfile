# 构建阶段
FROM golang:1.21-alpine AS builder

# 安装必要的包
RUN apk add --no-cache git ca-certificates

# 设置工作目录
WORKDIR /workspace

# 复制go mod文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o lightweight-descheduler \
    ./cmd/main.go

# 运行阶段
FROM scratch

# 从构建阶段复制ca证书
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# 从构建阶段复制二进制文件
COPY --from=builder /workspace/lightweight-descheduler /app/lightweight-descheduler

# 创建非root用户
USER 65534:65534

# 设置入口点
ENTRYPOINT ["/app/lightweight-descheduler"]

# 设置标签
LABEL maintainer="your-email@example.com"
LABEL description="轻量级 Kubernetes 重调度器"
LABEL version="v1.0.0"
