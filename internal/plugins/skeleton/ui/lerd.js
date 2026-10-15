// lerd runs plugin pages in a sandboxed frame, so they reach the API through the
// dashboard rather than fetching it themselves. lerd.api only reaches this
// plugin's own endpoints; lerd.context holds the site a site tab is showing.
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

  return {
    context: {
      site: params.get('site') || '',
      domain: params.get('domain') || '',
      branch: params.get('branch') || ''
    },
    api(path, { method = 'GET', body } = {}) {
      const id = ++next;
      return new Promise((resolve, reject) => {
        pending.set(id, { resolve, reject });
        window.parent.postMessage({ type: 'lerd:api', id, method, path, body }, '*');
      });
    }
  };
})();
