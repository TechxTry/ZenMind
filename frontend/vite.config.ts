import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { readFileSync, existsSync } from 'node:fs'
import { resolve } from 'node:path'

const repoRoot = resolve(__dirname, '..')

function readAppVersion(): string {
  const candidates = [
    resolve(__dirname, '.app-version'),
    resolve(repoRoot, 'VERSION'),
  ]
  for (const p of candidates) {
    if (existsSync(p)) {
      return readFileSync(p, 'utf8').trim()
    }
  }
  return 'dev'
}

const appVersion = readAppVersion()
const githubUrl = 'https://github.com/TechxTry/ZenMind'

export default defineConfig({
  plugins: [react()],
  define: {
    __ZENMIND_VERSION__: JSON.stringify(appVersion),
    __ZENMIND_GITHUB_URL__: JSON.stringify(githubUrl),
  },
  build: {
    sourcemap: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
