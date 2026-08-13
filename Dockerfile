# syntax=docker/dockerfile:1
# ============================================================
# StegGo V2.0 - 插件化离线部署镜像
#
# 构建目标（--build-arg TARGET=...，默认 all）：
#   cli  -> 仅 CLI（steggo）
#   tui  -> 仅 TUI（steggo-tui）
#   all  -> CLI + TUI（默认，纯 Go 无 cgo，完全离线）
#   gui  -> CLI + TUI + Linux 桌面 GUI（steggo-gui，需 cgo；运行需图形环境）
#
# 版本（--build-arg VERSION=2.0.0，注入 CLI version 命令）：
#   docker build --build-arg VERSION=2.0.0 -t steggo:2.0.0 .
#
# 多架构（需 buildx）：
#   docker buildx build --platform linux/amd64,linux/arm64 -t steggo:2.0.0 .
# ============================================================
ARG TARGET=all
ARG VERSION=2.0.0

# ---------- 构建阶段：CLI + TUI（纯 Go，CGO_ENABLED=0） ----------
FROM golang:1.26-alpine AS builder-core

ARG VERSION
WORKDIR /src

# 依赖缓存层
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/steggo ./cmd/cli \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/steggo-tui ./cmd/tui

# ---------- 构建阶段：GUI（可选，需 cgo + X11 开发库，体积较大） ----------
FROM golang:1.26-bookworm AS builder-gui

WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc libc6-dev pkg-config \
    libgl1-mesa-dev xorg-dev libglfw3-dev \
 && rm -rf /var/lib/apt/lists/*

# GUI 为独立 module，先单独拉取依赖
COPY cmd/gui/go.mod cmd/gui/go.sum ./cmd/gui/
RUN cd cmd/gui && go mod download

COPY . .
RUN cd cmd/gui && CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o /out/steggo-gui .

# ---------- 运行基础层：系统设施 ----------
FROM alpine:3.21 AS runtime

RUN apk add --no-cache tzdata ca-certificates \
 && adduser -D -u 10001 steg \
 && mkdir -p /data \
 && chown steg:steg /data

WORKDIR /data
USER steg

# ---------- 插件化终态：按 TARGET 选择 ----------
FROM runtime AS final-cli
COPY --from=builder-core /out/steggo /usr/local/bin/steggo
ENTRYPOINT ["steggo"]
CMD ["--help"]

FROM runtime AS final-tui
COPY --from=builder-core /out/steggo-tui /usr/local/bin/steggo-tui
ENTRYPOINT ["steggo-tui"]
CMD ["--help"]

FROM runtime AS final-all
COPY --from=builder-core /out/steggo /usr/local/bin/steggo
COPY --from=builder-core /out/steggo-tui /usr/local/bin/steggo-tui
ENTRYPOINT ["steggo"]
CMD ["--help"]

FROM final-all AS final-gui
COPY --from=builder-gui /out/steggo-gui /usr/local/bin/steggo-gui

ARG TARGET
FROM final-${TARGET}
