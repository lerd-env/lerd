// Plugin pages run sandboxed, so lerd.api reaches this plugin's own endpoints
// through the dashboard. lerd.context holds the site and the dashboard's language.
window.lerd = (() => {
  const params = new URLSearchParams(location.search);
  const pending = new Map();
  const edits = new Map();
  const terminals = new Map();
  let maximizing = null;
  let next = 0;

  window.addEventListener('message', (e) => {
    const msg = e.data;
    if (e.source !== window.parent || !msg) return;
    if (msg.type === 'lerd:maximized' && maximizing) {
      clearTimeout(maximizing.timer);
      maximizing.resolve(!!msg.on);
      maximizing = null;
      return;
    }
    const term = terminals.get(msg.id);
    if (term && msg.type === 'lerd:terminal:opened' && e.ports[0]) {
      terminals.delete(msg.id);
      clearTimeout(term.timer);
      return term.resolve(e.ports[0]);
    }
    const ed = edits.get(msg.id);
    if (ed && msg.type === 'lerd:edit:opened') return clearTimeout(ed.timer);
    if (ed && msg.type === 'lerd:edit:result') {
      edits.delete(msg.id);
      return ed.resolve(typeof msg.content === 'string' ? msg.content : null);
    }
    if (msg.type !== 'lerd:api:result') return;
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
    },
    // Asks to cover the whole dashboard (true) or go back (false), resolving
    // with where the page ended up. An older lerd never answers, so it rejects
    // after a moment and the page can hide its button.
    maximize(on) {
      return new Promise((resolve, reject) => {
        if (maximizing) {
          clearTimeout(maximizing.timer);
          maximizing.reject(new Error('superseded'));
        }
        const mine = { resolve, reject };
        mine.timer = setTimeout(() => {
          if (maximizing === mine) maximizing = null;
          reject(new Error('this lerd cannot maximize plugin pages'));
        }, 1500);
        maximizing = mine;
        window.parent.postMessage({ type: 'lerd:maximize', on: !!on }, '*');
      });
    },
    // Opens a pty endpoint, path relative like api's, and resolves with a
    // MessagePort; docs/features/plugins.md lists its messages. An older lerd
    // never answers, so it rejects after a moment.
    terminal(path, { cols = 80, rows = 24 } = {}) {
      const id = ++next;
      const sep = path.includes('?') ? '&' : '?';
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          terminals.delete(id);
          reject(new Error('this lerd has no terminals for plugin pages'));
        }, 1500);
        terminals.set(id, { resolve, timer });
        window.parent.postMessage({ type: 'lerd:terminal', id, path: path + sep + 'cols=' + cols + '&rows=' + rows }, '*');
      });
    },
    // Opens a file in the dashboard's editor and resolves with the saved text,
    // or null when the user cancels. An older lerd never answers, so it
    // rejects after a moment and the page can offer its own editor instead.
    edit({ path, content, label }) {
      const id = ++next;
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          edits.delete(id);
          reject(new Error('this lerd has no editor for plugin pages'));
        }, 1500);
        edits.set(id, { resolve, timer });
        window.parent.postMessage({ type: 'lerd:edit', id, path, content, label }, '*');
      });
    }
  };
})();
