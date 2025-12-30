import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import VueSetupExtend from 'vite-plugin-vue-setup-extend'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig({
  plugins: [
    vue(),
    VueSetupExtend(),
    AutoImport({
      resolvers: [ElementPlusResolver()]
    }),
    Components({
      resolvers: [ElementPlusResolver()]
    })
  ],
  resolve: {
    alias: {
      '@': '/src',
      '~': '/src/assets'
    }
  },
  define: {
    __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: "true",
  },
  // Wails 特定配置
  server: {
    host: 'localhost',
    port: 34115,
    strictPort: true,
    watch: { usePolling: true }
  },
  build: {
    target: 'esnext',
    minify: 'esbuild',
    sourcemap: false,
    rollupOptions: {
      external: ['./wailsjs/go/models']
    }
  }
})