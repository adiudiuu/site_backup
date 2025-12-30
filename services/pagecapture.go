package services

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// PageCaptureService 页面抓取服务
type PageCaptureService struct {
	client           *http.Client
	baseURL          *url.URL
	resources        map[string]*ResourceInfo
	tempDir          string
	maxFiles         int
	fileCount        int
	mutex            sync.RWMutex // 保护共享资源的互斥锁
	debug            bool         // 调试模式
	progressCallback ProgressCallback
	progressInfo     ProgressInfo
	progressMutex    sync.RWMutex
	stopRequested    bool               // 停止请求标志
	currentOptions   CaptureOptions     // 当前的抓取选项
	stopMutex        sync.RWMutex       // 保护停止标志的互斥锁
	cancelFunc       context.CancelFunc // 用于取消操作的函数
	ctx              context.Context    // 上下文，用于协程间通信
}

// NewPageCaptureService 创建新的页面抓取服务
func NewPageCaptureService() *PageCaptureService {
	// 创建自定义的HTTP传输
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true, // 禁用自动压缩处理，我们手动处理
	}

	return &PageCaptureService{
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// 允许最多10次重定向
				if len(via) >= 10 {
					return fmt.Errorf("重定向次数过多")
				}

				// 添加调试信息
				if len(via) > 0 {
					fmt.Printf("重定向检测: 从 %s 重定向到 %s (第 %d 次重定向)\n",
						via[len(via)-1].URL.String(), req.URL.String(), len(via))
				}

				// 保持请求头
				for key, val := range via[0].Header {
					req.Header[key] = val
				}
				return nil
			},
		},
		resources: make(map[string]*ResourceInfo),
		debug:     false, // 默认关闭调试
		progressInfo: ProgressInfo{
			Phase:          "idle",
			TotalFiles:     0,
			CompletedFiles: 0,
			CurrentFile:    "",
			FileProgress:   0,
			FileList:       make([]FileInfo, 0),
		},
	}
}

// SetDebug 设置调试模式
func (s *PageCaptureService) SetDebug(debug bool) {
	s.debug = debug
}

// SetProgressCallback 设置进度回调
func (s *PageCaptureService) SetProgressCallback(callback ProgressCallback) {
	s.progressCallback = callback
}

// GetCurrentProgress 获取当前进度信息
func (s *PageCaptureService) GetCurrentProgress() ProgressInfo {
	s.progressMutex.RLock()
	defer s.progressMutex.RUnlock()

	s.debugPrintf("GetCurrentProgress调用: Phase=%s, TotalFiles=%d, CompletedFiles=%d, FileListLen=%d\n",
		s.progressInfo.Phase, s.progressInfo.TotalFiles, s.progressInfo.CompletedFiles, len(s.progressInfo.FileList))

	// 返回进度信息的副本
	progress := ProgressInfo{
		Phase:          s.progressInfo.Phase,
		TotalFiles:     s.progressInfo.TotalFiles,
		CompletedFiles: s.progressInfo.CompletedFiles,
		CurrentFile:    s.progressInfo.CurrentFile,
		FileProgress:   s.progressInfo.FileProgress,
		FileList:       make([]FileInfo, len(s.progressInfo.FileList)),
	}

	// 复制文件列表
	copy(progress.FileList, s.progressInfo.FileList)

	// 打印前几个文件的状态
	for i, file := range progress.FileList {
		if i < 3 {
			s.debugPrintf("文件 %d: %s - %s\n", i, file.Name, file.Status)
		}
	}

	return progress
}

// updateProgress 更新进度信息 - 改进版本
func (s *PageCaptureService) updateProgress(phase string, currentFile string, fileProgress int) {
	s.progressMutex.Lock()
	defer s.progressMutex.Unlock()

	s.progressInfo.Phase = phase
	s.progressInfo.CurrentFile = currentFile
	s.progressInfo.FileProgress = fileProgress

	s.debugPrintf("进度更新: Phase=%s, CurrentFile=%s, FileProgress=%d%%\n", phase, currentFile, fileProgress)

	if s.progressCallback != nil {
		s.progressCallback(s.progressInfo)
	}
}

// updateFileStatus 更新文件状态 - 线程安全版本
func (s *PageCaptureService) updateFileStatus(url, status string, progress int) {
	s.progressMutex.Lock()
	defer s.progressMutex.Unlock()

	found := false
	for i := range s.progressInfo.FileList {
		if s.progressInfo.FileList[i].URL == url {
			oldStatus := s.progressInfo.FileList[i].Status
			s.progressInfo.FileList[i].Status = status
			s.progressInfo.FileList[i].Progress = progress
			s.debugPrintf("更新文件状态: %s %s -> %s (进度: %d%%)\n", s.progressInfo.FileList[i].Name, oldStatus, status, progress)
			found = true
			break
		}
	}

	if !found {
		s.debugPrintf("警告: 未找到要更新的文件 URL: %s\n", url)
		return // 如果找不到文件，直接返回，不更新统计
	}

	// 重新计算完成文件数 - 确保准确性
	completed := 0
	failed := 0
	downloading := 0
	pending := 0

	for _, file := range s.progressInfo.FileList {
		switch file.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		case "downloading":
			downloading++
		case "pending":
			pending++
		}
	}

	s.progressInfo.CompletedFiles = completed
	s.debugPrintf("状态统计更新: 完成=%d, 失败=%d, 下载中=%d, 等待=%d, 总数=%d\n",
		completed, failed, downloading, pending, len(s.progressInfo.FileList))

	// 更新阶段状态
	if completed+failed == len(s.progressInfo.FileList) && len(s.progressInfo.FileList) > 0 {
		if s.progressInfo.Phase == "downloading" {
			s.progressInfo.Phase = "saving"
			s.progressInfo.CurrentFile = "保存文件中..."
			s.debugPrintf("所有文件下载完成，切换到保存阶段\n")
		}
	}

	// 触发进度回调
	if s.progressCallback != nil {
		s.progressCallback(s.progressInfo)
	}
}

// updateFileSize 更新文件大小
func (s *PageCaptureService) updateFileSize(url string, size int64) {
	s.progressMutex.Lock()
	defer s.progressMutex.Unlock()

	formattedSize := s.formatFileSize(size)
	s.debugPrintf("更新文件大小: %s -> %d 字节 -> %s\n", url, size, formattedSize)

	for i := range s.progressInfo.FileList {
		if s.progressInfo.FileList[i].URL == url {
			s.progressInfo.FileList[i].Size = formattedSize
			s.progressInfo.FileList[i].TotalSize = size
			// 下载完成时，设置已下载大小等于总大小
			s.progressInfo.FileList[i].DownloadedSize = size
			s.debugPrintf("文件大小已更新: %s = %s (总大小: %d 字节, 已下载: %d 字节)\n",
				s.progressInfo.FileList[i].Name, formattedSize, size, size)
			break
		}
	}

	if s.progressCallback != nil {
		s.progressCallback(s.progressInfo)
	}
}

// updateFileDownloadProgress 更新文件下载进度（包括已下载大小）
func (s *PageCaptureService) updateFileDownloadProgress(url string, downloadedSize, totalSize int64, status string, progress int) {
	s.progressMutex.Lock()
	defer s.progressMutex.Unlock()

	for i := range s.progressInfo.FileList {
		if s.progressInfo.FileList[i].URL == url {
			oldStatus := s.progressInfo.FileList[i].Status
			s.progressInfo.FileList[i].Status = status
			s.progressInfo.FileList[i].Progress = progress
			s.progressInfo.FileList[i].DownloadedSize = downloadedSize
			if totalSize > 0 {
				s.progressInfo.FileList[i].TotalSize = totalSize
			}
			s.debugPrintf("更新下载进度: %s %s -> %s (进度: %d%%, 已下载: %d/%d 字节)\n",
				s.progressInfo.FileList[i].Name, oldStatus, status, progress, downloadedSize, totalSize)
			break
		}
	}

	// 更新统计信息（重用现有逻辑）
	completed := 0
	failed := 0
	downloading := 0
	pending := 0

	for _, file := range s.progressInfo.FileList {
		switch file.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		case "downloading":
			downloading++
		default:
			pending++
		}
	}

	s.progressInfo.CompletedFiles = completed

	if s.progressCallback != nil {
		s.progressCallback(s.progressInfo)
	}
}

// formatFileSize 格式化文件大小
func (s *PageCaptureService) formatFileSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	sizes := []string{"B", "KB", "MB", "GB", "TB"}

	originalBytes := float64(bytes)
	exp := 0

	// 找到合适的单位
	for originalBytes >= unit && exp < len(sizes)-1 {
		originalBytes /= unit
		exp++
	}

	// 防护性检查
	if exp >= len(sizes) {
		exp = len(sizes) - 1
	}

	result := fmt.Sprintf("%.1f %s", originalBytes, sizes[exp])

	// 调试信息 - 添加防护性检查
	if s != nil {
		s.debugPrintf("格式化文件大小: 原始字节数 -> %s (exp=%d)\n", result, exp)
	}

	return result
}

// resetState 重置所有状态，确保每次下载都是全新开始
func (s *PageCaptureService) resetState() {
	s.mutex.Lock()
	s.progressMutex.Lock()
	defer s.mutex.Unlock()
	defer s.progressMutex.Unlock()

	s.debugPrintf("=== 重置所有状态 ===\n")

	// 重置停止标志
	s.resetStopFlag()

	// 重置context（如果之前有的话，先取消）
	if s.cancelFunc != nil {
		s.debugPrintf("取消之前的context\n")
		s.cancelFunc()
		// 给协程一点时间来响应取消信号
		time.Sleep(100 * time.Millisecond)
	}
	s.ctx = nil
	s.cancelFunc = nil

	// 清空资源映射
	oldResourceCount := len(s.resources)
	s.resources = make(map[string]*ResourceInfo)
	s.fileCount = 0

	// 重置进度信息 - 确保完全清空
	oldFileListCount := len(s.progressInfo.FileList)
	s.progressInfo = ProgressInfo{
		Phase:          "analyzing",
		TotalFiles:     0,
		CompletedFiles: 0,
		CurrentFile:    "准备开始...",
		FileProgress:   0,
		FileList:       make([]FileInfo, 0), // 使用make确保是新的slice
	}

	// 清理临时目录（如果存在）
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
	}

	// 重置其他可能的状态
	// 注意：不重置baseURL，因为它在整个抓取过程中都需要使用
	s.tempDir = ""

	s.debugPrintf("状态重置完成: 清理了 %d 个资源, %d 个文件记录\n", oldResourceCount, oldFileListCount)
	s.debugPrintf("=== 状态重置完成 ===\n")
}

// debugPrintf 调试输出函数
func (s *PageCaptureService) debugPrintf(format string, args ...interface{}) {
	if s.debug {
		fmt.Printf(format, args...)
	}
}

// ProgressCallback 进度回调函数类型
type ProgressCallback func(progress ProgressInfo)

// ProgressInfo 进度信息
type ProgressInfo struct {
	Phase          string     `json:"phase"` // analyzing, downloading, saving, complete
	TotalFiles     int        `json:"totalFiles"`
	CompletedFiles int        `json:"completedFiles"`
	CurrentFile    string     `json:"currentFile"`
	FileProgress   int        `json:"fileProgress"` // 0-100
	FileList       []FileInfo `json:"fileList"`
}

// FileInfo 文件信息
type FileInfo struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Size           string `json:"size"`           // 格式化后的大小显示
	TotalSize      int64  `json:"totalSize"`      // 总大小（字节）
	DownloadedSize int64  `json:"downloadedSize"` // 已下载大小（字节）
	Status         string `json:"status"`         // pending, downloading, completed, failed
	Progress       int    `json:"progress"`       // 0-100
	URL            string `json:"url"`
}

// CaptureOptions 抓取选项
type CaptureOptions struct {
	IncludeImages       bool   `json:"includeImages"`
	IncludeStyles       bool   `json:"includeStyles"`
	IncludeScripts      bool   `json:"includeScripts"`
	FollowRedirects     bool   `json:"followRedirects"`
	IncludeFonts        bool   `json:"includeFonts"`
	IncludeVideos       bool   `json:"includeVideos"`
	RemoveAnalytics     bool   `json:"removeAnalytics"`
	RemoveTracking      bool   `json:"removeTracking"`
	RemoveAds           bool   `json:"removeAds"`
	RemoveTagManager    bool   `json:"removeTagManager"`
	RemoveMaliciousTags bool   `json:"removeMaliciousTags"`
	CorrectFileNames    bool   `json:"correctFileNames"`
	Timeout             int    `json:"timeout"`
	CreateZip           bool   `json:"createZip"`
	MaxFiles            int    `json:"maxFiles"`
	MaxDepth            int    `json:"maxDepth"`
	MaxConcurrency      int    `json:"maxConcurrency"`
	ForceEncoding       string `json:"forceEncoding"`
}

// CaptureResult 抓取结果
type CaptureResult struct {
	StatusCode      int                 `json:"statusCode"`
	ContentType     string              `json:"contentType"`
	ContentLength   int64               `json:"contentLength"`
	Content         string              `json:"content"`
	Headers         map[string][]string `json:"headers"`
	Duration        int64               `json:"duration"`
	ZipPath         string              `json:"zipPath,omitempty"`
	ZipSize         int64               `json:"zipSize,omitempty"`
	FilesCount      int                 `json:"filesCount,omitempty"`
	DownloadedFiles []string            `json:"downloadedFiles,omitempty"`
	FileDetails     []FileInfo          `json:"fileDetails,omitempty"`
	SuccessCount    int                 `json:"successCount"`
	FailedCount     int                 `json:"failedCount"`
}

// ResourceInfo 资源信息
type ResourceInfo struct {
	URL       string
	LocalPath string
	Type      string
	Content   []byte
}

// DownloadTask 下载任务
type DownloadTask struct {
	URL          string
	ResourceType string
	Element      *goquery.Selection
	AttrName     string
}

// DownloadResult 下载结果
type DownloadResult struct {
	Task      DownloadTask
	LocalPath string
	Success   bool
	Error     error
}

// CapturePage 抓取页面内容
func (s *PageCaptureService) CapturePage(targetURL string, options CaptureOptions) (*CaptureResult, error) {
	startTime := time.Now()

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancelFunc = cancel

	// 验证URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("无效的URL格式: %v", err)
	}
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https"
		targetURL = parsedURL.String()
		parsedURL, _ = url.Parse(targetURL)
	}

	s.baseURL = parsedURL
	s.maxFiles = options.MaxFiles
	if s.maxFiles <= 0 {
		s.maxFiles = 200
	}

	// 保存当前选项
	s.currentOptions = options

	// 临时启用调试模式来诊断问题
	s.debug = true

	// 重置所有状态 - 确保每次下载都是全新开始
	s.resetState()

	// 立即更新初始进度状态
	s.updateProgress("analyzing", "开始分析页面...", 0)

	// 设置超时 - 对于大文件需要更长时间
	timeout := options.Timeout
	if timeout < 120 {
		timeout = 120 // 最少2分钟
	}
	s.client.Timeout = time.Duration(timeout) * time.Second
	s.debugPrintf("设置HTTP超时: %d 秒\n", timeout)

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "page_capture_*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %v", err)
	}
	s.tempDir = tempDir
	defer os.RemoveAll(tempDir)

	// 下载主页面（带重试机制）
	htmlContent, resp, err := s.downloadPageWithRetryAndEncoding(targetURL, 3, options.ForceEncoding)
	if err != nil {
		return nil, fmt.Errorf("下载主页面失败: %v", err)
	}

	// 检查是否发生了重定向，如果是则更新 baseURL
	finalURL := resp.Request.URL
	if finalURL.String() != targetURL {
		s.debugPrintf("检测到重定向: %s -> %s\n", targetURL, finalURL.String())
		s.baseURL = finalURL
		s.debugPrintf("已更新 baseURL 为: %s\n", s.baseURL.String())
	}

	// 解析HTML并下载资源
	s.debugPrintf("开始处理HTML和下载资源...\n")
	modifiedHTML, err := s.processHTMLAndDownloadResources(htmlContent, options)
	if err != nil {
		s.debugPrintf("处理HTML失败: %v\n", err)
		return nil, fmt.Errorf("处理HTML失败: %v", err)
	}
	s.debugPrintf("HTML处理完成\n")

	// 保存文件
	s.debugPrintf("=== 开始保存所有文件 ===\n")
	s.updateProgress("saving", "正在保存文件...", 85)
	err = s.saveAllFiles(modifiedHTML)
	if err != nil {
		s.debugPrintf("保存文件失败: %v\n", err)
		return nil, fmt.Errorf("保存文件失败: %v", err)
	}
	s.debugPrintf("所有文件保存完成\n")

	// 更新进度：保存文件
	s.updateProgress("saving", "保存文件中...", 90)

	// 创建ZIP
	s.debugPrintf("=== 开始创建ZIP文件 ===\n")
	s.updateProgress("saving", "正在创建ZIP文件...", 90)
	zipPath, zipSize, err := s.createZipFile()
	if err != nil {
		s.debugPrintf("创建ZIP失败: %v\n", err)
		return nil, fmt.Errorf("创建ZIP失败: %v", err)
	}
	s.debugPrintf("ZIP文件创建完成: %s (大小: %.2f MB)\n", zipPath, float64(zipSize)/(1024*1024))

	// 更新进度：完成
	s.debugPrintf("=== 备份流程完成 ===\n")
	s.updateProgress("complete", "备份完成", 100)

	// 构建详细的文件信息
	s.progressMutex.RLock()
	fileDetails := make([]FileInfo, len(s.progressInfo.FileList))
	copy(fileDetails, s.progressInfo.FileList)
	successCount := s.progressInfo.CompletedFiles
	failedCount := len(s.progressInfo.FileList) - successCount
	s.progressMutex.RUnlock()

	// 构建结果
	result := &CaptureResult{
		StatusCode:      resp.StatusCode,
		ContentType:     resp.Header.Get("Content-Type"),
		ContentLength:   int64(len(htmlContent)),
		Content:         modifiedHTML, // 添加处理后的HTML内容
		Headers:         resp.Header,
		Duration:        time.Since(startTime).Milliseconds(),
		ZipPath:         zipPath,
		ZipSize:         zipSize,
		FilesCount:      len(s.resources) + 1,
		DownloadedFiles: s.getFileList(),
		FileDetails:     fileDetails,
		SuccessCount:    successCount,
		FailedCount:     failedCount,
	}

	return result, nil
}

// downloadPage 下载页面
func (s *PageCaptureService) downloadPage(targetURL string) (string, *http.Response, error) {
	// 创建请求
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置更完整的请求头来模拟真实浏览器
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	// 如果URL包含Referer信息，设置Referer头
	if parsedURL, err := url.Parse(targetURL); err == nil {
		if parsedURL.Host != "" {
			req.Header.Set("Referer", fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host))
		}
	}

	// 执行请求
	resp, err := s.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 添加调试信息：显示最终请求的URL和状态码
	s.debugPrintf("HTTP请求完成: 原始URL=%s, 最终URL=%s, 状态码=%d\n",
		targetURL, resp.Request.URL.String(), resp.StatusCode)

	// 检查状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", resp, fmt.Errorf("HTTP错误: %d %s", resp.StatusCode, resp.Status)
	}

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp, fmt.Errorf("读取响应失败: %v", err)
	}

	// 检查内容是否为空
	if len(body) == 0 {
		return "", resp, fmt.Errorf("响应内容为空")
	}

	// 使用简化的编码检测
	content := s.simpleEncodingDetection(body, resp.Header.Get("Content-Type"))

	// 检查是否返回了错误页面或重定向页面（智能检测）
	if s.isAntiCrawlerPage(content) {
		return "", resp, fmt.Errorf("网站拒绝访问，可能存在反爬虫机制")
	}

	return content, resp, nil
}

// decompressIfNeeded 检查并解压缩内容 - 修复版本
func (s *PageCaptureService) decompressIfNeeded(body []byte, contentEncoding string) ([]byte, error) {
	s.debugPrintf("Content-Encoding: %s\n", contentEncoding)
	s.debugPrintf("原始内容长度: %d 字节\n", len(body))
	if len(body) >= 4 {
		s.debugPrintf("前4字节: %v (hex: %02x %02x %02x %02x)\n",
			body[:4], body[0], body[1], body[2], body[3])
	}

	// 检查 GZIP 魔数 (0x1f 0x8b)
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		s.debugPrintf("检测到GZIP魔数，开始解压缩\n")
		return s.decompressGzip(body)
	}

	// 如果Content-Encoding明确指示了gzip，即使没有魔数也尝试解压
	if strings.Contains(strings.ToLower(contentEncoding), "gzip") {
		s.debugPrintf("Content-Encoding指示GZIP，尝试强制解压缩\n")
		if decompressed, err := s.decompressGzip(body); err == nil {
			return decompressed, nil
		} else {
			s.debugPrintf("强制GZIP解压缩失败: %v\n", err)
		}
	}

	// 检查 Deflate 压缩
	if strings.Contains(strings.ToLower(contentEncoding), "deflate") {
		s.debugPrintf("Content-Encoding指示Deflate，尝试解压缩\n")
		// TODO: 实现deflate解压缩
	}

	// 检查 Brotli 压缩
	if strings.Contains(strings.ToLower(contentEncoding), "br") {
		s.debugPrintf("Content-Encoding指示Brotli，但暂不支持，返回原始内容\n")
		// Brotli压缩暂不支持，建议不请求br编码
		return body, fmt.Errorf("不支持Brotli压缩，请移除br编码请求")
	}

	s.debugPrintf("内容未压缩或不支持的压缩格式\n")
	return body, nil
}

// decompressGzip GZIP解压缩辅助函数
func (s *PageCaptureService) decompressGzip(body []byte) ([]byte, error) {
	reader, err := gzip.NewReader(strings.NewReader(string(body)))
	if err != nil {
		s.debugPrintf("创建GZIP读取器失败: %v\n", err)
		return body, err
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		s.debugPrintf("GZIP解压缩失败: %v\n", err)
		return body, err
	}

	s.debugPrintf("GZIP解压缩成功，原始大小: %d，解压后大小: %d\n", len(body), len(decompressed))
	if len(decompressed) > 0 {
		s.debugPrintf("解压后前200字符: %s\n", string(decompressed[:min(len(decompressed), 200)]))
	}
	return decompressed, nil
}

// readWithProgress 带进度监控的读取函数
func (s *PageCaptureService) readWithProgress(reader io.Reader, totalSize int64, url string) ([]byte, error) {
	buffer := make([]byte, 32*1024) // 32KB缓冲区
	var result []byte
	var downloaded int64
	lastProgress := 0

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			result = append(result, buffer[:n]...)
			downloaded += int64(n)

			// 每下载5%更新一次进度
			if totalSize > 0 {
				progress := int(downloaded * 100 / totalSize)
				if progress >= lastProgress+5 || progress == 100 {
					s.debugPrintf("下载进度: %s - %d%% (%.2f/%.2f MB)\n",
						url, progress,
						float64(downloaded)/(1024*1024),
						float64(totalSize)/(1024*1024))
					s.updateFileDownloadProgress(url, downloaded, totalSize, "downloading", progress)
					lastProgress = progress
				}
			} else {
				// 如果没有总大小信息，仍然更新已下载大小
				s.updateFileDownloadProgress(url, downloaded, 0, "downloading", 0)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// downloadLargeFileInChunks 分块并发下载大文件
func (s *PageCaptureService) downloadLargeFileInChunks(url string, totalSize int64, client *http.Client) ([]byte, error) {
	const chunkSize = 2 * 1024 * 1024 // 2MB per chunk
	const maxConcurrency = 4          // 最多4个并发

	numChunks := (totalSize + chunkSize - 1) / chunkSize
	s.debugPrintf("分块下载: %d 个块，每块 %.2f MB\n", numChunks, float64(chunkSize)/(1024*1024))

	type chunkResult struct {
		index int
		data  []byte
		err   error
	}

	chunks := make([][]byte, numChunks)
	resultChan := make(chan chunkResult, numChunks)
	semaphore := make(chan struct{}, maxConcurrency)

	var wg sync.WaitGroup

	// 启动下载协程
	for i := int64(0); i < numChunks; i++ {
		wg.Add(1)
		go func(chunkIndex int64) {
			defer wg.Done()
			semaphore <- struct{}{}        // 获取信号量
			defer func() { <-semaphore }() // 释放信号量

			start := chunkIndex * chunkSize
			end := start + chunkSize - 1
			if end >= totalSize {
				end = totalSize - 1
			}

			// 检查是否被取消（安全检查，避免nil指针）
			if s.ctx != nil {
				select {
				case <-s.ctx.Done():
					resultChan <- chunkResult{int(chunkIndex), nil, fmt.Errorf("分块下载被取消: %v", s.ctx.Err())}
					return
				default:
				}
			}

			// 创建请求时使用安全的context
			ctx := s.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				resultChan <- chunkResult{int(chunkIndex), nil, err}
				return
			}

			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			req.Header.Set("Accept", "video/*,*/*;q=0.8")
			req.Header.Set("Accept-Encoding", "identity") // 禁用压缩

			resp, err := client.Do(req)
			if err != nil {
				resultChan <- chunkResult{int(chunkIndex), nil, err}
				return
			}
			defer resp.Body.Close()

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				resultChan <- chunkResult{int(chunkIndex), nil, err}
				return
			}

			s.debugPrintf("块 %d/%d 下载完成 (%.2f MB)\n", chunkIndex+1, numChunks, float64(len(data))/(1024*1024))
			resultChan <- chunkResult{int(chunkIndex), data, nil}
		}(i)
	}

	// 等待所有下载完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果并更新进度
	completedChunks := 0
	var totalDownloaded int64 = 0

	for result := range resultChan {
		if result.err != nil {
			return nil, fmt.Errorf("块 %d 下载失败: %v", result.index, result.err)
		}
		chunks[result.index] = result.data
		completedChunks++
		totalDownloaded += int64(len(result.data))

		// 更新下载进度
		progress := int(completedChunks * 100 / int(numChunks))
		s.updateFileDownloadProgress(url, totalDownloaded, totalSize, "downloading", progress)

		s.debugPrintf("分块下载进度: %d/%d 块完成 (%d%%), 已下载: %.2f MB\n",
			completedChunks, numChunks, progress, float64(totalDownloaded)/(1024*1024))
	}

	// 合并所有块
	var finalData []byte
	for _, chunk := range chunks {
		finalData = append(finalData, chunk...)
	}

	s.debugPrintf("分块下载完成，总大小: %.2f MB\n", float64(len(finalData))/(1024*1024))
	return finalData, nil
}

// getFileNameFromURL 从URL提取文件名
func (s *PageCaptureService) getFileNameFromURL(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "unknown"
	}

	fileName := path.Base(parsedURL.Path)
	if fileName == "" || fileName == "." || fileName == "/" {
		return "index"
	}

	return fileName
}

// getEncodingByName 根据编码名称获取编码
func (s *PageCaptureService) getEncodingByName(name string) encoding.Encoding {
	name = strings.ToLower(strings.TrimSpace(name))

	switch name {
	case "utf-8", "utf8":
		return unicode.UTF8
	case "gbk", "gb2312", "gb18030":
		return simplifiedchinese.GBK
	case "big5":
		return traditionalchinese.Big5
	case "shift_jis", "shift-jis", "sjis":
		return japanese.ShiftJIS
	case "euc-jp":
		return japanese.EUCJP
	case "euc-kr":
		return korean.EUCKR
	case "iso-8859-1", "latin1":
		return charmap.ISO8859_1
	case "windows-1252", "cp1252":
		return charmap.Windows1252
	default:
		return nil
	}
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// simpleEncodingDetection 简化的编码检测 - 修复版本
func (s *PageCaptureService) simpleEncodingDetection(body []byte, contentType string) string {
	s.debugPrintf("开始编码检测，Content-Type: %s\n", contentType)
	s.debugPrintf("原始内容长度: %d 字节\n", len(body))

	if len(body) == 0 {
		s.debugPrintf("内容为空，返回空字符串\n")
		return ""
	}

	// 检查是否已经是有效的UTF-8
	if utf8.Valid(body) {
		result := string(body)
		s.debugPrintf("内容是有效的UTF-8\n")

		// 检查是否看起来像HTML内容
		lowerResult := strings.ToLower(result)
		if strings.Contains(lowerResult, "<html") ||
			strings.Contains(lowerResult, "<head") ||
			strings.Contains(lowerResult, "<body") ||
			strings.Contains(lowerResult, "<!doctype") {
			s.debugPrintf("UTF-8内容包含HTML标签，使用UTF-8\n")
			s.debugPrintf("UTF-8内容前200字符: %s\n", result[:min(len(result), 200)])
			return result
		} else {
			s.debugPrintf("UTF-8内容不包含HTML标签，可能是压缩数据或其他格式\n")
			s.debugPrintf("内容前100字符: %s\n", result[:min(len(result), 100)])
			// 继续尝试其他编码
		}
	}

	s.debugPrintf("内容不是有效的UTF-8，开始编码转换\n")

	// 从Content-Type头中提取编码
	if contentType != "" {
		if strings.Contains(strings.ToLower(contentType), "charset=") {
			parts := strings.Split(strings.ToLower(contentType), "charset=")
			if len(parts) > 1 {
				charset := strings.TrimSpace(strings.Split(parts[1], ";")[0])
				s.debugPrintf("从Content-Type检测到编码: %s\n", charset)
				if encoding := s.getEncodingByName(charset); encoding != nil {
					decoder := encoding.NewDecoder()
					if result, _, err := transform.Bytes(decoder, body); err == nil {
						resultStr := string(result)
						s.debugPrintf("使用%s编码转换成功，长度: %d\n", charset, len(resultStr))
						s.debugPrintf("转换后前200字符: %s\n", resultStr[:min(len(resultStr), 200)])
						return resultStr
					} else {
						s.debugPrintf("使用%s编码转换失败: %v\n", charset, err)
					}
				}
			}
		}
	}

	// 尝试从HTML内容中检测编码（使用原始字节的字符串表示）
	bodyStr := string(body[:min(len(body), 2048)])
	if strings.Contains(strings.ToLower(bodyStr), "charset=") {
		re := regexp.MustCompile(`charset\s*=\s*["']?([^"'\s>]+)`)
		matches := re.FindStringSubmatch(strings.ToLower(bodyStr))
		if len(matches) > 1 {
			charset := matches[1]
			s.debugPrintf("从HTML meta标签检测到编码: %s\n", charset)
			if encoding := s.getEncodingByName(charset); encoding != nil {
				decoder := encoding.NewDecoder()
				if result, _, err := transform.Bytes(decoder, body); err == nil {
					resultStr := string(result)
					s.debugPrintf("使用%s编码转换成功，长度: %d\n", charset, len(resultStr))
					s.debugPrintf("转换后前200字符: %s\n", resultStr[:min(len(resultStr), 200)])
					return resultStr
				} else {
					s.debugPrintf("使用%s编码转换失败: %v\n", charset, err)
				}
			}
		}
	}

	// 尝试常见的编码
	encodings := []struct {
		name string
		enc  encoding.Encoding
	}{
		{"UTF-8", unicode.UTF8},
		{"GBK", simplifiedchinese.GBK},
		{"GB18030", simplifiedchinese.GB18030},
		{"Big5", traditionalchinese.Big5},
		{"Windows-1252", charmap.Windows1252},
		{"ISO-8859-1", charmap.ISO8859_1},
	}

	for _, item := range encodings {
		s.debugPrintf("尝试%s编码...\n", item.name)
		decoder := item.enc.NewDecoder()
		if result, _, err := transform.Bytes(decoder, body); err == nil {
			resultStr := string(result)
			s.debugPrintf("%s编码转换成功，长度: %d\n", item.name, len(resultStr))
			s.debugPrintf("转换后前200字符: %s\n", resultStr[:min(len(resultStr), 200)])

			// 检查转换后的内容是否看起来像HTML
			lowerResult := strings.ToLower(resultStr)
			if strings.Contains(lowerResult, "<html") ||
				strings.Contains(lowerResult, "<head") ||
				strings.Contains(lowerResult, "<body") ||
				strings.Contains(lowerResult, "<!doctype") {
				s.debugPrintf("使用%s编码转换成功，包含HTML标签\n", item.name)
				return resultStr
			}
		} else {
			s.debugPrintf("%s编码转换失败: %v\n", item.name, err)
		}
	}

	// 最后降级为直接字符串转换
	s.debugPrintf("所有编码转换都失败，使用原始字符串\n")
	directStr := string(body)
	s.debugPrintf("原始字符串长度: %d\n", len(directStr))
	s.debugPrintf("原始字符串前200字符: %s\n", directStr[:min(len(directStr), 200)])
	return directStr
}

// downloadPageWithRetry 带重试机制的页面下载
func (s *PageCaptureService) downloadPageWithRetry(targetURL string, maxRetries int) (string, *http.Response, error) {
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			// 重试前等待一段时间
			waitTime := time.Duration(i) * time.Second
			time.Sleep(waitTime)
		}

		content, resp, err := s.downloadPage(targetURL)
		if err == nil {
			return content, resp, nil
		}

		lastErr = err

		// 如果是某些特定错误，不进行重试
		if strings.Contains(err.Error(), "access denied") ||
			strings.Contains(err.Error(), "forbidden") ||
			strings.Contains(err.Error(), "反爬虫") {
			break
		}
	}

	return "", nil, fmt.Errorf("重试%d次后仍然失败: %v", maxRetries, lastErr)
}

// downloadPageWithRetryAndEncoding 带重试机制和编码选项的页面下载
func (s *PageCaptureService) downloadPageWithRetryAndEncoding(targetURL string, maxRetries int, forceEncoding string) (string, *http.Response, error) {
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			// 重试前等待一段时间
			waitTime := time.Duration(i) * time.Second
			time.Sleep(waitTime)
		}

		content, resp, err := s.downloadPageWithEncoding(targetURL, forceEncoding)
		if err == nil {
			return content, resp, nil
		}

		lastErr = err

		// 如果是某些特定错误，不进行重试
		if strings.Contains(err.Error(), "access denied") ||
			strings.Contains(err.Error(), "forbidden") ||
			strings.Contains(err.Error(), "反爬虫") {
			break
		}
	}

	return "", nil, fmt.Errorf("重试%d次后仍然失败: %v", maxRetries, lastErr)
}

// downloadPageWithEncoding 支持编码选项的页面下载
func (s *PageCaptureService) downloadPageWithEncoding(targetURL string, forceEncoding string) (string, *http.Response, error) {
	// 创建请求
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置请求头（与原函数相同）
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	if parsedURL, err := url.Parse(targetURL); err == nil {
		if parsedURL.Host != "" {
			req.Header.Set("Referer", fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host))
		}
	}

	// 执行请求
	resp, err := s.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", resp, fmt.Errorf("HTTP错误: %d %s", resp.StatusCode, resp.Status)
	}

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp, fmt.Errorf("读取响应失败: %v", err)
	}

	// 检查内容是否为空
	if len(body) == 0 {
		return "", resp, fmt.Errorf("响应内容为空")
	}

	// 输出响应头信息用于调试
	s.debugPrintf("响应头信息:\n")
	for key, values := range resp.Header {
		s.debugPrintf("  %s: %s\n", key, strings.Join(values, ", "))
	}

	s.debugPrintf("原始响应体长度: %d 字节\n", len(body))
	if len(body) >= 10 {
		s.debugPrintf("原始响应体前10字节: %v\n", body[:10])
		s.debugPrintf("原始响应体前10字节(hex): %02x\n", body[:10])
	}

	// 检查并解压缩内容
	body, err = s.decompressIfNeeded(body, resp.Header.Get("Content-Encoding"))
	if err != nil {
		return "", resp, fmt.Errorf("解压缩失败: %v", err)
	}

	s.debugPrintf("解压缩后内容长度: %d 字节\n", len(body))
	if len(body) >= 200 {
		s.debugPrintf("解压缩后内容前200字符: %s\n", string(body[:200]))
	}

	// 简化的编码处理
	var content string
	if forceEncoding != "" && forceEncoding != "auto" {
		// 使用指定编码
		if encoding := s.getEncodingByName(forceEncoding); encoding != nil {
			decoder := encoding.NewDecoder()
			if result, _, err := transform.Bytes(decoder, body); err == nil {
				content = string(result)
			} else {
				content = string(body) // 降级处理
			}
		} else {
			content = string(body)
		}
	} else {
		// 自动检测编码
		content = s.simpleEncodingDetection(body, resp.Header.Get("Content-Type"))
	}

	// 检查是否返回了错误页面或重定向页面（智能检测）
	if s.isAntiCrawlerPage(content) {
		return "", resp, fmt.Errorf("网站拒绝访问，可能存在反爬虫机制")
	}

	return content, resp, nil
}

// processHTMLAndDownloadResources 处理HTML并下载资源（并发版本）
func (s *PageCaptureService) processHTMLAndDownloadResources(htmlContent string, options CaptureOptions) (string, error) {
	s.debugPrintf("=== 开始处理HTML和下载资源 ===\n")
	s.debugPrintf("HTML内容长度: %d 字符\n", len(htmlContent))
	s.debugPrintf("HTML前500字符: %s\n", htmlContent[:min(len(htmlContent), 500)])

	// 更新进度
	s.updateProgress("analyzing", "解析HTML文档...", 0)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		s.debugPrintf("HTML解析失败: %v\n", err)
		return htmlContent, err
	}

	s.debugPrintf("HTML解析成功\n")

	// 验证HTML内容的有效性
	title := doc.Find("title").Text()
	s.debugPrintf("页面标题: %s\n", title)

	// 检查基本HTML结构
	htmlTag := doc.Find("html")
	headTag := doc.Find("head")
	bodyTag := doc.Find("body")
	s.debugPrintf("HTML结构检查: html标签=%d, head标签=%d, body标签=%d\n",
		htmlTag.Length(), headTag.Length(), bodyTag.Length())

	// 检查是否有基本的HTML内容
	if htmlTag.Length() == 0 && headTag.Length() == 0 && bodyTag.Length() == 0 {
		s.debugPrintf("警告: 没有找到基本的HTML结构，可能是编码问题\n")
		s.debugPrintf("原始HTML内容前1000字符: %s\n", htmlContent[:min(len(htmlContent), 1000)])
	}

	s.updateProgress("analyzing", "收集资源列表...", 10)

	// 收集所有下载任务
	var tasks []DownloadTask

	s.debugPrintf("开始收集资源任务...\n")
	s.debugPrintf("选项: IncludeStyles=%v, IncludeScripts=%v, IncludeImages=%v, IncludeVideos=%v\n",
		options.IncludeStyles, options.IncludeScripts, options.IncludeImages, options.IncludeVideos)
	s.debugPrintf("HTML文档解析成功，开始查找资源...\n")

	// 收集CSS文件下载任务
	if options.IncludeStyles {
		s.debugPrintf("收集CSS文件...\n")
		cssLinks := doc.Find("link[rel=stylesheet]")
		s.debugPrintf("找到 %d 个CSS link标签\n", cssLinks.Length())

		cssLinks.Each(func(i int, sel *goquery.Selection) {
			if href, exists := sel.Attr("href"); exists {
				// 清理URL中的空白字符
				href = strings.TrimSpace(href)
				s.debugPrintf("发现CSS: '%s'\n", href)
				absoluteURL := s.resolveURL(href)
				if absoluteURL != "" {
					s.debugPrintf("添加CSS任务: %s\n", absoluteURL)
					tasks = append(tasks, DownloadTask{
						URL:          absoluteURL,
						ResourceType: "css",
						Element:      sel,
						AttrName:     "href",
					})
				} else {
					s.debugPrintf("CSS URL解析失败: %s\n", href)
				}
			} else {
				s.debugPrintf("CSS link标签没有href属性\n")
			}
		})

		// 也检查所有link标签
		allLinks := doc.Find("link")
		s.debugPrintf("总共找到 %d 个link标签\n", allLinks.Length())
	} else {
		s.debugPrintf("跳过CSS文件收集 (IncludeStyles=false)\n")
	}

	// 收集JavaScript文件下载任务
	if options.IncludeScripts {
		s.debugPrintf("收集JavaScript文件...\n")
		jsScripts := doc.Find("script[src]")
		s.debugPrintf("找到 %d 个带src的script标签\n", jsScripts.Length())

		jsScripts.Each(func(i int, sel *goquery.Selection) {
			if src, exists := sel.Attr("src"); exists {
				// 清理URL中的空白字符
				src = strings.TrimSpace(src)
				s.debugPrintf("发现JS: '%s'\n", src)
				absoluteURL := s.resolveURL(src)
				if absoluteURL != "" {
					s.debugPrintf("添加JS任务: %s\n", absoluteURL)
					tasks = append(tasks, DownloadTask{
						URL:          absoluteURL,
						ResourceType: "js",
						Element:      sel,
						AttrName:     "src",
					})
				} else {
					s.debugPrintf("JS URL解析失败: %s\n", src)
				}
			}
		})

		// 也检查所有script标签
		allScripts := doc.Find("script")
		s.debugPrintf("总共找到 %d 个script标签\n", allScripts.Length())
	} else {
		s.debugPrintf("跳过JavaScript文件收集 (IncludeScripts=false)\n")
	}

	// 收集图片下载任务
	if options.IncludeImages {
		s.debugPrintf("收集图片文件...\n")

		// 处理普通的 img[src] 标签
		imgTags := doc.Find("img[src]")
		s.debugPrintf("找到 %d 个带src的img标签\n", imgTags.Length())

		imgTags.Each(func(i int, sel *goquery.Selection) {
			if src, exists := sel.Attr("src"); exists {
				// 清理URL中的空白字符
				src = strings.TrimSpace(src)
				s.debugPrintf("发现图片: '%s'\n", src)

				// 如果是 Data URI，跳过下载但保留在HTML中
				if strings.HasPrefix(src, "data:") {
					s.debugPrintf("跳过 Data URI 图片: %s\n", src[:min(len(src), 50)]+"...")
					return // 不添加到下载任务，保持原样
				}

				absoluteURL := s.resolveURL(src)
				if absoluteURL != "" {
					s.debugPrintf("添加图片任务: %s -> %s\n", src, absoluteURL)
					tasks = append(tasks, DownloadTask{
						URL:          absoluteURL,
						ResourceType: "images",
						Element:      sel,
						AttrName:     "src",
					})
				} else {
					s.debugPrintf("图片URL解析失败: %s (baseURL: %v)\n", src, s.baseURL)
				}
			}
		})

		// 处理 picture > source[srcset] 标签（响应式图片）
		pictureSources := doc.Find("picture source[srcset]")
		s.debugPrintf("找到 %d 个picture source标签\n", pictureSources.Length())

		pictureSources.Each(func(i int, sel *goquery.Selection) {
			if srcset, exists := sel.Attr("srcset"); exists {
				// 清理URL中的空白字符
				srcset = strings.TrimSpace(srcset)
				s.debugPrintf("发现picture source srcset: '%s'\n", srcset)

				// 解析srcset中的URL（可能包含多个URL和描述符）
				urls := s.parseSrcset(srcset)
				for _, url := range urls {
					// 如果是 Data URI，跳过下载但保留在HTML中
					if strings.HasPrefix(url, "data:") {
						s.debugPrintf("跳过 Data URI picture source图片: %s\n", url[:min(len(url), 50)]+"...")
						continue // 不添加到下载任务，保持原样
					}

					absoluteURL := s.resolveURL(url)
					if absoluteURL != "" {
						s.debugPrintf("添加picture source图片任务: %s\n", absoluteURL)
						tasks = append(tasks, DownloadTask{
							URL:          absoluteURL,
							ResourceType: "images",
							Element:      sel,
							AttrName:     "srcset",
						})
					} else {
						s.debugPrintf("picture source URL解析失败: %s\n", url)
					}
				}
			}
		})

		// 处理 img[srcset] 标签（响应式图片）
		imgSrcsetTags := doc.Find("img[srcset]")
		s.debugPrintf("找到 %d 个带srcset的img标签\n", imgSrcsetTags.Length())

		imgSrcsetTags.Each(func(i int, sel *goquery.Selection) {
			if srcset, exists := sel.Attr("srcset"); exists {
				// 清理URL中的空白字符
				srcset = strings.TrimSpace(srcset)
				s.debugPrintf("发现img srcset: '%s'\n", srcset)

				// 解析srcset中的URL
				urls := s.parseSrcset(srcset)
				for _, url := range urls {
					absoluteURL := s.resolveURL(url)
					if absoluteURL != "" {
						s.debugPrintf("添加img srcset图片任务: %s\n", absoluteURL)
						tasks = append(tasks, DownloadTask{
							URL:          absoluteURL,
							ResourceType: "images",
							Element:      sel,
							AttrName:     "srcset",
						})
					} else {
						s.debugPrintf("img srcset URL解析失败: %s\n", url)
					}
				}
			}
		})

		// 处理懒加载图片 - data-src 属性
		lazyImgTags := doc.Find("img[data-src]")
		s.debugPrintf("找到 %d 个懒加载图片(data-src)\n", lazyImgTags.Length())

		lazyImgTags.Each(func(i int, sel *goquery.Selection) {
			if dataSrc, exists := sel.Attr("data-src"); exists {
				// 清理URL中的空白字符
				dataSrc = strings.TrimSpace(dataSrc)
				s.debugPrintf("发现懒加载图片: '%s'\n", dataSrc)

				// 如果是 Data URI，直接处理但不下载
				if strings.HasPrefix(dataSrc, "data:") {
					s.debugPrintf("处理 Data URI 懒加载图片: %s\n", dataSrc[:min(len(dataSrc), 50)]+"...")
					// 对于懒加载的 Data URI，将其复制到 src 属性以确保图片能显示
					sel.SetAttr("src", dataSrc)
					s.debugPrintf("Data URI 懒加载图片：设置src属性为data-src的值\n")
					return // 不添加到下载任务
				}

				absoluteURL := s.resolveURL(dataSrc)
				if absoluteURL != "" {
					s.debugPrintf("添加懒加载图片任务: %s\n", absoluteURL)
					tasks = append(tasks, DownloadTask{
						URL:          absoluteURL,
						ResourceType: "images",
						Element:      sel,
						AttrName:     "data-src",
					})
				} else {
					s.debugPrintf("懒加载图片URL解析失败: %s\n", dataSrc)
				}
			}
		})

		// 处理懒加载响应式图片 - data-srcset 属性
		lazyImgSrcsetTags := doc.Find("img[data-srcset]")
		s.debugPrintf("找到 %d 个懒加载响应式图片(data-srcset)\n", lazyImgSrcsetTags.Length())

		lazyImgSrcsetTags.Each(func(i int, sel *goquery.Selection) {
			if dataSrcset, exists := sel.Attr("data-srcset"); exists {
				// 清理URL中的空白字符
				dataSrcset = strings.TrimSpace(dataSrcset)
				s.debugPrintf("发现懒加载响应式图片: '%s'\n", dataSrcset)

				// 解析data-srcset中的URL
				urls := s.parseSrcset(dataSrcset)
				for _, url := range urls {
					absoluteURL := s.resolveURL(url)
					if absoluteURL != "" {
						s.debugPrintf("添加懒加载响应式图片任务: %s\n", absoluteURL)
						tasks = append(tasks, DownloadTask{
							URL:          absoluteURL,
							ResourceType: "images",
							Element:      sel,
							AttrName:     "data-srcset",
						})
					} else {
						s.debugPrintf("懒加载响应式图片URL解析失败: %s\n", url)
					}
				}
			}
		})

		// 处理其他常见的懒加载属性
		otherLazyAttributes := []string{"data-original", "data-lazy-src", "data-bg", "data-background"}
		for _, attr := range otherLazyAttributes {
			lazyTags := doc.Find(fmt.Sprintf("img[%s]", attr))
			if lazyTags.Length() > 0 {
				s.debugPrintf("找到 %d 个使用%s属性的懒加载图片\n", lazyTags.Length(), attr)

				lazyTags.Each(func(i int, sel *goquery.Selection) {
					if lazyUrl, exists := sel.Attr(attr); exists {
						lazyUrl = strings.TrimSpace(lazyUrl)
						s.debugPrintf("发现%s懒加载图片: '%s'\n", attr, lazyUrl)

						// 如果是 Data URI，直接处理但不下载
						if strings.HasPrefix(lazyUrl, "data:") {
							s.debugPrintf("处理 Data URI %s懒加载图片: %s\n", attr, lazyUrl[:min(len(lazyUrl), 50)]+"...")
							// 对于懒加载的 Data URI，将其复制到 src 属性以确保图片能显示
							sel.SetAttr("src", lazyUrl)
							s.debugPrintf("Data URI %s懒加载图片：设置src属性\n", attr)
							return // 不添加到下载任务
						}

						absoluteURL := s.resolveURL(lazyUrl)
						if absoluteURL != "" {
							s.debugPrintf("添加%s懒加载图片任务: %s\n", attr, absoluteURL)
							tasks = append(tasks, DownloadTask{
								URL:          absoluteURL,
								ResourceType: "images",
								Element:      sel,
								AttrName:     attr,
							})
						} else {
							s.debugPrintf("%s懒加载图片URL解析失败: %s\n", attr, lazyUrl)
						}
					}
				})
			}
		}

		// 也检查所有img标签
		allImages := doc.Find("img")
		s.debugPrintf("总共找到 %d 个img标签\n", allImages.Length())

		// 检查所有picture标签
		allPictures := doc.Find("picture")
		s.debugPrintf("总共找到 %d 个picture标签\n", allPictures.Length())
	} else {
		s.debugPrintf("跳过图片文件收集 (IncludeImages=false)\n")
	}

	// 收集视频下载任务
	if options.IncludeVideos {
		s.debugPrintf("收集视频文件...\n")

		// video[src]
		videoTags := doc.Find("video[src]")
		s.debugPrintf("找到 %d 个带src的video标签\n", videoTags.Length())
		videoTags.Each(func(i int, sel *goquery.Selection) {
			if src, exists := sel.Attr("src"); exists {
				// 清理URL中的空白字符
				src = strings.TrimSpace(src)
				s.debugPrintf("发现视频: '%s'\n", src)
				absoluteURL := s.resolveURL(src)
				if absoluteURL != "" {
					s.debugPrintf("添加视频任务: %s\n", absoluteURL)
					tasks = append(tasks, DownloadTask{
						URL:          absoluteURL,
						ResourceType: "videos",
						Element:      sel,
						AttrName:     "src",
					})
				} else {
					s.debugPrintf("视频URL解析失败: %s\n", src)
				}
			}
		})

		// video source[src]
		videoSources := doc.Find("video source[src]")
		s.debugPrintf("找到 %d 个video source标签\n", videoSources.Length())
		videoSources.Each(func(i int, sel *goquery.Selection) {
			if src, exists := sel.Attr("src"); exists {
				// 清理URL中的空白字符
				src = strings.TrimSpace(src)
				s.debugPrintf("发现视频源: '%s'\n", src)
				absoluteURL := s.resolveURL(src)
				if absoluteURL != "" {
					s.debugPrintf("添加视频源任务: %s\n", absoluteURL)
					tasks = append(tasks, DownloadTask{
						URL:          absoluteURL,
						ResourceType: "videos",
						Element:      sel,
						AttrName:     "src",
					})
				} else {
					s.debugPrintf("视频源URL解析失败: %s\n", src)
				}
			}
		})

		// audio[src]
		audioTags := doc.Find("audio[src]")
		s.debugPrintf("找到 %d 个带src的audio标签\n", audioTags.Length())
		audioTags.Each(func(i int, sel *goquery.Selection) {
			if src, exists := sel.Attr("src"); exists {
				// 清理URL中的空白字符
				src = strings.TrimSpace(src)
				s.debugPrintf("发现音频: '%s'\n", src)
				absoluteURL := s.resolveURL(src)
				if absoluteURL != "" {
					s.debugPrintf("添加音频任务: %s\n", absoluteURL)
					tasks = append(tasks, DownloadTask{
						URL:          absoluteURL,
						ResourceType: "videos",
						Element:      sel,
						AttrName:     "src",
					})
				} else {
					s.debugPrintf("音频URL解析失败: %s\n", src)
				}
			}
		})

		// audio source[src]
		audioSources := doc.Find("audio source[src]")
		s.debugPrintf("找到 %d 个audio source标签\n", audioSources.Length())
		audioSources.Each(func(i int, sel *goquery.Selection) {
			if src, exists := sel.Attr("src"); exists {
				// 清理URL中的空白字符
				src = strings.TrimSpace(src)
				s.debugPrintf("发现音频源: '%s'\n", src)
				absoluteURL := s.resolveURL(src)
				if absoluteURL != "" {
					s.debugPrintf("添加音频源任务: %s\n", absoluteURL)
					tasks = append(tasks, DownloadTask{
						URL:          absoluteURL,
						ResourceType: "videos",
						Element:      sel,
						AttrName:     "src",
					})
				} else {
					s.debugPrintf("音频源URL解析失败: %s\n", src)
				}
			}
		})
	} else {
		s.debugPrintf("跳过视频文件收集 (IncludeVideos=false)\n")
	}

	// 收集预加载资源下载任务 (preload, prefetch, modulepreload)
	preloadSelectors := []string{
		"link[rel='preload']",
		"link[rel='prefetch']",
		"link[rel='modulepreload']",
	}

	var allPreloadLinks *goquery.Selection
	totalPreloadCount := 0

	for _, selector := range preloadSelectors {
		links := doc.Find(selector)
		count := links.Length()
		if count > 0 {
			s.debugPrintf("找到 %d 个%s链接\n", count, selector)
			totalPreloadCount += count
			if allPreloadLinks == nil {
				allPreloadLinks = links
			} else {
				allPreloadLinks = allPreloadLinks.Union(links)
			}
		}
	}

	s.debugPrintf("总共找到 %d 个预加载链接\n", totalPreloadCount)

	if allPreloadLinks != nil {
		allPreloadLinks.Each(func(i int, sel *goquery.Selection) {
			if href, exists := sel.Attr("href"); exists {
				asType, hasAs := sel.Attr("as")
				href = strings.TrimSpace(href)

				// 如果是 Data URI，跳过下载但保留在HTML中
				if strings.HasPrefix(href, "data:") {
					s.debugPrintf("跳过 Data URI preload资源: %s\n", href[:min(len(href), 50)]+"...")
					return
				}

				s.debugPrintf("发现preload资源: '%s' (as='%s')\n", href, asType)

				var resourceType string
				var shouldInclude bool

				if hasAs {
					switch strings.ToLower(asType) {
					case "script":
						resourceType = "js"
						shouldInclude = options.IncludeScripts
						s.debugPrintf("preload脚本文件: %s\n", href)
					case "style":
						resourceType = "css"
						shouldInclude = options.IncludeStyles
						s.debugPrintf("preload样式文件: %s\n", href)
					case "font":
						resourceType = "fonts"
						shouldInclude = options.IncludeFonts
						s.debugPrintf("preload字体文件: %s\n", href)
					case "image":
						resourceType = "images"
						shouldInclude = options.IncludeImages
						s.debugPrintf("preload图片文件: %s\n", href)
					default:
						s.debugPrintf("未知的preload资源类型: %s，跳过\n", asType)
						return
					}
				} else {
					// 如果没有as属性，尝试根据文件扩展名判断
					lowerHref := strings.ToLower(href)
					if strings.Contains(lowerHref, ".js") {
						resourceType = "js"
						shouldInclude = options.IncludeScripts
					} else if strings.Contains(lowerHref, ".css") {
						resourceType = "css"
						shouldInclude = options.IncludeStyles
					} else if strings.Contains(lowerHref, ".woff") || strings.Contains(lowerHref, ".ttf") || strings.Contains(lowerHref, ".otf") {
						resourceType = "fonts"
						shouldInclude = options.IncludeFonts
					} else {
						s.debugPrintf("无法确定preload资源类型，跳过: %s\n", href)
						return
					}
					s.debugPrintf("根据扩展名推断preload资源类型: %s -> %s\n", href, resourceType)
				}

				if shouldInclude {
					absoluteURL := s.resolveURL(href)
					if absoluteURL != "" {
						s.debugPrintf("添加preload任务: %s (%s)\n", absoluteURL, resourceType)
						tasks = append(tasks, DownloadTask{
							URL:          absoluteURL,
							ResourceType: resourceType,
							Element:      sel,
							AttrName:     "href",
						})
					} else {
						s.debugPrintf("preload资源URL解析失败: %s\n", href)
					}
				} else {
					s.debugPrintf("跳过preload资源（选项未启用）: %s (%s)\n", href, resourceType)
				}
			}
		})
	}

	// 收集CSS中的字体文件任务
	if options.IncludeFonts {
		s.debugPrintf("=== 开始收集CSS字体任务 ===\n")
		styleElements := doc.Find("style")
		s.debugPrintf("找到 %d 个style标签用于字体收集\n", styleElements.Length())

		styleElements.Each(func(i int, sel *goquery.Selection) {
			cssContent := sel.Text()
			s.debugPrintf("分析第 %d 个style标签的字体URL，内容长度: %d\n", i+1, len(cssContent))

			// 查找所有URL引用
			urlRegex := regexp.MustCompile(`url\s*\(\s*['"]?([^'")]+)['"]?\s*\)`)
			matches := urlRegex.FindAllStringSubmatch(cssContent, -1)

			for _, match := range matches {
				if len(match) > 1 {
					resourceURL := match[1]
					// 检查是否是字体文件
					if s.isFontFile(resourceURL) {
						absoluteURL := s.resolveURL(resourceURL)
						if absoluteURL != "" {
							s.debugPrintf("添加CSS字体任务: %s\n", absoluteURL)
							tasks = append(tasks, DownloadTask{
								URL:          absoluteURL,
								ResourceType: "fonts",
								Element:      sel,
								AttrName:     "text",
							})
						}
					}
				}
			}
		})

		// 也检查外部CSS文件中的字体（如果已经下载了CSS文件）
		linkElements := doc.Find("link[rel='stylesheet']")
		linkElements.Each(func(i int, sel *goquery.Selection) {
			href, exists := sel.Attr("href")
			if exists && href != "" {
				absoluteURL := s.resolveURL(href)
				if absoluteURL != "" {
					// 检查是否已经下载了这个CSS文件
					s.mutex.RLock()
					if resource, exists := s.resources[absoluteURL]; exists && len(resource.Content) > 0 {
						s.mutex.RUnlock()
						s.debugPrintf("分析已下载的CSS文件中的字体: %s\n", absoluteURL)

						// 查找CSS内容中的字体URL
						urlRegex := regexp.MustCompile(`url\s*\(\s*['"]?([^'")]+)['"]?\s*\)`)
						matches := urlRegex.FindAllStringSubmatch(string(resource.Content), -1)

						for _, match := range matches {
							if len(match) > 1 {
								resourceURL := match[1]
								if s.isFontFile(resourceURL) {
									fontAbsoluteURL := s.resolveURL(resourceURL)
									if fontAbsoluteURL != "" {
										s.debugPrintf("添加外部CSS字体任务: %s\n", fontAbsoluteURL)
										tasks = append(tasks, DownloadTask{
											URL:          fontAbsoluteURL,
											ResourceType: "fonts",
											Element:      sel,
											AttrName:     "href",
										})
									}
								}
							}
						}
					} else {
						s.mutex.RUnlock()
					}
				}
			}
		})
		s.debugPrintf("=== CSS字体任务收集完成 ===\n")
	}

	s.debugPrintf("=== 任务收集完成 ===\n")
	s.debugPrintf("收集到 %d 个下载任务（包含重复）\n", len(tasks))

	// 去重任务
	uniqueTasks := s.deduplicateTasks(tasks)
	duplicateCount := len(tasks) - len(uniqueTasks)

	if duplicateCount > 0 {
		s.debugPrintf("去除 %d 个重复任务，剩余 %d 个唯一任务\n", duplicateCount, len(uniqueTasks))
	} else {
		s.debugPrintf("没有发现重复任务\n")
	}

	// 使用去重后的任务列表
	tasks = uniqueTasks

	s.debugPrintf("最终任务列表: %d 个唯一任务\n", len(tasks))
	for i, task := range tasks {
		if i < 5 { // 只显示前5个任务
			s.debugPrintf("任务 %d: %s (%s)\n", i+1, task.URL, task.ResourceType)
		}
	}
	if len(tasks) > 5 {
		s.debugPrintf("... 还有 %d 个任务\n", len(tasks)-5)
	}

	// 并发下载所有资源
	maxConcurrency := options.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 默认10个并发
	}

	s.debugPrintf("准备开始下载，最大并发数: %d\n", maxConcurrency)

	// 初始化进度信息 - 确保状态正确
	s.progressMutex.Lock()
	s.progressInfo.Phase = "downloading"
	s.progressInfo.TotalFiles = len(tasks)
	s.progressInfo.CompletedFiles = 0
	s.progressInfo.CurrentFile = "准备下载资源文件..."
	s.progressInfo.FileProgress = 0

	// 重新创建文件列表，确保没有残留数据
	s.progressInfo.FileList = make([]FileInfo, 0, len(tasks))

	for i, task := range tasks {
		fileName := s.getFileNameFromURL(task.URL)
		fileInfo := FileInfo{
			Name:           fileName,
			Type:           task.ResourceType,
			Size:           "等待下载...",
			TotalSize:      0,
			DownloadedSize: 0,
			Status:         "pending",
			Progress:       0,
			URL:            task.URL,
		}
		s.progressInfo.FileList = append(s.progressInfo.FileList, fileInfo)
		s.debugPrintf("初始化文件 %d: %s -> URL: %s\n", i, fileName, task.URL)
	}

	// 立即触发一次进度回调，确保前端能看到初始状态
	if s.progressCallback != nil {
		s.progressCallback(s.progressInfo)
	}
	s.progressMutex.Unlock()

	s.debugPrintf("初始化文件列表完成，共 %d 个文件\n", len(tasks))

	// 如果没有找到任何资源文件，至少添加主页面到文件列表
	if len(tasks) == 0 {
		s.debugPrintf("没有找到资源文件，添加主页面到文件列表\n")
		s.progressMutex.Lock()

		// 安全获取baseURL字符串
		baseURLStr := ""
		if s.baseURL != nil {
			baseURLStr = s.baseURL.String()
		}

		s.progressInfo.FileList = []FileInfo{
			{
				Name:     "index.html",
				Type:     "html",
				Size:     s.formatFileSize(int64(len(htmlContent))),
				Status:   "completed",
				Progress: 100,
				URL:      baseURLStr,
			},
		}
		s.progressInfo.TotalFiles = 1
		s.progressInfo.CompletedFiles = 1
		s.progressInfo.Phase = "saving"
		s.progressInfo.CurrentFile = "保存主页面..."

		// 触发进度回调
		if s.progressCallback != nil {
			s.progressCallback(s.progressInfo)
		}
		s.progressMutex.Unlock()

		// 没有资源文件需要下载，直接跳过下载阶段
		s.debugPrintf("没有资源文件需要下载，跳过下载阶段\n")
	} else {
		// 有资源文件需要下载
		s.updateProgress("downloading", "开始下载资源文件...", 0)

		s.debugPrintf("开始并发下载 %d 个资源，并发数: %d\n", len(tasks), maxConcurrency)
		results := s.downloadResourcesConcurrently(tasks, maxConcurrency)

		// 统计结果并更新HTML中的URL
		successCount := 0
		for _, result := range results {
			if result.Success && result.LocalPath != "" {
				// 根据属性类型进行不同的处理
				if result.Task.AttrName == "srcset" || result.Task.AttrName == "data-srcset" {
					// 对于srcset和data-srcset属性，需要替换其中的URL
					s.updateSrcsetAttribute(result.Task.Element, result.Task.URL, result.LocalPath)
				} else {
					// 对于普通属性（src, href, data-src等），直接替换
					result.Task.Element.SetAttr(result.Task.AttrName, result.LocalPath)

					// 对于懒加载图片，同时设置标准属性以确保图片能正常显示
					if result.Task.AttrName == "data-src" {
						// 检查当前src是否是Data URI，如果是则不覆盖
						if currentSrc, exists := result.Task.Element.Attr("src"); exists && strings.HasPrefix(currentSrc, "data:") {
							s.debugPrintf("保留Data URI占位符，不覆盖src属性: %s\n", currentSrc[:min(len(currentSrc), 50)]+"...")
						} else {
							// 将data-src的内容也设置到src属性，确保图片能显示
							result.Task.Element.SetAttr("src", result.LocalPath)
							s.debugPrintf("懒加载图片：同时设置src属性: %s\n", result.LocalPath)
						}
					} else if result.Task.AttrName == "data-srcset" {
						// 将data-srcset的内容也设置到srcset属性
						if srcset, exists := result.Task.Element.Attr("data-srcset"); exists {
							result.Task.Element.SetAttr("srcset", srcset)
							s.debugPrintf("懒加载响应式图片：同时设置srcset属性\n")
						}
					}
				}
				successCount++
			}
		}

		s.debugPrintf("资源下载完成: 成功 %d/%d 个\n", successCount, len(tasks))
	}

	// 处理CSS中的背景图片和字体（这些需要特殊处理，暂时保持同步）
	if options.IncludeImages || options.IncludeFonts {
		s.updateProgress("analyzing", "处理CSS样式...", 80)
		s.debugPrintf("开始处理内联CSS样式...\n")
		styleElements := doc.Find("style")
		s.debugPrintf("找到 %d 个style标签\n", styleElements.Length())

		styleElements.Each(func(i int, sel *goquery.Selection) {
			s.debugPrintf("处理第 %d 个style标签\n", i+1)
			cssContent := sel.Text()
			s.debugPrintf("CSS内容长度: %d 字符\n", len(cssContent))

			if options.IncludeImages {
				s.debugPrintf("处理CSS中的图片...\n")
				cssContent = s.processCSSContent(cssContent)
				s.debugPrintf("CSS图片处理完成\n")
			}
			if options.IncludeFonts {
				s.debugPrintf("处理CSS中的字体...\n")
				cssContent = s.processFontContent(cssContent)
				s.debugPrintf("CSS字体处理完成\n")
			}
			sel.SetText(cssContent)
			s.debugPrintf("第 %d 个style标签处理完成\n", i+1)
		})
		s.debugPrintf("所有CSS样式处理完成\n")
	}

	html, err := doc.Html()
	if err != nil {
		return htmlContent, err
	}

	// 删除第三方跟踪代码
	if options.RemoveAnalytics || options.RemoveTracking || options.RemoveAds || options.RemoveTagManager || options.RemoveMaliciousTags {
		html = s.removeThirdPartyCode(html, options)
	}

	// 格式化HTML
	formattedHTML := s.formatHTML(html)
	return formattedHTML, nil
}

// downloadResourcesConcurrently 并发下载资源
func (s *PageCaptureService) downloadResourcesConcurrently(tasks []DownloadTask, maxConcurrency int) []DownloadResult {
	if len(tasks) == 0 {
		return nil
	}

	// 限制任务数量
	if len(tasks) > s.maxFiles {
		s.debugPrintf("任务数量超过限制，从 %d 截断到 %d\n", len(tasks), s.maxFiles)
		tasks = tasks[:s.maxFiles]
	} else {
		s.debugPrintf("任务数量 %d 在限制范围内 (maxFiles: %d)\n", len(tasks), s.maxFiles)
	}

	// 创建通道
	taskChan := make(chan DownloadTask, len(tasks))
	resultChan := make(chan DownloadResult, len(tasks))

	// 启动工作协程
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrency && i < len(tasks); i++ {
		wg.Add(1)
		go s.downloadWorker(taskChan, resultChan, &wg)
	}

	// 发送任务
	for _, task := range tasks {
		taskChan <- task
	}
	close(taskChan)

	// 等待所有工作协程完成
	s.debugPrintf("等待所有下载工作协程完成...\n")
	wg.Wait()
	close(resultChan)

	// 收集结果
	var results []DownloadResult
	for result := range resultChan {
		results = append(results, result)
	}

	s.debugPrintf("下载工作协程完成，收集到 %d 个结果\n", len(results))
	return results
}

// downloadWorker 下载工作协程
func (s *PageCaptureService) downloadWorker(taskChan <-chan DownloadTask, resultChan chan<- DownloadResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for task := range taskChan {
		// 检查context是否被取消（安全检查，避免nil指针）
		if s.ctx != nil {
			select {
			case <-s.ctx.Done():
				s.debugPrintf("下载工作协程收到取消信号: %v\n", s.ctx.Err())
				resultChan <- DownloadResult{
					Task:    task,
					Success: false,
					Error:   fmt.Errorf("备份已停止: %v", s.ctx.Err()),
				}
				return // 直接退出协程
			default:
				// 继续执行
			}
		}

		// 检查是否收到停止请求（双重保护）
		if s.isStopRequested() {
			resultChan <- DownloadResult{
				Task:    task,
				Success: false,
				Error:   fmt.Errorf("备份已停止"),
			}
			continue
		}

		// 检查文件数量限制
		s.mutex.RLock()
		currentCount := s.fileCount
		s.mutex.RUnlock()

		if currentCount >= s.maxFiles {
			resultChan <- DownloadResult{
				Task:    task,
				Success: false,
				Error:   fmt.Errorf("达到最大文件数限制"),
			}
			continue
		}

		// 检查是否已下载
		s.mutex.RLock()
		if resource, exists := s.resources[task.URL]; exists {
			s.mutex.RUnlock()
			s.debugPrintf("资源已存在，跳过下载: %s -> %s\n", task.URL, resource.LocalPath)
			// 更新文件状态为已完成（重要：修复状态不同步问题）
			s.updateFileStatus(task.URL, "completed", 100)
			resultChan <- DownloadResult{
				Task:      task,
				LocalPath: resource.LocalPath,
				Success:   true,
			}
			continue
		}
		s.mutex.RUnlock()

		// 更新状态为下载中
		s.updateFileStatus(task.URL, "downloading", 0)

		// 更新当前文件信息
		fileName := s.getFileNameFromURL(task.URL)
		s.updateProgress("downloading", fmt.Sprintf("正在下载: %s", fileName), 0)

		// 下载资源
		s.debugPrintf("开始下载资源: %s (类型: %s)\n", task.URL, task.ResourceType)
		localPath := s.downloadResourceSync(task.URL, task.ResourceType)

		// 更新最终状态
		if localPath != "" {
			s.updateFileStatus(task.URL, "completed", 100)
			s.debugPrintf("文件下载成功: %s -> %s\n", task.URL, localPath)
		} else {
			s.updateFileStatus(task.URL, "failed", 0)
			s.debugPrintf("文件下载失败: %s\n", task.URL)
		}

		resultChan <- DownloadResult{
			Task:      task,
			LocalPath: localPath,
			Success:   localPath != "",
		}
	}
}

// downloadResourceSync 同步下载单个资源（线程安全版本）
func (s *PageCaptureService) downloadResourceSync(resourceURL, resourceType string) string {
	// 检查是否已下载（只读锁）
	s.mutex.RLock()
	if resource, exists := s.resources[resourceURL]; exists {
		s.mutex.RUnlock()
		return resource.LocalPath
	}

	// 检查文件数量限制
	if s.fileCount >= s.maxFiles {
		s.mutex.RUnlock()
		return ""
	}
	s.mutex.RUnlock()

	// 在锁外进行网络下载（这是耗时操作）
	s.debugPrintf("开始下载: %s (%s)\n", resourceURL, resourceType)

	// 为大文件创建更长超时的客户端
	client := s.client
	if resourceType == "videos" {
		// 视频文件使用更长的超时时间（10分钟）
		client = &http.Client{
			Timeout:   10 * time.Minute,
			Transport: s.client.Transport,
		}
		s.debugPrintf("使用扩展超时(10分钟)下载视频文件\n")
	}

	// 创建带context的请求（安全检查）
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL, nil)
	if err != nil {
		s.debugPrintf("创建请求失败: %s - %v\n", resourceURL, err)
		return ""
	}

	// 根据资源类型设置不同的请求头
	if resourceType == "videos" {
		// 视频文件使用专门的请求头，确保获取完整文件
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "video/webm,video/ogg,video/*;q=0.9,application/ogg;q=0.7,audio/*;q=0.6,*/*;q=0.5")
		req.Header.Set("Accept-Encoding", "identity") // 禁用压缩，保持原始二进制数据
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Connection", "keep-alive")
		// 重要：不设置Range头，让服务器返回完整文件（200 OK而不是206 Partial Content）
	} else {
		// 其他资源使用通用请求头
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "*/*")
	}

	s.debugPrintf("发送HTTP请求: %s\n", resourceURL)
	resp, err := client.Do(req)
	if err != nil {
		s.debugPrintf("下载失败: %s - %v\n", resourceURL, err)
		// 更新文件状态为失败
		s.updateFileStatus(resourceURL, "failed", 0)
		return ""
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode == http.StatusPartialContent {
		// 206 Partial Content - 这意味着我们收到的是部分内容
		// 对于视频文件，我们需要完整的文件，所以重新请求完整内容
		if resourceType == "videos" {
			s.debugPrintf("检测到206部分内容响应，重新请求完整视频文件: %s\n", resourceURL)
			resp.Body.Close()
			return s.downloadCompleteVideoFile(resourceURL, client)
		}
	} else if resp.StatusCode != http.StatusOK {
		s.debugPrintf("HTTP错误: %s - %d %s\n", resourceURL, resp.StatusCode, resp.Status)
		// 更新文件状态为失败
		s.updateFileStatus(resourceURL, "failed", 0)
		return ""
	}

	s.debugPrintf("HTTP响应状态: %d %s\n", resp.StatusCode, resp.Status)

	// 对于视频文件，输出详细的响应头信息
	if resourceType == "videos" {
		s.debugPrintf("=== 视频文件响应头详情 ===\n")
		for key, values := range resp.Header {
			s.debugPrintf("  %s: %s\n", key, strings.Join(values, ", "))
		}
		s.debugPrintf("=== 响应头详情结束 ===\n")
	}

	// 获取文件大小信息
	contentLength := resp.ContentLength
	s.debugPrintf("HTTP响应头 Content-Length: %d 字节\n", contentLength)
	if contentLength > 0 {
		s.debugPrintf("下载中: %s - 预期大小: %.2f MB\n", resourceURL, float64(contentLength)/(1024*1024))
		// 设置总大小信息
		s.updateFileDownloadProgress(resourceURL, 0, contentLength, "downloading", 0)
	} else {
		s.debugPrintf("Content-Length无效(%d)，将在下载完成后更新文件大小\n", contentLength)
		// 即使没有总大小，也要更新状态为下载中
		s.updateFileDownloadProgress(resourceURL, 0, 0, "downloading", 0)
	}

	// 为大文件添加分块并发下载
	var content []byte
	if resourceType == "videos" && contentLength > 5*1024*1024 {
		// 视频文件超过5MB就使用分块下载，提高成功率
		s.debugPrintf("视频文件，启用分块并发下载...\n")
		resp.Body.Close() // 关闭当前连接
		content, err = s.downloadLargeFileInChunks(resourceURL, contentLength, client)
	} else if contentLength > 10*1024*1024 {
		s.debugPrintf("大文件下载，启用进度监控...\n")
		content, err = s.readWithProgress(resp.Body, contentLength, resourceURL)
	} else {
		// 小文件直接读取，但仍需更新进度
		s.debugPrintf("小文件直接下载...\n")
		content, err = io.ReadAll(resp.Body)

		// 小文件下载完成后立即更新进度为100%
		if err == nil && len(content) > 0 {
			actualSize := int64(len(content))
			s.updateFileDownloadProgress(resourceURL, actualSize, actualSize, "downloading", 100)
		}
	}

	if err != nil {
		s.debugPrintf("读取失败: %s - %v\n", resourceURL, err)
		// 更新文件状态为失败
		s.updateFileStatus(resourceURL, "failed", 0)
		return ""
	}

	s.debugPrintf("下载完成: %s - 实际大小: %.2f MB\n", resourceURL, float64(len(content))/(1024*1024))
	s.debugPrintf("尝试更新状态，URL: %s\n", resourceURL)

	// 用实际下载的文件大小更新（这样可以处理Content-Length为-1的情况）
	actualSize := int64(len(content))
	if actualSize > 0 {
		s.debugPrintf("实际文件大小: %d 字节 (%.2f MB)\n", actualSize, float64(actualSize)/(1024*1024))

		// 如果原来没有总大小信息（Content-Length无效），现在更新完整的进度信息
		if contentLength <= 0 {
			s.debugPrintf("更新无Content-Length文件的完整进度信息\n")
			s.updateFileDownloadProgress(resourceURL, actualSize, actualSize, "downloading", 100)
		}

		s.updateFileSize(resourceURL, actualSize)
	}

	// 对视频文件进行完整性检查（仅用于调试，不影响保存）
	if resourceType == "videos" && len(content) > 0 {
		s.debugPrintf("开始验证视频文件: %s (大小: %d 字节)\n", resourceURL, len(content))

		// 输出文件头信息用于调试
		if len(content) >= 16 {
			s.debugPrintf("视频文件头16字节: %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x\n",
				content[0], content[1], content[2], content[3], content[4], content[5], content[6], content[7],
				content[8], content[9], content[10], content[11], content[12], content[13], content[14], content[15])
		}

		if s.validateVideoFile(content, resourceURL) {
			s.debugPrintf("视频文件完整性验证通过: %s\n", resourceURL)
		} else {
			s.debugPrintf("警告: 视频文件格式验证失败，但仍会保存: %s\n", resourceURL)
			// 注意：验证失败不阻止文件保存，因为可能是新格式或特殊编码
		}
	}

	// 对于CSS和JS文件，尝试处理编码问题
	if resourceType == "css" || resourceType == "js" {
		if !utf8.Valid(content) {
			// 使用简化的编码检测
			convertedContent := s.simpleEncodingDetection(content, resp.Header.Get("Content-Type"))
			content = []byte(convertedContent)
		}
	}

	// 生成本地路径
	localPath := s.generateLocalPath(resourceURL, resourceType)

	// 修正文件扩展名（如果启用了该选项）
	if s.currentOptions.CorrectFileNames {
		contentType := resp.Header.Get("Content-Type")
		correctedLocalPath := s.correctFileExtension(localPath, resourceType, content, contentType)
		if correctedLocalPath != localPath {
			s.debugPrintf("文件路径已修正: %s -> %s\n", localPath, correctedLocalPath)
			localPath = correctedLocalPath
		}
	} else {
		s.debugPrintf("文件名修正功能已禁用，保持原始路径: %s\n", localPath)
	}

	// 获取写锁来存储结果（只锁定存储操作）
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 再次检查是否已被其他协程下载
	if resource, exists := s.resources[resourceURL]; exists {
		return resource.LocalPath
	}

	// 再次检查文件数量限制
	if s.fileCount >= s.maxFiles {
		return ""
	}

	// 存储资源信息
	s.resources[resourceURL] = &ResourceInfo{
		URL:       resourceURL,
		LocalPath: localPath,
		Type:      resourceType,
		Content:   content,
	}

	s.fileCount++
	return localPath
}

// downloadResource 下载单个资源（已废弃，使用downloadResourceSync）
// 这个函数保留是为了向后兼容，但实际调用downloadResourceSync
func (s *PageCaptureService) downloadResource(resourceURL, resourceType string) string {
	s.debugPrintf("警告: 使用了废弃的downloadResource函数，建议使用downloadResourceSync\n")
	return s.downloadResourceSync(resourceURL, resourceType)
}

// resolveURL 解析URL
func (s *PageCaptureService) resolveURL(resourceURL string) string {
	baseURLStr := ""
	if s.baseURL != nil {
		baseURLStr = s.baseURL.String()
	}
	s.debugPrintf("解析URL: %s (基础URL: %s)\n", resourceURL, baseURLStr)

	// 清理资源URL - 移除所有前后空白字符
	resourceURL = strings.TrimSpace(resourceURL)
	s.debugPrintf("清理后的URL: '%s'\n", resourceURL)

	// 如果baseURL为nil，无法解析相对URL
	if s.baseURL == nil {
		s.debugPrintf("警告: baseURL为nil，无法解析相对URL: %s\n", resourceURL)
		return ""
	}

	if strings.HasPrefix(resourceURL, "http://") || strings.HasPrefix(resourceURL, "https://") {
		s.debugPrintf("绝对URL: %s\n", resourceURL)
		return resourceURL
	}

	if strings.HasPrefix(resourceURL, "//") {
		result := s.baseURL.Scheme + ":" + resourceURL
		s.debugPrintf("协议相对URL: %s\n", result)
		return result
	}

	if strings.HasPrefix(resourceURL, "/") {
		result := s.baseURL.Scheme + "://" + s.baseURL.Host + resourceURL
		s.debugPrintf("根相对URL: %s\n", result)
		return result
	}

	// 相对路径处理
	baseDir := path.Dir(s.baseURL.Path)
	if baseDir == "." || baseDir == "/" {
		baseDir = ""
	}

	// 处理 ./ 开头的相对路径
	if strings.HasPrefix(resourceURL, "./") {
		resourceURL = resourceURL[2:] // 移除 "./"
	}

	// 确保路径正确拼接
	var result string
	if baseDir == "" {
		result = s.baseURL.Scheme + "://" + s.baseURL.Host + "/" + resourceURL
	} else {
		result = s.baseURL.Scheme + "://" + s.baseURL.Host + baseDir + "/" + resourceURL
	}

	s.debugPrintf("相对路径解析: %s -> %s (baseDir: %s)\n", resourceURL, result, baseDir)
	return result
}

// generateLocalPath 生成本地路径
func (s *PageCaptureService) generateLocalPath(resourceURL, resourceType string) string {
	parsedURL, err := url.Parse(resourceURL)
	if err != nil {
		// 使用MD5生成文件名
		hash := fmt.Sprintf("%x", md5.Sum([]byte(resourceURL)))
		return fmt.Sprintf("static/%s/%s%s", resourceType, hash, s.getExtensionByType(resourceType))
	}

	filename := path.Base(parsedURL.Path)
	if filename == "" || filename == "." {
		hash := fmt.Sprintf("%x", md5.Sum([]byte(resourceURL)))
		filename = hash + s.getExtensionByType(resourceType)
	}

	// 预处理文件名：移除查询参数和错误后缀（如果启用了文件名修正）
	if s.currentOptions.CorrectFileNames {
		filename = s.removeIncorrectExtensions(filename, resourceType)
	}

	// 对于视频文件，尝试保持原始扩展名
	if resourceType == "videos" {
		originalExt := strings.ToLower(path.Ext(filename))
		if s.isValidVideoExtension(originalExt) {
			// 如果已有有效的视频扩展名，保持不变
			s.debugPrintf("保持原始视频扩展名: %s\n", originalExt)
		} else {
			// 如果没有扩展名或扩展名无效，添加默认扩展名
			if !strings.Contains(filename, ".") {
				filename += s.getExtensionByType(resourceType)
				s.debugPrintf("添加默认视频扩展名: %s\n", filename)
			}
		}
	} else {
		// 非视频文件，智能处理扩展名
		correctExt := s.getExtensionByType(resourceType)
		lowerFilename := strings.ToLower(filename)

		// 检查是否已经有合适的扩展名
		hasValidExtension := false
		validExtensions := s.getValidExtensionsForType(resourceType)
		for _, ext := range validExtensions {
			if strings.HasSuffix(lowerFilename, ext) {
				hasValidExtension = true
				break
			}
		}

		// 只有在没有有效扩展名时才添加
		if !hasValidExtension {
			filename += correctExt
		}
	}

	return fmt.Sprintf("static/%s/%s", resourceType, filename)
}

// deduplicateTasks 去重下载任务，保留第一个出现的任务
func (s *PageCaptureService) deduplicateTasks(tasks []DownloadTask) []DownloadTask {
	seen := make(map[string]bool)
	var uniqueTasks []DownloadTask

	for _, task := range tasks {
		if !seen[task.URL] {
			seen[task.URL] = true
			uniqueTasks = append(uniqueTasks, task)
		} else {
			s.debugPrintf("跳过重复任务: %s\n", task.URL)
		}
	}

	return uniqueTasks
}

// getExtensionByType 获取扩展名
func (s *PageCaptureService) getExtensionByType(resourceType string) string {
	switch resourceType {
	case "css":
		return ".css"
	case "js":
		return ".js"
	case "images":
		return ".jpg"
	case "videos":
		return ".mp4"
	case "fonts":
		return ".woff2"
	default:
		return ".txt"
	}
}

// getValidExtensionsForType 获取资源类型的所有有效扩展名
func (s *PageCaptureService) getValidExtensionsForType(resourceType string) []string {
	switch resourceType {
	case "css":
		return []string{".css"}
	case "js":
		return []string{".js", ".mjs", ".jsx", ".ts", ".tsx"}
	case "images":
		return []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".ico", ".avif"}
	case "videos":
		return []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".mkv", ".m4v"}
	case "fonts":
		return []string{".woff", ".woff2", ".ttf", ".otf", ".eot"}
	default:
		return []string{".txt"}
	}
}

// correctFileExtension 根据资源类型和内容修正文件扩展名
func (s *PageCaptureService) correctFileExtension(originalPath string, resourceType string, content []byte, contentType string) string {
	s.debugPrintf("修正文件扩展名: %s (类型: %s, Content-Type: %s)\n", originalPath, resourceType, contentType)

	// 获取原始文件名（不含路径）
	fileName := path.Base(originalPath)
	dir := path.Dir(originalPath)

	// 移除查询参数和锚点
	if idx := strings.Index(fileName, "?"); idx != -1 {
		fileName = fileName[:idx]
	}
	if idx := strings.Index(fileName, "#"); idx != -1 {
		fileName = fileName[:idx]
	}

	// 根据资源类型和内容特征确定正确的扩展名
	correctExt := s.detectCorrectExtension(resourceType, content, contentType, fileName)

	// 如果文件名已经有正确的扩展名，直接返回
	if strings.HasSuffix(strings.ToLower(fileName), correctExt) {
		correctedPath := path.Join(dir, fileName)
		s.debugPrintf("文件扩展名已正确: %s\n", correctedPath)
		return correctedPath
	}

	// 移除错误的扩展名并添加正确的扩展名
	nameWithoutExt := s.removeIncorrectExtensions(fileName, resourceType)
	correctedFileName := nameWithoutExt + correctExt
	correctedPath := path.Join(dir, correctedFileName)

	s.debugPrintf("文件扩展名已修正: %s -> %s\n", originalPath, correctedPath)
	return correctedPath
}

// detectCorrectExtension 检测正确的文件扩展名
func (s *PageCaptureService) detectCorrectExtension(resourceType string, content []byte, contentType string, fileName string) string {
	s.debugPrintf("检测文件扩展名: 类型=%s, Content-Type=%s, 文件名=%s\n", resourceType, contentType, fileName)

	// 首先根据Content-Type判断
	if contentType != "" {
		contentTypeLower := strings.ToLower(contentType)
		switch {
		case strings.Contains(contentTypeLower, "text/css"):
			s.debugPrintf("根据Content-Type检测为CSS文件\n")
			return ".css"
		case strings.Contains(contentTypeLower, "text/javascript") ||
			strings.Contains(contentTypeLower, "application/javascript") ||
			strings.Contains(contentTypeLower, "application/x-javascript"):
			s.debugPrintf("根据Content-Type检测为JS文件\n")
			return ".js"
		case strings.Contains(contentTypeLower, "image/jpeg"):
			return ".jpg"
		case strings.Contains(contentTypeLower, "image/png"):
			return ".png"
		case strings.Contains(contentTypeLower, "image/gif"):
			return ".gif"
		case strings.Contains(contentTypeLower, "image/webp"):
			return ".webp"
		case strings.Contains(contentTypeLower, "image/svg"):
			return ".svg"
		case strings.Contains(contentTypeLower, "font/woff2"):
			return ".woff2"
		case strings.Contains(contentTypeLower, "font/woff"):
			return ".woff"
		case strings.Contains(contentTypeLower, "font/ttf") || strings.Contains(contentTypeLower, "application/x-font-ttf"):
			return ".ttf"
		case strings.Contains(contentTypeLower, "video/mp4"):
			return ".mp4"
		case strings.Contains(contentTypeLower, "video/webm"):
			return ".webm"
		}
	}

	// 如果Content-Type不可靠，根据内容特征判断
	if len(content) > 0 {
		// 检查更多内容以提高准确性
		checkLength := min(len(content), 2000)
		contentStr := string(content[:checkLength])
		contentLower := strings.ToLower(contentStr)

		s.debugPrintf("分析文件内容特征 (前%d字节)\n", checkLength)

		// 优先进行跨类型检测，因为resourceType可能不准确

		// CSS文件特征检测
		cssIndicators := []string{
			"{", "}", "color:", "background:", "font-", "margin:", "padding:",
			"@import", "@media", "@keyframes", "display:", "position:",
			"width:", "height:", "border:", "text-", "line-height:",
		}
		cssScore := 0
		for _, indicator := range cssIndicators {
			if strings.Contains(contentLower, indicator) {
				cssScore++
			}
		}
		if cssScore >= 3 {
			s.debugPrintf("内容特征检测为CSS文件 (得分: %d)\n", cssScore)
			return ".css"
		}

		// JavaScript文件特征检测
		jsIndicators := []string{
			"function", "var ", "let ", "const ", "return", "if(", "else",
			"document.", "window.", "console.", "alert(", "typeof",
			"null", "undefined", "true", "false", "this.", "prototype",
			"addEventListener", "getElementById", "querySelector",
		}
		jsScore := 0
		for _, indicator := range jsIndicators {
			if strings.Contains(contentLower, indicator) {
				jsScore++
			}
		}
		if jsScore >= 3 {
			s.debugPrintf("内容特征检测为JS文件 (得分: %d)\n", jsScore)
			return ".js"
		}

		// 图片文件头检测
		if len(content) >= 4 {
			// JPEG文件头
			if content[0] == 0xFF && content[1] == 0xD8 {
				s.debugPrintf("文件头检测为JPEG图片\n")
				return ".jpg"
			}
			// PNG文件头
			if content[0] == 0x89 && content[1] == 0x50 && content[2] == 0x4E && content[3] == 0x47 {
				s.debugPrintf("文件头检测为PNG图片\n")
				return ".png"
			}
			// GIF文件头
			if len(content) >= 3 && string(content[:3]) == "GIF" {
				s.debugPrintf("文件头检测为GIF图片\n")
				return ".gif"
			}
			// WebP文件头
			if len(content) >= 12 && string(content[8:12]) == "WEBP" {
				s.debugPrintf("文件头检测为WebP图片\n")
				return ".webp"
			}
		}

		// SVG文件特征
		if strings.Contains(contentLower, "<svg") || strings.Contains(contentLower, "xmlns=\"http://www.w3.org/2000/svg\"") {
			s.debugPrintf("内容特征检测为SVG图片\n")
			return ".svg"
		}

		// HTML文件特征
		if strings.Contains(contentLower, "<html") || strings.Contains(contentLower, "<!doctype") {
			s.debugPrintf("内容特征检测为HTML文件\n")
			return ".html"
		}

		// 根据原始resourceType进行备用检测
		switch resourceType {
		case "css":
			if cssScore > 0 {
				s.debugPrintf("备用CSS检测 (得分: %d)\n", cssScore)
				return ".css"
			}
		case "js":
			if jsScore > 0 {
				s.debugPrintf("备用JS检测 (得分: %d)\n", jsScore)
				return ".js"
			}
		}
	}

	// 最后根据原始文件名中的扩展名判断（更智能的检测）
	lowerFileName := strings.ToLower(fileName)

	// 按优先级检测文件名中的扩展名
	extensionMap := map[string]string{
		".css":   ".css",
		".js":    ".js",
		".jpeg":  ".jpg",
		".jpg":   ".jpg",
		".png":   ".png",
		".gif":   ".gif",
		".webp":  ".webp",
		".svg":   ".svg",
		".woff2": ".woff2",
		".woff":  ".woff",
		".ttf":   ".ttf",
		".otf":   ".otf",
		".eot":   ".eot",
		".mp4":   ".mp4",
		".webm":  ".webm",
		".avi":   ".avi",
		".mov":   ".mov",
	}

	for ext, correctExt := range extensionMap {
		if strings.Contains(lowerFileName, ext) {
			s.debugPrintf("文件名中检测到扩展名: %s -> %s\n", ext, correctExt)
			return correctExt
		}
	}

	// 特殊处理：根据文件名模式推断
	if strings.Contains(lowerFileName, "jquery") || strings.Contains(lowerFileName, "bootstrap") ||
		strings.Contains(lowerFileName, "angular") || strings.Contains(lowerFileName, "react") ||
		strings.Contains(lowerFileName, "vue") || strings.Contains(lowerFileName, "lodash") {
		s.debugPrintf("根据知名库名推断为JS文件\n")
		return ".js"
	}

	if strings.Contains(lowerFileName, "style") || strings.Contains(lowerFileName, "theme") ||
		strings.Contains(lowerFileName, "bootstrap") && resourceType == "css" {
		s.debugPrintf("根据样式文件名推断为CSS文件\n")
		return ".css"
	}

	// 默认根据资源类型返回
	defaultExt := s.getExtensionByType(resourceType)
	s.debugPrintf("使用默认扩展名: %s\n", defaultExt)
	return defaultExt
}

// removeIncorrectExtensions 移除错误的扩展名
func (s *PageCaptureService) removeIncorrectExtensions(fileName string, resourceType string) string {
	s.debugPrintf("清理文件名: '%s' (类型: %s)\n", fileName, resourceType)

	result := fileName

	// 移除查询参数和锚点
	if idx := strings.Index(result, "?"); idx != -1 {
		result = result[:idx]
	}
	if idx := strings.Index(result, "#"); idx != -1 {
		result = result[:idx]
	}

	// 移除常见的错误后缀（包括中文）
	incorrectSuffixes := []string{
		// 英文后缀
		".download", ".tmp", ".temp", ".backup", ".bak", ".old", ".new",
		".gz", ".zip", ".tar", ".rar", ".7z", ".bz2",
		".map", ".dev", ".prod", ".test", ".debug", ".release",
		".cache", ".lock", ".log", ".out", ".err",
		// 中文后缀
		".下载", ".临时", ".备份", ".缓存", ".测试",
		"下载", "临时", "备份", "缓存", "测试", // 没有点的版本
		// 其他常见后缀
		".part", ".crdownload", ".downloading",
		".1", ".2", ".3", ".copy", ".orig",
	}

	// 多次清理，直到没有更多后缀可以移除
	changed := true
	for changed {
		changed = false
		oldResult := result

		for _, suffix := range incorrectSuffixes {
			if strings.HasSuffix(result, suffix) {
				result = result[:len(result)-len(suffix)]
				changed = true
				s.debugPrintf("移除后缀 '%s': %s -> %s\n", suffix, oldResult, result)
				break
			}
		}

		// 移除末尾的点和空格
		newResult := strings.TrimRight(result, ". ")
		if newResult != result {
			result = newResult
			changed = true
		}
	}

	// 特殊处理：如果文件名包含版本号或哈希，保留主要部分
	result = s.extractMainFileName(result, resourceType)

	// 如果结果为空或只有扩展名，生成一个默认名称
	if result == "" || strings.HasPrefix(result, ".") || len(result) < 2 {
		hash := fmt.Sprintf("%x", md5.Sum([]byte(fileName)))[:8]
		result = resourceType + "_" + hash
		s.debugPrintf("生成默认文件名: %s\n", result)
	}

	s.debugPrintf("文件名清理完成: '%s' -> '%s'\n", fileName, result)
	return result
}

// extractMainFileName 提取主要文件名部分
func (s *PageCaptureService) extractMainFileName(fileName string, resourceType string) string {
	if fileName == "" {
		return ""
	}

	// 对于包含版本号的文件，尝试提取主要名称
	// 例如: jquery-3.5.1.min -> jquery
	// 例如: bootstrap.bundle.min -> bootstrap

	parts := strings.Split(fileName, ".")
	if len(parts) > 1 {
		// 移除常见的修饰词
		mainParts := []string{}
		for _, part := range parts {
			lowerPart := strings.ToLower(part)
			// 跳过版本号、min、bundle等
			if !s.isVersionOrModifier(lowerPart) {
				mainParts = append(mainParts, part)
			}
		}

		if len(mainParts) > 0 {
			result := strings.Join(mainParts, ".")
			s.debugPrintf("提取主文件名: %s -> %s\n", fileName, result)
			return result
		}
	}

	// 对于包含连字符的文件名，取第一部分
	if strings.Contains(fileName, "-") {
		parts := strings.Split(fileName, "-")
		if len(parts) > 0 && len(parts[0]) > 2 {
			s.debugPrintf("提取连字符前的名称: %s -> %s\n", fileName, parts[0])
			return parts[0]
		}
	}

	return fileName
}

// isVersionOrModifier 检查是否是版本号或修饰词
func (s *PageCaptureService) isVersionOrModifier(part string) bool {
	// 版本号模式 (数字.数字.数字)
	if matched, _ := regexp.MatchString(`^\d+(\.\d+)*$`, part); matched {
		return true
	}

	// 常见修饰词
	modifiers := []string{
		"min", "minified", "compressed",
		"bundle", "bundled",
		"dev", "development", "debug",
		"prod", "production", "release",
		"latest", "stable", "beta", "alpha",
		"full", "lite", "slim",
		"es5", "es6", "es2015", "es2017", "es2018",
		"umd", "cjs", "esm", "amd",
	}

	for _, modifier := range modifiers {
		if part == modifier {
			return true
		}
	}

	return false
}

// isValidVideoExtension 检查是否是有效的视频扩展名
func (s *PageCaptureService) isValidVideoExtension(ext string) bool {
	validExtensions := []string{
		".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm",
		".mkv", ".m4v", ".3gp", ".ogv", ".ts", ".m3u8",
		".f4v", ".asf", ".rm", ".rmvb", ".vob", ".mpg",
		".mpeg", ".m2v", ".divx", ".xvid",
	}

	ext = strings.ToLower(ext)
	for _, validExt := range validExtensions {
		if ext == validExt {
			s.debugPrintf("识别到有效视频扩展名: %s\n", ext)
			return true
		}
	}

	s.debugPrintf("无效或未知视频扩展名: %s\n", ext)
	return false
}

// processCSSContent 处理CSS内容中的URL
func (s *PageCaptureService) processCSSContent(cssContent string) string {
	s.debugPrintf("开始处理CSS内容，长度: %d\n", len(cssContent))

	if s.fileCount >= s.maxFiles {
		s.debugPrintf("已达到最大文件数限制，跳过CSS处理\n")
		return cssContent
	}

	urlRegex := regexp.MustCompile(`url\s*\(\s*['"]?([^'")]+)['"]?\s*\)`)
	matches := urlRegex.FindAllString(cssContent, -1)
	s.debugPrintf("在CSS中找到 %d 个URL匹配\n", len(matches))

	result := urlRegex.ReplaceAllStringFunc(cssContent, func(match string) string {
		submatches := urlRegex.FindStringSubmatch(match)
		if len(submatches) > 1 {
			resourceURL := submatches[1]
			s.debugPrintf("CSS中发现URL: '%s'\n", resourceURL)
			absoluteURL := s.resolveURL(resourceURL)
			if absoluteURL != "" {
				s.debugPrintf("解析后的绝对URL: '%s'\n", absoluteURL)

				// 检查是否已经下载过
				s.mutex.RLock()
				if resource, exists := s.resources[absoluteURL]; exists {
					s.mutex.RUnlock()
					s.debugPrintf("使用已下载的资源: '%s'\n", resource.LocalPath)
					return strings.Replace(match, resourceURL, resource.LocalPath, 1)
				}
				s.mutex.RUnlock()

				// 生成本地路径（不立即下载，等待统一的下载任务处理）
				localPath := s.generateLocalPath(absoluteURL, "images")
				s.debugPrintf("生成的本地路径: '%s'\n", localPath)
				return strings.Replace(match, resourceURL, localPath, 1)
			}
		}
		return match
	})

	s.debugPrintf("CSS内容处理完成\n")
	return result
}

// processFontContent 处理CSS内容中的字体文件URL
func (s *PageCaptureService) processFontContent(cssContent string) string {
	s.debugPrintf("开始处理CSS字体内容，长度: %d\n", len(cssContent))

	if s.fileCount >= s.maxFiles {
		s.debugPrintf("已达到最大文件数限制，跳过字体处理\n")
		return cssContent
	}

	// 处理所有CSS中的url()引用，检查是否为字体文件
	urlRegex := regexp.MustCompile(`url\s*\(\s*['"]?([^'")]+)['"]?\s*\)`)
	matches := urlRegex.FindAllString(cssContent, -1)
	s.debugPrintf("在CSS中找到 %d 个URL匹配\n", len(matches))

	result := urlRegex.ReplaceAllStringFunc(cssContent, func(urlMatch string) string {
		submatches := urlRegex.FindStringSubmatch(urlMatch)
		if len(submatches) > 1 {
			resourceURL := submatches[1]
			s.debugPrintf("CSS中发现URL: '%s'\n", resourceURL)

			// 检查是否是字体文件
			if s.isFontFile(resourceURL) {
				s.debugPrintf("识别为字体文件: '%s'\n", resourceURL)
				absoluteURL := s.resolveURL(resourceURL)
				if absoluteURL != "" {
					s.debugPrintf("解析后的绝对URL: '%s'\n", absoluteURL)

					// 检查是否已经下载过
					s.mutex.RLock()
					if resource, exists := s.resources[absoluteURL]; exists {
						s.mutex.RUnlock()
						s.debugPrintf("使用已下载的字体资源: '%s'\n", resource.LocalPath)
						return strings.Replace(urlMatch, resourceURL, resource.LocalPath, 1)
					}
					s.mutex.RUnlock()

					// 生成本地路径（不立即下载，等待统一的下载任务处理）
					localPath := s.generateLocalPath(absoluteURL, "fonts")
					s.debugPrintf("生成的字体本地路径: '%s'\n", localPath)
					return strings.Replace(urlMatch, resourceURL, localPath, 1)
				}
			}
		}
		return urlMatch
	})

	s.debugPrintf("CSS字体处理完成\n")
	return result
}

// isFontFile 检查是否是字体文件
func (s *PageCaptureService) isFontFile(url string) bool {
	lowerURL := strings.ToLower(url)

	// 检查常见字体文件扩展名
	fontExtensions := []string{".woff", ".woff2", ".ttf", ".otf", ".eot"}
	for _, ext := range fontExtensions {
		if strings.Contains(lowerURL, ext) {
			return true
		}
	}

	// 检查Google Fonts域名
	if strings.Contains(lowerURL, "fonts.googleapis.com") ||
		strings.Contains(lowerURL, "fonts.gstatic.com") {
		return true
	}

	return false
}

// parseSrcset 解析srcset属性，提取所有图片URL
func (s *PageCaptureService) parseSrcset(srcset string) []string {
	s.debugPrintf("解析srcset: '%s'\n", srcset)

	var urls []string

	// srcset格式: "url1 descriptor1, url2 descriptor2, ..."
	// 例如: "/image1.jpg 1x, /image2.jpg 2x" 或 "/image1.jpg 480w, /image2.jpg 800w"

	// 按逗号分割不同的候选图片
	candidates := strings.Split(srcset, ",")

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		s.debugPrintf("处理srcset候选项: '%s'\n", candidate)

		// 分割URL和描述符
		// URL可能包含空格，所以我们从右边开始查找描述符
		parts := strings.Fields(candidate)
		if len(parts) == 0 {
			continue
		}

		// 第一部分总是URL
		url := parts[0]

		// 如果有多个部分，最后一部分可能是描述符（如 1x, 2x, 480w, 800w等）
		// 但我们只需要URL部分

		// 清理URL
		url = strings.TrimSpace(url)
		if url != "" {
			s.debugPrintf("从srcset提取URL: '%s'\n", url)
			// 即使是 Data URI 也要添加到列表中，但在后续处理中会被跳过下载
			urls = append(urls, url)
		}
	}

	s.debugPrintf("srcset解析完成，提取到 %d 个URL\n", len(urls))
	return urls
}

// updateSrcsetAttribute 更新srcset或data-srcset属性中的URL
func (s *PageCaptureService) updateSrcsetAttribute(element *goquery.Selection, originalURL, localPath string) {
	// 尝试获取srcset或data-srcset属性
	var srcset string
	var attrName string
	var exists bool

	if srcset, exists = element.Attr("srcset"); exists {
		attrName = "srcset"
	} else if srcset, exists = element.Attr("data-srcset"); exists {
		attrName = "data-srcset"
	} else {
		s.debugPrintf("警告: 元素没有srcset或data-srcset属性\n")
		return
	}

	s.debugPrintf("更新srcset属性: 原始URL='%s', 本地路径='%s'\n", originalURL, localPath)
	s.debugPrintf("原始srcset: '%s'\n", srcset)

	// 解析原始URL以便进行精确匹配
	originalAbsoluteURL := s.resolveURL(originalURL)

	// 替换srcset中的URL
	// 我们需要找到包含这个URL的候选项并替换它
	candidates := strings.Split(srcset, ",")
	var updatedCandidates []string

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}

		// 提取这个候选项中的URL
		parts := strings.Fields(candidate)
		if len(parts) == 0 {
			updatedCandidates = append(updatedCandidates, candidate)
			continue
		}

		candidateURL := strings.TrimSpace(parts[0])
		candidateAbsoluteURL := s.resolveURL(candidateURL)

		// 检查是否匹配我们要替换的URL
		if candidateAbsoluteURL == originalAbsoluteURL || candidateURL == originalURL {
			s.debugPrintf("找到匹配的srcset候选项: '%s'\n", candidate)

			// 替换URL部分，保留描述符
			if len(parts) > 1 {
				// 有描述符，保留它们
				descriptor := strings.Join(parts[1:], " ")
				updatedCandidate := localPath + " " + descriptor
				updatedCandidates = append(updatedCandidates, updatedCandidate)
				s.debugPrintf("替换后的候选项: '%s'\n", updatedCandidate)
			} else {
				// 没有描述符，只替换URL
				updatedCandidates = append(updatedCandidates, localPath)
				s.debugPrintf("替换后的候选项: '%s'\n", localPath)
			}
		} else {
			// 不匹配，保持原样
			updatedCandidates = append(updatedCandidates, candidate)
		}
	}

	// 重新组合srcset
	newSrcset := strings.Join(updatedCandidates, ", ")
	element.SetAttr(attrName, newSrcset)

	s.debugPrintf("更新后的%s: '%s'\n", attrName, newSrcset)
}

// saveAllFiles 保存所有文件 - 改进版本
func (s *PageCaptureService) saveAllFiles(htmlContent string) error {
	s.debugPrintf("开始保存文件到: %s\n", s.tempDir)

	// 检查临时目录是否存在
	if s.tempDir == "" {
		return fmt.Errorf("临时目录为空")
	}

	// 确保临时目录存在
	if err := os.MkdirAll(s.tempDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}

	// 保存index.html
	indexPath := filepath.Join(s.tempDir, "index.html")
	s.debugPrintf("保存index.html到: %s\n", indexPath)
	s.debugPrintf("HTML内容长度: %d 字符\n", len(htmlContent))
	s.debugPrintf("HTML内容前200字符: %s\n", htmlContent[:min(len(htmlContent), 200)])

	err := os.WriteFile(indexPath, []byte(htmlContent), 0644)
	if err != nil {
		return fmt.Errorf("保存index.html失败: %v", err)
	}

	// 验证文件是否成功保存
	if info, err := os.Stat(indexPath); err == nil {
		s.debugPrintf("保存index.html成功: %s (大小: %d 字节)\n", indexPath, info.Size())
	} else {
		s.debugPrintf("警告: 无法验证index.html文件: %v\n", err)
	}

	// 保存资源文件
	s.debugPrintf("开始保存 %d 个资源文件\n", len(s.resources))
	savedCount := 0
	totalResources := len(s.resources)

	for url, resource := range s.resources {
		savedCount++
		s.debugPrintf("保存资源文件 %d/%d: %s -> %s\n", savedCount, totalResources, url, resource.LocalPath)
		fullPath := filepath.Join(s.tempDir, resource.LocalPath)
		s.debugPrintf("保存资源: %s -> %s (大小: %d 字节)\n", url, fullPath, len(resource.Content))

		// 创建目录
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			s.debugPrintf("创建目录失败: %s - %v\n", dir, err)
			continue // 跳过无法创建目录的文件
		}

		// 保存文件
		err = os.WriteFile(fullPath, resource.Content, 0644)
		if err != nil {
			s.debugPrintf("保存文件失败: %s - %v\n", fullPath, err)
			continue // 跳过无法保存的文件
		}

		// 验证文件是否成功保存
		if info, err := os.Stat(fullPath); err == nil {
			s.debugPrintf("保存资源成功: %s (大小: %d 字节)\n", fullPath, info.Size())

			// 对视频文件进行简化的完整性检查（避免读取整个大文件）
			if resource.Type == "videos" {
				// 只检查文件大小，不读取内容
				if info.Size() != int64(len(resource.Content)) {
					s.debugPrintf("警告: 保存的视频文件大小不匹配! 原始: %d, 保存: %d\n",
						len(resource.Content), info.Size())
				} else {
					s.debugPrintf("视频文件大小验证通过: %d 字节\n", info.Size())
				}
			}
		} else {
			s.debugPrintf("警告: 无法验证资源文件: %s - %v\n", fullPath, err)
		}
	}

	s.debugPrintf("资源文件保存完成: %d/%d 个成功\n", savedCount, len(s.resources))

	// 列出临时目录中的所有文件
	s.debugPrintf("临时目录内容:\n")
	filepath.Walk(s.tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(s.tempDir, path)
			s.debugPrintf("  文件: %s (大小: %d 字节)\n", relPath, info.Size())
		}
		return nil
	})

	return nil
}

// createZipFile 创建ZIP文件 - 改进版本
func (s *PageCaptureService) createZipFile() (string, int64, error) {
	zipFileName := fmt.Sprintf("webpage_%d.zip", time.Now().Unix())
	zipPath := filepath.Join(os.TempDir(), zipFileName)

	s.debugPrintf("开始创建ZIP文件: %s\n", zipPath)
	s.debugPrintf("源目录: %s\n", s.tempDir)

	// 检查源目录是否存在
	if _, err := os.Stat(s.tempDir); os.IsNotExist(err) {
		return "", 0, fmt.Errorf("源目录不存在: %s", s.tempDir)
	}

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", 0, fmt.Errorf("创建ZIP文件失败: %v", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	fileCount := 0
	totalSize := int64(0)

	err = filepath.Walk(s.tempDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			s.debugPrintf("遍历文件时出错: %s - %v\n", filePath, err)
			return err
		}

		if info.IsDir() {
			s.debugPrintf("跳过目录: %s\n", filePath)
			return nil
		}

		relPath, err := filepath.Rel(s.tempDir, filePath)
		if err != nil {
			s.debugPrintf("获取相对路径失败: %s - %v\n", filePath, err)
			return err
		}

		s.debugPrintf("添加文件到ZIP: %s (大小: %d 字节)\n", relPath, info.Size())

		zipFileWriter, err := zipWriter.Create(relPath)
		if err != nil {
			s.debugPrintf("在ZIP中创建文件失败: %s - %v\n", relPath, err)
			return err
		}

		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			s.debugPrintf("读取文件失败: %s - %v\n", filePath, err)
			return err
		}

		written, err := zipFileWriter.Write(fileContent)
		if err != nil {
			s.debugPrintf("写入ZIP文件失败: %s - %v\n", relPath, err)
			return err
		}

		s.debugPrintf("成功写入文件: %s (%d 字节)\n", relPath, written)
		fileCount++
		totalSize += int64(written)
		return nil
	})

	if err != nil {
		s.debugPrintf("创建ZIP过程中出错: %v\n", err)
		return "", 0, err
	}

	// 确保所有数据都写入
	err = zipWriter.Close()
	if err != nil {
		s.debugPrintf("关闭ZIP写入器失败: %v\n", err)
		return "", 0, err
	}

	err = zipFile.Close()
	if err != nil {
		s.debugPrintf("关闭ZIP文件失败: %v\n", err)
		return "", 0, err
	}

	// 获取最终的ZIP文件大小
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		s.debugPrintf("获取ZIP文件信息失败: %v\n", err)
		return zipPath, 0, nil
	}

	s.debugPrintf("ZIP文件创建完成: %s\n", zipPath)
	s.debugPrintf("包含文件数: %d\n", fileCount)
	s.debugPrintf("原始总大小: %d 字节\n", totalSize)
	s.debugPrintf("ZIP文件大小: %d 字节\n", zipInfo.Size())

	return zipPath, zipInfo.Size(), nil
}

// formatHTML 格式化HTML代码
func (s *PageCaptureService) formatHTML(html string) string {
	// 简单的HTML格式化
	formatted := html

	// 添加适当的换行和缩进
	formatted = strings.ReplaceAll(formatted, "><", ">\n<")
	formatted = strings.ReplaceAll(formatted, "</head>", "</head>\n")
	formatted = strings.ReplaceAll(formatted, "</body>", "\n</body>")
	formatted = strings.ReplaceAll(formatted, "</html>", "\n</html>")

	// 处理常见标签的换行
	tags := []string{"div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "nav", "section", "article", "header", "footer", "main"}
	for _, tag := range tags {
		formatted = strings.ReplaceAll(formatted, fmt.Sprintf("</%s>", tag), fmt.Sprintf("</%s>\n", tag))
		formatted = strings.ReplaceAll(formatted, fmt.Sprintf("<%s", tag), fmt.Sprintf("\n<%s", tag))
	}

	// 清理多余的空行
	lines := strings.Split(formatted, "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanLines = append(cleanLines, trimmed)
		}
	}

	// 添加基本缩进
	var indentedLines []string
	indent := 0
	for _, line := range cleanLines {
		trimmed := strings.TrimSpace(line)

		// 减少缩进（闭合标签）
		if strings.HasPrefix(trimmed, "</") && !strings.Contains(trimmed, "<meta") && !strings.Contains(trimmed, "<link") && !strings.Contains(trimmed, "<img") {
			indent--
			if indent < 0 {
				indent = 0
			}
		}

		// 添加缩进
		indentStr := strings.Repeat("  ", indent)
		indentedLines = append(indentedLines, indentStr+trimmed)

		// 增加缩进（开放标签）
		if strings.HasPrefix(trimmed, "<") && !strings.HasPrefix(trimmed, "</") &&
			!strings.Contains(trimmed, "<meta") && !strings.Contains(trimmed, "<link") &&
			!strings.Contains(trimmed, "<img") && !strings.HasSuffix(trimmed, "/>") {
			indent++
		}
	}

	return strings.Join(indentedLines, "\n")
}

// getFileList 获取文件列表
func (s *PageCaptureService) getFileList() []string {
	files := []string{"index.html"}

	for _, resource := range s.resources {
		files = append(files, resource.LocalPath)
	}

	return files
}

// removeThirdPartyCode 删除第三方跟踪代码
func (s *PageCaptureService) removeThirdPartyCode(html string, options CaptureOptions) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	// 删除统计分析代码
	if options.RemoveAnalytics {
		s.removeAnalyticsCode(doc)
	}

	// 删除跟踪代码
	if options.RemoveTracking {
		s.removeTrackingCode(doc)
	}

	// 删除广告代码
	if options.RemoveAds {
		s.removeAdsCode(doc)
	}

	// 删除标签管理器
	if options.RemoveTagManager {
		s.removeTagManagerCode(doc)
	}

	// 删除恶意标签
	if options.RemoveMaliciousTags {
		s.removeMaliciousTags(doc)
	}

	result, err := doc.Html()
	if err != nil {
		return html
	}

	return result
}

// removeAnalyticsCode 删除统计分析代码
func (s *PageCaptureService) removeAnalyticsCode(doc *goquery.Document) {
	// Google Analytics 相关
	analyticsSelectors := []string{
		"script[src*='google-analytics.com']",
		"script[src*='googletagmanager.com/gtag']",
		"script[src*='gtag/js']",
		"script[src*='analytics.js']",
		"script[src*='gtag.js']",
		"script[src*='ga.js']",
		// 百度统计
		"script[src*='hm.baidu.com']",
		"script[src*='cnzz.com']",
		// Mixpanel
		"script[src*='mixpanel.com']",
		// Segment
		"script[src*='segment.com']",
		"script[src*='segment.io']",
	}

	for _, selector := range analyticsSelectors {
		doc.Find(selector).Remove()
	}

	// 删除包含分析代码的内联脚本
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		content := s.Text()
		if containsAnalyticsCode(content) {
			s.Remove()
		}
	})
}

// removeTrackingCode 删除跟踪代码
func (s *PageCaptureService) removeTrackingCode(doc *goquery.Document) {
	// Facebook Pixel, TikTok Pixel, Hotjar 等
	trackingSelectors := []string{
		"script[src*='connect.facebook.net']",
		"script[src*='analytics.tiktok.com']",
		"script[src*='snapchat.com/web-sdk']",
		"script[src*='hotjar.com']",
		"script[src*='crazyegg.com']",
		"script[src*='clarity.ms']",
		"script[src*='mouseflow.com']",
		"script[src*='fullstory.com']",
	}

	for _, selector := range trackingSelectors {
		doc.Find(selector).Remove()
	}

	// 删除包含跟踪代码的内联脚本
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		content := s.Text()
		if containsTrackingCode(content) {
			s.Remove()
		}
	})
}

// removeAdsCode 删除广告代码
func (s *PageCaptureService) removeAdsCode(doc *goquery.Document) {
	// Google Ads, Taboola, PopAds 等
	adsSelectors := []string{
		"script[src*='googlesyndication.com']",
		"script[src*='doubleclick.net']",
		"script[src*='taboola.com']",
		"script[src*='outbrain.com']",
		"script[src*='popads.net']",
		"script[src*='propellerads.com']",
		"script[src*='adcash.com']",
		"script[src*='affiliate.js']",
		"script[src*='redirect.js']",
		"ins.adsbygoogle",
	}

	for _, selector := range adsSelectors {
		doc.Find(selector).Remove()
	}

	// 删除包含广告代码的内联脚本
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		content := s.Text()
		if containsAdsCode(content) {
			s.Remove()
		}
	})
}

// removeTagManagerCode 删除标签管理器代码
func (s *PageCaptureService) removeTagManagerCode(doc *goquery.Document) {
	// Google Tag Manager
	tagManagerSelectors := []string{
		"script[src*='googletagmanager.com/gtm.js']",
		"noscript iframe[src*='googletagmanager.com']",
	}

	for _, selector := range tagManagerSelectors {
		doc.Find(selector).Remove()
	}

	// 删除包含GTM代码的内联脚本
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		content := s.Text()
		if containsTagManagerCode(content) {
			s.Remove()
		}
	})
}

// containsAnalyticsCode 检查是否包含分析代码
func containsAnalyticsCode(content string) bool {
	analyticsKeywords := []string{
		"google-analytics.com",
		"gtag(",
		"ga(",
		"_gaq",
		"GoogleAnalyticsObject",
		"hm.baidu.com",
		"_hmt",
		"mixpanel",
		"analytics.track",
		"segment.com",
	}

	contentLower := strings.ToLower(content)
	for _, keyword := range analyticsKeywords {
		if strings.Contains(contentLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// containsTrackingCode 检查是否包含跟踪代码
func containsTrackingCode(content string) bool {
	trackingKeywords := []string{
		"fbq(",
		"facebook.net",
		"ttq.track",
		"tiktok",
		"snaptr(",
		"hotjar",
		"hj(",
		"crazyegg",
		"clarity",
		"mouseflow",
		"fullstory",
	}

	contentLower := strings.ToLower(content)
	for _, keyword := range trackingKeywords {
		if strings.Contains(contentLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// containsAdsCode 检查是否包含广告代码
func containsAdsCode(content string) bool {
	adsKeywords := []string{
		"googlesyndication",
		"adsbygoogle",
		"doubleclick",
		"taboola",
		"outbrain",
		"popads",
		"propellerads",
		"adcash",
		"affiliate",
		"redirect",
	}

	contentLower := strings.ToLower(content)
	for _, keyword := range adsKeywords {
		if strings.Contains(contentLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// containsTagManagerCode 检查是否包含标签管理器代码
func containsTagManagerCode(content string) bool {
	tagManagerKeywords := []string{
		"googletagmanager.com",
		"dataLayer",
		"GTM-",
	}

	contentLower := strings.ToLower(content)
	for _, keyword := range tagManagerKeywords {
		if strings.Contains(contentLower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

// removeMaliciousTags 删除可能被恶意利用的HTML标签
func (s *PageCaptureService) removeMaliciousTags(doc *goquery.Document) {
	// 删除 <base> 标签 - 可能被用于劫持所有相对链接
	doc.Find("base").Remove()

	// 删除 <meta http-equiv="refresh"> 标签 - 可能被用于自动跳转到恶意网站
	doc.Find("meta[http-equiv='refresh']").Remove()
	doc.Find("meta[http-equiv='Refresh']").Remove()
	doc.Find("meta[http-equiv='REFRESH']").Remove()

	// 删除 <meta name="referrer"> 标签 - 可能被用于伪造来源
	doc.Find("meta[name='referrer']").Remove()
	doc.Find("meta[name='Referrer']").Remove()
	doc.Find("meta[name='REFERRER']").Remove()

	// 删除其他可能的恶意 meta 标签
	// 删除可能用于重定向的 meta 标签
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		httpEquiv, exists := s.Attr("http-equiv")
		if exists {
			httpEquivLower := strings.ToLower(httpEquiv)
			// 删除各种可能的重定向相关的 meta 标签
			if httpEquivLower == "refresh" || httpEquivLower == "location" || httpEquivLower == "redirect" {
				s.Remove()
			}
		}

		name, exists := s.Attr("name")
		if exists {
			nameLower := strings.ToLower(name)
			// 删除可能影响安全的 meta 标签
			if nameLower == "referrer" || nameLower == "redirect" || nameLower == "location" {
				s.Remove()
			}
		}

		content, exists := s.Attr("content")
		if exists {
			contentLower := strings.ToLower(content)
			// 检查 content 属性中是否包含可疑的跳转指令
			if strings.Contains(contentLower, "url=") && (strings.Contains(contentLower, "http://") || strings.Contains(contentLower, "https://")) {
				// 如果 content 包含 URL 跳转，可能是恶意的
				httpEquiv, hasHttpEquiv := s.Attr("http-equiv")
				if hasHttpEquiv && strings.ToLower(httpEquiv) == "refresh" {
					s.Remove()
				}
			}
		}
	})

	// 删除可能包含恶意跳转的内联脚本
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		content := s.Text()
		if containsMaliciousCode(content) {
			s.Remove()
		}
	})
}

// isAntiCrawlerPage 智能检测是否是反爬虫页面
func (s *PageCaptureService) isAntiCrawlerPage(content string) bool {
	contentLower := strings.ToLower(content)

	// 首先检查是否是正常的HTML页面
	if !strings.Contains(contentLower, "<html") && !strings.Contains(contentLower, "<!doctype") {
		s.debugPrintf("内容不包含HTML标签，可能是错误页面\n")
		return true
	}

	// 检查页面长度，太短的页面可能是错误页面
	if len(content) < 500 {
		s.debugPrintf("页面内容太短(%d字符)，可能是错误页面\n", len(content))
		// 但如果包含基本HTML结构，可能是正常的简单页面
		if strings.Contains(contentLower, "<body") || strings.Contains(contentLower, "<head") {
			return false
		}
		return true
	}

	// 检查是否包含明显的反爬虫或错误信息
	// 但要确保这些词不是在正常内容中出现
	antiCrawlerPatterns := []string{
		"access denied",
		"403 forbidden",
		"blocked by",
		"captcha required",
		"please enable javascript",
		"bot detected",
		"automated requests",
		"rate limit exceeded",
		"cloudflare", // 移除这个，因为很多正常网站使用cloudflare
	}

	// 计算匹配的模式数量
	matchCount := 0
	for _, pattern := range antiCrawlerPatterns {
		if strings.Contains(contentLower, pattern) {
			matchCount++
			s.debugPrintf("检测到可疑模式: %s\n", pattern)
		}
	}

	// 如果匹配多个模式，更可能是反爬虫页面
	if matchCount >= 2 {
		s.debugPrintf("检测到多个反爬虫模式(%d个)，判定为反爬虫页面\n", matchCount)
		return true
	}

	// 检查页面标题是否包含错误信息
	titleRegex := regexp.MustCompile(`<title[^>]*>([^<]*)</title>`)
	titleMatches := titleRegex.FindStringSubmatch(contentLower)
	if len(titleMatches) > 1 {
		title := strings.TrimSpace(titleMatches[1])
		s.debugPrintf("页面标题: %s\n", title)

		errorTitles := []string{
			"access denied",
			"forbidden",
			"blocked",
			"error",
			"captcha",
			"bot detection",
		}

		for _, errorTitle := range errorTitles {
			if strings.Contains(title, errorTitle) {
				s.debugPrintf("页面标题包含错误信息: %s\n", errorTitle)
				return true
			}
		}
	}

	// 检查是否有正常的页面内容指标
	normalContentIndicators := []string{
		"<nav", "<header", "<footer", "<main", "<article", "<section",
		"<div", "<p", "<img", "<a href", "<script", "<style", "<link",
	}

	normalContentCount := 0
	for _, indicator := range normalContentIndicators {
		if strings.Contains(contentLower, indicator) {
			normalContentCount++
		}
	}

	// 如果有足够的正常内容指标，认为是正常页面
	if normalContentCount >= 5 {
		s.debugPrintf("检测到足够的正常内容指标(%d个)，判定为正常页面\n", normalContentCount)
		return false
	}

	// 默认情况下，如果有基本HTML结构且没有明显的反爬虫特征，认为是正常页面
	s.debugPrintf("反爬虫检测完成，判定为正常页面\n")
	return false
}

// containsMaliciousCode 检查是否包含恶意代码
func containsMaliciousCode(content string) bool {
	contentLower := strings.ToLower(content)

	// 首先检查是否是已知的合法库
	legitimateLibraries := []string{
		"lazysizes", // lazysizes 懒加载库
		"lazyload",  // 其他懒加载库
		"swiper",    // Swiper 轮播库
		"jquery",    // jQuery 库
		"bootstrap", // Bootstrap 库
		"vue.js",    // Vue.js 库
		"react",     // React 库
		"angular",   // Angular 库
		"lodash",    // Lodash 工具库
		"moment",    // Moment.js 时间库
		"gsap",      // GSAP 动画库
	}

	// 如果包含已知合法库的标识，不认为是恶意代码
	for _, lib := range legitimateLibraries {
		if strings.Contains(contentLower, lib) {
			return false
		}
	}

	// 检查是否是压缩的合法库（通常包含版权信息）
	copyrightPatterns := []string{
		"copyright", "license", "mit", "apache", "bsd",
		"author", "version", "github.com", "jsdelivr", "cdnjs",
		"unpkg.com", "npm", "bower",
	}

	copyrightCount := 0
	for _, pattern := range copyrightPatterns {
		if strings.Contains(contentLower, pattern) {
			copyrightCount++
		}
	}

	// 如果包含多个版权/许可证相关信息，很可能是合法库
	if copyrightCount >= 2 {
		return false
	}

	maliciousKeywords := []string{
		"window.location.href",
		"window.location.replace",
		"window.location.assign",
		"document.location.href",
		"document.location.replace",
		"location.href",
		"location.replace",
		"top.location",
		"parent.location",
		// 检查是否有可疑的重定向模式
		"setTimeout.*location",
		"setInterval.*location",
		// 检查是否有 base 标签操作
		"document.createElement.*base",
		"<base",
		// 检查是否有动态创建 meta refresh
		"http-equiv.*refresh",
		"meta.*refresh",
	}

	for _, keyword := range maliciousKeywords {
		if strings.Contains(contentLower, strings.ToLower(keyword)) {
			return true
		}
	}

	// 使用正则表达式检查更复杂的模式
	maliciousPatterns := []string{
		`location\s*=\s*['"][^'"]*['"]`, // location = "url"
		`location\s*\.\s*href\s*=`,      // location.href =
		`window\s*\.\s*open\s*\(`,       // window.open(
		`document\s*\.\s*write.*<base`,  // document.write 包含 base 标签
		`setTimeout\s*\(.*location`,     // setTimeout 中包含 location
		`setInterval\s*\(.*location`,    // setInterval 中包含 location
	}

	for _, pattern := range maliciousPatterns {
		matched, err := regexp.MatchString(pattern, contentLower)
		if err == nil && matched {
			return true
		}
	}

	return false
}

// StopCapture 停止页面抓取
func (s *PageCaptureService) StopCapture() error {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()

	s.stopRequested = true
	s.debugPrintf("收到停止请求，开始取消所有操作\n")

	// 取消所有正在进行的操作
	if s.cancelFunc != nil {
		s.debugPrintf("调用context取消函数\n")
		s.cancelFunc()
	}

	// 更新进度状态
	s.progressMutex.Lock()
	s.progressInfo.Phase = "stopped"
	s.progressInfo.CurrentFile = "备份已停止"
	s.progressMutex.Unlock()

	// 触发进度回调
	if s.progressCallback != nil {
		s.progressCallback(s.progressInfo)
	}

	s.debugPrintf("停止请求处理完成\n")
	return nil
}

// isStopRequested 检查是否收到停止请求
func (s *PageCaptureService) isStopRequested() bool {
	s.stopMutex.RLock()
	defer s.stopMutex.RUnlock()
	return s.stopRequested
}

// resetStopFlag 重置停止标志
func (s *PageCaptureService) resetStopFlag() {
	s.stopMutex.Lock()
	defer s.stopMutex.Unlock()
	s.stopRequested = false
}

// validateVideoFile 验证视频文件的完整性
func (s *PageCaptureService) validateVideoFile(content []byte, url string) bool {
	if len(content) < 12 {
		s.debugPrintf("视频文件太小，可能不完整: %s (大小: %d 字节)\n", url, len(content))
		return false
	}

	// 检查常见视频文件的魔数（文件头）
	// header := content[:12] // 暂时不需要，直接使用content

	// MP4 文件检查
	if len(content) >= 8 {
		// MP4 文件通常以 "ftyp" 开头（在偏移4处）
		if string(content[4:8]) == "ftyp" {
			s.debugPrintf("检测到 MP4 文件格式: %s\n", url)
			return s.validateMP4File(content)
		}
	}

	// WebM 文件检查
	if len(content) >= 4 {
		// WebM 文件以 EBML 头开始
		if content[0] == 0x1A && content[1] == 0x45 && content[2] == 0xDF && content[3] == 0xA3 {
			s.debugPrintf("检测到 WebM 文件格式: %s\n", url)
			return true
		}
	}

	// AVI 文件检查
	if len(content) >= 12 {
		// AVI 文件以 "RIFF" 开头，"AVI " 在偏移8处
		if string(content[0:4]) == "RIFF" && string(content[8:12]) == "AVI " {
			s.debugPrintf("检测到 AVI 文件格式: %s\n", url)
			return true
		}
	}

	// MOV/QuickTime 文件检查
	if len(content) >= 8 {
		// MOV 文件可能有多种 ftyp
		if string(content[4:8]) == "ftyp" {
			// 检查 MOV 特定的品牌
			if len(content) >= 12 {
				brand := string(content[8:12])
				if brand == "qt  " || brand == "mov " {
					s.debugPrintf("检测到 MOV 文件格式: %s\n", url)
					return true
				}
			}
		}
	}

	// 如果无法识别格式，但文件大小合理，认为可能是有效的
	if len(content) > 1024 {
		s.debugPrintf("无法识别视频格式，但文件大小合理: %s (大小: %d 字节)\n", url, len(content))
		return true
	}

	s.debugPrintf("视频文件格式验证失败: %s\n", url)
	return false
}

// validateMP4File 验证MP4文件的完整性
func (s *PageCaptureService) validateMP4File(content []byte) bool {
	if len(content) < 32 {
		return false
	}

	// 检查是否有基本的MP4盒子结构
	offset := 0
	boxCount := 0

	for offset < len(content)-8 && boxCount < 10 { // 最多检查10个盒子
		// 读取盒子大小（前4字节）
		if offset+8 > len(content) {
			break
		}

		boxSize := int(content[offset])<<24 | int(content[offset+1])<<16 | int(content[offset+2])<<8 | int(content[offset+3])
		boxType := string(content[offset+4 : offset+8])

		s.debugPrintf("MP4 盒子: %s, 大小: %d\n", boxType, boxSize)

		// 检查盒子大小是否合理
		if boxSize < 8 || boxSize > len(content) {
			if boxSize == 0 {
				// 盒子大小为0表示延伸到文件末尾
				break
			} else if boxSize == 1 {
				// 64位大小，跳过这种复杂情况
				break
			} else {
				s.debugPrintf("MP4 盒子大小异常: %d\n", boxSize)
				return false
			}
		}

		// 检查常见的MP4盒子类型
		switch boxType {
		case "ftyp", "moov", "mdat", "free", "skip", "wide":
			boxCount++
		}

		offset += boxSize
	}

	return boxCount > 0
}

// downloadCompleteVideoFile 下载完整的视频文件（不使用Range请求）
func (s *PageCaptureService) downloadCompleteVideoFile(resourceURL string, client *http.Client) string {
	s.debugPrintf("开始下载完整视频文件: %s\n", resourceURL)

	// 创建新的请求，确保不包含Range头（安全检查）
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", resourceURL, nil)
	if err != nil {
		s.debugPrintf("创建完整视频请求失败: %v\n", err)
		return ""
	}

	// 设置请求头，明确不要Range请求
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "video/webm,video/ogg,video/*;q=0.9,application/ogg;q=0.7,audio/*;q=0.6,*/*;q=0.5")
	req.Header.Set("Accept-Encoding", "identity") // 禁用压缩
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")
	// 明确不设置Range头

	resp, err := client.Do(req)
	if err != nil {
		s.debugPrintf("完整视频请求失败: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	// 这次我们只接受200 OK
	if resp.StatusCode != http.StatusOK {
		s.debugPrintf("完整视频请求HTTP错误: %d %s\n", resp.StatusCode, resp.Status)
		return ""
	}

	s.debugPrintf("完整视频请求成功: %d %s\n", resp.StatusCode, resp.Status)

	// 获取完整文件大小
	contentLength := resp.ContentLength
	s.debugPrintf("完整视频文件大小: %d 字节 (%.2f MB)\n", contentLength, float64(contentLength)/(1024*1024))

	// 更新进度信息
	if contentLength > 0 {
		s.updateFileDownloadProgress(resourceURL, 0, contentLength, "downloading", 0)
	}

	// 读取完整内容
	var content []byte
	if contentLength > 10*1024*1024 { // 大于10MB使用进度监控
		content, err = s.readWithProgress(resp.Body, contentLength, resourceURL)
	} else {
		content, err = io.ReadAll(resp.Body)
		if err == nil && len(content) > 0 {
			actualSize := int64(len(content))
			s.updateFileDownloadProgress(resourceURL, actualSize, actualSize, "downloading", 100)
		}
	}

	if err != nil {
		s.debugPrintf("读取完整视频内容失败: %v\n", err)
		return ""
	}

	if len(content) == 0 {
		s.debugPrintf("完整视频内容为空\n")
		return ""
	}

	s.debugPrintf("完整视频下载成功: %s - 实际大小: %.2f MB\n", resourceURL, float64(len(content))/(1024*1024))

	// 验证视频文件
	if !s.validateVideoFile(content, resourceURL) {
		s.debugPrintf("完整视频文件验证失败: %s\n", resourceURL)
		return ""
	}

	// 更新文件大小
	actualSize := int64(len(content))
	s.updateFileSize(resourceURL, actualSize)

	// 生成本地路径并保存到资源映射
	localPath := s.generateLocalPath(resourceURL, "videos")

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 检查文件数量限制
	if s.fileCount >= s.maxFiles {
		return ""
	}

	// 存储资源信息
	s.resources[resourceURL] = &ResourceInfo{
		URL:       resourceURL,
		LocalPath: localPath,
		Type:      "videos",
		Content:   content,
	}

	s.fileCount++
	s.debugPrintf("完整视频文件保存成功: %s -> %s\n", resourceURL, localPath)
	return localPath
}
