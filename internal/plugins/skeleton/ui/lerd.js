// Plugin pages run sandboxed, so lerd.api reaches this plugin's own endpoints
// through the dashboard. lerd.context holds the site and the dashboard's language.
window.lerd = (() => {
  const params = new URLSearchParams(location.search);
  const pending = new Map();
  let next = 0;

  window.addEventListener('message', (e) => {
    const msg = e.data;
    if (e.source !== window.parent || !msg || msg.type !== 'lerd:api:result') return;
    const p = pending.get(msg.id);
    if (!p) return;
    pending.delete(msg.id);
    msg.status < 400 ? p.resolve(msg.body) : p.reject(new Error(msg.body));
  });

  // An Overview card has no height of its own, so tell the dashboard how tall
  // the page is whenever that changes.
  new ResizeObserver(() => {
    window.parent.postMessage({ type: 'lerd:resize', height: document.documentElement.scrollHeight }, '*');
  }).observe(document.documentElement);

  return {
    context: {
      site: params.get('site') || '',
      domain: params.get('domain') || '',
      branch: params.get('branch') || '',
      // The dashboard's language, like en, ro or zh-TW, for a page that
      // translates itself.
      lang: params.get('lang') || 'en'
    },
    api(path, { method = 'GET', body } = {}) {
      const id = ++next;
      return new Promise((resolve, reject) => {
        pending.set(id, { resolve, reject });
        window.parent.postMessage({ type: 'lerd:api', id, method, path, body }, '*');
      });
    },
    // Opens the dashboard's own diff on two versions of a file, each side named
    // by before and after, such as the server and this site.
    diff({ path, original, modified, before, after }) {
      window.parent.postMessage({ type: 'lerd:diff', path, original, modified, before, after }, '*');
    },
    // Opens one file read only in the dashboard's editor, label naming where it is.
    view({ path, content, label }) {
      window.parent.postMessage({ type: 'lerd:view', path, content, label }, '*');
    }
  };
})();
