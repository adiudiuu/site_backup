// Command mcp-server 把 SiteBackup 暴露为 MCP (Model Context Protocol) 工具
//
// 用法：
//   go run ./cmd/mcp-server
//   go build -o bin/sitebackup-mcp ./cmd/mcp-server
//
// 注册到 Claude Desktop (claude_desktop_config.json)：
//   {
//     "mcpServers": {
//       "sitebackup": {
//         "command": "D:\\path\\to\\sitebackup-mcp.exe"
//       }
//     }
//   }
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"sitebackup/services"
)

func main() {
	s := server.NewMCPServer(
		"sitebackup",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// 工具 1: capture_page — 标准抓取（自取主页面）
	s.AddTool(
		mcp.NewTool("capture_page",
			mcp.WithDescription("抓取整页 HTML 及其所有子资源（CSS/JS/图片/视频/字体/preload），可剥离 5 类隐私代码（analytics/tracking/ads/GTM/malicious），返回 ZIP 路径。"),
			mcp.WithString("url",
				mcp.Required(),
				mcp.Description("目标 URL，例如 https://example.com"),
			),
			mcp.WithString("html",
				mcp.Description("可选：预取的 HTML 内容。若已用 puppeteer-mcp / firecrawl 等拿到 HTML，传此参数可跳过内部 fetch（解决反爬虫/SPA/JS 渲染问题）"),
			),
			mcp.WithObject("options",
				mcp.Description("抓取选项（可省略，使用默认值）"),
				mcp.Properties(map[string]any{
					"includeImages":     map[string]any{"type": "boolean", "default": true},
					"includeStyles":     map[string]any{"type": "boolean", "default": true},
					"includeScripts":    map[string]any{"type": "boolean", "default": true},
					"includeFonts":      map[string]any{"type": "boolean", "default": true},
					"includeVideos":     map[string]any{"type": "boolean", "default": true},
					"followRedirects":   map[string]any{"type": "boolean", "default": true},
					"removeAnalytics":   map[string]any{"type": "boolean", "default": true},
					"removeTracking":    map[string]any{"type": "boolean", "default": true},
					"removeAds":         map[string]any{"type": "boolean", "default": true},
					"removeTagManager":  map[string]any{"type": "boolean", "default": true},
					"removeMaliciousTags": map[string]any{"type": "boolean", "default": true},
					"correctFileNames":  map[string]any{"type": "boolean", "default": false},
					"timeout":           map[string]any{"type": "integer", "default": 120, "minimum": 60, "maximum": 300},
					"maxFiles":          map[string]any{"type": "integer", "default": 200, "minimum": 10, "maximum": 1000},
					"maxConcurrency":    map[string]any{"type": "integer", "default": 10, "minimum": 1, "maximum": 50},
					"forceEncoding":     map[string]any{"type": "string", "description": "UTF-8/GBK/GB18030/Big5/... 留空则自动检测"},
				}),
			),
		),
		handleCapturePage,
	)

	// 工具 2: get_capture_progress — 查询当前抓取进度
	// （简化为返回 last-known 状态；本服务一次性跑完返回，host 可不调）
	s.AddTool(
		mcp.NewTool("get_capture_progress",
			mcp.WithDescription("查询当前抓取进度（MCP 模式下，capture_page 同步返回结果；本工具保留扩展点）"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(`{"phase":"complete","note":"MCP mode is synchronous; capture_page returns when done"}`), nil
		},
	)

	// 工具 3: list_capabilities — 让 host 知道本服务能做什么
	s.AddTool(
		mcp.NewTool("list_capabilities",
			mcp.WithDescription("列出 SiteBackup MCP 服务支持的能力与典型组合用法"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			caps := map[string]any{
				"version": "1.0.0",
				"tools":   []string{"capture_page", "get_capture_progress", "list_capabilities"},
				"supports": []string{
					"HTML/CSS/JS/IMG/VIDEO/AUDIO/FONT/PRELOAD 资源",
					"懒加载图片 (data-src/data-srcset/data-original/data-lazy-src/data-bg)",
					"5 类隐私清理 (analytics/tracking/ads/GTM/malicious)",
					"多编码自动检测 (UTF-8/GBK/GB18030/Big5/...)",
					"并发下载 (默认 10，可配 1-50)",
					"重试机制 (3 次，403/access-denied 不重试)",
					"预取 HTML 模式 (与 puppeteer-mcp 组合，绕开反爬)",
				},
				"limitations": []string{
					"不执行 JS 渲染（除非用预取 HTML 模式）",
					"不支持 SPA 路由",
					"不支持登录后内容",
					"反爬虫站点需要预取 HTML 模式",
				},
				"composition_examples": []map[string]string{
					{
						"name":        "puppeteer + sitebackup 组合",
						"description": "先用 puppeteer-mcp 拿 HTML（带 JS 渲染/反爬绕过），再传给 sitebackup 做解析和打包",
						"steps": `1. puppeteer_mcp.browse({url}) → html
2. sitebackup.capture_page({url, html}) → zipPath`,
					},
					{
						"name":        "firecrawl + sitebackup 组合",
						"description": "用 firecrawl 拿清洗过的 markdown，SiteBackup 同时保留原始 HTML+资源",
						"steps": `1. firecrawl.scrape({url}) → {markdown, html}
2. sitebackup.capture_page({url, html: html}) → zipPath`,
					},
				},
			}
			b, _ := json.MarshalIndent(caps, "", "  ")
			return mcp.NewToolResultText(string(b)), nil
		},
	)

	// stdio 传输（Claude Desktop / Cursor / Continue 等都用 stdio）
	log.SetOutput(os.Stderr) // MCP 用 stdout 通信，log 必须走 stderr
	log.Println("sitebackup-mcp server starting (stdio)...")
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// handleCapturePage capture_page 工具处理函数
func handleCapturePage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url, err := req.RequireString("url")
	if err != nil {
		return mcp.NewToolResultError("url 参数必填"), nil
	}

	html := req.GetString("html", "")

	// 解析 options
	opts := services.CaptureOptions{
		IncludeImages:     true,
		IncludeStyles:     true,
		IncludeScripts:    true,
		IncludeFonts:      true,
		IncludeVideos:     true,
		FollowRedirects:   true,
		RemoveAnalytics:   true,
		RemoveTracking:    true,
		RemoveAds:         true,
		RemoveTagManager:  true,
		RemoveMaliciousTags: true,
		Timeout:           120,
		MaxFiles:          200,
		MaxConcurrency:    10,
		CreateZip:         true,
	}

	if args, ok := req.Params.Arguments.(map[string]any); ok {
		if rawOpts, ok := args["options"].(map[string]any); ok {
			applyOptions(&opts, rawOpts)
		}
	}

	resp, err := services.Capture(services.CaptureRequest{
		URL:     url,
		HTML:    html,
		Options: opts,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("抓取失败: %v", err)), nil
	}

	out, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func applyOptions(opts *services.CaptureOptions, raw map[string]any) {
	if v, ok := raw["includeImages"].(bool); ok { opts.IncludeImages = v }
	if v, ok := raw["includeStyles"].(bool); ok { opts.IncludeStyles = v }
	if v, ok := raw["includeScripts"].(bool); ok { opts.IncludeScripts = v }
	if v, ok := raw["includeFonts"].(bool); ok { opts.IncludeFonts = v }
	if v, ok := raw["includeVideos"].(bool); ok { opts.IncludeVideos = v }
	if v, ok := raw["followRedirects"].(bool); ok { opts.FollowRedirects = v }
	if v, ok := raw["removeAnalytics"].(bool); ok { opts.RemoveAnalytics = v }
	if v, ok := raw["removeTracking"].(bool); ok { opts.RemoveTracking = v }
	if v, ok := raw["removeAds"].(bool); ok { opts.RemoveAds = v }
	if v, ok := raw["removeTagManager"].(bool); ok { opts.RemoveTagManager = v }
	if v, ok := raw["removeMaliciousTags"].(bool); ok { opts.RemoveMaliciousTags = v }
	if v, ok := raw["correctFileNames"].(bool); ok { opts.CorrectFileNames = v }
	if v, ok := raw["timeout"].(float64); ok && v > 0 { opts.Timeout = int(v) }
	if v, ok := raw["maxFiles"].(float64); ok && v > 0 { opts.MaxFiles = int(v) }
	if v, ok := raw["maxConcurrency"].(float64); ok && v > 0 { opts.MaxConcurrency = int(v) }
	if v, ok := raw["forceEncoding"].(string); ok { opts.ForceEncoding = v }
}
