import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    host: '0.0.0.0',
    port: 5173,
    hmr: {
      clientPort: 5173,
      protocol: 'ws'
    },
    proxy: {
      '/config': {
        target: 'http://webrtc-server:8080',
        changeOrigin: true
      },
      '/offer': {
        target: 'http://webrtc-server:8080',
        changeOrigin: true
      },
      '/ice-candidate': {
        target: 'http://webrtc-server:8080',
        changeOrigin: true
      },
      '/synth-code': {
        target: 'http://webrtc-server:8080',
        changeOrigin: true
      },
      '/stop': {
        target: 'http://webrtc-server:8080',
        changeOrigin: true
      }
    }
  },
  build: {
    sourcemap: true,
    outDir: 'dist',
    assetsDir: 'assets'
  }
}); 
