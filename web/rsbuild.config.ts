import path from 'node:path'

import { defineConfig } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'

export default defineConfig({
  plugins: [pluginReact(), pluginTailwindcss()],
  source: { entry: { index: './src/main.tsx' } },
  resolve: { alias: { '@': path.resolve(import.meta.dirname, './src') } },
  html: { template: './index.html' },
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:3000',
      '/auth': 'http://127.0.0.1:3000',
      '/v1': 'http://127.0.0.1:3000',
      '/openapi.json': 'http://127.0.0.1:3000',
    },
  },
})
