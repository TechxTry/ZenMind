# ---- Stage 1: Build ----
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 先复制依赖文件，利用 Docker layer 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/zmcli ./cmd/zmcli

# ---- Stage 2: Runtime ----
FROM alpine:3.19

RUN apk add --no-cache tzdata ca-certificates
ENV TZ=Asia/Shanghai

WORKDIR /app

# 仅复制二进制和配置文件
COPY --from=builder /app/server .
COPY --from=builder /app/zmcli .
COPY config/etl_tables.yaml ./config/etl_tables.yaml

EXPOSE 8080

CMD ["./server"]
