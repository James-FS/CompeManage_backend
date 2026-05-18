# 构建阶段
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 复制 go mod 文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并构建
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# 运行阶段
FROM alpine:latest

WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata

# 从构建阶段复制二进制文件
COPY --from=builder /app/main .
COPY --from=builder /app/.env ./

EXPOSE 8080

CMD ["./main"]