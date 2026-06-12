package services

import "fmt"

// CaptureRequest 统一的抓取请求入口（用于 MCP / CLI / 测试等非 Wails 场景）
type CaptureRequest struct {
	URL     string         // 目标 URL
	HTML    string         // 预取 HTML（可选；非空时跳过内部 fetch）
	Options CaptureOptions // 抓取选项
}

// CaptureResponse 统一的抓取响应（只暴露核心字段，避免泄漏内部细节）
type CaptureResponse struct {
	URL          string   `json:"url"`
	ZipPath      string   `json:"zipPath"`
	ZipSize      int64    `json:"zipSize"`
	FilesCount   int      `json:"filesCount"`
	SuccessCount int      `json:"successCount"`
	FailedCount  int      `json:"failedCount"`
	DurationMs   int64    `json:"durationMs"`
	StatusCode   int      `json:"statusCode"`
	Downloaded   []string `json:"downloaded"`
}

// Capture 统一抓取入口：自动判断是否走预取 HTML 路径
//
// 用法 1（标准）：services.Capture(services.CaptureRequest{URL: "...", Options: ...})
// 用法 2（MCP 组合）：services.Capture(services.CaptureRequest{
//     URL:  "...",
//     HTML: puppeteerHTML,  // 来自 host 的 puppeteer-mcp
//     Options: ...,
// })
func Capture(req CaptureRequest) (*CaptureResponse, error) {
	if req.URL == "" {
		return nil, fmt.Errorf("URL 不能为空")
	}
	if req.Options.MaxFiles == 0 {
		req.Options.MaxFiles = 200
	}
	if req.Options.Timeout == 0 {
		req.Options.Timeout = 120
	}
	if req.Options.MaxConcurrency == 0 {
		req.Options.MaxConcurrency = 10
	}

	svc := NewPageCaptureService()
	var (
		result *CaptureResult
		err    error
	)

	if req.HTML != "" {
		// 走预取 HTML 路径（典型场景：MCP host 用 puppeteer 拿 HTML 后调这里）
		result, err = svc.CapturePageWithHTML(req.URL, req.HTML, req.Options)
	} else {
		// 标准路径：自己 fetch
		result, err = svc.CapturePage(req.URL, req.Options)
	}

	if err != nil {
		return nil, err
	}

	return &CaptureResponse{
		URL:          req.URL,
		ZipPath:      result.ZipPath,
		ZipSize:      result.ZipSize,
		FilesCount:   int(result.FilesCount),
		SuccessCount: result.SuccessCount,
		FailedCount:  result.FailedCount,
		DurationMs:   result.Duration,
		StatusCode:   result.StatusCode,
		Downloaded:   result.DownloadedFiles,
	}, nil
}
