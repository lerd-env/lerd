import { defineConfig } from 'vite';
import { svelte, vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/postcss';
import { resolve } from 'node:path';

const r = (p: string) => resolve(__dirname, p);

// Swap specific real components for demo-only versions (e.g. the LAN-share QR,
// whose image is daemon-served). Matches the import by filename so the relative
// path inside the app doesn't matter.
const demoOverrides = {
  name: 'demo-overrides',
  enforce: 'pre' as const,
  resolveId(source: string) {
    if (source.endsWith('/LANShareLink.svelte')) return r('demo/overrides/LANShareLink.svelte');
    return null;
  },
};

// Builds the real lerd UI (src/) as a standalone, backend-free demo bundle
// and drops it into the docs site at docs/public/demo so the marketing page
// can iframe the genuine app. Fixtures + stubs live under demo/.
export default defineConfig({
  root: r('demo'),
  base: '/demo/',
  plugins: [demoOverrides, svelte({ preprocess: vitePreprocess() })],
  css: { postcss: { plugins: [tailwindcss()] } },
  resolve: {
    alias: {
      $src: r('src'),
      $lib: r('src/lib'),
      $components: r('src/components'),
      $stores: r('src/stores'),
      $tabs: r('src/tabs'),
    },
    conditions: ['browser'],
  },
  build: {
    outDir: r('../../../docs/public/demo'),
    emptyOutDir: true,
    target: 'es2022',
    assetsDir: 'assets',
    sourcemap: false,
  },
});
