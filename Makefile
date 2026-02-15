WAILS_DIR  := kiro-launcher
WAILS      := $(shell which wails 2>/dev/null || echo "$(HOME)/go/bin/wails")
PNPM       := pnpm

.PHONY: dev build build-mac build-mac-x64 build-win clean install help

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "  dev            开发模式 (wails dev, 热重载)"
	@echo "  build          构建当前平台 release"
	@echo "  build-mac      构建 macOS arm64 (Apple Silicon)"
	@echo "  build-mac-x64  构建 macOS x86_64 (Intel)"
	@echo "  build-win      构建 Windows x86_64"
	@echo "  install        安装前端依赖"
	@echo "  clean          清理构建产物"

install:
	cd $(WAILS_DIR) && $(PNPM) install

dev: install
	cd $(WAILS_DIR) && $(WAILS) dev

build: install
	cd $(WAILS_DIR) && $(WAILS) build -clean
	@echo "✅ 构建完成: $(WAILS_DIR)/build/bin/"

build-mac: install
	bash build.sh mac-arm64

build-mac-x64: install
	bash build.sh mac-x64

build-win: install
	bash build.sh win

clean:
	rm -rf $(WAILS_DIR)/build/bin $(WAILS_DIR)/dist release
	@echo "✅ 已清理"
