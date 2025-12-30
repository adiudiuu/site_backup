<template>
    <div class="page-capture-container">
        <!-- 页面标题 -->
        <n-grid :cols="1" :x-gap="20" :y-gap="20">
            <n-grid-item>
                <n-card class="header-card">
                    <template #header>
                        <div class="header-content">
                            <n-icon size="24" color="#2080f0">
                                <CameraOutline />
                            </n-icon>
                            <span class="header-title">网页备份工具</span>
                            <div class="header-links">
                                <n-button text type="primary" size="small" @click="openGitHub">
                                    <template #icon>
                                        <n-icon size="16">
                                            <svg viewBox="0 0 24 24">
                                                <path fill="currentColor" d="M12,2A10,10 0 0,0 2,12C2,16.42 4.87,20.17 8.84,21.5C9.34,21.58 9.5,21.27 9.5,21C9.5,20.77 9.5,20.14 9.5,19.31C6.73,19.91 6.14,17.97 6.14,17.97C5.68,16.81 5.03,16.5 5.03,16.5C4.12,15.88 5.1,15.9 5.1,15.9C6.1,15.97 6.63,16.93 6.63,16.93C7.5,18.45 8.97,18 9.54,17.76C9.63,17.11 9.89,16.67 10.17,16.42C7.95,16.17 5.62,15.31 5.62,11.5C5.62,10.39 6,9.5 6.65,8.79C6.55,8.54 6.2,7.5 6.75,6.15C6.75,6.15 7.59,5.88 9.5,7.17C10.29,6.95 11.15,6.84 12,6.84C12.85,6.84 13.71,6.95 14.5,7.17C16.41,5.88 17.25,6.15 17.25,6.15C17.8,7.5 17.45,8.54 17.35,8.79C18,9.5 18.38,10.39 18.38,11.5C18.38,15.32 16.04,16.16 13.81,16.41C14.17,16.72 14.5,17.33 14.5,18.26C14.5,19.6 14.5,20.68 14.5,21C14.5,21.27 14.66,21.59 15.17,21.5C19.14,20.16 22,16.42 22,12A10,10 0 0,0 12,2Z" />
                                            </svg>
                                        </n-icon>
                                    </template>
                                    GitHub
                                </n-button>
                            </div>
                        </div>
                    </template>
                    <n-space vertical size="small">
                        <n-text depth="3">完整备份网页内容，包括HTML、CSS、JavaScript、图片等所有资源，支持隐私清理功能</n-text>
                        <n-alert type="warning" :show-icon="false" style="margin-top: 8px;">
                            <template #header>
                                <n-text strong style="color: #f56565;">⚠️ 重要声明</n-text>
                            </template>
                            <n-space vertical size="small">
                                <n-text depth="2" style="font-size: 13px;">
                                    <strong>本工具仅供学习和研究使用，请勿用于任何违法活动！</strong>
                                </n-text>
                                <n-text depth="3" style="font-size: 12px;">
                                    • 仅适用于简单静态网页，复杂的懒加载内容无法备份<br/>
                                    • 请遵守目标网站的使用条款和 robots.txt<br/>
                                    • 使用者需自行承担使用责任
                                </n-text>
                            </n-space>
                        </n-alert>
                    </n-space>
                </n-card>
            </n-grid-item>
        </n-grid>

        <!-- 主要内容区域 - 响应式栅格布局 -->
        <n-grid :cols="24" :x-gap="20" :y-gap="20" responsive="screen">
            <!-- 左侧配置区域 -->
            <n-grid-item :span="24" :md="10" :lg="8" :xl="7">
                <n-card title="📝 备份配置" size="small">
                    <n-form :model="form" label-placement="top">
                        <n-form-item label="目标网址" required>
                            <n-input v-model:value="form.url" placeholder="请输入要备份的网页 URL，如：https://example.com"
                                size="large" @keyup.enter="captureUrl" />
                        </n-form-item>

                        <n-form-item label="保存目录" required>
                            <n-input v-model:value="saveDirectory" placeholder="请选择保存备份文件的目录" readonly size="large">
                                <template #suffix>
                                    <n-button text type="primary" @click="selectDirectory">
                                        选择目录
                                    </n-button>
                                </template>
                            </n-input>
                        </n-form-item>
                        <!-- 快速配置 -->
                        <n-form-item>
                            <template #label>
                                <n-space align="center" size="small">
                                    <span>备份内容</span>
                                    <n-tooltip trigger="hover" placement="top">
                                        <template #trigger>
                                            <n-icon size="12" color="#ccc" style="opacity: 0.6;">
                                                <svg viewBox="0 0 24 24">
                                                    <path fill="currentColor"
                                                        d="M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M12,17A1.5,1.5 0 0,1 10.5,15.5A1.5,1.5 0 0,1 12,14A1.5,1.5 0 0,1 13.5,15.5A1.5,1.5 0 0,1 12,17M12,10.5C10.07,10.5 8.5,8.93 8.5,7C8.5,5.07 10.07,3.5 12,3.5C13.93,3.5 15.5,5.07 15.5,7C15.5,8.93 13.93,10.5 12,10.5Z" />
                                                </svg>
                                            </n-icon>
                                        </template>
                                        <div style="max-width: 350px;">
                                            <div><strong>选择要备份的资源类型</strong></div>
                                            <div style="margin-top: 8px;">
                                                <div><strong>图片</strong>：JPG、PNG、GIF、WebP、SVG等图片文件</div>
                                                <div><strong>样式</strong>：CSS样式表，保持页面外观</div>
                                                <div><strong>脚本</strong>：JavaScript文件，保持页面功能</div>
                                                <div><strong>视频</strong>：MP4、WebM等视频文件（文件较大）</div>
                                            </div>
                                            <div style="margin-top: 8px; color: #666;">
                                                建议根据需要选择，视频文件会显著增加下载时间
                                            </div>
                                        </div>
                                    </n-tooltip>
                                </n-space>
                            </template>
                            <n-checkbox-group v-model:value="quickOptions">
                                <n-space>
                                    <n-checkbox value="images" label="图片" />
                                    <n-checkbox value="styles" label="样式" />
                                    <n-checkbox value="scripts" label="脚本" />
                                    <n-checkbox value="videos" label="视频" />
                                </n-space>
                            </n-checkbox-group>
                        </n-form-item>

                        <n-form-item>
                            <template #label>
                                <n-space align="center" size="small">
                                    <span>隐私清理</span>
                                    <n-tooltip trigger="hover" placement="top">
                                        <template #trigger>
                                            <n-icon size="12" color="#ccc" style="opacity: 0.6;">
                                                <svg viewBox="0 0 24 24">
                                                    <path fill="currentColor"
                                                        d="M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M12,17A1.5,1.5 0 0,1 10.5,15.5A1.5,1.5 0 0,1 12,14A1.5,1.5 0 0,1 13.5,15.5A1.5,1.5 0 0,1 12,17M12,10.5C10.07,10.5 8.5,8.93 8.5,7C8.5,5.07 10.07,3.5 12,3.5C13.93,3.5 15.5,5.07 15.5,7C15.5,8.93 13.93,10.5 12,10.5Z" />
                                                </svg>
                                            </n-icon>
                                        </template>
                                        <div style="max-width: 350px;">
                                            <div><strong>自动移除第三方跟踪代码</strong></div>
                                            <div style="margin-top: 8px;">
                                                <div><strong>统计代码</strong>：Google Analytics、百度统计、CNZZ等</div>
                                                <div><strong>跟踪代码</strong>：Facebook Pixel、TikTok Pixel、Hotjar等</div>
                                                <div><strong>广告代码</strong>：Google Ads、Taboola、PopAds等</div>
                                            </div>
                                            <div style="margin-top: 8px; color: #18a058;">
                                                ✅ 保护隐私，防止数据收集和行为跟踪
                                            </div>
                                        </div>
                                    </n-tooltip>
                                </n-space>
                            </template>
                            <n-checkbox-group v-model:value="privacyOptions">
                                <n-space>
                                    <n-checkbox value="analytics" label="统计代码" />
                                    <n-checkbox value="tracking" label="跟踪代码" />
                                    <n-checkbox value="ads" label="广告代码" />
                                </n-space>
                            </n-checkbox-group>
                        </n-form-item>

                        <n-form-item>
                            <template #label>
                                <n-space align="center" size="small">
                                    <span>文件处理</span>
                                    <n-tooltip trigger="hover" placement="top">
                                        <template #trigger>
                                            <n-icon size="12" color="#ccc" style="opacity: 0.6;">
                                                <svg viewBox="0 0 24 24">
                                                    <path fill="currentColor"
                                                        d="M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M12,17A1.5,1.5 0 0,1 10.5,15.5A1.5,1.5 0 0,1 12,14A1.5,1.5 0 0,1 13.5,15.5A1.5,1.5 0 0,1 12,17M12,10.5C10.07,10.5 8.5,8.93 8.5,7C8.5,5.07 10.07,3.5 12,3.5C13.93,3.5 15.5,5.07 15.5,7C15.5,8.93 13.93,10.5 12,10.5Z" />
                                                </svg>
                                            </n-icon>
                                        </template>
                                        <div style="max-width: 350px;">
                                            <div><strong>文件名和扩展名处理选项</strong></div>
                                            <div style="margin-top: 8px;">
                                                <div><strong>修正文件名</strong>：自动修正下载文件的扩展名</div>
                                                <div>• 移除错误后缀（.下载、.临时等）</div>
                                                <div>• 根据内容检测真实文件类型</div>
                                                <div>• 确保文件有正确的扩展名</div>
                                            </div>
                                            <div style="margin-top: 8px; color: #666;">
                                                例：script.js.下载 → script.js
                                            </div>
                                        </div>
                                    </n-tooltip>
                                </n-space>
                            </template>
                            <n-space>
                                <n-checkbox v-model:checked="options.correctFileNames" label="修正文件名" />
                            </n-space>
                        </n-form-item>

                        <!-- 高级选项折叠 -->
                        <n-collapse>
                            <n-collapse-item name="advanced">
                                <template #header>
                                    <n-space align="center" size="small">
                                        <span>高级选项</span>
                                        <n-tooltip trigger="hover" placement="top">
                                            <template #trigger>
                                                <n-icon size="12" color="#ccc" style="opacity: 0.6;">
                                                    <svg viewBox="0 0 24 24">
                                                        <path fill="currentColor"
                                                            d="M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M12,17A1.5,1.5 0 0,1 10.5,15.5A1.5,1.5 0 0,1 12,14A1.5,1.5 0 0,1 13.5,15.5A1.5,1.5 0 0,1 12,17M12,10.5C10.07,10.5 8.5,8.93 8.5,7C8.5,5.07 10.07,3.5 12,3.5C13.93,3.5 15.5,5.07 15.5,7C15.5,8.93 13.93,10.5 12,10.5Z" />
                                                    </svg>
                                                </n-icon>
                                            </template>
                                            <div style="max-width: 400px;">
                                                <div><strong>高级下载参数配置</strong></div>
                                                <div style="margin-top: 8px;">
                                                    <div><strong>超时时间</strong>：HTTP请求超时设置，防止大文件下载失败</div>
                                                    <div><strong>最大文件数</strong>：保护系统性能，防止下载过多文件</div>
                                                    <div><strong>并发数</strong>：平衡下载速度和稳定性</div>
                                                </div>
                                                <div style="margin-top: 8px; color: #666;">
                                                    建议根据网络情况和系统性能调整这些参数
                                                </div>
                                            </div>
                                        </n-tooltip>
                                    </n-space>
                                </template>
                                <n-space vertical size="small">
                                    <n-form-item label="超时时间">
                                        <n-input-number v-model:value="options.timeout" :min="60" :max="300" :step="10"
                                            size="small" />
                                        <template #suffix>秒</template>
                                    </n-form-item>
                                    <n-form-item label="最大文件数">
                                        <n-input-number v-model:value="options.maxFiles" :min="200" :max="1000"
                                            :step="50" size="small" />
                                        <template #suffix>个</template>
                                    </n-form-item>
                                    <n-form-item label="并发数">
                                        <n-input-number v-model:value="options.maxConcurrency" :min="1" :max="20"
                                            :step="1" size="small" />
                                        <template #suffix>个</template>
                                    </n-form-item>
                                </n-space>
                            </n-collapse-item>
                        </n-collapse>
                    </n-form>

                    <!-- 备份按钮 -->
                    <n-divider />
                    <div class="action-buttons">
                        <n-button v-if="!isCapturing" type="primary" size="large" block @click="captureUrl"
                            :disabled="!form.url.trim() || !saveDirectory.trim()">
                            <template #icon>
                                <n-icon>
                                    <CameraOutline />
                                </n-icon>
                            </template>
                            开始备份
                        </n-button>

                        <n-button v-if="isCapturing" type="error" size="large" block @click="stopCapture">
                            <template #icon>
                                <n-icon>
                                    <StopOutline />
                                </n-icon>
                            </template>
                            停止备份
                        </n-button>

                        <n-space justify="space-between" style="margin-top: 12px;">
                            <n-button size="small" @click="clearResults" :disabled="isCapturing">
                                <template #icon>
                                    <n-icon>
                                        <RefreshOutline />
                                    </n-icon>
                                </template>
                                清空结果
                            </n-button>
                            <n-button size="small" @click="showDocumentation">
                                <template #icon>
                                    <n-icon>
                                        <DocumentTextOutline />
                                    </n-icon>
                                </template>
                                功能说明
                            </n-button>
                        </n-space>
                    </div>
                </n-card>
            </n-grid-item>

            <!-- 右侧进度和结果区域 -->
            <n-grid-item :span="24" :md="14" :lg="16" :xl="17">
                <!-- 空状态 -->
                <n-card v-if="!isCapturing && !captureResult" class="status-card empty">
                    <div class="empty-state">
                        <n-icon size="48" color="#d0d0d0">
                            <CameraOutline />
                        </n-icon>
                        <n-text depth="3">请配置备份参数并点击"开始备份"</n-text>
                    </div>
                </n-card>

                <!-- 备份进度 -->
                <n-card v-if="isCapturing" class="status-card progress" title="🚀 备份进行中">
                    <template #header-extra>
                        <n-tag type="info">{{ captureProgress.phase === 'analyzing' ? '分析中' : captureProgress.phase ===
                            'downloading' ? '下载中' : '保存中' }}</n-tag>
                    </template>

                    <n-space vertical size="large">
                        <!-- 总体进度 -->
                        <div class="overall-progress">
                            <div class="progress-info">
                                <span class="progress-label">{{ getPhaseText(captureProgress.phase) }}</span>
                                <span class="progress-count">{{ captureProgress.completedFiles }}/{{
                                    captureProgress.totalFiles
                                    }}</span>
                            </div>
                            <n-progress type="line"
                                :percentage="Math.round((captureProgress.completedFiles / Math.max(captureProgress.totalFiles, 1)) * 100)"
                                :show-indicator="false" :height="12" border-radius="6px"
                                :color="captureProgress.phase === 'complete' ? '#18a058' : '#2080f0'" />
                            <n-text v-if="captureProgress.currentFile" depth="3"
                                style="font-size: 12px; margin-top: 8px;">
                                {{ captureProgress.currentFile }}
                            </n-text>
                        </div>

                        <!-- 文件列表 - 可折叠 -->
                        <div v-if="captureProgress.fileList.length > 0" class="file-list-section">
                            <n-collapse :default-expanded-names="['fileList']">
                                <n-collapse-item title="" name="fileList">
                                    <template #header>
                                        <n-text strong>文件下载详情 ({{ captureProgress.fileList.length }})</n-text>
                                    </template>

                                    <n-data-table :columns="fileTableColumns" :data="sortedFileList" :pagination="false"
                                        size="small" striped :row-props="() => ({ style: 'height: 32px;' })" />
                                </n-collapse-item>
                            </n-collapse>
                        </div>
                    </n-space>
                </n-card>

                <!-- 备份结果 -->
                <n-card v-if="captureResult && !isCapturing" class="status-card result"
                    :title="captureResult.success ? '✅ 备份完成' : '❌ 备份失败'">
                    <template #header-extra>
                        <n-tag :type="captureResult.success ? 'success' : 'error'">
                            {{ captureResult.success ? '成功' : '失败' }}
                        </n-tag>
                    </template>

                    <n-space vertical>
                        <!-- 基本信息 -->
                        <n-descriptions :column="2" bordered size="small">
                            <n-descriptions-item label="网址">
                                <n-tooltip trigger="hover" placement="top">
                                    <template #trigger>
                                        <n-text class="url-text">{{ captureResult.url }}</n-text>
                                    </template>
                                    {{ captureResult.url }}
                                </n-tooltip>
                            </n-descriptions-item>
                            <n-descriptions-item label="状态码">
                                <n-tag :type="captureResult.statusCode === 200 ? 'success' : 'warning'">
                                    {{ captureResult.statusCode }}
                                </n-tag>
                            </n-descriptions-item>
                            <n-descriptions-item label="文件数量">
                                <n-text>{{ captureResult.filesCount }} 个</n-text>
                            </n-descriptions-item>
                            <n-descriptions-item label="ZIP大小">
                                <n-text>{{ formatBytes(captureResult.zipSize || 0) }}</n-text>
                            </n-descriptions-item>
                        </n-descriptions>

                        <!-- 文件统计 -->
                        <div v-if="captureProgress.fileList.length > 0" class="file-statistics">
                            <n-space>
                                <n-tag type="success">成功: {{ getFileStats().completed }}</n-tag>
                                <n-tag v-if="getFileStats().failed > 0" type="error">失败: {{ getFileStats().failed
                                    }}</n-tag>
                                <n-tag type="info">总计: {{ captureProgress.fileList.length }}</n-tag>
                            </n-space>
                        </div>

                        <!-- 保存状态 -->
                        <n-alert v-if="captureResult.success" type="success" title="备份文件已保存">
                            <n-space vertical>
                                <n-text>完整的网页已备份并保存到：{{ saveDirectory }}</n-text>
                                <n-space>
                                    <n-button type="primary" size="small" @click="openSaveDirectory">
                                        <template #icon>
                                            <n-icon>
                                                <FolderOpenOutline />
                                            </n-icon>
                                        </template>
                                        打开文件夹
                                    </n-button>
                                    <n-button type="info" size="small" @click="copyDirectoryPath">
                                        <template #icon>
                                            <n-icon>
                                                <CopyOutline />
                                            </n-icon>
                                        </template>
                                        复制路径
                                    </n-button>
                                </n-space>
                            </n-space>
                        </n-alert>

                        <!-- 文件列表 (结果页面也显示) - 可折叠 -->
                        <div v-if="captureProgress.fileList.length > 0" class="file-list-section">
                            <n-collapse :default-expanded-names="['resultFileList']">
                                <n-collapse-item title="" name="resultFileList">
                                    <template #header>
                                        <n-text strong>文件下载详情 ({{ captureProgress.fileList.length }})</n-text>
                                    </template>

                                    <n-data-table :columns="fileTableColumns" :data="sortedFileList" :pagination="false"
                                        size="small" striped :row-props="() => ({ style: 'height: 32px;' })" />
                                </n-collapse-item>
                            </n-collapse>
                        </div>
                    </n-space>
                </n-card>
            </n-grid-item>
        </n-grid>

        <!-- 功能说明弹窗 -->
        <n-modal v-model:show="showDocModal" preset="card" title="📖 页面捕获隐私清理功能说明"
            style="width: 90%; max-width: 1000px;">
            <div v-html="documentationContent" class="documentation-content"></div>
        </n-modal>

        <!-- 测试页面弹窗 -->
        <n-modal v-model:show="showTestModal" preset="card" title="🧪 测试页面代码" style="width: 90%; max-width: 1000px;">
            <n-code :code="testPageContent" language="html" show-line-numbers />
        </n-modal>
    </div>
</template>

<script setup lang="ts">
import { ref, inject, onMounted, onUnmounted, h, watch, computed, nextTick, onBeforeUnmount } from 'vue'
import { useMessage, useDialog } from 'naive-ui'
import { onBeforeRouteLeave } from 'vue-router'
import {
    RefreshOutline,
    ArchiveOutline,
    DocumentTextOutline,
    CodeOutline,
    CheckmarkCircle,
    CloseCircle,
    TimeOutline,
    DocumentOutline,
    ImageOutline,
    VideocamOutline,
    MusicalNotesOutline,
    CodeSlashOutline,
    ColorPaletteOutline,
    ChevronUpOutline,
    ChevronDownOutline,
    CameraOutline,
    StopOutline,
    FolderOpenOutline,
    CopyOutline
} from '@vicons/ionicons5'
import api from '@/api'

const message = useMessage()
const dialog = useDialog()

// 注入全局 loading
const globalLoading = inject('globalLoading') as any

// 表单数据
const form = ref({
    url: ''
})

// 抓取选项
const options = ref({
    includeImages: true,
    includeStyles: true,
    includeScripts: true,
    followRedirects: true,
    includeFonts: true,
    includeVideos: true,
    removeAnalytics: true,
    removeTracking: true,
    removeAds: true,
    removeTagManager: true,
    removeMaliciousTags: true,
    correctFileNames: true,
    timeout: 300,
    maxFiles: 200,
    maxDepth: 1,
    maxConcurrency: 10,
    forceEncoding: 'auto'
})

// 抓取结果和进度状态
const captureResult = ref<any>(null)
const isCapturing = ref(false)
const captureProgress = ref({
    phase: '', // 'analyzing', 'downloading', 'saving', 'complete'
    totalFiles: 0,
    completedFiles: 0,
    currentFile: '',
    fileProgress: 0,
    downloadSpeed: '',
    estimatedTime: '',
    fileList: [] as Array<{
        name: string,
        type: string,
        size: string,
        totalSize: number,
        downloadedSize: number,
        status: 'pending' | 'downloading' | 'completed' | 'failed',
        progress: number,
        url: string
    }>
})

// 弹窗控制
const showDocModal = ref(false)
const showTestModal = ref(false)

// 界面状态
const quickOptions = ref(['images', 'styles', 'scripts', 'videos'])
const privacyOptions = ref(['analytics', 'tracking', 'ads'])

// 排序后的文件列表
const sortedFileList = computed(() => {
    if (!captureProgress.value.fileList || captureProgress.value.fileList.length === 0) {
        return []
    }

    // 定义状态优先级
    const statusPriority = {
        'downloading': 1,  // 下载中 - 最高优先级
        'pending': 2,      // 等待下载 - 其次
        'completed': 3,    // 下载完成 - 再次
        'failed': 0        // 下载失败 - 特殊处理，在所有完成后显示在最上面
    }

    // 检查是否所有文件都已完成（completed 或 failed）
    const allCompleted = captureProgress.value.fileList.every(file =>
        file.status === 'completed' || file.status === 'failed'
    )

    return [...captureProgress.value.fileList].sort((a, b) => {
        // 如果所有文件都已完成，失败的文件显示在最上面
        if (allCompleted) {
            if (a.status === 'failed' && b.status !== 'failed') return -1
            if (b.status === 'failed' && a.status !== 'failed') return 1
        }

        // 正常情况下按优先级排序
        const priorityA = statusPriority[a.status] || 999
        const priorityB = statusPriority[b.status] || 999

        if (priorityA !== priorityB) {
            return priorityA - priorityB
        }

        // 相同状态按文件名排序
        return a.name.localeCompare(b.name)
    })
})

// 同步快速选项和详细选项
watch(quickOptions, (newVal) => {
    options.value.includeImages = newVal.includes('images')
    options.value.includeStyles = newVal.includes('styles')
    options.value.includeScripts = newVal.includes('scripts')
    options.value.includeVideos = newVal.includes('videos')
}, { immediate: true })

watch(privacyOptions, (newVal) => {
    options.value.removeAnalytics = newVal.includes('analytics')
    options.value.removeTracking = newVal.includes('tracking')
    options.value.removeAds = newVal.includes('ads')
}, { immediate: true })



// 文档内容
const documentationContent = ref('')
const testPageContent = ref('')



// 保存目录（从本地缓存加载）
const saveDirectory = ref(localStorage.getItem('pageCapture_saveDirectory') || '')

// 辅助方法
const getPhaseText = (phase: string) => {
    const phases: Record<string, string> = {
        'analyzing': '🔍 分析页面结构',
        'downloading': '⬇️ 下载资源文件',
        'saving': '💾 保存文件',
        'complete': '✅ 备份完成'
    }
    return phases[phase] || '处理中...'
}

const getFileIcon = (type: string) => {
    const icons: Record<string, any> = {
        'css': ColorPaletteOutline,
        'js': CodeSlashOutline,
        'images': ImageOutline,
        'videos': VideocamOutline,
        'fonts': MusicalNotesOutline,
        'html': DocumentOutline
    }
    return icons[type] || DocumentOutline
}

const getFileTypeColor = (type: string) => {
    const colors: Record<string, string> = {
        'css': 'info',
        'js': 'warning',
        'images': 'success',
        'videos': 'error',
        'fonts': 'default',
        'html': 'primary'
    }
    return colors[type] || 'default'
}

// 计算文件统计
const getFileStats = () => {
    const stats = {
        completed: 0,
        failed: 0,
        downloading: 0,
        pending: 0
    }

    captureProgress.value.fileList.forEach(file => {
        if (file.status === 'completed') {
            stats.completed++
        } else if (file.status === 'failed') {
            stats.failed++
        } else if (file.status === 'downloading') {
            stats.downloading++
        } else {
            stats.pending++
        }
    })

    return stats
}

// 文件表格列配置 - 简化版
const fileTableColumns = [
    {
        title: '文件',
        key: 'name',
        ellipsis: true,
        render: (row: any) => {
            console.log('渲染文件:', row.name, row.type, 'URL:', row.url)

            // 优先显示完整路径，如果没有则显示文件名
            let displayName = row.url || row.name || '未知文件'
            let fullPath = displayName

            // 如果是完整URL，提取路径部分进行显示
            if (displayName.startsWith('http')) {
                try {
                    const urlObj = new URL(displayName)
                    displayName = urlObj.pathname
                    if (displayName === '/' || displayName === '') {
                        displayName = '/index.html'
                    }
                } catch (e) {
                    // URL解析失败，使用原始字符串
                }
            }

            // 如果路径太长，进行省略处理
            const maxLength = 60
            if (displayName.length > maxLength) {
                const start = displayName.substring(0, 25)
                const end = displayName.substring(displayName.length - 30)
                displayName = `${start}...${end}`
            }

            // 判断是否可以点击（有URL就可以点击）
            const isClickable = !!row.url

            return h('div', {
                class: 'file-name-cell',
                title: fullPath, // 完整路径作为tooltip
                style: {
                    display: 'flex',
                    alignItems: 'center',
                    cursor: isClickable ? 'pointer' : 'default',
                    padding: '2px 4px',
                    borderRadius: '4px',
                    transition: 'all 0.2s ease'
                },
                onClick: isClickable ? () => window.open(row.url, '_blank') : undefined,
                onMouseenter: isClickable ? (e) => {
                    const target = e.target as HTMLElement
                    if (target) {
                        target.style.backgroundColor = '#f0f8ff'
                        target.style.transform = 'scale(1.02)'
                    }
                } : undefined,
                onMouseleave: isClickable ? (e) => {
                    const target = e.target as HTMLElement
                    if (target) {
                        target.style.backgroundColor = 'transparent'
                        target.style.transform = 'scale(1)'
                    }
                } : undefined
            }, [
                h('n-icon', {
                    style: { marginRight: '6px', fontSize: '12px' }
                }, [
                    h(getFileIcon(row.type))
                ]),
                h('span', {
                    style: {
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                        color: isClickable ? '#1890ff' : 'inherit',
                        textDecoration: isClickable ? 'underline' : 'none'
                    }
                }, displayName)
            ])
        }
    },
    {
        title: '大小',
        key: 'size',
        width: 120,
        render: (row: any) => {
            const isDownloading = row.status === 'downloading'
            const isCompleted = row.status === 'completed'
            const hasSizeInfo = row.totalSize > 0 || row.downloadedSize > 0

            // 如果有详细的大小信息，显示已下载/总大小
            if (hasSizeInfo && (isDownloading || isCompleted)) {
                const downloadedText = formatBytes(row.downloadedSize || 0)
                const totalText = row.totalSize > 0 ? formatBytes(row.totalSize) : '未知'

                return h('div', {
                    style: {
                        fontSize: '11px',
                        color: isDownloading ? '#f0a020' : '#666',
                        lineHeight: '1.2'
                    }
                }, [
                    h('div', {}, `${downloadedText}/${totalText}`),
                    // 如果是下载中且有总大小，显示百分比
                    isDownloading && row.totalSize > 0 ? h('div', {
                        style: { color: '#999', fontSize: '10px' }
                    }, `${row.progress}%`) : null
                ].filter(Boolean))
            }

            // 否则显示原来的格式
            return h('span', {
                style: {
                    fontSize: '11px',
                    color: '#666'
                }
            }, row.size || '等待下载...')
        }
    },
    {
        title: '进度',
        key: 'progress',
        width: 120,
        render: (row: any) => {
            const progress = row.progress || 0
            const isDownloading = row.status === 'downloading'
            const isCompleted = row.status === 'completed'
            const isFailed = row.status === 'failed'

            let progressColor = '#70c0e8'
            if (isCompleted) progressColor = '#18a058'
            else if (isFailed) progressColor = '#d03050'
            else if (isDownloading) progressColor = '#f0a020'

            return h('div', {
                style: {
                    display: 'flex',
                    alignItems: 'center',
                    gap: '6px'
                }
            }, [
                h('div', {
                    style: {
                        width: '60px',
                        height: '4px',
                        backgroundColor: '#f0f0f0',
                        borderRadius: '2px',
                        overflow: 'hidden'
                    }
                }, [
                    h('div', {
                        style: {
                            width: `${progress}%`,
                            height: '100%',
                            backgroundColor: progressColor,
                            transition: 'width 0.3s ease'
                        }
                    })
                ]),
                h('span', {
                    style: {
                        fontSize: '10px',
                        color: '#666',
                        minWidth: '30px'
                    }
                }, `${progress}%`)
            ])
        }
    },
    {
        title: '状态',
        key: 'status',
        width: 120,
        render: (row: any) => {
            console.log('渲染状态:', row.name, row.status)

            let statusText = '⏳ 等待'
            let statusColor = '#70c0e8'

            switch (row.status) {
                case 'completed':
                    statusText = '✅ 成功'
                    statusColor = '#18a058'
                    break
                case 'failed':
                    statusText = '❌ 失败'
                    statusColor = '#d03050'
                    break
                case 'downloading':
                    statusText = '🔄 正在下载'
                    statusColor = '#f0a020'
                    break
                default:
                    statusText = '⏳ 等待'
                    statusColor = '#70c0e8'
            }

            // 为下载中的状态添加特殊样式
            const isDownloading = row.status === 'downloading'
            const isFailed = row.status === 'failed'

            return h('span', {
                style: {
                    fontSize: '12px',
                    color: statusColor,
                    fontWeight: isDownloading ? '600' : '500',
                    backgroundColor: isDownloading ? 'rgba(240, 160, 32, 0.1)' :
                        isFailed ? 'rgba(208, 48, 80, 0.1)' : 'transparent',
                    padding: isDownloading || isFailed ? '2px 6px' : '0',
                    borderRadius: isDownloading || isFailed ? '4px' : '0',
                    border: isDownloading ? '1px solid rgba(240, 160, 32, 0.3)' :
                        isFailed ? '1px solid rgba(208, 48, 80, 0.3)' : 'none'
                }
            }, statusText)
        }
    }
]



// 格式化字节大小
const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 Bytes'
    const k = 1024
    const sizes = ['Bytes', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

// 打开文件URL
const openFileUrl = async (url: string) => {
    try {
        console.log('尝试打开URL:', url)

        // 调用后端API打开URL
        const result = await api('open_url', { url: url })

        if (result && result.code === 200) {
            message.success('已在默认浏览器中打开链接')
        } else {
            // 如果后端API失败，尝试前端方式
            window.open(url, '_blank')
            message.info('已尝试打开链接')
        }
    } catch (error) {
        console.error('打开URL失败:', error)

        // 备用方案：直接在新窗口打开
        try {
            window.open(url, '_blank')
            message.info('已在新窗口打开链接')
        } catch (fallbackError) {
            console.error('备用方案也失败:', fallbackError)

            // 最后的备用方案：复制到剪贴板
            try {
                await navigator.clipboard.writeText(url)
                message.warning('无法直接打开链接，已复制到剪贴板')
            } catch (clipboardError) {
                message.error('无法打开链接，请手动复制：' + url)
            }
        }
    }
}

// 抓取页面
const captureUrl = async () => {
    if (!form.value.url.trim()) {
        message.error('请输入要抓取的 URL')
        return
    }

    // 预处理URL
    let processedUrl = form.value.url.trim()

    // 如果没有协议，自动添加https://
    if (!processedUrl.startsWith('http://') && !processedUrl.startsWith('https://')) {
        processedUrl = 'https://' + processedUrl
        form.value.url = processedUrl
    }

    // 验证 URL 格式
    try {
        const url = new URL(processedUrl)
        // 检查是否为有效的域名
        if (!url.hostname || url.hostname.length < 3) {
            message.error('请输入有效的网站地址')
            return
        }
    } catch (error) {
        message.error('请输入有效的 URL 格式，例如：https://example.com')
        return
    }

    // 验证保存目录
    if (!saveDirectory.value.trim()) {
        message.error('请先选择保存目录')
        return
    }

    // 开始备份流程
    isCapturing.value = true
    captureResult.value = null

    // 文件列表默认展开，无需设置

    // 完全重置进度状态 - 确保前后两次备份不相互影响
    captureProgress.value = {
        phase: 'analyzing',
        totalFiles: 0,
        completedFiles: 0,
        currentFile: '正在分析页面...',
        fileProgress: 0,
        downloadSpeed: '',
        estimatedTime: '',
        fileList: [] // 确保是空数组
    }

    // 强制触发界面更新
    await nextTick()

    try {
        // 开始进度轮询
        startProgressPolling()

        const result = await api('capture_page', {
            url: processedUrl,
            options: JSON.stringify(options.value)
        })

        if (result.code === 200) {
            captureProgress.value.phase = 'saving'
            captureProgress.value.currentFile = '保存文件中...'

            // 获取最新的进度状态（包含最终的文件状态）
            await getProgress()

            // 如果轮询没有获取到文件列表，使用后端返回的数据作为备用
            if (captureProgress.value.fileList.length === 0 && result.data.fileDetails && result.data.fileDetails.length > 0) {
                console.log('使用后端返回的文件详情作为备用数据')
                captureProgress.value.fileList = result.data.fileDetails.map((file: any) => ({
                    name: file.name,
                    type: file.type,
                    size: file.size,
                    status: file.status,
                    progress: file.progress,
                    url: file.url
                }))
                captureProgress.value.totalFiles = result.data.fileDetails.length
                captureProgress.value.completedFiles = result.data.successCount || 0
            }

            captureResult.value = {
                success: true,
                url: processedUrl,
                timestamp: new Date().toLocaleString(),
                statusCode: result.data.statusCode || 200,
                contentType: result.data.contentType,
                contentLength: result.data.contentLength,
                duration: result.data.duration,
                filesCount: result.data.filesCount,
                zipPath: result.data.zipPath,
                zipSize: result.data.zipSize,
                downloadedFiles: result.data.downloadedFiles,
                successCount: result.data.successCount,
                failedCount: result.data.failedCount
            }

            // 保存ZIP文件到指定目录
            if (result.data.zipPath) {
                if (saveDirectory.value) {
                    await saveZipToDirectory(result.data.zipPath)
                } else {
                    message.warning('未选择保存目录，ZIP文件已生成但未保存')
                }
            }

            captureProgress.value.phase = 'complete'
            message.success(`备份完成！共处理 ${result.data.filesCount} 个文件`)
        } else {
            captureResult.value = {
                success: false,
                url: form.value.url,
                timestamp: new Date().toLocaleString(),
                error: result.msg || '抓取失败',
                statusCode: result.data?.statusCode || 0
            }
            message.error(result.msg || '页面备份失败')
        }
    } catch (error) {
        console.error('Page capture error:', error)

        let errorMessage = '未知错误'
        if (error instanceof Error) {
            errorMessage = error.message
        } else if (typeof error === 'string') {
            errorMessage = error
        }

        captureResult.value = {
            success: false,
            url: form.value.url,
            timestamp: new Date().toLocaleString(),
            error: errorMessage
        }

        // 提供更友好的错误提示
        if (errorMessage.includes('网络') || errorMessage.includes('network')) {
            message.error('网络连接失败，请检查网络连接或URL是否正确')
        } else if (errorMessage.includes('超时') || errorMessage.includes('timeout')) {
            message.error('请求超时，请尝试增加超时时间或稍后重试')
        } else if (errorMessage.includes('格式') || errorMessage.includes('format')) {
            message.error('URL格式不正确，请检查输入的网址')
        } else if (errorMessage.includes('access denied') || errorMessage.includes('forbidden') || errorMessage.includes('反爬虫')) {
            message.error('网站拒绝访问，可能存在反爬虫机制。建议：1) 稍后重试 2) 检查URL是否需要登录 3) 尝试在浏览器中先访问该页面')
        } else if (errorMessage.includes('重定向') || errorMessage.includes('redirect')) {
            message.error('页面重定向次数过多，请检查URL是否正确')
        } else if (errorMessage.includes('404') || errorMessage.includes('not found')) {
            message.error('页面不存在(404)，请检查URL是否正确')
        } else if (errorMessage.includes('500') || errorMessage.includes('server error')) {
            message.error('服务器内部错误(500)，请稍后重试')
        } else if (errorMessage.includes('响应内容为空')) {
            message.error('页面内容为空，可能是动态加载的页面或需要JavaScript渲染')
        } else {
            message.error('页面备份失败：' + errorMessage)
        }
    } finally {
        isCapturing.value = false
        stopProgressPolling()
    }
}

// 清空结果
const clearResults = () => {
    captureResult.value = null
    message.info('已清空备份结果')
}

// 保存ZIP文件到指定目录
const saveZipToDirectory = async (zipPath: string) => {
    try {


        // 生成文件名：网站域名_时间戳.zip
        const urlObj = new URL(captureResult.value.url)
        const domain = urlObj.hostname.replace(/[^a-zA-Z0-9]/g, '_')
        const timestamp = new Date().toISOString().slice(0, 19).replace(/[:-]/g, '')
        const fileName = `${domain}_${timestamp}.zip`

        // 调用Go后端的文件保存方法
        const response = await api('save_zip_to_directory', {
            sourcePath: zipPath,
            targetDirectory: saveDirectory.value,
            fileName: fileName
        })



        if (response && response.code === 200) {
            message.success(`ZIP文件已保存到: ${saveDirectory.value}\\${fileName}`)
        } else {
            message.error('保存失败：' + (response?.msg || '未知错误'))
        }
    } catch (error) {
        console.error('Save zip error:', error)
        message.error('保存失败：' + (error as Error).message)
    }
}

// 自动下载ZIP文件（保留作为备用）
const autoDownloadZip = async (zipPath: string) => {
    try {


        // 调用Go后端的文件下载方法
        const response = await api('download_file', {
            filePath: zipPath
        })



        if (response && response.code === 200 && response.data) {
            // 处理Base64编码的二进制数据
            let binaryData
            if (typeof response.data === 'string') {
                // 后端返回Base64编码的字符串，直接解码
                try {
                    const binaryString = atob(response.data)
                    binaryData = new Uint8Array(binaryString.length)
                    for (let i = 0; i < binaryString.length; i++) {
                        binaryData[i] = binaryString.charCodeAt(i)
                    }

                } catch (e) {
                    console.error('Base64解码失败:', e)
                    throw new Error('Base64解码失败: ' + (e as Error).message)
                }
            } else if (Array.isArray(response.data)) {
                // 兼容旧的数组格式
                binaryData = new Uint8Array(response.data)
            } else {
                throw new Error('不支持的数据格式: ' + typeof response.data)
            }

            // 创建下载链接
            const blob = new Blob([binaryData], { type: 'application/zip' })
            const url = window.URL.createObjectURL(blob)
            const link = document.createElement('a')
            link.href = url

            // 生成文件名：网站域名_时间戳.zip
            const urlObj = new URL(captureResult.value.url)
            const domain = urlObj.hostname.replace(/[^a-zA-Z0-9]/g, '_')
            const timestamp = new Date().toISOString().slice(0, 19).replace(/[:-]/g, '')
            link.download = `${domain}_${timestamp}.zip`

            document.body.appendChild(link)
            link.click()
            document.body.removeChild(link)
            window.URL.revokeObjectURL(url)



            message.success(`ZIP文件已下载: ${link.download}`)
        } else {
            message.error('下载失败：' + (response?.msg || '服务器返回数据为空'))
        }
    } catch (error) {
        console.error('Auto download error:', error)
        message.error('自动下载失败：' + (error as Error).message)
    }
}



// 选择保存目录
const selectDirectory = async () => {
    try {
        // 调用Go后端的目录选择方法
        const result = await api('select_directory', {})

        if (result && result.code === 200 && result.data) {
            saveDirectory.value = result.data
            // 缓存到本地存储
            localStorage.setItem('pageCapture_saveDirectory', result.data)
            message.success('目录选择成功')
        } else if (result && result.code === 400) {
            // 用户取消选择
            message.info('已取消选择目录')
        } else {
            message.error('选择目录失败：' + (result?.msg || '未知错误'))
        }
    } catch (error) {
        console.error('Select directory error:', error)
        message.error('选择目录异常：' + (error as Error).message)
    }
}

// 格式化文件路径显示
const formatFilePath = (filePath: string): string => {
    // 如果是index.html，显示为根文件
    if (filePath === 'index.html') {
        return '📄 ' + filePath
    }
    // 如果在static目录下，添加文件夹图标
    if (filePath.startsWith('static/')) {
        const parts = filePath.split('/')
        if (parts.length >= 3) {
            const folder = parts[1]
            const file = parts[2]
            const folderIcon = folder === 'css' ? '🎨' : folder === 'js' ? '⚡' : folder === 'images' ? '🖼️' : '📁'
            return `${folderIcon} static/${folder}/${file}`
        }
    }
    return '📄 ' + filePath
}

// 显示功能说明文档
const showDocumentation = () => {
    documentationContent.value = `
        <h2>🛡️ 页面捕获隐私清理功能</h2>
        
        <h3>功能概述</h3>
        <p>在页面备份工具中新增了隐私清理功能，可以自动删除网页中的第三方跟踪、统计、广告代码，保护用户隐私。</p>
        
        <h3>隐私清理选项</h3>
        
        <h4>1. 删除统计分析代码 ✅</h4>
        <ul>
            <li>Google Analytics / gtag.js / GA4</li>
            <li>百度统计 / CNZZ</li>
            <li>Mixpanel / Segment</li>
        </ul>
        
        <h4>2. 删除跟踪代码 ✅</h4>
        <ul>
            <li>Facebook Pixel</li>
            <li>TikTok Pixel / Snapchat Pixel</li>
            <li>Hotjar / CrazyEgg / Clarity</li>
        </ul>
        
        <h4>3. 删除广告代码 ✅</h4>
        <ul>
            <li>Google Ads / DoubleClick</li>
            <li>Taboola / Outbrain</li>
            <li>PopAds / PropellerAds / AdCash</li>
            <li>affiliate.js / redirect.js</li>
        </ul>
        
        <h4>4. 删除标签管理器 ✅</h4>
        <ul>
            <li>Google Tag Manager (GTM)</li>
        </ul>
        
        <h4>5. 删除恶意标签 ✅</h4>
        <ul>
            <li><code>&lt;base href="..."&gt;</code> - 防止劫持所有相对链接</li>
            <li><code>&lt;meta http-equiv="refresh"&gt;</code> - 防止自动跳转到恶意网站</li>
            <li><code>&lt;meta name="referrer"&gt;</code> - 防止来源伪造</li>
            <li>恶意JavaScript重定向代码</li>
        </ul>
        
        <h3>安全防护</h3>
        
        <h4>恶意标签防护</h4>
        <ol>
            <li><strong>&lt;base&gt; 标签劫持防护</strong> - 自动删除所有 base 标签，防止恶意网站劫持页面中的所有相对链接</li>
            <li><strong>自动跳转防护</strong> - 删除 meta refresh 标签，防止页面自动跳转到钓鱼网站或恶意网站</li>
            <li><strong>来源伪造防护</strong> - 删除 meta referrer 标签，防止恶意网站伪造访问来源</li>
            <li><strong>JavaScript重定向防护</strong> - 检测并删除包含恶意重定向的JavaScript代码</li>
        </ol>
        
        <h3>使用方法</h3>
        <ol>
            <li>在页面捕获界面中，找到"隐私清理"选项组</li>
            <li>根据需要勾选要删除的第三方代码类型</li>
            <li>开始备份，系统会自动清理选中的代码类型</li>
            <li>备份完成后，生成的HTML文件将不包含被清理的第三方代码</li>
        </ol>
        
        <h3>注意事项</h3>
        <ul>
            <li>隐私清理功能默认启用，确保用户隐私安全</li>
            <li>恶意标签清理功能默认启用，提供额外的安全防护</li>
            <li>清理过程不会影响页面的基本功能和样式</li>
            <li>被清理的代码包括外部引用和内联代码</li>
            <li>清理后的页面在离线环境下浏览更加安全</li>
        </ul>
    `
    showDocModal.value = true
}

// 显示测试页面代码
const showTestPage = () => {
    testPageContent.value = 'HTML测试页面包含以下内容：\n\n' +
        '1. 恶意标签示例：\n' +
        '   - <base href="https://evil-site.com/">\n' +
        '   - <meta http-equiv="refresh" content="5;url=https://phishing-site.com">\n' +
        '   - <meta name="referrer" content="no-referrer">\n\n' +
        '2. 第三方跟踪代码：\n' +
        '   - Google Analytics\n' +
        '   - Facebook Pixel\n' +
        '   - 百度统计\n' +
        '   - Google Tag Manager\n\n' +
        '3. 恶意JavaScript代码：\n' +
        '   - window.location.href 重定向\n' +
        '   - setTimeout 延时跳转\n' +
        '   - 动态创建base标签\n\n' +
        '这些代码在启用隐私清理功能后会被自动删除。'

    showTestModal.value = true
}

// 进度轮询变量
let progressPollingInterval: NodeJS.Timeout | null = null

// 添加获取进度的API - 改进版本
const getProgress = async () => {
    try {
        const result = await api('get_capture_progress', {})
        console.log('轮询API响应:', result)

        if (result && result.code === 200 && result.data) {
            const data = result.data
            console.log('轮询获取进度详情:', {
                phase: data.phase,
                totalFiles: data.totalFiles,
                completedFiles: data.completedFiles,
                currentFile: data.currentFile,
                fileListLength: data.fileList ? data.fileList.length : 0
            })

            // 更新进度状态 - 确保所有字段都正确更新
            captureProgress.value = {
                phase: data.phase || 'analyzing',
                totalFiles: data.totalFiles || 0,
                completedFiles: data.completedFiles || 0,
                currentFile: data.currentFile || '处理中...',
                fileProgress: data.fileProgress || 0,
                downloadSpeed: data.downloadSpeed || '',
                estimatedTime: data.estimatedTime || '',
                fileList: data.fileList || []
            }

            console.log('更新后的进度状态:', {
                phase: captureProgress.value.phase,
                totalFiles: captureProgress.value.totalFiles,
                completedFiles: captureProgress.value.completedFiles,
                fileListLength: captureProgress.value.fileList.length
            })

            // 如果阶段是complete，停止轮询
            if (data.phase === 'complete') {
                console.log('检测到完成状态，准备停止轮询')
                setTimeout(() => {
                    if (!isCapturing.value) {
                        stopProgressPolling()
                    }
                }, 1000) // 延迟1秒停止，确保最后的状态更新完成
            }
        } else {
            console.log('轮询API无数据或失败:', result)
        }
    } catch (error) {
        console.log('获取进度失败:', error)
    }
}

// 开始进度轮询 - 改进版本
const startProgressPolling = () => {
    console.log('开始进度轮询')

    // 清理之前的轮询
    if (progressPollingInterval) {
        clearInterval(progressPollingInterval)
        progressPollingInterval = null
    }

    // 立即获取一次进度
    getProgress()

    // 设置定时轮询
    progressPollingInterval = setInterval(async () => {
        if (isCapturing.value) {
            await getProgress()
        } else {
            console.log('备份已完成，停止轮询')
            stopProgressPolling()
        }
    }, 800) // 每800ms轮询一次，避免过于频繁

    console.log('进度轮询已启动')
}

// 停止进度轮询 - 改进版本
const stopProgressPolling = () => {
    console.log('停止进度轮询')
    if (progressPollingInterval) {
        clearInterval(progressPollingInterval)
        progressPollingInterval = null
        console.log('进度轮询已停止')
    }
}

// 组件挂载时初始化
onMounted(() => {
    // 确保保存目录正确加载
    const savedDir = localStorage.getItem('pageCapture_saveDirectory')
    if (savedDir && savedDir !== saveDirectory.value) {
        saveDirectory.value = savedDir
        console.log('从本地存储加载保存目录:', savedDir)
    }
})

// 停止备份
const stopCapture = async () => {
    try {
        // 调用后端停止备份的API
        await api('stop_capture', {})

        // 停止轮询
        stopProgressPolling()

        // 重置状态
        isCapturing.value = false
        captureProgress.value.phase = 'stopped'
        captureProgress.value.currentFile = '备份已停止'

        message.warning('备份已停止')
    } catch (error) {
        console.error('停止备份失败:', error)
        message.error('停止备份失败')
    }
}

// 打开保存目录
const openSaveDirectory = async () => {
    try {
        // 调用后端打开文件夹的API
        await api('open_directory', { path: saveDirectory.value })
        message.success('已打开文件夹')
    } catch (error) {
        console.error('打开文件夹失败:', error)
        message.error('打开文件夹失败')
    }
}

// 复制目录路径
const copyDirectoryPath = async () => {
    try {
        // 使用浏览器的剪贴板API
        await navigator.clipboard.writeText(saveDirectory.value)
        message.success('路径已复制到剪贴板')
    } catch (error) {
        console.error('复制路径失败:', error)
        message.error('复制路径失败')
    }
}

// 打开 GitHub 页面
const openGitHub = async () => {
    try {
        await api('open_url', { url: 'https://github.com/adiudiuu/site_backup' })
    } catch (error) {
        console.error('打开 GitHub 失败:', error)
        message.error('打开 GitHub 失败')
    }
}

// Vue 路由离开守卫
onBeforeRouteLeave((to, from, next) => {
    if (isCapturing.value) {
        // 显示确认对话框
        dialog.warning({
            title: '确认离开',
            content: '正在进行网页备份，离开页面将停止当前的备份任务。确定要离开吗？',
            positiveText: '停止并离开',
            negativeText: '继续备份',
            onPositiveClick: () => {
                // 用户选择离开，停止备份
                stopCapture().then(() => {
                    next() // 允许路由跳转
                })
            },
            onNegativeClick: () => {
                // 用户选择继续备份，阻止路由跳转
                next(false)
            },
            onClose: () => {
                // 用户关闭对话框，阻止路由跳转
                next(false)
            }
        })
    } else {
        // 没有正在进行的备份，直接允许离开
        next()
    }
})

// 页面离开前的确认（浏览器刷新/关闭标签页时的备用处理）
const beforeUnloadHandler = (event: BeforeUnloadEvent) => {
    if (isCapturing.value) {
        event.preventDefault()
        event.returnValue = '正在进行备份，确定要离开吗？'
        return '正在进行备份，确定要离开吗？'
    }
}

// 组件挂载时初始化
onMounted(() => {
    // 确保保存目录正确加载
    const savedDir = localStorage.getItem('pageCapture_saveDirectory')
    if (savedDir && savedDir !== saveDirectory.value) {
        saveDirectory.value = savedDir
        console.log('从本地存储加载保存目录:', savedDir)
    }

    // 添加浏览器页面刷新/关闭的监听（作为备用）
    window.addEventListener('beforeunload', beforeUnloadHandler)
})

// 组件卸载时清理轮询和监听器
onUnmounted(() => {
    stopProgressPolling()
    // 清理浏览器事件监听
    window.removeEventListener('beforeunload', beforeUnloadHandler)
})


</script>

<style scoped>
:deep(.n-card .n-card__header) {
    padding-bottom: 12px;
}

:deep(.n-descriptions .n-descriptions-item) {
    padding: 6px 0;
}

:deep(.n-form-item) {
    margin-bottom: 16px;
}

.documentation-content {
    line-height: 1.6;
    font-size: 14px;
}

.documentation-content h2 {
    color: #2080f0;
    border-bottom: 2px solid #2080f0;
    padding-bottom: 8px;
    margin-bottom: 16px;
}

.documentation-content h3 {
    color: #18a058;
    margin-top: 24px;
    margin-bottom: 12px;
}

.documentation-content h4 {
    color: #f0a020;
    margin-top: 16px;
    margin-bottom: 8px;
}

.documentation-content ul,
.documentation-content ol {
    margin-left: 20px;
    margin-bottom: 12px;
}

.documentation-content li {
    margin-bottom: 4px;
}

.documentation-content code {
    background-color: #f5f5f5;
    padding: 2px 6px;
    border-radius: 4px;
    font-family: 'Courier New', monospace;
    color: #d03050;
}

/* 进度卡片样式 */
.progress-card {
    border: 1px solid #e0e7ff !important;
    background: linear-gradient(135deg, #f8faff 0%, #f1f5ff 100%) !important;
}

.overall-progress {
    padding: 16px;
    background: white;
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.progress-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
}

.phase-text {
    font-weight: 600;
    color: #1f2937;
    font-size: 16px;
}

.progress-stats {
    color: #6b7280;
    font-size: 14px;
    font-weight: 500;
}

.progress-info {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-top: 8px;
    font-size: 12px;
    color: #6b7280;
}

.current-file {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.download-speed {
    color: #059669;
    font-weight: 500;
}

/* 页面布局样式 - 使用栅格布局 */
.page-capture-container {
    padding: 20px;
    max-width: 1400px;
    margin: 0 auto;
}

/* 响应式调整 */
@media (max-width: 768px) {
    .page-capture-container {
        padding: 10px;
    }
}

.header-card {
    margin-bottom: 20px;
}

.header-content {
    display: flex;
    align-items: center;
    gap: 12px;
    justify-content: space-between;
    width: 100%;
}

.header-links {
    margin-left: auto;
}

.header-title {
    font-size: 20px;
    font-weight: 600;
    color: #1f2937;
}

/* 响应式标题 */
@media (max-width: 768px) {
    .header-title {
        font-size: 18px;
    }
    
    .header-content {
        flex-direction: column;
        align-items: flex-start;
        gap: 8px;
    }
    
    .header-links {
        margin-left: 0;
        align-self: flex-end;
    }
}

.action-buttons {
    margin-top: 16px;
}

.status-card {
    height: fit-content;
}

.status-card.empty {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 300px;
}

/* 响应式空状态 */
@media (max-width: 768px) {
    .status-card.empty {
        min-height: 200px;
    }
}

.empty-state {
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
}

.overall-progress {
    padding: 16px;
    background: #f8faff;
    border-radius: 8px;
    border: 1px solid #e0e7ff;
}

.progress-info {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
}

.progress-label {
    font-weight: 600;
    color: #1f2937;
}

.progress-count {
    color: #6b7280;
    font-size: 14px;
}

.file-statistics {
    margin-top: 16px;
}

/* 简化的文件列表样式 */
.file-list-section {
    margin-top: 0;
}

.file-name-cell {
    display: flex;
    align-items: center;
    min-width: 0;
}

.file-name-text {
    font-weight: 500;
    color: #1f2937;
    font-size: 13px;
}

.url-text {
    display: block;
    max-width: 300px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    cursor: help;
}

/* 响应式 URL 文本 */
@media (max-width: 768px) {
    .url-text {
        max-width: 200px;
    }
}

@media (max-width: 480px) {
    .url-text {
        max-width: 150px;
    }
}

.file-icon-small {
    color: #6b7280;
}

.file-size-text {
    font-size: 13px;
    color: #6b7280;
    font-weight: 500;
}

.status-cell {
    display: flex;
    align-items: center;
}

.status-text {
    font-size: 12px;
    font-weight: 500;
}

.progress-cell {
    display: flex;
    flex-direction: column;
    gap: 4px;
}

.progress-percentage {
    text-align: center;
}

.progress-text {
    font-size: 12px;
    font-weight: 500;
    color: #f59e0b;
}

/* 结果文件列表样式 */
.result-file-list {
    margin-top: 16px;
}

.result-file-list .file-items {
    max-height: 400px;
    overflow-y: auto;
    border: 1px solid #e5e7eb;
    border-radius: 6px;
    background: white;
}

.status-text {
    font-size: 12px;
    font-weight: 500;
}

.status-text.success {
    color: #10b981;
}

.status-text.error {
    color: #ef4444;
}

.status-text.pending {
    color: #6b7280;
}

.file-statistics {
    margin-top: 16px;
}
</style>