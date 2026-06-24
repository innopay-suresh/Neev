import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    proxy: {
      '/ws': {
        target: 'http://server:8080',
        ws: true,
      },
      '/api': {
        target: 'http://server:8080',
      }
    }
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor':  ['react', 'react-dom'],
          'motion':        ['framer-motion'],
          'icons':         ['lucide-react'],
        },
      },
    },
  },
  define: {
    'import.meta.env.VITE_RELAY_URL': JSON.stringify(process.env.VITE_RELAY_URL || ''),
  },
})
