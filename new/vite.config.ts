import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // Keep the framework cacheable when application code changes between releases.
    target: 'es2020',
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          return id.includes('/node_modules/react') ? 'react' : undefined
        },
      },
    },
  },
})
