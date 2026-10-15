// Runs on the pages of every site this plugin is switched on for, in the site's
// own origin. lerd serves it, and the plugin's endpoints, under /_lerd/plugins/,
// so a plain same-origin fetch reaches them. Keep it small and self-contained:
// it shares the page with the app, so it draws inside a shadow root.
(() => {
  const name = document.currentScript.src.split('/').slice(-2, -1)[0];
  const host = document.createElement('div');
  const root = host.attachShadow({ mode: 'closed' });
  root.innerHTML = `
    <style>
      button { position: fixed; right: 12px; bottom: 12px; z-index: 2147483647;
        font: 12px system-ui, sans-serif; padding: 6px 10px; border-radius: 999px;
        border: 0; background: #1f2937; color: #fff; cursor: pointer; opacity: .85; }
    </style>
    <button type="button">${name}</button>`;
  const button = root.querySelector('button');
  button.addEventListener('click', async () => {
    const res = await fetch(`/_lerd/plugins/${name}/api/greet`);
    button.textContent = res.ok ? (await res.text()).trim() : `${name}: ${res.status}`;
  });
  document.addEventListener('DOMContentLoaded', () => document.body.appendChild(host));
})();
