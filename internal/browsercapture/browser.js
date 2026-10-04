(function () {
  // Injected by lerd. Reports this page's JavaScript errors to the lerd dashboard.
  if (window.__lerdBrowserCapture) return;
  window.__lerdBrowserCapture = true;
  var cfg = __LERD_CONFIG__;
  var endpoint = cfg.endpoint;
  var queue = [], seen = {}, sent = 0, linked = 0, timer = null, MAX = 50;
  // One id per page view, a SPA navigation included, so the dashboard groups
  // each view's events together. The first view takes the id of the request
  // that served the page, which nginx put on this script's tag.
  var docRid = (document.currentScript && document.currentScript.getAttribute('data-rid')) || '';
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
    try { return JSON.stringify(simplify(v, 0, [])); } catch (e) { return String(v); }
  }

  // simplify turns a value into something JSON can show and a reader can use:
  // errors, responses and DOM nodes by what identifies them, Map and Set by
  // their contents, a cycle as [Circular], kept to 3 levels and 20 entries.
  function simplify(v, depth, seen) {
    if (v === null || typeof v !== 'object' && typeof v !== 'function') return v === undefined ? null : v;
    if (typeof v === 'function') return '[Function ' + (v.name || 'anonymous') + ']';
    if (v instanceof Error) return v.name + ': ' + v.message;
    if (typeof Response !== 'undefined' && v instanceof Response) return v.status + ' ' + v.url;
    if (typeof XMLHttpRequest !== 'undefined' && v instanceof XMLHttpRequest) return v.status + ' ' + v.responseURL;
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

  // timing reads a navigation or resource entry as milliseconds from its own
  // start, the phases the waterfall draws. A cross-origin entry without
  // Timing-Allow-Origin has no phases, so it yields nothing.
  var PHASES = ['domainLookupStart', 'domainLookupEnd', 'connectStart', 'secureConnectionStart', 'connectEnd', 'requestStart', 'responseStart', 'responseEnd', 'domContentLoadedEventEnd', 'loadEventEnd'];
  function timing(e) {
    if (!e || !(e.requestStart > 0)) return undefined;
    var t = {};
    PHASES.forEach(function (k) { if (e[k] > 0) t[k] = Math.round((e[k] - e.startTime) * 10) / 10; });
    return t;
  }
  // A fetch's resource entry only exists once its body is read, so a linked
  // request's phases are looked up when the batch goes out.
  function entryFor(url, started) {
    if (!window.performance || !performance.getEntriesByName) return undefined;
    var list = performance.getEntriesByName(url).filter(function (e) { return e.startTime >= started - 1; });
    return list[list.length - 1];
  }

  function flush() {
    timer = null;
    if (!queue.length) return;
    queue.forEach(function (ev) {
      if (ev.lookup) ev.timing = timing(entryFor(ev.lookup[0], ev.lookup[1]));
      delete ev.lookup;
    });
    var body = JSON.stringify(queue);
    queue = [];
    var blob = new Blob([body], { type: 'application/json' });
    if (!(navigator.sendBeacon && navigator.sendBeacon(endpoint, blob)) && origFetch) {
      origFetch.call(window, endpoint, { method: 'POST', body: body, keepalive: true, headers: { 'Content-Type': 'application/json' } }).catch(function () {});
    }
  }

  function report(ev) {
    var key = ev.type + '|' + ev.message + '|' + (ev.file || '') + ':' + (ev.line || 0) + '|' + (ev.rid || '');
    // Linked requests have a budget of their own, so a chatty page cannot use
    // up the one its errors need.
    if (seen[key] || (ev.type === 'request' ? linked++ >= MAX : sent >= MAX)) return;
    seen[key] = true;
    if (ev.type !== 'request') sent++;
    ev.message = cut(ev.message, 2000);
    if (ev.stack) ev.stack = cut(ev.stack, 8000);
    ev.url = location.href;
    ev.page = page;
    ev.ua = navigator.userAgent;
    ev.at = new Date().toISOString();
    if (cfg.verbose) say('captured ' + (ev.level ? 'console.' + ev.level : ev.type) + ':', ev.message);
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
  // network reports one finished fetch or XHR. A response that names the PHP
  // request behind it (X-Lerd-Rid) is reported as a linked request whatever its
  // status, so the dashboard can list what the page sent. A failure is also
  // reported on its own when the site asks for that class. Status 0 means no
  // response at all; the browser hides why (CORS, refused, DNS, offline look
  // the same to a script), so only the facts are reported.
  function network(method, url, status, rid, started, via) {
    method = String(method || 'GET').toUpperCase();
    url = String(url);
    var cross = crossOrigin(url);
    var ms = Math.round((now() - started) * 10) / 10;
    if (rid) {
      var abs = url;
      try { abs = new URL(url, location.href).href; } catch (e) {}
      report({ type: 'request', via: via, method: method, request: url, status: status, rid: rid, cross: cross, duration_ms: ms, message: status + ' ' + method + ' ' + url, lookup: [abs, started] });
    }
    if (!failed(status)) return;
    report({
      type: 'network', method: method, request: url, status: status, cross: cross, rid: rid, duration_ms: ms,
      message: (status === 0 ? 'no response ' + (cross ? '(cross-origin) ' : '') : status + ' ') + method + ' ' + url
    });
  }
  function now() { return window.performance && performance.now ? performance.now() : Date.now(); }

  var origFetch = window.fetch;
  if (origFetch) {
    window.fetch = function (input, init) {
      var method = (init && init.method) || (input && input.method) || 'GET';
      var url = typeof input === 'string' ? input : (input && input.url) || String(input);
      var started = now();
      return origFetch.apply(this, arguments).then(function (res) {
        network(method, url, res.status, res.headers.get('X-Lerd-Rid') || '', started, 'fetch');
        return res;
      }, function (err) {
        if (!err || err.name !== 'AbortError') network(method, url, 0, '', started, 'fetch');
        throw err;
      });
    };
  }
  if (window.XMLHttpRequest) {
    var open = XMLHttpRequest.prototype.open, send = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function (method, url) {
      this.__lerd = [method, url];
      return open.apply(this, arguments);
    };
    XMLHttpRequest.prototype.send = function () {
      var xhr = this, aborted = false, started = now();
      if (xhr.__lerd) {
        xhr.addEventListener('abort', function () { aborted = true; });
        xhr.addEventListener('loadend', function () {
          if (aborted) return;
          var rid = '';
          try { rid = xhr.getResponseHeader('X-Lerd-Rid') || ''; } catch (e) {}
          network(xhr.__lerd[0], xhr.__lerd[1], xhr.status, rid, started, 'xhr');
        });
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
    page = how === 'load' && docRid ? docRid : Date.now().toString(36) + Math.random().toString(36).slice(2, 10);
    seen = {};
    sent = 0;
    linked = 0;
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

  // How the page itself loaded, from DNS to the load event, tied to the request
  // that served it so its waterfall can draw the browser's side.
  function loaded() {
    var nav = window.performance && performance.getEntriesByType && performance.getEntriesByType('navigation')[0];
    var t = timing(nav);
    if (t) report({ type: 'timing', message: 'page loaded in ' + Math.round(t.loadEventEnd || t.responseEnd) + ' ms', timing: t, origin: Math.round(performance.timeOrigin) });
  }
  if (docRid) {
    if (document.readyState === 'complete') setTimeout(loaded, 0);
    else window.addEventListener('load', function () { setTimeout(loaded, 0); });
  }

  var watching = ['uncaught errors', 'unhandled rejections']
    .concat((cfg.console || []).map(function (l) { return 'console.' + l; }))
    .concat(classes.map(function (c) { return c + ' requests'; }))
    .concat(cfg.resources ? ['failed resources'] : [])
    .concat((cfg.events || []).map(function (d) { return d.event; }))
    .concat(cfg.navigation ? ['page views'] : []);
  say('browser capture active, reporting ' + watching.join(', ') + ' to the lerd dashboard');
})();
