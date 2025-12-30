// 扩展 Window 接口以支持 Wails
declare global {
    interface Window {
        go?: {
            main?: {
                App?: any;
            };
        };
    }
}

// 简化的 API 调用映射 - 只保留 PageCapture 相关的 API
const apiMap: Record<string, (data: any) => Promise<string>> = {
    'capture_page': (data: any) => window.go!.main!.App!.CapturePage(data.url, data.options || '{}'),
    'get_capture_progress': (data: any) => window.go!.main!.App!.GetCaptureProgress(),
    'download_file': (data: any) => window.go!.main!.App!.DownloadFile(data.filePath),
    'select_directory': (data: any) => window.go!.main!.App!.SelectDirectory(),
    'save_zip_to_directory': (data: any) => window.go!.main!.App!.SaveZipToDirectory(data.sourcePath, data.targetDirectory, data.fileName),
    'open_directory': (data: any) => window.go!.main!.App!.OpenDirectory(data.path),
    'stop_capture': (data: any) => window.go!.main!.App!.StopCapture(),
    'open_url': (data: any) => window.go!.main!.App!.OpenUrl(data.url),
};

// API 调用函数 - 简化版本，只处理 PageCapture 相关功能
const api = async (uri: string, data: any = {}) => {
    try {
        // 检查 Wails 环境
        if (!window.go?.main?.App) {
            console.warn(`API ${uri}: Wails not available`);
            return { code: 500, msg: 'Wails 环境不可用' };
        }

        const apiFunction = apiMap[uri];
        if (!apiFunction) {
            console.error(`API ${uri}: Function not found`);
            return { code: 404, msg: 'API 不存在' };
        }

        const res = await apiFunction(data);

        // 添加调试信息
        console.log(`API ${uri} response:`, res);

        // 检查响应是否为字符串且不为空
        if (typeof res !== 'string' || !res.trim()) {
            console.error(`API ${uri}: Invalid response`, res);
            return { code: 500, msg: '服务器返回了无效的响应' };
        }

        let parsedData;
        try {
            parsedData = JSON.parse(res);
        } catch (error) {
            console.error(`API ${uri}: JSON parse error`, error);
            return { code: 500, msg: 'JSON解析失败' };
        }

        return parsedData || {};
    } catch (error) {
        console.error(`API ${uri} error:`, error);
        return { code: 500, msg: `API调用失败: ${(error as Error).message}` };
    }
}

export default api