# Wails项目构建Makefile

.PHONY: run build-win build-mac

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