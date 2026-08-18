# StegGo V2.2 Makefile
# Windows 用户请使用 build.ps1 或直接运行 go build。

GO      ?= go
LDFLAGS := -s -w
VERSION ?= 2.2.0
BIN     := steggo
TUI     := steggo-tui

.PHONY: all build tui gui test test-race vet lint clean install smoke docker cross termux wasm

all: build tui

build: ## 构建 CLI
	$(GO) build -trimpath -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" -o $(BIN) ./cmd/cli

tui: ## 构建 TUI
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(TUI) ./cmd/tui

gui: ## 构建 GUI（需要 cgo + C 编译器；Windows 需 MinGW-w64，Linux 需 xorg-dev）
	cd cmd/gui && CGO_ENABLED=1 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o ../../steggo-gui .

termux: ## 构建 Android Termux ARM64 包（移动端离线解密）
	@mkdir -p dist/termux
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/termux/steggo ./cmd/cli
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/termux/steggo-tui ./cmd/tui
	@echo "  dist/termux/{steggo,steggo-tui}（在 Termux 中 chmod +x 后运行）"

wasm: ## 构建 WASM 浏览器离线审计
	@mkdir -p dist
	GOOS=js GOARCH=wasm CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/steggo.wasm ./wasm
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" dist/
	@echo "  dist/steggo.wasm + wasm_exec.js（配合 wasm/index.html 使用）"

cross: ## 交叉构建 dist/（Linux/macOS/Windows × amd64/arm64）
	@mkdir -p dist
	@set -e; for os in linux darwin windows; do \
		for arch in amd64 arm64; do \
			ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
			GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath \
				-ldflags "$(LDFLAGS) -X main.version=$(VERSION)" \
				-o dist/steggo-$$os-$$arch$$ext ./cmd/cli; \
			GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath \
				-ldflags "$(LDFLAGS)" \
				-o dist/steggo-tui-$$os-$$arch$$ext ./cmd/tui; \
			echo "  dist/steggo{-tui,}-$$os-$$arch$$ext"; \
		done; \
	done

docker: ## 插件化镜像：make docker TARGET=all|cli|tui|gui
	docker build --build-arg TARGET=$(TARGET) --build-arg VERSION=$(VERSION) -t steggo:$(VERSION) .

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
	$(GO) install -trimpath -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" ./cmd/cli
