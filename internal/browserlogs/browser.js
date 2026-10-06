(function () {
  // Injected by lerd. Reports this page's JavaScript errors to the lerd dashboard.
  if (window.__lerdBrowserLogs) return;
  window.__lerdBrowserLogs = true;
  var cfg = __LERD_CONFIG__;
  var endpoint = cfg.endpoint;
  var queue = [], seen = {}, sent = 0, timer = null, MAX = 50;
  // One id per page view, a SPA navigation included, so the dashboard groups
  // each view's events together.
  var page = '', lastHref = '';
  // Kept before console is wrapped, so lerd's own lines are never captured.
  var info = console.info.bind(console);
  var badge = ['%clerd', 'background:#ff2d20;color:#fff;border-radius:3px;padding:0 4px'];
  function say() { info.apply(null, badge.concat(Array.prototype.slice.call(arguments))); }

  function cut(s, n) {
    s = String(s == null ? '' : s);
    return s.length > n ? s.slice(0, n) + '…' : s;
  }

  function describe(v) {
    if (v instanceof Error) return v.stack || (v.name + ': ' + v.message);
    if (typeof v === 'string') return v;
    if (v === undefined) return 'undefined';
    try {
      var s = simplify(v, 0, []);
      return typeof s === 'string' ? s : JSON.stringify(s);
    } catch (e) { return String(v); }
  }

  // simplify turns a value into something JSON can show and a reader can use:
  // errors, responses and DOM nodes by what identifies them, Map and Set by
  // their contents, a cycle as [Circular], kept to 3 levels and 20 entries.
  // An axios response (what Inertia's events carry) is a plain object, so it
  // is recognised by shape; its body and headers would bury the URL.
  function isAxiosResponse(v) {
    return typeof v.status === 'number' && v.config && typeof v.config === 'object' && typeof v.config.url === 'string';
  }

  function simplify(v, depth, seen) {
    if (v === null || typeof v !== 'object' && typeof v !== 'function') return v === undefined ? null : v;
    if (typeof v === 'function') return '[Function ' + (v.name || 'anonymous') + ']';
    if (v instanceof Error) return v.name + ': ' + v.message;
    if (typeof Response !== 'undefined' && v instanceof Response) return v.status + ' ' + v.url;
    if (typeof XMLHttpRequest !== 'undefined' && v instanceof XMLHttpRequest) return v.status + ' ' + v.responseURL;
    if (isAxiosResponse(v)) return v.status + ' ' + (v.config.method ? String(v.config.method).toUpperCase() + ' ' : '') + ((v.request && v.request.responseURL) || v.config.url);
    if (v.nodeType === 1) return v.tagName.toLowerCase() + (v.id ? '#' + v.id : '');
    if (typeof v.nodeType === 'number') return v.nodeName;
    if (seen.indexOf(v) >= 0) return '[Circular]';
    if (depth >= 3) return Array.isArray(v) ? '[Array(' + v.length + ')]' : '[Object]';
    seen = seen.concat([v]);
    var list = v instanceof Set ? Array.from(v) : Array.isArray(v) ? v : null;
    if (list) {
      var items = list.slice(0, 20).map(function (x) { return simplify(x, depth + 1, seen); });
      if (list.length > 20) items.push('… ' + (list.length - 20) + ' more');
      return items;
    }
    var entries = v instanceof Map ? Array.from(v.entries()) : Object.keys(v).map(function (k) { return [k, v[k]]; });
    var out = {};
    entries.slice(0, 20).forEach(function (e) { out[String(e[0])] = simplify(e[1], depth + 1, seen); });
    if (entries.length > 20) out['…'] = (entries.length - 20) + ' more';
    return out;
  }

  function flush() {
    timer = null;
    if (!queue.length) return;
    var body = JSON.stringify(queue);
    queue = [];
    var blob = new Blob([body], { type: 'application/json' });
    if (!(navigator.sendBeacon && navigator.sendBeacon(endpoint, blob)) && origFetch) {
      origFetch.call(window, endpoint, { method: 'POST', body: body, keepalive: true, headers: { 'Content-Type': 'application/json' } }).catch(function () {});
    }
  }

  function report(ev) {
    // An identical report within a second is a loop, not the user doing it
    // again, so only that is dropped; MAX still caps a page view.
    var key = ev.type + '|' + ev.message + '|' + (ev.file || '') + ':' + (ev.line || 0);
    var now = Date.now();
    if (now - (seen[key] || -Infinity) < 1000 || sent >= MAX) return;
    seen[key] = now;
    sent++;
    ev.message = cut(ev.message, 2000);
    if (ev.stack) ev.stack = cut(ev.stack, 8000);
    ev.url = location.href;
    ev.page = page;
    ev.ua = navigator.userAgent;
    ev.at = new Date().toISOString();
    queue.push(ev);
    if (!timer) timer = setTimeout(flush, 300);
  }

  window.addEventListener('error', function (e) {
    if (!e.error && !e.message) return;
    report({
      type: 'error',
      message: e.message || describe(e.error),
      stack: e.error && e.error.stack,
      file: e.filename, line: e.lineno, col: e.colno
    });
  });

  window.addEventListener('unhandledrejection', function (e) {
    var r = e.reason;
    report({
      type: 'rejection',
      message: r instanceof Error ? (r.name + ': ' + r.message) : describe(r),
      stack: r instanceof Error ? r.stack : undefined
    });
  });

  (cfg.console || []).forEach(function (level) {
    var orig = console[level];
    if (typeof orig !== 'function') return;
    console[level] = function () {
      var args = Array.prototype.slice.call(arguments);
      var err = args.filter(function (a) { return a instanceof Error; })[0];
      report({ type: 'console', level: level, message: args.map(describe).join(' '), stack: err && err.stack });
      return orig.apply(this, arguments);
    };
  });

  var classes = cfg.network || [];
  function failed(status) {
    return (status >= 400 && status < 500 && classes.indexOf('4xx') >= 0) ||
      (status >= 500 && status < 600 && classes.indexOf('5xx') >= 0) ||
      (status === 0 && classes.indexOf('failed') >= 0);
  }
  function crossOrigin(url) {
    try { return new URL(url, location.href).origin !== location.origin; } catch (e) { return false; }
  }
  // Status 0 means no response at all. The browser hides why (CORS, refused,
  // DNS, offline all look the same to a script), so only the facts are reported.
  function network(method, url, status) {
    if (!failed(status)) return;
    method = String(method || 'GET').toUpperCase();
    url = String(url);
    var cross = crossOrigin(url);
    report({
      type: 'network', method: method, request: url, status: status, cross: cross,
      message: (status === 0 ? 'no response ' + (cross ? '(cross-origin) ' : '') : status + ' ') + method + ' ' + url
    });
  }

  var origFetch = window.fetch;
  if (classes.length && origFetch) {
    window.fetch = function (input, init) {
      var method = (init && init.method) || (input && input.method) || 'GET';
      var url = typeof input === 'string' ? input : (input && input.url) || String(input);
      return origFetch.apply(this, arguments).then(function (res) {
        network(method, url, res.status);
        return res;
      }, function (err) {
        if (!err || err.name !== 'AbortError') network(method, url, 0);
        throw err;
      });
    };
  }
  if (classes.length && window.XMLHttpRequest) {
    var open = XMLHttpRequest.prototype.open, send = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function (method, url) {
      this.__lerd = [method, url];
      return open.apply(this, arguments);
    };
    XMLHttpRequest.prototype.send = function () {
      var xhr = this, aborted = false;
      if (xhr.__lerd) {
        xhr.addEventListener('abort', function () { aborted = true; });
        xhr.addEventListener('loadend', function () { if (!aborted) network(xhr.__lerd[0], xhr.__lerd[1], xhr.status); });
      }
      return send.apply(this, arguments);
    };
  }

  // A failed <img>, <script> or stylesheet fires an error that does not bubble,
  // so it is only seen in the capture phase.
  if (cfg.resources) {
    window.addEventListener('error', function (e) {
      var el = e.target;
      if (!el || el === window || !el.tagName) return;
      var url = el.currentSrc || el.src || el.href || '';
      if (!url) return;
      var tag = el.tagName.toLowerCase();
      report({ type: 'resource', tag: tag, request: url, cross: crossOrigin(url), message: 'failed to load <' + tag + '> ' + url });
    }, true);
  }

  // Events the site asked for, such as a frontend library's error events. A
  // capturing listener on window sees them whether they were dispatched on
  // window, document or an element, and whether or not they bubble.
  function pick(obj, path) {
    var parts = path.split('.');
    for (var i = 0; i < parts.length && obj != null; i++) obj = obj[parts[i]];
    return obj;
  }
  (cfg.events || []).forEach(function (def) {
    window.addEventListener(def.event, function (e) {
      var value = def.message ? pick(e, def.message) : undefined;
      var name = def.label || def.event;
      report({ type: 'event', name: def.event, label: def.label, message: value === undefined ? name : name + ': ' + describe(value) });
    }, true);
  });

  window.addEventListener('pagehide', flush);

  function view(how) {
    if (location.href === lastHref) return;
    lastHref = location.href;
    page = Date.now().toString(36) + Math.random().toString(36).slice(2, 10);
    seen = {};
    sent = 0;
    if (cfg.navigation) report({ type: 'navigation', nav: how, message: location.href });
  }
  ['pushState', 'replaceState'].forEach(function (name) {
    var orig = history[name];
    history[name] = function () {
      var out = orig.apply(this, arguments);
      view(name === 'pushState' ? 'push' : 'replace');
      return out;
    };
  });
  window.addEventListener('popstate', function () { view('pop'); });
  window.addEventListener('hashchange', function () { view('hash'); });
  view('load');

  var watching = ['uncaught errors', 'unhandled rejections']
    .concat((cfg.console || []).map(function (l) { return 'console.' + l; }))
    .concat(classes.map(function (c) { return c + ' requests'; }))
    .concat(cfg.resources ? ['failed resources'] : [])
    .concat((cfg.events || []).map(function (d) { return d.event; }))
    .concat(cfg.navigation ? ['page views'] : []);
  say('browser logs active, reporting ' + watching.join(', ') + ' to the lerd dashboard' + (cfg.lens ? ': ' + cfg.lens : ''));
})();
