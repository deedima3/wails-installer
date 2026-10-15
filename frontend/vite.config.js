import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  root: 'demo',
  build: { outDir: '../demo-dist', emptyOutDir: true },
  test: { root: '.', include: ['tests/**/*.test.ts'] }
});
