import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from "@tailwindcss/vite"
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    // Let Vite handle chunking automatically to avoid React bundling issues
    // Manual chunking was causing "Cannot read properties of undefined" errors
    // when React-dependent packages were split across different chunks
    chunkSizeWarningLimit: 1500,
    reportCompressedSize: true,
  },
  server: {
    proxy: {
      // REST API proxy (for backward compatibility)
      '/api': {
        target: 'http://localhost:8500',
        changeOrigin: true,
        secure: false,
      },
      // gRPC-Web HTTP Gateway proxy
      '/grpc': {
        target: 'http://localhost:8500',
        changeOrigin: true,
        secure: false,
        rewrite: (path) => path.replace(/^\/grpc/, ''),
      },
      // WebSocket proxy (for legacy WebSocket connections)
      '/ws': {
        target: 'ws://localhost:8500',
        ws: true,
        changeOrigin: true,
      },
      // gRPC service proxies
      '/sqlstudio.database.DatabaseService': {
        target: 'http://localhost:8500',
        changeOrigin: true,
        secure: false,
      },
      '/sqlstudio.query.QueryService': {
        target: 'http://localhost:8500',
        changeOrigin: true,
        secure: false,
      },
      '/sqlstudio.table.TableService': {
        target: 'http://localhost:8500',
        changeOrigin: true,
        secure: false,
      },
      '/sqlstudio.auth.AuthService': {
        target: 'http://localhost:8500',
        changeOrigin: true,
        secure: false,
      },
    },
  },
})
