import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'node:path';

// The debug bar is one self-contained script lerd-ui hands to site pages, its
// stylesheet and fonts inlined, built next to the dashboard's own files.
export default defineConfig({
  plugins: [svelte()],
  define: { 'process.env.NODE_ENV': JSON.stringify('production') },
  resolve: {
    alias: [
      { find: '$lib', replacement: resolve(__dirname, 'src/lib') },
      { find: '$components', replacement: resolve(__dirname, 'src/components') },
      { find: '$stores', replacement: resolve(__dirname, 'src/stores') },
      { find: '$tabs', replacement: resolve(__dirname, 'src/tabs') }
    ],
    conditions: ['browser']
  },
  build: {
    outDir: 'dist',
    emptyOutDir: false,
    target: 'es2022',
    lib: { entry: 'src/debugbar/main.ts', formats: ['iife'], name: 'lerdDebugbar', fileName: () => 'debugbar.js' },
    rollupOptions: { output: { inlineDynamicImports: true, entryFileNames: 'debugbar.js' } }
  }
});
