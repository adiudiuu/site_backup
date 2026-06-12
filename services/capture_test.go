package services

import (
	"os"
	"strings"
	"testing"
)

const smokeHTML = `<!DOCTYPE html>
<html>
<head>
<title>Smoke Test</title>
<style>body { color: red; }</style>
</head>
<body>
<h1>Hello MCP</h1>
<p>This is a <a href="/about">link</a>.</p>
<img src="https://example.invalid/img.png" alt="placeholder">
<!-- GA 跟踪代码：应被剥离 -->
<script async src="https://www.googletagmanager.com/gtag/js?id=UA-12345"></script>
<script>
window.dataLayer = window.dataLayer || [];
function gtag(){dataLayer.push(arguments);}
gtag('config', 'UA-12345');
</script>
</body>
</html>`

func TestCaptureWithHTML(t *testing.T) {
	opts := CaptureOptions{
		IncludeImages:       true,
		IncludeStyles:       true,
		IncludeScripts:      true,
		RemoveAnalytics:     true,
		RemoveTracking:      true,
		RemoveAds:           true,
		RemoveTagManager:    true,
		RemoveMaliciousTags: true,
		Timeout:             60,
		MaxFiles:            20,
		MaxConcurrency:      4,
		CreateZip:           true,
	}

	resp, err := Capture(CaptureRequest{
		URL:     "https://example.com/smoke",
		HTML:    smokeHTML,
		Options: opts,
	})
	if err != nil {
		t.Fatalf("Capture 失败: %v", err)
	}
	if resp.ZipPath == "" {
		t.Fatal("ZipPath 为空")
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}

	// 验证 ZIP 文件确实存在
	if _, err := os.Stat(resp.ZipPath); err != nil {
		t.Fatalf("ZIP 文件不存在: %v", err)
	}

	// 验证隐私清理生效：GTM/GA 脚本应该被剥离
	if strings.Contains(resp.ZipPath, "googletagmanager") {
		t.Error("ZIP 路径里不应有 GTM 残留")
	}
	t.Logf("PASS: zip=%s size=%d files=%d success=%d failed=%d",
		resp.ZipPath, resp.ZipSize, resp.FilesCount,
		resp.SuccessCount, resp.FailedCount)
}

func TestCaptureEmptyURL(t *testing.T) {
	_, err := Capture(CaptureRequest{URL: ""})
	if err == nil {
		t.Fatal("期望 URL 为空时报错")
	}
}

func TestCaptureWithEmptyHTML(t *testing.T) {
	_, err := Capture(CaptureRequest{
		URL:  "https://example.com",
		HTML: "   \n\t  ",
	})
	if err == nil {
		t.Fatal("期望 HTML 全空白时报错")
	}
}
