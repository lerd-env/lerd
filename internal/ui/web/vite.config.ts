import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { resolve } from 'node:path';

export default defineConfig(() => ({
  base: '/',
  plugins: [svelte()],
  resolve: {
    alias: {
      $lib: resolve(import.meta.dirname, 'src/lib'),
      $components: resolve(import.meta.dirname, 'src/components'),
      $stores: resolve(import.meta.dirname, 'src/stores'),
      $tabs: resolve(import.meta.dirname, 'src/tabs')
    },
    conditions: ['browser']
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    manifest: true,
    sourcemap: false,
    target: 'es2022',
    // Embedded in the binary and served from localhost, so chunk size costs nothing.
    chunkSizeWarningLimit: 5000,
    rollupOptions: {
      output: {
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]'
      }
    }
  },
  server: {
    port: 5173,
    // The browser logs script lives with the Go package that embeds it;
    // its tests import it from there.
    fs: { allow: ['.', resolve(import.meta.dirname, '../../browserlogs')] },
    proxy: {
      '/api': { target: 'http://localhost:7073', changeOrigin: true, ws: true },
      '/icons': 'http://localhost:7073',
      '/plugins': 'http://localhost:7073',
      '/manifest.webmanifest': 'http://localhost:7073',
      '/sw.js': 'http://localhost:7073',
      '/offline.html': 'http://localhost:7073'
    }
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
    // A store test resets the module registry and re-imports, paying the
    // transform cost again while every other file is transforming too, which
    // outruns the 5s default on a loaded machine. The assertions themselves
    // finish in milliseconds.
    testTimeout: 20000,
    hookTimeout: 20000,
    // One worker per core means one jsdom and one transform pipeline per core,
    // which on a 32-thread machine is several gigabytes of resident memory for
    // a suite that finishes just as fast on eight.
    maxWorkers: 8
  }
}));
