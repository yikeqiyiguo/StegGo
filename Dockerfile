# syntax=docker/dockerfile:1
# StegGo V1.0 - 完全离线部署镜像（CLI 版）
# GUI/TUI 需要终端/图形环境，请使用宿主机二进制。

# ---------- 构建阶段 ----------
FROM golang:1.26-alpine AS builder

WORKDIR /src

# 依赖缓存层
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/steggo ./cmd/steggo \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/steggo-tui ./cmd/steggo-tui

# ---------- 运行阶段 ----------
FROM alpine:3.21

# 完全离线：不安装任何网络工具
RUN apk add --no-cache tzdata ca-certificates \
 && adduser -D -u 10001 steg \
 && mkdir -p /data \
 && chown steg:steg /data

WORKDIR /data
USER steg

COPY --from=builder /out/steggo /usr/local/bin/steggo
COPY --from=builder /out/steggo-tui /usr/local/bin/steggo-tui

# 仅供容器内批处理/服务化使用；密码请通过 -p 传入或由环境变量注入
ENTRYPOINT ["steggo"]
CMD ["--help"]
