// Demo runtime stubs — make the real lerd UI run with no backend.
// Imported FIRST (before App) so window.fetch / WebSocket / open are patched
// before any store ever calls them. Everything below is fixtures + a tiny
// in-memory mock backend so clicking around the demo behaves like the app.
import version from './fixtures/version.json';
import sitesFixture from './fixtures/sites.json';
import servicesFixture from './fixtures/services.json';
import presetsFixture from './fixtures/presets.json';
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
import databasesFixture from './fixtures/databases.json';

// Demo follows the system theme (auto). Reset any stale value a previous demo
// session may have pinned, so it isn't stuck on a forced light/dark.
try {
  localStorage.setItem('lerd-theme', 'auto');
} catch {
  /* private mode */
}

// Mutable state so mock mutations (e.g. creating a worktree) persist across reloads of the list.
const sites = structuredClone(sitesFixture) as Array<Record<string, unknown>>;
const services = structuredClone(servicesFixture) as Array<Record<string, unknown>>;
const presets = structuredClone(presetsFixture) as Array<Record<string, unknown>>;

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

// Per-site request-timing analytics (the site Overview's Request timing view,
// served at /api/sites/<domain>/analytics). The real view reads a durable SQLite
// store; each profile below is expanded into the same shape, with the throughput
// series and recent-request timestamps placed relative to now so the chart and
// list read as live. acme and shopfront run busy with flagged slow routes and a
// cold start; every other site falls back to DEFAULT_ANALYTICS so the panel is
// never empty. Route p95s and the cold flag exercise the severity colours, the
// "cold excluded" note, and the greyed cold row.
const LATENCY_EDGES = [25, 50, 100, 250, 500, 1000];

interface DemoRouteStat {
  route: string;
  method: string;
  example: string;
  p50_millis: number;
  p95_millis: number;
  recent_p95_millis: number;
  multiplier: number;
  samples: number;
}

interface DemoRecent {
  agoSec: number; // seconds before now, so the list reads as live
  method: string;
  route: string;
  uri: string;
  status: number;
  millis: number;
  cold?: boolean;
}

interface AnalyticsProfile {
  samples: number;
  cold_starts: number;
  median_millis: number;
  p95_millis: number;
  status: { c2xx: number; c3xx: number; c4xx: number; c5xx: number };
  distribution: number[]; // one count per LATENCY_EDGES bucket, last is the open >1s bucket
  throughput: number[]; // per-minute counts, oldest first, ending at the current minute
  routes: DemoRouteStat[];
  recent: DemoRecent[]; // newest first
}

// wave builds a smooth per-minute throughput series of length n around avg, so
// each profile gets a realistic curve without a hand-written array.
function wave(n: number, avg: number): number[] {
  return Array.from({ length: n }, (_, i) => Math.max(1, Math.round(avg + avg * 0.5 * Math.sin(i / 2))));
}

const ANALYTICS_PROFILES: Record<string, AnalyticsProfile> = {
  'acme.test': {
    samples: 1846,
    cold_starts: 3,
    median_millis: 72,
    p95_millis: 240,
    status: { c2xx: 1720, c3xx: 88, c4xx: 34, c5xx: 4 },
    distribution: [90, 360, 720, 430, 130, 40, 6],
    throughput: wave(24, 15),
    routes: [
      { route: 'GET /', method: 'GET', example: '/', p50_millis: 78, p95_millis: 150, recent_p95_millis: 138, multiplier: 3.5, samples: 1846 },
      { route: 'POST /checkout', method: 'POST', example: '', p50_millis: 190, p95_millis: 512, recent_p95_millis: 512, multiplier: 12.5, samples: 63 },
      { route: 'GET /orders/:id', method: 'GET', example: '/orders/42', p50_millis: 96, p95_millis: 233, recent_p95_millis: 233, multiplier: 5.7, samples: 214 },
      { route: 'GET /dashboard', method: 'GET', example: '/dashboard', p50_millis: 120, p95_millis: 268, recent_p95_millis: 260, multiplier: 6.4, samples: 96 },
      { route: 'GET /cart', method: 'GET', example: '/cart', p50_millis: 60, p95_millis: 176, recent_p95_millis: 176, multiplier: 4.1, samples: 148 },
      { route: 'GET /products', method: 'GET', example: '/products', p50_millis: 44, p95_millis: 92, recent_p95_millis: 88, multiplier: 1.8, samples: 402 },
    ],
    recent: [
      { agoSec: 4, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 147 },
      { agoSec: 18, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 130 },
      { agoSec: 46, method: 'GET', route: 'GET /products', uri: '/products', status: 200, millis: 70 },
      { agoSec: 62, method: 'POST', route: 'POST /checkout', uri: '/checkout', status: 302, millis: 199 },
      { agoSec: 75, method: 'GET', route: 'GET /cart', uri: '/cart', status: 200, millis: 88 },
      { agoSec: 121, method: 'GET', route: 'GET /orders/:id', uri: '/orders/42', status: 200, millis: 233 },
      { agoSec: 140, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 138 },
      { agoSec: 168, method: 'GET', route: 'GET /dashboard', uri: '/dashboard', status: 200, millis: 268 },
      { agoSec: 205, method: 'GET', route: 'GET /products', uri: '/products', status: 200, millis: 66 },
      { agoSec: 232, method: 'GET', route: 'GET /cart', uri: '/cart', status: 404, millis: 41 },
      { agoSec: 300, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 106 },
      { agoSec: 360, method: 'GET', route: 'GET /orders/:id', uri: '/orders/99', status: 200, millis: 210 },
      { agoSec: 900, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 613, cold: true },
    ],
  },
  'shopfront.test': {
    samples: 921,
    cold_starts: 1,
    median_millis: 44,
    p95_millis: 150,
    status: { c2xx: 900, c3xx: 12, c4xx: 9, c5xx: 0 },
    distribution: [140, 420, 300, 60, 8, 2, 0],
    throughput: wave(24, 9),
    routes: [
      { route: 'GET /', method: 'GET', example: '/', p50_millis: 40, p95_millis: 96, recent_p95_millis: 92, multiplier: 2.4, samples: 921 },
      { route: 'GET /cart', method: 'GET', example: '/cart', p50_millis: 58, p95_millis: 176, recent_p95_millis: 176, multiplier: 3.7, samples: 148 },
      { route: 'GET /catalog', method: 'GET', example: '/catalog', p50_millis: 52, p95_millis: 120, recent_p95_millis: 118, multiplier: 2.9, samples: 260 },
      { route: 'POST /cart/add', method: 'POST', example: '', p50_millis: 70, p95_millis: 150, recent_p95_millis: 150, multiplier: 3.8, samples: 88 },
    ],
    recent: [
      { agoSec: 9, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 62 },
      { agoSec: 33, method: 'GET', route: 'GET /catalog', uri: '/catalog', status: 200, millis: 118 },
      { agoSec: 51, method: 'POST', route: 'POST /cart/add', uri: '/cart/add', status: 200, millis: 150 },
      { agoSec: 88, method: 'GET', route: 'GET /cart', uri: '/cart', status: 200, millis: 176 },
      { agoSec: 140, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 48 },
      { agoSec: 210, method: 'GET', route: 'GET /catalog', uri: '/catalog', status: 200, millis: 96 },
      { agoSec: 720, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 388, cold: true },
    ],
  },
};

// DEFAULT_ANALYTICS is a healthy, populated profile for every other demo site, so
// the panel shows real numbers rather than the empty "watching for requests" card.
const DEFAULT_ANALYTICS: AnalyticsProfile = {
  samples: 816,
  cold_starts: 0,
  median_millis: 30,
  p95_millis: 70,
  status: { c2xx: 804, c3xx: 6, c4xx: 6, c5xx: 0 },
  distribution: [260, 180, 60, 10, 0, 0, 0],
  throughput: wave(24, 6),
  routes: [
    { route: 'GET /', method: 'GET', example: '/', p50_millis: 34, p95_millis: 78, recent_p95_millis: 72, multiplier: 1.9, samples: 420 },
    { route: 'GET /login', method: 'GET', example: '/login', p50_millis: 28, p95_millis: 62, recent_p95_millis: 60, multiplier: 1.6, samples: 96 },
    { route: 'GET /api/health', method: 'GET', example: '/api/health', p50_millis: 8, p95_millis: 18, recent_p95_millis: 16, multiplier: 1.1, samples: 300 },
  ],
  recent: [
    { agoSec: 6, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 34 },
    { agoSec: 24, method: 'GET', route: 'GET /api/health', uri: '/api/health', status: 200, millis: 12 },
    { agoSec: 58, method: 'GET', route: 'GET /login', uri: '/login', status: 200, millis: 60 },
    { agoSec: 132, method: 'GET', route: 'GET /', uri: '/', status: 200, millis: 44 },
    { agoSec: 240, method: 'GET', route: 'GET /api/health', uri: '/api/health', status: 200, millis: 10 },
  ],
};

// Per-site application logs (Logs tab → App logs, the default sub-tab), served
// over REST at /api/app-logs/<domain>[/<file>]. A realistic Laravel run: mostly
// INFO with a WARNING and one ERROR carrying a stack trace, so the expandable
// detail and the level colours both have something to show.
const APP_LOG_FILES = [
  { name: 'laravel.log', size: 48213 },
  { name: 'laravel-2026-07-07.log', size: 15922 },
];

function appLogEntries(): Array<Record<string, unknown>> {
  const now = Date.now();
  const at = (secAgo: number) => new Date(now - secAgo * 1000).toISOString().replace('T', ' ').slice(0, 19);
  return [
    { level: 'INFO', date: at(640), message: 'User authenticated', detail: 'local.INFO: User authenticated {"user_id":42,"guard":"web"}' },
    { level: 'INFO', date: at(600), message: 'Order placed', detail: 'local.INFO: Order placed {"order_id":900,"total":"249.00"}' },
    { level: 'WARNING', date: at(320), message: 'Coupon code not found, ignoring', detail: 'local.WARNING: Coupon code not found, ignoring {"code":"SUMMER"}' },
    { level: 'INFO', date: at(180), message: 'Shipment notification queued', detail: 'local.INFO: Shipment notification queued {"job":"App\\\\Jobs\\\\SendShipmentNotification"}' },
    { level: 'ERROR', date: at(70), message: 'Stripe charge failed: card_declined', detail: 'local.ERROR: Stripe charge failed: card_declined {"exception":"[object] (Stripe\\\\Exception\\\\CardException(code: 402): Your card was declined.)"}\n#0 /app/Services/Billing.php(67): Stripe\\Charge::create()\n#1 /app/Http/Controllers/CheckoutController.php(63): App\\Services\\Billing->charge()\n#2 {main}' },
    { level: 'INFO', date: at(20), message: 'Cache warmed', detail: 'local.INFO: Cache warmed {"keys":128}' },
  ];
}

// analyticsFor expands a profile into the analytics response the view expects,
// stamping the throughput points and recent list with times relative to now.
function analyticsFor(domain: string, range: string): unknown {
  const p = ANALYTICS_PROFILES[domain] ?? DEFAULT_ANALYTICS;
  const now = Date.now();
  const minute = 60_000;
  const nowMin = Math.floor(now / minute) * minute;
  return {
    site: domain,
    range,
    samples: p.samples,
    cold_starts: p.cold_starts,
    median_millis: p.median_millis,
    p95_millis: p.p95_millis,
    status: p.status,
    distribution: p.distribution.map((count, i) => ({ upper_millis: LATENCY_EDGES[i] ?? 0, count })),
    throughput: p.throughput.map((count, i) => ({
      at_millis: nowMin - (p.throughput.length - 1 - i) * minute,
      count,
    })),
    routes: p.routes,
    recent: p.recent.map((r) => ({
      at_millis: now - r.agoSec * 1000,
      method: r.method,
      route: r.route,
      uri: r.uri,
      status: r.status,
      millis: r.millis,
      cold: !!r.cold,
    })),
  };
}

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

// ---- Overview "Actions" section: per-framework command sets + doctor ----
// The command cards and the doctor card both call per-site endpoints; give them
// framework-appropriate fixtures so the section looks like a real project.
const LARAVEL_COMMANDS = [
  { name: 'migrate', label: 'Migrate', command: 'php artisan migrate', icon: 'database', description: 'Run pending database migrations', confirm: true },
  { name: 'migrate-fresh', label: 'Fresh + seed', command: 'php artisan migrate:fresh --seed', icon: 'refresh', description: 'Drop all tables, re-migrate and seed', confirm: true },
  { name: 'optimize-clear', label: 'Clear caches', command: 'php artisan optimize:clear', icon: 'broom', description: 'Flush config, route, view and event caches' },
  { name: 'key-generate', label: 'App key', command: 'php artisan key:generate', icon: 'key', description: 'Generate the application key' },
  { name: 'route-list', label: 'Routes', command: 'php artisan route:list', icon: 'list', description: 'List the registered routes' },
  { name: 'storage-link', label: 'Storage link', command: 'php artisan storage:link', icon: 'link', description: 'Symlink public/storage to storage/app/public' },
];
const SYMFONY_COMMANDS = [
  { name: 'migrate', label: 'Migrate', command: 'php bin/console doctrine:migrations:migrate', icon: 'database', description: 'Apply Doctrine migrations', confirm: true },
  { name: 'cache-clear', label: 'Clear cache', command: 'php bin/console cache:clear', icon: 'broom', description: 'Clear the Symfony cache' },
  { name: 'router', label: 'Routes', command: 'php bin/console debug:router', icon: 'list', description: 'List the configured routes' },
];
const WORDPRESS_COMMANDS = [
  { name: 'cache-flush', label: 'Flush cache', command: 'wp cache flush', icon: 'broom', description: 'Flush the object cache' },
  { name: 'plugin-list', label: 'Plugins', command: 'wp plugin list', icon: 'list', description: 'List installed plugins' },
  { name: 'core-update', label: 'Update core', command: 'wp core update', icon: 'arrow-up', description: 'Update WordPress core', confirm: true },
];

function frameworkOf(domain: string): string {
  return (sites.find((s) => s.domain === domain)?.framework as string) || '';
}
function commandsFor(domain: string): Array<Record<string, unknown>> {
  switch (frameworkOf(domain)) {
    case 'laravel': return LARAVEL_COMMANDS;
    case 'symfony': return SYMFONY_COMMANDS;
    case 'wordpress': return WORDPRESS_COMMANDS;
    default: return [];
  }
}

const DOCTOR_LARAVEL = {
  checks: [
    { name: 'app_key', label: 'Application key', status: 'ok' },
    { name: 'migrations', label: 'Migrations', status: 'warn', detail: '2 pending migrations', fix: 'migrate' },
    { name: 'env_drift', label: '.env drift', status: 'ok' },
    { name: 'storage_link', label: 'Storage link', status: 'ok' },
  ],
  failures: 0,
  warnings: 1,
};
const DOCTOR_OK = {
  checks: [{ name: 'serving', label: 'Serving over HTTPS', status: 'ok' }],
  failures: 0,
  warnings: 0,
};
function doctorFor(domain: string): Record<string, unknown> {
  return frameworkOf(domain) === 'laravel' ? DOCTOR_LARAVEL : DOCTOR_OK;
}

// Running a command card streams the same SSE contract the daemon emits.
function commandRunSSE(domain: string, name: string): Response {
  const cmd = commandsFor(domain).find((c) => c.name === name);
  const line = (cmd?.command as string) || name;
  const body =
    `event: stdout\ndata: $ ${line}\n\n` +
    `event: stdout\ndata: Running…\n\n` +
    `event: stdout\ndata: Done.\n\n` +
    `event: done\ndata: ${JSON.stringify({ exit: 0, durationMs: 640 })}\n\n`;
  return new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } });
}

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

// Installing a preset streams newline-delimited JSON phase events ending in a
// `done`. Mark the preset installed and drop a minimal service into the live
// list so the picker and the services grid update like the real flow.
function presetInstallStream(name: string, version: string): Response {
  const preset = presets.find((p) => p.name === name);
  const image = (preset?.image as string) || `docker.io/library/${name}:latest`;
  if (preset) {
    preset.installed = true;
    if (version) preset.installed_tags = [...((preset.installed_tags as string[]) || []), version];
  }
  const svcName = version ? `${name}-${version}` : name;
  if (!services.some((s) => s.name === svcName)) {
    services.push({
      name: svcName,
      status: 'active',
      version: version || 'latest',
      env_vars: {},
      dashboard: (preset?.dashboard as string) || undefined,
      custom: true,
      preset_owned: true,
      site_count: 0,
      pinned: false,
      migration_supported: false,
      can_rollback: false,
    });
  }
  const body =
    `${JSON.stringify({ phase: 'pulling_image', image })}\n` +
    `${JSON.stringify({ phase: 'starting_unit' })}\n` +
    `${JSON.stringify({ phase: 'waiting_ready' })}\n` +
    `${JSON.stringify({ phase: 'done', name: svcName })}\n`;
  return new Response(body, { status: 200, headers: { 'content-type': 'application/x-ndjson' } });
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
  if (path === '/api/services/presets') return jsonResponse(presets);

  // An engine's databases. An engine with no fixture reports none rather than
  // falling through to the empty catch-all, which the tab reads as an error.
  const engine = path.match(/^\/api\/databases\/([^/]+)$/);
  if (engine && method === 'GET') {
    const name = decodeURIComponent(engine[1]);
    const known = (databasesFixture as Record<string, unknown>)[name];
    return jsonResponse(
      known ?? {
        service: name,
        family: '',
        status: 'active',
        supports_create: false,
        supports_snapshot: false,
        databases: [],
      },
    );
  }

  // Installing a preset streams progress; anything under presets/<name>.
  const presetInstall = path.match(/^\/api\/services\/presets\/([^/]+)$/);
  if (presetInstall && method === 'POST')
    return presetInstallStream(decodeURIComponent(presetInstall[1]), qs.get('version') || '');

  // Per-site request-timing analytics — a busy profile with flagged slow routes
  // and a cold start for a couple of sites, a healthy populated one for the rest.
  const analyticsMatch = path.match(/^\/api\/sites\/([^/]+)\/analytics$/);
  if (analyticsMatch) {
    const domain = decodeURIComponent(analyticsMatch[1]);
    return jsonResponse(analyticsFor(domain, qs.get('range') || '1h'));
  }

  // Per-site application logs (Logs tab → App logs). List files, then a file's
  // entries; the /clear POST falls through to {ok:true} like other actions.
  const appLogsMatch = path.match(/^\/api\/app-logs\/([^/]+)(?:\/([^/]+))?$/);
  if (appLogsMatch && method === 'GET') {
    const file = appLogsMatch[2];
    if (!file) return jsonResponse({ files: APP_LOG_FILES });
    if (file !== 'clear') return jsonResponse({ entries: appLogEntries() });
  }

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

  // Overview "Actions": command list, doctor report, and running a command.
  const cmdRun = path.match(/^\/api\/sites\/([^/]+)\/commands\/([^/]+)\/run$/);
  if (cmdRun && method === 'POST')
    return commandRunSSE(decodeURIComponent(cmdRun[1]), decodeURIComponent(cmdRun[2]));
  const cmdList = path.match(/^\/api\/sites\/([^/]+)\/commands$/);
  if (cmdList) return jsonResponse({ commands: commandsFor(decodeURIComponent(cmdList[1])) });
  const doctorMatch = path.match(/^\/api\/sites\/([^/]+)\/doctor$/);
  if (doctorMatch) return jsonResponse(doctorFor(decodeURIComponent(doctorMatch[1])));

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
  // acme — the other routes surfaced in Request timing, so the database icon on
  // each slow route (which deep-links to the Queries lens filtered by that route)
  // lands on the queries captured behind it instead of an empty lens: a slow
  // write on checkout, a classic N+1 on the dashboard, light reads on the rest.
  const R = (rid: string, request: string, kind: string, extra: Record<string, unknown>, over: number) =>
    out.push(make(over, 'acme', 'acme.test', rid, request, kind, extra));
  R('req-acme-checkout', 'POST /checkout', 'query', { data: { sql: 'select * from `carts` where `user_id` = ? limit 1', bindings: [42], time_ms: 1.1, connection: 'mysql', rw_type: 'read' }, src: { file: 'app/Http/Controllers/CheckoutController.php', line: 33 } }, 6800);
  R('req-acme-checkout', 'POST /checkout', 'query', { data: { sql: 'insert into `orders` (`user_id`, `total`, `status`) values (?, ?, ?)', bindings: [42, '249.00', 'pending'], time_ms: 3.2, connection: 'mysql', rw_type: 'write' }, src: { file: 'app/Http/Controllers/CheckoutController.php', line: 41 } }, 6700);
  R('req-acme-checkout', 'POST /checkout', 'query', { data: { sql: 'update `inventory` set `stock` = `stock` - ? where `product_id` = ?', bindings: [1, 12], time_ms: 214.0, connection: 'mysql', rw_type: 'write' }, src: { file: 'app/Services/Inventory.php', line: 58 } }, 6600);
  R('req-acme-checkout', 'POST /checkout', 'job', { data: { class: 'App\\Jobs\\ChargeCard', status: 'queued', connection: 'redis' }, src: { file: 'app/Http/Controllers/CheckoutController.php', line: 63 } }, 6500);
  const DASH = [1, 2, 3, 4, 5];
  for (const uid of DASH)
    R('req-acme-dash', 'GET /dashboard', 'query', { data: { sql: 'select count(*) from `orders` where `user_id` = ?', bindings: [uid], time_ms: 2.0 + uid / 10, connection: 'mysql', rw_type: 'read' }, src: { file: 'app/Http/Controllers/DashboardController.php', line: 27 } }, 5200 - uid * 40);
  R('req-acme-dash', 'GET /dashboard', 'query', { data: { sql: 'select * from `users` where `team_id` = ?', bindings: [3], time_ms: 1.8, connection: 'mysql', rw_type: 'read' }, src: { file: 'app/Http/Controllers/DashboardController.php', line: 22 } }, 5250);
  R('req-acme-home', 'GET /', 'query', { data: { sql: 'select * from `products` where `featured` = ? limit 12', bindings: [1], time_ms: 4.6, connection: 'mysql', rw_type: 'read' }, src: { file: 'app/Http/Controllers/HomeController.php', line: 19 } }, 4200);
  R('req-acme-home', 'GET /', 'query', { data: { sql: 'select * from `categories` order by `sort` asc', bindings: [], time_ms: 0.7, connection: 'mysql', rw_type: 'read' }, src: { file: 'app/Http/Controllers/HomeController.php', line: 24 } }, 4100);
  R('req-acme-products', 'GET /products', 'query', { data: { sql: 'select * from `products` order by `created_at` desc limit 24 offset 0', bindings: [], time_ms: 5.3, connection: 'mysql', rw_type: 'read' }, src: { file: 'app/Http/Controllers/ProductController.php', line: 30 } }, 3600);
  return out;
}

// Canned log lines for the Logs tab's streamed views (FPM/container, queue,
// schedule, reverb, and the host dev-server journal), picked off the stream path
// so each sub-tab shows realistic output instead of a forever-connecting empty
// viewer. Timestamps are stamped relative to now so the tail reads as live.
function logLinesFor(url: string): string[] {
  const now = Date.now();
  const t = (secAgo: number) => new Date(now - secAgo * 1000).toISOString().replace('T', ' ').slice(0, 19);
  if (/\/queue\//.test(url) || /\/horizon\//.test(url))
    return [
      `[${t(52)}] Processing: App\\Jobs\\SendShipmentNotification`,
      `[${t(52)}] Processed:  App\\Jobs\\SendShipmentNotification`,
      `[${t(30)}] Processing: App\\Jobs\\ChargeCard`,
      `[${t(29)}] Processed:  App\\Jobs\\ChargeCard`,
      `[${t(8)}] Processing: App\\Jobs\\RebuildSearchIndex`,
    ];
  if (/\/schedule\//.test(url))
    return [
      `[${t(120)}] Running scheduled command: php artisan telescope:prune --hours=48`,
      `[${t(120)}] Command "telescope:prune" ran successfully.`,
      `[${t(60)}] Running scheduled command: php artisan sitemap:generate`,
      `[${t(60)}] Command "sitemap:generate" ran successfully.`,
    ];
  if (/\/reverb\//.test(url))
    return [
      `${t(40)}  Connection 8f2ac1 established`,
      `${t(38)}  Subscribed to channel: orders.42`,
      `${t(12)}  Broadcasting App\\Events\\OrderShipped on orders.42`,
    ];
  if (/\/worker\//.test(url))
    return [
      `  VITE v5.4.10  ready in 214 ms`,
      `  ➜  Local:   https://acme.test:5173/`,
      `  ➜  press h + enter to show help`,
      `${t(6)} [vite] hmr update /resources/js/app.js`,
    ];
  // FPM / container: an access + php-error mix (the default runtime tab)
  return [
    `[${t(18)}] 127.0.0.1  "GET /"  200  138ms`,
    `[${t(15)}] 127.0.0.1  "GET /products"  200  70ms`,
    `[${t(9)}] 127.0.0.1  "POST /checkout"  302  199ms`,
    `[${t(6)}] PHP Warning:  Undefined array key "coupon" in /app/Http/Controllers/CheckoutController.php on line 52`,
    `[${t(4)}] 127.0.0.1  "GET /orders/42"  200  233ms`,
  ];
}

// EventSource that "connects" then replays the canned debug events or log lines.
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
    const u = String(url);
    const isDumps = u.includes('/api/dumps/stream');
    const isLog = !isDumps && /\/logs(\/|$|\?)/.test(u);
    setTimeout(() => {
      this.readyState = 1;
      this.emit('open', { type: 'open' });
      if (isDumps) {
        for (const e of debugEvents()) this.emit('message', { data: JSON.stringify(e) });
      } else if (isLog) {
        for (const line of logLinesFor(u)) this.emit('message', { data: line });
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
