import { mount } from 'svelte';
import App from './App.svelte';
import './app.css';
import { initTheme } from '$stores/theme';

initTheme();

if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    // Page was already under an SW: any new worker that activates is an update,
    // so reload once to drop the stale shell/cache. Without this a released
    // sw.js only takes effect after a manual hard-reload, which silently breaks
    // new same-origin routes (e.g. the /_svc/ dashboard proxies) until cleared.
    const wasControlled = !!navigator.serviceWorker.controller;
    let reloading = false;
    navigator.serviceWorker
      .register('/sw.js')
      .then((reg) => {
        console.info('[lerd] SW registered, scope=', reg.scope);
        void reg.update();
        reg.addEventListener('updatefound', () => {
          const nw = reg.installing;
          if (!nw) return;
          nw.addEventListener('statechange', () => {
            if (nw.state === 'activated' && wasControlled && !reloading) {
              reloading = true;
              window.location.reload();
            }
          });
        });
      })
      .catch((err) => {
        console.warn('[lerd] SW registration failed:', err);
      });
  });
} else {
  console.warn('[lerd] serviceWorker unavailable (insecure context? private mode?)');
}

const target = document.getElementById('app');
if (!target) throw new Error('missing #app root');

export default mount(App, { target });
