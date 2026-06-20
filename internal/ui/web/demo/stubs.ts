// Demo runtime stubs — make the real lerd UI run with no backend.
// Imported FIRST (before App) so window.fetch / WebSocket / open are patched
// before any store ever calls them. Everything below is fixtures + a tiny
// in-memory mock backend so clicking around the demo behaves like the app.
import version from './fixtures/version.json';
import sitesFixture from './fixtures/sites.json';
import servicesFixture from './fixtures/services.json';
import status from './fixtures/status.json';
import accessMode from './fixtures/access-mode.json';
import settings from './fixtures/settings.json';
import phpVersions from './fixtures/php-versions.json';
import nodeVersions from './fixtures/node-versions.json';
import phpInstallable from './fixtures/php-installable.json';
import lanStatus from './fixtures/lan_status.json';
import dumpsStatus from './fixtures/dumps_status.json';
import profilerStatus from './fixtures/profiler_status.json';
import stats from './fixtures/stats.json';
import workersHealth from './fixtures/workers_health.json';

// Demo follows the system theme (auto). Reset any stale value a previous demo
// session may have pinned, so it isn't stuck on a forced light/dark.
try {
  localStorage.setItem('lerd-theme', 'auto');
} catch {
  /* private mode */
}

// Mutable state so mock mutations (e.g. creating a worktree) persist across reloads of the list.
const sites = structuredClone(sitesFixture) as Array<Record<string, unknown>>;
const services = structuredClone(servicesFixture);

// Static GET fixtures keyed by exact path.
const ROUTES: Record<string, unknown> = {
  '/api/version': version,
  '/api/status': status,
  '/api/access-mode': accessMode,
  '/api/settings': settings,
  '/api/php-versions': phpVersions,
  '/api/node-versions': nodeVersions,
  '/api/php-installable': phpInstallable,
  '/api/lan/status': lanStatus,
  '/api/dumps/status': dumpsStatus,
  '/api/devtools/status': { enabled: true },
  '/api/profiler/status': profilerStatus,
  '/api/stats': stats,
  '/api/workers/health': workersHealth,
};

// ---- Example payloads for the editor / REPL tabs ----
// Hosts are the rootless Podman container names on lerd's shared network, not
// 127.0.0.1 — this mirrors the env lerd actually injects (see services env_vars).
const ENV_TEXT = `APP_NAME="Acme"
APP_ENV=local
APP_KEY=base64:0aF3l9Qx7sample0key0not0real0value0here=
APP_DEBUG=true
APP_URL=https://acme.test

LOG_CHANNEL=stack
LOG_LEVEL=debug

DB_CONNECTION=mysql
DB_HOST=lerd-mysql
DB_PORT=3306
DB_DATABASE=acme
DB_USERNAME=root
DB_PASSWORD=lerd

REDIS_HOST=lerd-redis
REDIS_PORT=6379
REDIS_PASSWORD=null

CACHE_STORE=redis
QUEUE_CONNECTION=redis
SESSION_DRIVER=redis

MAIL_MAILER=smtp
MAIL_HOST=lerd-mailpit
MAIL_PORT=1025
MAIL_FROM_ADDRESS="hello@acme.test"

SCOUT_DRIVER=meilisearch
MEILISEARCH_HOST=http://lerd-meilisearch:7700

FILESYSTEM_DISK=s3
AWS_ENDPOINT=http://lerd-rustfs:9000
`;

const NGINX_TEXT = `server {
    listen 443 ssl;
    http2 on;
    server_name acme.test;
    root "/home/dev/code/acme/public";

    ssl_certificate     "/home/dev/.config/lerd/certs/acme.test.crt";
    ssl_certificate_key "/home/dev/.config/lerd/certs/acme.test.key";

    index index.php;
    charset utf-8;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \\.php$ {
        fastcgi_pass unix:/home/dev/.config/lerd/run/php8.4-fpm.sock;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
    }

    location ~ /\\.(?!well-known).* {
        deny all;
    }
}
`;

const PHP_INI_TEXT = `; Lerd-managed php.ini overrides — PHP 8.4
memory_limit = 512M
max_execution_time = 120
upload_max_filesize = 64M
post_max_size = 64M
display_errors = On
error_reporting = E_ALL

[opcache]
opcache.enable = 1
opcache.jit = tracing
opcache.jit_buffer_size = 64M

[xdebug]
xdebug.mode = off
xdebug.start_with_request = trigger
xdebug.client_port = 9003
`;

// Tinker output is framed: \x1e splits blocks, "<line>\x1f" tags the source line.
const TINKER_RESPONSE = {
  ok: true,
  mode: 'tinker',
  stdout: '\x1e2\x1f=> "1247.00"\x1e3\x1f=> 1428\x1e4\x1f=> Illuminate\\Support\\Collection {#42 [2, 4, 6]}',
  stderr: '',
  exit_code: 0,
};

const TINKER_DRAFT = `// Demo REPL — edit and hit Run
$total = Order::where('status', 'paid')->sum('total');
User::count();
collect([1, 2, 3])->map(fn ($n) => $n * 2);`;

const WORKTREE_OPTIONS = {
  default_branch_label: 'main',
  local_branches: ['main', 'staging', 'feature/checkout-flow'],
  remote_branches: ['origin/main', 'origin/develop', 'origin/release/2.0'],
  build_options: [
    { value: 'auto', label: 'Auto-detect (composer + npm)' },
    { value: 'install', label: 'Install dependencies' },
    { value: 'build', label: 'Install + build assets' },
    { value: 'none', label: 'Skip' },
  ],
  build_default: 'auto',
  db_options: [
    { value: 'share', label: 'Share the main database' },
    { value: 'empty', label: 'Fresh empty database' },
    { value: 'reset', label: 'Copy, then reset & migrate' },
  ],
  can_migrate: true,
};

// Pre-seed the Tinker editor for each site so the tab isn't empty.
try {
  for (const s of sites) {
    const key = `tinker:${s.domain}:draft`;
    if (!localStorage.getItem(key)) localStorage.setItem(key, TINKER_DRAFT);
  }
} catch {
  /* ignore */
}

function jsonResponse(data: unknown): Response {
  return new Response(JSON.stringify(data), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}
function textResponse(s: string): Response {
  return new Response(s, { status: 200, headers: { 'content-type': 'text/plain' } });
}

function worktreeAddSSE(qs: URLSearchParams): Response {
  const domain = qs.get('domain') || '';
  const branch = qs.get('new_branch') || qs.get('existing_branch') || 'feature/demo';
  const site = sites.find((s) => s.domain === domain);
  const slug = branch.replace(/[^a-z0-9]+/gi, '-').toLowerCase();
  const wtDomain = `${slug}.${domain}`;
  if (site) {
    if (!site.branch) site.branch = 'main';
    const list = (site.worktrees as Array<Record<string, unknown>>) || [];
    if (!list.some((w) => w.branch === branch)) {
      site.worktrees = [
        ...list,
        {
          branch,
          domain: wtDomain,
          path: `${site.path}/${slug}`,
          lan_share_url: 'https://lerd.sh',
        },
      ];
    }
  }
  const body =
    `event: log\ndata: creating worktree ${branch}…\n\n` +
    `event: log\ndata: ✓ checked out ${branch}\n\n` +
    `event: log\ndata: ✓ wrote nginx vhost · ${wtDomain}\n\n` +
    `event: done\ndata: ${JSON.stringify({ ok: true, branch, domain: wtDomain })}\n\n`;
  return new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } });
}

const realFetch = window.fetch.bind(window);
window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const raw = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
  let path = raw;
  let search = '';
  try {
    const u = new URL(raw, location.href);
    path = u.pathname;
    search = u.search;
  } catch {
    /* keep raw */
  }
  if (path.length > 1 && path.endsWith('/')) path = path.slice(0, -1);
  const method = (init?.method ?? 'GET').toUpperCase();
  const qs = new URLSearchParams(search);

  // Live (mutable) collections
  if (path === '/api/sites') return jsonResponse(sites);
  if (path === '/api/services') return jsonResponse(services);

  // Worktrees
  if (path === '/api/sites/worktree-options') return jsonResponse(WORKTREE_OPTIONS);
  if (path === '/api/sites/worktree-add') return worktreeAddSSE(qs);
  if (path.includes('/worktree:remove')) {
    const domain = qs.get('domain') || path.split('/')[3];
    const branch = qs.get('branch') || '';
    const site = sites.find((s) => s.domain === domain);
    if (site)
      site.worktrees = ((site.worktrees as Array<Record<string, unknown>>) || []).filter(
        (w) => w.branch !== branch
      );
    return jsonResponse({ ok: true });
  }

  // Per-site .env editor — GET reads only; saves/restores fall through to {ok:true}
  if (method === 'GET') {
    if (/\/env\/files$/.test(path)) return jsonResponse(['.env', '.env.example']);
    if (/\/env\/backups\/.+/.test(path)) return textResponse(ENV_TEXT);
    if (/\/env\/backups$/.test(path)) return jsonResponse([]);
    if (/\/env$/.test(path)) return textResponse(ENV_TEXT);
  }

  // Tinker REPL (POST)
  if (/\/tinker$/.test(path)) return jsonResponse(TINKER_RESPONSE);

  // Nginx — global /api/nginx and per-site /api/sites/<domain>/nginx (+ /backups)
  if (path.includes('/nginx')) {
    if (path.includes('/backups')) return /\/backups\/.+/.test(path) ? textResponse(NGINX_TEXT) : jsonResponse([]);
    if (method === 'GET') return jsonResponse({ path: '/home/dev/.config/lerd/nginx/acme.test.conf', content: NGINX_TEXT, exists: true });
    return jsonResponse({ ok: true, content: NGINX_TEXT, exists: true });
  }
  // php.ini config (per PHP version, or per-site for FrankenPHP) — GET reads only
  if (method === 'GET' && /\/php-versions\/[^/]+\/config$/.test(path))
    return jsonResponse({ path: '~/.config/lerd/php/8.4/php.ini', content: PHP_INI_TEXT, exists: true });

  // Static fixtures
  if (path in ROUTES) return jsonResponse(ROUTES[path]);

  // Toggles / actions: pretend they succeeded so the UI flips optimistically.
  if (method !== 'GET' && method !== 'HEAD') return jsonResponse({ ok: true });
  // Anything else we didn't fixture (push keys, favicons, …) — harmless empty.
  if (path.startsWith('/api/')) return jsonResponse({});
  return realFetch(input as RequestInfo, init);
};

// External opens (a site's .test URL, a service dashboard) have no server in the
// demo — route them to a styled mockup page instead of a dead tab. lerd.sh links
// (docs, the LAN-share QR target) open for real.
const realOpen = window.open.bind(window);
(window as unknown as { open: typeof window.open }).open = ((
  url?: string | URL,
  target?: string,
  features?: string
) => {
  const u = String(url ?? '');
  if (/lerd\.sh/.test(u) || u === '' || u.startsWith('#')) return realOpen(url, target, features);
  let host = u;
  try {
    host = new URL(u, location.href).host || u;
  } catch {
    /* keep u */
  }
  return realOpen(
    `preview.html?host=${encodeURIComponent(host)}&url=${encodeURIComponent(u)}`,
    '_blank',
    'noopener'
  );
}) as typeof window.open;

// ---- Debug window test data ----
// The Debug tab consumes a shared EventSource at /api/dumps/stream; each message
// is a JSON DumpEvent (kind = dump|query|job|view|mail|cache|event|http). We
// emit a couple of realistic request traces so every lens has content.
function debugEvents(): Array<Record<string, unknown>> {
  const now = Date.now();
  let n = 0;
  const make = (
    over: number,
    site: string,
    domain: string,
    rid: string,
    request: string,
    kind: string,
    extra: Record<string, unknown>
  ) => ({
    v: 1,
    id: `${kind}-${site}-${++n}`,
    ts: new Date(now - over).toISOString(),
    kind,
    ctx: { type: 'fpm', site, domain, request, rid },
    src: extra.src ?? { file: 'app/Http/Controllers/OrderController.php', line: 48 },
    label: extra.label,
    text: extra.text,
    data: extra.data,
  });
  const out: Array<Record<string, unknown>> = [];
  // acme — a full Laravel request lifecycle (covers all eight lenses)
  const A = (kind: string, extra: Record<string, unknown>, over: number) =>
    out.push(make(over, 'acme', 'acme.test', 'req-acme-1', 'GET /orders/42', kind, extra));
  A('query', { data: { sql: 'select * from `orders` where `id` = ?', bindings: [42], time_ms: 0.6, connection: 'mysql', rw_type: 'read' }, src: { file: 'app/Models/Order.php', line: 31 } }, 9000);
  A('query', { data: { sql: 'select * from `order_items` where `order_id` in (?, ?, ?)', bindings: [42, 43, 44], time_ms: 1.4, connection: 'mysql', rw_type: 'read' }, src: { file: 'app/Models/Order.php', line: 52 } }, 8800);
  A('dump', { text: 'App\\Models\\Order {#812\n  id: 42,\n  total: "249.00",\n  status: "shipped",\n}', src: { file: 'app/Http/Controllers/OrderController.php', line: 48 } }, 8600);
  A('event', { data: { name: 'App\\Events\\OrderShipped', listeners: 2 }, src: { file: 'app/Events/OrderShipped.php', line: 18 } }, 8400);
  A('mail', { data: { subject: 'Your order has shipped', from: ['shop@acme.test'], to: ['jane@acme.test'] }, src: { file: 'app/Mail/OrderShipped.php', line: 22 } }, 8200);
  A('job', { data: { class: 'App\\Jobs\\SendShipmentNotification', status: 'processed', connection: 'redis' }, src: { file: 'app/Jobs/SendShipmentNotification.php', line: 14 } }, 8000);
  A('http', { data: { method: 'POST', url: 'https://api.stripe.com/v1/charges', status: 200 }, src: { file: 'app/Services/Billing.php', line: 67 } }, 7800);
  A('view', { data: { name: 'orders.show', path: 'resources/views/orders/show.blade.php', data_keys: ['order', 'user', 'items'] }, src: { file: 'resources/views/orders/show.blade.php', line: 1 } }, 7600);
  A('cache', { data: { op: 'hit', key: 'user.42', store: 'redis' }, src: { file: 'app/Http/Middleware/Authenticate.php', line: 20 } }, 7400);
  // shopfront — a Symfony request (no cache lens)
  const S = (kind: string, extra: Record<string, unknown>, over: number) =>
    out.push(make(over, 'shopfront', 'shopfront.test', 'req-shop-1', 'GET /cart', kind, extra));
  S('query', { data: { sql: 'SELECT t0.* FROM cart t0 WHERE t0.id = ?', bindings: [7], time_ms: 0.9, connection: 'pgsql', rw_type: 'read' }, src: { file: 'src/Repository/CartRepository.php', line: 40 } }, 5000);
  S('dump', { text: 'App\\Entity\\Cart {#311\n  items: 3,\n  total: "84.00",\n}', src: { file: 'src/Controller/CartController.php', line: 29 } }, 4800);
  S('event', { data: { name: 'kernel.request' }, src: { file: 'src/EventSubscriber/LocaleSubscriber.php', line: 33 } }, 4600);
  S('http', { data: { method: 'GET', url: 'https://api.exchangerate.host/latest', status: 200 }, src: { file: 'src/Service/Fx.php', line: 18 } }, 4400);
  S('mail', { data: { subject: 'You left items in your cart', from: ['shop@shopfront.test'], to: ['sam@shopfront.test'] }, src: { file: 'src/Mailer/CartReminder.php', line: 12 } }, 4200);
  return out;
}

// EventSource that "connects" then replays the canned debug events once.
class DemoEventSource {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;
  readyState = 0;
  onopen: ((ev: unknown) => void) | null = null;
  onmessage: ((ev: unknown) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  private listeners: Record<string, Array<(ev: unknown) => void>> = {};

  constructor(url: string) {
    const isDumps = String(url).includes('/api/dumps/stream');
    setTimeout(() => {
      this.readyState = 1;
      this.emit('open', { type: 'open' });
      if (isDumps) {
        for (const e of debugEvents()) this.emit('message', { data: JSON.stringify(e) });
      }
    }, 0);
  }
  addEventListener(type: string, cb: (ev: unknown) => void) {
    (this.listeners[type] ||= []).push(cb);
  }
  removeEventListener(type: string, cb: (ev: unknown) => void) {
    this.listeners[type] = (this.listeners[type] || []).filter((f) => f !== cb);
  }
  private emit(type: string, ev: unknown) {
    (this as unknown as Record<string, ((e: unknown) => void) | null>)['on' + type]?.(ev);
    (this.listeners[type] || []).forEach((cb) => cb(ev));
  }
  close() {
    this.readyState = 2;
  }
}
(window as unknown as { EventSource: unknown }).EventSource = DemoEventSource;

// A WebSocket that "connects" and then stays quiet — no live pushes, no
// reconnect storm. The UI shows itself as connected and runs off the fixtures.
class DemoWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  readonly CONNECTING = 0;
  readonly OPEN = 1;
  readonly CLOSING = 2;
  readonly CLOSED = 3;
  readyState = 0;
  onopen: ((ev: unknown) => void) | null = null;
  onmessage: ((ev: unknown) => void) | null = null;
  onclose: ((ev: unknown) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  private listeners: Record<string, Array<(ev: unknown) => void>> = {};

  constructor() {
    setTimeout(() => {
      this.readyState = 1;
      const ev = { type: 'open' };
      this.onopen?.(ev);
      (this.listeners['open'] || []).forEach((cb) => cb(ev));
    }, 0);
  }
  addEventListener(type: string, cb: (ev: unknown) => void) {
    (this.listeners[type] ||= []).push(cb);
  }
  removeEventListener(type: string, cb: (ev: unknown) => void) {
    this.listeners[type] = (this.listeners[type] || []).filter((f) => f !== cb);
  }
  send() {
    /* no-op */
  }
  close() {
    this.readyState = 3;
  }
}
(window as unknown as { WebSocket: unknown }).WebSocket = DemoWebSocket;

// The app registers a service worker in main.ts; the demo skips that entirely.
