import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
export default defineConfig(({ mode }) => ({
  plugins: [svelte()],
  base: './',
  server: { host: '127.0.0.1', strictPort: true },
  build: {
    outDir: mode === 'native' ? 'dist-native' : 'dist',
    rolldownOptions: { external: ['/wails/runtime.js'] },
  },
}));
