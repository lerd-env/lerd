import type { DumpEvent } from '$lib/dumpEvent';

// Captured events for acme.test's requests, so opening a slow route or a recent
// request in the demo shows its queries, logs, outgoing calls and the rest
// instead of an empty inspector. One request id per route.

interface Capture {
  route: string;
  agoSec: number;
  events: Array<{ kind: string; file: string; line: number; data: Record<string, unknown> }>;
}

const ORDER_ITEMS = 'select * from `order_items` where `order_items`.`order_id` = ? and `order_items`.`order_id` is not null';

const CAPTURES: Record<string, Capture> = {
  'POST /checkout': {
    route: 'POST /checkout',
    agoSec: 62,
    events: [
      { kind: 'query', file: 'app/Http/Controllers/CheckoutController.php', line: 31, data: { sql: 'select * from `users` where `id` = ? limit 1', bindings: [42], time_ms: 1.4, connection: 'pgsql', rw_type: 'read' } },
      { kind: 'query', file: 'app/Http/Controllers/CheckoutController.php', line: 38, data: { sql: 'select * from `carts` where `user_id` = ? limit 1', bindings: [42], time_ms: 2.1, connection: 'pgsql', rw_type: 'read' } },
      ...[1201, 1202, 1203, 1204, 1205, 1206].map((id, i) => ({
        kind: 'query', file: 'app/Models/Order.php', line: 58,
        data: { sql: ORDER_ITEMS, bindings: [id], time_ms: 3.2 + i * 0.4, connection: 'pgsql', rw_type: 'read' },
      })),
      { kind: 'cache', file: 'app/Services/Pricing.php', line: 22, data: { key: 'pricing:tax-rates:eu', op: 'miss' } },
      { kind: 'cache', file: 'app/Services/Pricing.php', line: 27, data: { key: 'pricing:tax-rates:eu', op: 'write' } },
      { kind: 'log', file: 'app/Http/Controllers/CheckoutController.php', line: 49, data: { message: 'Coupon code not found, ignoring', level: 'warning', channel: 'local' } },
      { kind: 'http', file: 'app/Services/Billing.php', line: 67, data: { method: 'POST', url: 'https://api.stripe.com/v1/payment_intents', status: 200, time_ms: 284 } },
      { kind: 'query', file: 'app/Http/Controllers/CheckoutController.php', line: 71, data: { sql: 'insert into `orders` (`user_id`, `total`, `status`, `updated_at`, `created_at`) values (?, ?, ?, ?, ?)', bindings: [42, '249.00', 'paid', '2026-10-16 09:14:02', '2026-10-16 09:14:02'], time_ms: 4.8, connection: 'pgsql', rw_type: 'write' } },
      { kind: 'event', file: 'app/Http/Controllers/CheckoutController.php', line: 78, data: { name: 'App\\Events\\OrderPlaced', listeners: ['App\\Listeners\\SendOrderConfirmation', 'App\\Listeners\\ReserveStock'] } },
      { kind: 'job', file: 'app/Listeners/SendOrderConfirmation.php', line: 19, data: { class: 'App\\Jobs\\SendShipmentNotification', status: 'queued', queue: 'default' } },
      { kind: 'mail', file: 'app/Listeners/SendOrderConfirmation.php', line: 24, data: { subject: 'Your order #900 is confirmed', to: ['customer@example.com'], mailable: 'App\\Mail\\OrderConfirmed' } },
      { kind: 'log', file: 'app/Http/Controllers/CheckoutController.php', line: 84, data: { message: 'Order placed', level: 'info', channel: 'local' } },
    ],
  },
  'GET /dashboard': {
    route: 'GET /dashboard',
    agoSec: 168,
    events: [
      { kind: 'query', file: 'app/Http/Controllers/DashboardController.php', line: 18, data: { sql: 'select count(*) as aggregate from `orders` where `created_at` >= ?', bindings: ['2026-10-09 00:00:00'], time_ms: 38.6, connection: 'pgsql', rw_type: 'read' } },
      { kind: 'query', file: 'app/Http/Controllers/DashboardController.php', line: 22, data: { sql: 'select sum(`total`) as aggregate from `orders` where `status` = ?', bindings: ['paid'], time_ms: 41.2, connection: 'pgsql', rw_type: 'read' } },
      { kind: 'cache', file: 'app/Http/Controllers/DashboardController.php', line: 27, data: { key: 'dashboard:top-products', op: 'hit' } },
      { kind: 'view', file: 'app/Http/Controllers/DashboardController.php', line: 31, data: { name: 'dashboard.index', path: 'resources/views/dashboard/index.blade.php' } },
      { kind: 'view', file: 'resources/views/dashboard/index.blade.php', line: 12, data: { name: 'components.chart', path: 'resources/views/components/chart.blade.php' } },
    ],
  },
  'GET /orders/:id': {
    route: 'GET /orders/:id',
    agoSec: 121,
    events: [
      { kind: 'query', file: 'app/Http/Controllers/OrderController.php', line: 24, data: { sql: 'select * from `orders` where `id` = ? limit 1', bindings: [42], time_ms: 1.9, connection: 'pgsql', rw_type: 'read' } },
      { kind: 'query', file: 'app/Http/Controllers/OrderController.php', line: 25, data: { sql: ORDER_ITEMS, bindings: [42], time_ms: 6.3, connection: 'pgsql', rw_type: 'read' } },
      { kind: 'http', file: 'app/Services/Shipping.php', line: 40, data: { method: 'GET', url: 'https://api.shipping.example/v2/tracking/1Z999AA10123456784', status: 200, time_ms: 132 } },
      { kind: 'view', file: 'app/Http/Controllers/OrderController.php', line: 30, data: { name: 'orders.show', path: 'resources/views/orders/show.blade.php' } },
    ],
  },
  'GET /cart': {
    route: 'GET /cart',
    agoSec: 75,
    events: [
      { kind: 'query', file: 'app/Http/Controllers/CartController.php', line: 15, data: { sql: 'select * from `carts` where `session_id` = ? limit 1', bindings: ['b7f3c0'], time_ms: 1.2, connection: 'pgsql', rw_type: 'read' } },
      { kind: 'cache', file: 'app/Services/Pricing.php', line: 22, data: { key: 'pricing:tax-rates:eu', op: 'hit' } },
      { kind: 'view', file: 'app/Http/Controllers/CartController.php', line: 21, data: { name: 'cart.index', path: 'resources/views/cart/index.blade.php' } },
    ],
  },
  'GET /': {
    route: 'GET /',
    agoSec: 4,
    events: [
      { kind: 'cache', file: 'app/Http/Controllers/HomeController.php', line: 14, data: { key: 'home:featured', op: 'hit' } },
      { kind: 'query', file: 'app/Http/Controllers/HomeController.php', line: 17, data: { sql: 'select * from `products` where `featured` = ? order by `position` asc limit 8', bindings: [true], time_ms: 3.1, connection: 'pgsql', rw_type: 'read' } },
      { kind: 'view', file: 'app/Http/Controllers/HomeController.php', line: 19, data: { name: 'welcome', path: 'resources/views/welcome.blade.php' } },
    ],
  },
  'GET /products': {
    route: 'GET /products',
    agoSec: 46,
    events: [
      { kind: 'query', file: 'app/Http/Controllers/ProductController.php', line: 16, data: { sql: 'select * from `products` order by `created_at` desc limit 24 offset 0', time_ms: 2.7, connection: 'pgsql', rw_type: 'read' } },
      { kind: 'view', file: 'app/Http/Controllers/ProductController.php', line: 18, data: { name: 'products.index', path: 'resources/views/products/index.blade.php' } },
    ],
  },
};

export const ridFor = (route: string): string | undefined =>
  CAPTURES[route] ? 'demo-' + route.replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '').toLowerCase() : undefined;

export function demoRequestEvents(): DumpEvent[] {
  const now = Date.now();
  const out: DumpEvent[] = [];
  for (const c of Object.values(CAPTURES)) {
    const rid = ridFor(c.route)!;
    c.events.forEach((e, i) => {
      out.push({
        v: 1,
        id: `${rid}-${i}`,
        ts: new Date(now - c.agoSec * 1000 + i * 12).toISOString(),
        kind: e.kind,
        ctx: { type: 'fpm', site: 'acme', domain: 'acme.test', request: c.route, rid },
        src: { file: '/home/dev/code/acme/' + e.file, line: e.line },
        data: e.data,
      });
    });
  }
  // PHP's own summary closes each request, which splits its time into app and database.
  for (const c of Object.values(CAPTURES)) {
    const rid = ridFor(c.route)!;
    const db = c.events.filter((e) => e.kind === 'query').reduce((sum, e) => sum + Number(e.data.time_ms), 0);
    const calls = c.events.filter((e) => e.kind === 'http').reduce((sum, e) => sum + Number(e.data.time_ms), 0);
    out.push({
      v: 1,
      id: `${rid}-request`,
      ts: new Date(now - c.agoSec * 1000 + c.events.length * 12 + 5).toISOString(),
      kind: 'request',
      ctx: { type: 'fpm', site: 'acme', domain: 'acme.test', request: c.route, rid },
      src: { file: '/home/dev/code/acme/public/index.php', line: 20 },
      data: { time_ms: Math.round(db + calls + 38), memory_peak: 18 * 1024 * 1024 },
    });
  }
  // Oldest first, the order lerd-ui stores events in.
  return out.sort((a, b) => a.ts.localeCompare(b.ts));
}
