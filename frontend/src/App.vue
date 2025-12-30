<template>
    <n-config-provider :theme="null" :theme-overrides="lightThemeOverrides">
        <n-loading-bar-provider>
            <n-message-provider>
                <n-dialog-provider>
                    <n-notification-provider>
                        <router-view />
                    </n-notification-provider>
                </n-dialog-provider>
            </n-message-provider>
        </n-loading-bar-provider>
    </n-config-provider>
</template>

<script setup lang="ts">
import { lightThemeOverrides } from '@/utils/theme'
import { initFontConfig } from '@/utils/font-config'

// 初始化字体配置
initFontConfig()
</script>

<style>
</style>
