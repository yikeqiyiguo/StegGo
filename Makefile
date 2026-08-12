# StegGo V1.0 Makefile
# Windows 用户请使用 build.ps1 或直接运行 go build。

GO      ?= go
LDFLAGS := -s -w
VERSION ?= 1.0.0
BIN     := steggo
TUI     := steggo-tui

.PHONY: all build tui gui test vet lint clean install smoke

all: build tui

build: ## 构建 CLI
	$(GO) build -trimpath -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" -o $(BIN) ./cmd/steggo

tui: ## 构建 TUI
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(TUI) ./cmd/steggo-tui

gui: ## 构建 GUI（需要 cgo + C 编译器；Windows 需 MinGW-w64，Linux 需 xorg-dev）
	cd cmd/steggo-gui && CGO_ENABLED=1 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o ../../steggo-gui .

test: ## 运行全部单元测试
	$(GO) test -v ./...

test-race: ## 竞态检测测试（需 amd64/arm64 工具链）
	$(GO) test -race ./...

vet: ## 静态检查
	$(GO) vet ./...

lint: ## 代码风格检查（需安装 golangci-lint）
	golangci-lint run ./...

smoke: build ## 冒烟测试：信息/版本/能力矩阵
	./$(BIN) version
	./$(BIN) info
	./$(BIN) capacity -i testdata/carrier.png

clean: ## 清理构建产物
	rm -f $(BIN) $(TUI) steggo-gui steggo.exe steggo-tui.exe
	rm -rf dist/ build/

install: build ## 安装到 GOBIN
	$(GO) install -trimpath -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" ./cmd/steggo
