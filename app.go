package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sitebackup/services"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx                context.Context
	pageCaptureService *services.PageCaptureService
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		pageCaptureService: services.NewPageCaptureService(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// beforeClose is called when the application is about to quit,
// either by clicking the window close button or calling runtime.Quit.
// Returning true will cause the application to continue, false will continue shutdown as normal.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	// 检查是否有正在进行的页面抓取
	progress := a.pageCaptureService.GetCurrentProgress()

	// 如果正在备份，询问用户是否确定要退出
	if progress.Phase == "analyzing" || progress.Phase == "downloading" || progress.Phase == "saving" {
		log.Printf("Application closing while capture in progress, phase: %s", progress.Phase)

		// 使用 Wails 的对话框询问用户
		selection, err := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
			Type:          wailsruntime.QuestionDialog,
			Title:         "确认退出",
			Message:       "正在进行网页备份，确定要退出吗？\n\n退出将停止当前的备份任务。",
			Buttons:       []string{"继续备份", "停止并退出"},
			DefaultButton: "继续备份",
			CancelButton:  "继续备份",
		})

		if err != nil {
			log.Printf("Error showing dialog: %v", err)
			return false // 出错时允许退出
		}

		// 如果用户选择继续备份，阻止关闭
		if selection == "继续备份" {
			return true // 阻止关闭
		}

		// 用户选择退出，停止备份
		log.Printf("User chose to exit, stopping capture")
		a.pageCaptureService.StopCapture()
	}

	return false // 允许关闭
}

// ApiResponse 通用API响应结构
type ApiResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// CapturePage 抓取页面内容
func (a *App) CapturePage(targetURL, optionsJson string) string {
	log.Printf("CapturePage called with URL: %s, options: %s", targetURL, optionsJson)

	// 验证URL
	if targetURL == "" || strings.TrimSpace(targetURL) == "" {
		response := ApiResponse{Code: 400, Msg: "URL不能为空"}
		result, _ := json.Marshal(response)
		return string(result)
	}

	// 解析选项
	var options services.CaptureOptions
	if optionsJson != "" {
		if err := json.Unmarshal([]byte(optionsJson), &options); err != nil {
			log.Printf("Failed to unmarshal options: %v", err)
			// 使用默认选项
			options = services.CaptureOptions{
				IncludeImages:   true,
				IncludeStyles:   true,
				IncludeScripts:  true,
				FollowRedirects: true,
				Timeout:         60,
				CreateZip:       true,
				MaxFiles:        200,
			}
		}
	} else {
		// 默认选项
		options = services.CaptureOptions{
			IncludeImages:   true,
			IncludeStyles:   true,
			IncludeScripts:  true,
			FollowRedirects: true,
			Timeout:         60,
			CreateZip:       true,
			MaxFiles:        200,
		}
	}

	// 设置超时范围
	if options.Timeout < 60 {
		options.Timeout = 60
	}
	if options.Timeout > 300 {
		options.Timeout = 300
	}

	// 设置文件数量范围
	if options.MaxFiles < 200 {
		options.MaxFiles = 200
	}
	if options.MaxFiles > 1000 {
		options.MaxFiles = 1000
	}

	// 强制启用ZIP创建
	options.CreateZip = true

	log.Printf("Using options: %+v", options)

	// 设置进度回调 - 使用Wails事件系统发送到前端
	a.pageCaptureService.SetProgressCallback(func(progress services.ProgressInfo) {
		log.Printf("Progress: %s - %d/%d files, current: %s",
			progress.Phase, progress.CompletedFiles, progress.TotalFiles, progress.CurrentFile)

		// 发送进度事件到前端
		wailsruntime.EventsEmit(a.ctx, "capture_progress", progress)

		// 打印文件列表状态（调试用）
		if len(progress.FileList) > 0 {
			completed := 0
			downloading := 0
			pending := 0
			failed := 0

			for _, file := range progress.FileList {
				switch file.Status {
				case "completed":
					completed++
				case "downloading":
					downloading++
				case "pending":
					pending++
				case "failed":
					failed++
				}
			}

			log.Printf("File status: completed=%d, downloading=%d, pending=%d, failed=%d",
				completed, downloading, pending, failed)
		}
	})

	// 执行页面抓取
	result, err := a.pageCaptureService.CapturePage(targetURL, options)
	if err != nil {
		log.Printf("Failed to capture page: %v", err)

		// 根据错误类型返回更具体的错误信息
		var errorMsg string
		if strings.Contains(err.Error(), "HTTP错误") {
			errorMsg = fmt.Sprintf("无法访问页面: %v", err)
		} else if strings.Contains(err.Error(), "不支持的内容类型") {
			errorMsg = fmt.Sprintf("页面类型不支持: %v", err)
		} else if strings.Contains(err.Error(), "请求失败") {
			errorMsg = fmt.Sprintf("网络请求失败: %v", err)
		} else {
			errorMsg = fmt.Sprintf("页面抓取失败: %v", err)
		}

		response := ApiResponse{Code: 500, Msg: errorMsg}
		responseResult, _ := json.Marshal(response)
		return string(responseResult)
	}

	// 检查结果是否有效
	if result == nil {
		log.Printf("Page capture returned nil result")
		response := ApiResponse{Code: 500, Msg: "页面抓取返回空结果"}
		responseResult, _ := json.Marshal(response)
		return string(responseResult)
	}

	log.Printf("Page captured successfully: status=%d, contentLength=%d, duration=%dms",
		result.StatusCode, result.ContentLength, result.Duration)

	// 检查内容是否为空或乱码
	if result.Content == "" {
		log.Printf("Warning: Captured content is empty")
		// 不设置默认内容，让前端知道内容为空但抓取成功
	}

	response := ApiResponse{Code: 200, Msg: "页面抓取成功", Data: result}
	responseResult, _ := json.Marshal(response)
	return string(responseResult)
}

// GetCaptureProgress 获取页面抓取进度
func (a *App) GetCaptureProgress() string {
	// 从页面抓取服务获取当前进度
	progress := a.pageCaptureService.GetCurrentProgress()

	response := ApiResponse{
		Code: 200,
		Msg:  "success",
		Data: progress,
	}

	result, _ := json.Marshal(response)
	return string(result)
}

// DownloadFile 下载文件并返回API响应格式
func (a *App) DownloadFile(filePath string) string {
	log.Printf("DownloadFile called with path: %s", filePath)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		response := ApiResponse{Code: 404, Msg: fmt.Sprintf("文件不存在: %s", filePath)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("读取文件失败: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	log.Printf("File downloaded successfully, size: %d bytes", len(content))

	// 使用Base64编码传输二进制数据，确保数据完整性
	base64Data := base64.StdEncoding.EncodeToString(content)

	// 返回成功响应，包含Base64编码的文件内容
	response := ApiResponse{Code: 200, Msg: "文件下载成功", Data: base64Data}
	result, _ := json.Marshal(response)
	return string(result)
}

// SelectDirectory 选择目录
func (a *App) SelectDirectory() string {
	log.Printf("SelectDirectory called")

	// 使用Wails的目录选择对话框
	selectedDir, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "选择保存目录",
	})

	if err != nil {
		log.Printf("Failed to open directory dialog: %v", err)
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("打开目录选择对话框失败: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	if selectedDir == "" {
		// 用户取消选择
		response := ApiResponse{Code: 400, Msg: "用户取消选择目录"}
		result, _ := json.Marshal(response)
		return string(result)
	}

	log.Printf("Directory selected: %s", selectedDir)
	response := ApiResponse{Code: 200, Msg: "目录选择成功", Data: selectedDir}
	result, _ := json.Marshal(response)
	return string(result)
}

// SaveZipToDirectory 保存ZIP文件到指定目录
func (a *App) SaveZipToDirectory(sourcePath, targetDirectory, fileName string) string {
	log.Printf("SaveZipToDirectory called: source=%s, target=%s, fileName=%s", sourcePath, targetDirectory, fileName)

	// 检查源文件是否存在
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		response := ApiResponse{Code: 404, Msg: fmt.Sprintf("源文件不存在: %s", sourcePath)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	// 检查目标目录是否存在
	if _, err := os.Stat(targetDirectory); os.IsNotExist(err) {
		response := ApiResponse{Code: 404, Msg: fmt.Sprintf("目标目录不存在: %s", targetDirectory)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	// 构建目标文件路径
	targetPath := filepath.Join(targetDirectory, fileName)

	// 复制文件
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("打开源文件失败: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}
	defer sourceFile.Close()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("创建目标文件失败: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, sourceFile)
	if err != nil {
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("复制文件失败: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	log.Printf("File saved successfully to: %s", targetPath)
	response := ApiResponse{Code: 200, Msg: "文件保存成功", Data: targetPath}
	result, _ := json.Marshal(response)
	return string(result)
}

// StopCapture 停止页面抓取
func (a *App) StopCapture() string {
	log.Printf("StopCapture called")

	// 调用页面抓取服务的停止方法
	err := a.pageCaptureService.StopCapture()
	if err != nil {
		log.Printf("Failed to stop capture: %v", err)
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("停止备份失败: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	log.Printf("Capture stopped successfully")
	response := ApiResponse{Code: 200, Msg: "备份已停止"}
	result, _ := json.Marshal(response)
	return string(result)
}

// OpenUrl 在默认浏览器中打开URL
func (a *App) OpenUrl(urlStr string) string {
	log.Printf("OpenUrl called with URL: %s", urlStr)

	// 验证URL格式
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		response := ApiResponse{Code: 400, Msg: fmt.Sprintf("无效的URL格式: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	// 确保URL有协议
	if parsedURL.Scheme == "" {
		urlStr = "https://" + urlStr
	}

	// 根据操作系统选择打开命令
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", urlStr)
	case "darwin": // macOS
		cmd = exec.Command("open", urlStr)
	case "linux":
		cmd = exec.Command("xdg-open", urlStr)
	default:
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("不支持的操作系统: %s", runtime.GOOS)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	// 执行命令
	err = cmd.Start()
	if err != nil {
		log.Printf("Failed to open URL: %v", err)
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("打开URL失败: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	log.Printf("URL opened successfully: %s", urlStr)
	response := ApiResponse{Code: 200, Msg: "链接已在默认浏览器中打开"}
	result, _ := json.Marshal(response)
	return string(result)
}

// OpenDirectory 打开指定目录
func (a *App) OpenDirectory(directoryPath string) string {
	log.Printf("OpenDirectory called with path: %s", directoryPath)
	log.Printf("Current OS: %s", runtime.GOOS)

	// 清理和标准化路径
	cleanPath := filepath.Clean(directoryPath)
	log.Printf("Cleaned path: %s", cleanPath)

	// 检查目录是否存在
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		response := ApiResponse{Code: 404, Msg: fmt.Sprintf("目录不存在: %s", cleanPath)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	// 根据操作系统选择打开命令
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", cleanPath)
	case "darwin": // macOS
		cmd = exec.Command("open", cleanPath)
	case "linux":
		cmd = exec.Command("xdg-open", cleanPath)
	default:
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("不支持的操作系统: %s", runtime.GOOS)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	// 执行命令
	err := cmd.Start()
	if err != nil {
		log.Printf("Failed to open directory: %v", err)
		response := ApiResponse{Code: 500, Msg: fmt.Sprintf("打开目录失败: %v", err)}
		result, _ := json.Marshal(response)
		return string(result)
	}

	log.Printf("Directory opened successfully: %s", cleanPath)
	response := ApiResponse{Code: 200, Msg: "目录已打开"}
	result, _ := json.Marshal(response)
	return string(result)
}
