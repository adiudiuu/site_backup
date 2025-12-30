import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig({
  plugins: [
    vue(),
    AutoImport({
      imports: [
        'vue',
        'vue-router'
      ],
      dts: true
    }),
    Components({
      resolvers: [NaiveUiResolver()],
      dts: true
    })
  ],
  resolve: {
    alias: {
      '@': '/src'
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