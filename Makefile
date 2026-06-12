# Wails项目构建Makefile

.PHONY: run build-win build-mac build-mcp build-mcp-win build-mcp-linux build-mcp-mac

# 开发运行
run:
	@echo "启动开发服务器..."
	wails dev

# 构建Windows版本
build-win:
	@echo "构建Windows版本..."
	wails build -platform windows/amd64 -o sitebackup-windows.exe

# 构建macOS版本 (Apple Silicon M芯片)
build-mac:
	@echo "构建macOS版本 (Apple Silicon M芯片)..."
	wails build -platform darwin/arm64 -o sitebackup-macos-arm.app
	@echo "移除隔离属性..."
	xattr -rd com.apple.quarantine build/bin/sitebackup-macos-arm.app 2>/dev/null || true

# ===== MCP Server 构建 =====
# MCP server 是一个独立的 CLI 程序，不依赖 Wails，可被 Claude Desktop / Cursor / Continue 等 MCP host 调用

build-mcp:
	@echo "构建MCP server (当前平台)..."
	mkdir -p bin
	go build -o bin/sitebackup-mcp ./cmd/mcp-server

build-mcp-win:
	@echo "构建MCP server (Windows)..."
	mkdir -p bin
	GOOS=windows GOARCH=amd64 go build -o bin/sitebackup-mcp.exe ./cmd/mcp-server

build-mcp-linux:
	@echo "构建MCP server (Linux)..."
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -o bin/sitebackup-mcp ./cmd/mcp-server

build-mcp-mac:
	@echo "构建MCP server (macOS)..."
	mkdir -p bin
	GOOS=darwin GOARCH=arm64 go build -o bin/sitebackup-mcp-mac ./cmd/mcp-server
	GOOS=darwin GOARCH=amd64 go build -o bin/sitebackup-mcp-mac-intel ./cmd/mcp-server

# 跑 MCP server（开发模式，用于手动测试）
mcp-test:
	@echo "启动MCP server (stdio)..."
	go run ./cmd/mcp-server
