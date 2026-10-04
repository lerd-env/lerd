# Debug package (lerd/debug)

lerd already records most of what a request does without any code in your app: its queries, logs, cache calls, views, jobs, the framework's own phases and more, all shown in the [Requests lens](queries.md#request-linking). `lerd/debug` is a small Composer package for the rest: rows of your own on a request's timeline, log lines, and tabs with tables, figures and charts.

```bash
composer require lerd/debug --dev
```

Outside lerd, in production or on a machine without it, every call returns at once and nothing is kept, not even in memory, so the package is safe to leave in.

## What lerd records on its own

None of this needs the package. lerd reads it through its PHP extension and the [framework definitions](../usage/framework-definitions.md), so it works the moment a site is linked.

**On every PHP site**

- Queries through PDO, with bindings, the line that ran them, slow queries and N+1 warnings
- `dump()` and `dd()` output
- Log records written through Monolog
- Mail sent through Symfony Mailer
- Twig templates, Symfony events, HttpClient calls and Messenger messages
- Exceptions reported to Sentry or Inspector, and messages sent through Symfony Notifier
- Storage operations through Flysystem
- The request and response: headers, query string, body, cookies and session, with credentials masked
- Time spent in nginx and the FPM queue, and, with [browser capture](browser-capture.md) on, the page's own loading phases and the requests it sends afterwards

**Per framework**

Each framework adds its phases to the timeline, names the route that matched and, where it keeps one, who the request runs as. Console commands that only loop, like queue workers, are left out, while the jobs they run are each listed on their own.

=== "Laravel"

    - **Timeline:** Bootstrap, Routing, Middleware, Controller (by its action, or a closure by file and line), each View, Terminate
    - **Route:** the route's name, and the middleware it ran through
    - **User:** the first user a guard resolves, with the guard's name
    - **Also:** cache calls with the store behind them, Redis commands, Eloquent models loaded, events, mail, notifications on every channel, HTTP client calls, jobs (a sync job listed under the request that ran it), the Storage disk an operation used
    - **Livewire:** each component's mount, property update, method call and render, with what it was handed
    - **Left out:** `queue:work`, `queue:listen`, `schedule:work`, Horizon and Reverb

=== "Symfony"

    - **Timeline:** Bootstrap, Routing, Handle, sending the response, Terminate, each Twig template
    - **User:** the user in the security token the firewall stores
    - **Also:** the session as it is saved, Messenger messages from dispatch to handled or failed, mail, notifier messages, HTTP client calls, events
    - **Workers:** `messenger:consume` is hidden by default, while each message it handles is listed on its own

=== "Yii 2"

    - **Timeline:** Bootstrap, Routing, the controller's filters, the action (as `controller/action`), each view file, sending the response
    - **Route:** the `controller/action` id, recorded even when a filter stops the action
    - **User:** the identity `yii\web\User` takes on login, cookie login or session restore
    - **Also:** the session, yii2-queue jobs
    - **Left out:** `queue/listen`

=== "CakePHP"

    - **Timeline:** Bootstrap, the middleware queue, the controller action (as `Controller@action`), each view, sending the response
    - **User:** the identity cakephp/authentication builds
    - **Also:** the session, cakephp/queue jobs
    - **Left out:** `queue worker`

=== "CodeIgniter 4"

    - **Timeline:** Bootstrap, Routing, the required and route filters, the controller, each view, sending the response
    - **Route:** the route pattern that matched
    - **User:** who logged in through Shield
    - **Also:** the session, codeigniter4/queue jobs
    - **Left out:** `queue:work`

=== "Drupal"

    - **Timeline:** kernel boot, routing, the controller (as `Class@method`), rendering the page, each Twig template, sending the response, terminate, cron
    - **Route:** the route name, such as `user.login`
    - **User:** the account Drupal sets for the request
    - **Also:** the session, events, queue workers' items as jobs
    - **Left out:** `drush watchdog:tail` and `drush runserver`

    Drupal caches rendered pages and blocks, so a cached page shows fewer phases than one rendered from scratch.

=== "TYPO3"

    - **Timeline:** Bootstrap, matching the site and the page, the frontend or backend middleware stack, the page handler, each Extbase controller, each Fluid view, sending the response
    - **Route:** the page uid on the frontend, the route identifier in the backend
    - **User:** the frontend or backend user
    - **Also:** the session, scheduler tasks as jobs
    - **Left out:** `messenger:consume`

=== "Magento"

    - **Timeline:** Bootstrap, launching the app, each router asked, the controller (as the full action name), rendering the layout, each `.phtml` template, sending the response
    - **Route:** the full action name, such as `catalog_product_view`
    - **User:** the logged-in customer, or the admin user
    - **Also:** the session, message queue consumers' messages as jobs
    - **Left out:** `queue:consumers:start`

=== "Tempest"

    - **Timeline:** Bootstrap with discovery, Routing, the route's middleware, Handle, each view, sending the response
    - **Route:** the route's URI pattern
    - **User:** the user on login and as the session resolves it
    - **Also:** the session, async commands as jobs
    - **Left out:** `command:monitor` and the log tails

=== "WordPress"

    WordPress and Bedrock are mostly plain functions, so less of them can be timed.

    - **Timeline:** the main query, and on the REST API the whole request and the endpoint
    - **Route:** the rewrite rule that matched, or the REST path
    - **User:** the logged-in user
    - **Also:** Action Scheduler jobs, by the hook they run

    WordPress talks to the database through mysqli, which lerd does not capture yet, so its queries do not show.

## What the package adds

Everything goes through the `Lerd\Debug\Lerd` class.

### Your own timeline rows

![A request's timeline in the Requests lens, where an app's own rows such as Price calculation sit among the framework's phases](/assets/screenshots/request-performance.png)

```php
use Lerd\Debug\{Lerd, Color};

// Time a callback; you get its return value back.
$pdf = Lerd::timeline()->measure('Render invoice PDF', fn () => $invoice->render(), 'billing', Color::Amber, ['order' => $order->id]);

// Mark a moment.
Lerd::timeline()->event('Payment authorised', 'billing', Color::Emerald)->stop();
```

Each row has a label, a **category** (free text such as `billing`, which becomes a filter), a **colour** from the `Color` enum (Blue, Indigo, Violet, Pink, Rose, Orange, Amber, Lime, Emerald, Teal, Cyan, Slate) and **details** shown when you hover it. A measured callback is still drawn when it throws.

For work you start and stop yourself:

```php
$sync = Lerd::timeline()->event('Stock sync', 'inventory', Color::Violet)->start();
// ... the work ...
$sync->with(['skus' => count($skus)])->stop();

// The same running event, by name, from another part of the app:
Lerd::timeline()->event('Stock sync')->stop();

// Around a callback:
$rows = Lerd::timeline()->event('Import feed')->run(fn () => $importer->run());

// For work that already happened:
Lerd::timeline()->event('Queue wait')->startAt($job->queuedAt)->duration($waitedMs)->stop();
```

Nothing reaches lerd until an event stops. Started first, it is a span; stopped without starting, a moment. Stopping twice does nothing.

### Log lines

```php
Lerd::info('Cache warmed', ['keys' => 120]);
Lerd::warning('Slow upstream', ['ms' => 2300, 'trace' => true]);
Lerd::notice('Hit the fast path', ['performance' => true]);
Lerd::log('error', 'Import failed', ['file' => $path]);
```

Lines land in the request's Log tab and on its timeline, on the `lerd` channel, at any PSR-3 level. Two context keys are switches: `trace` keeps the stack trace with the line, and `performance` also pins it to the top of the Performance tab.

### Who the request runs as

```php
Lerd::auth($user->id, $user->email, $user->name, 'api');
```

Most frameworks report this on their own (see above). Use it when your app authenticates its own way. Only the first user a request reports is shown.

### Tabs of your own

![The Cart tab from the example below, with counters, a key-value block, a table and a bar chart](/assets/screenshots/request-custom-tab.png)

```php
use Lerd\Debug\{Chart, Series};

Lerd::tab('Cart')->columns(2)
    ->counters(['items' => 3, 'total' => '€59.70'], 'Summary', span: 2)
    ->keyValue($cart->toArray(), 'Contents')
    ->table(['sku', 'qty', 'meta'], $cart->lines(), 'Lines')
    ->chart(Chart::bar(Series::make('orders', Color::Teal)->point('Mon', 3)->point('Tue', 5)), 'This week', span: 2);

// Anywhere else in the same request, the same tab gets another block:
Lerd::tab('Cart')->text('Coupon WELCOME applied');
```

| Method | Shows |
|---|---|
| `table($columns, $rows)` | Rows under named columns |
| `keyValue($values)` | Names and values, nested values as a tree |
| `counters($counters)` | Headline numbers side by side |
| `code($code, $language)` | Code or other preformatted text |
| `text($text)` | A paragraph |
| `chart($chart)` | A line, bar, pie or exploded pie chart |

Every block takes an optional heading and a `span`. `columns(n)` lays the tab out in up to four columns on a wide screen. `before('database')` and `after('performance')` place the tab among lerd's own.

Charts take one or more `Series`, each a name, a value per label and an optional colour:

```php
Chart::line(
    new Series('p50', ['10:00' => 120, '10:05' => 135]),
    Series::make('p95', Color::Rose)->point('10:00', 310)->point('10:05', 420),
);
```

## Setup per framework

The package works without any setup. A framework integration adds a switch to turn it off from the app's config, and clears what the package keeps between the jobs of a long-running worker.

=== "Laravel"

    Discovered on its own. Publish the config to get the switch:

    ```bash
    php artisan vendor:publish --tag=lerd-config
    ```

    `enabled` in `config/lerd.php` (`LERD_ENABLED`) turns the package off. Kept entries are cleared after each queued job and each Octane request, task or tick.

=== "Symfony"

    Register the bundle:

    ```php
    // config/bundles.php
    Lerd\Debug\Frameworks\Symfony\LerdDebugBundle::class => ['dev' => true],
    ```

    `lerd.enabled: false` in `config/packages/lerd.yaml` turns the package off. Kept entries are cleared between requests in a worker runtime (FrankenPHP, RoadRunner) and after each Messenger message.

=== "Yii 2"

    Add the bootstrap class to the app's config:

    ```php
    // config/web.php and config/console.php
    'bootstrap' => ['log', Lerd\Debug\Frameworks\Yii\Bootstrap::class],
    ```

    A `lerd.enabled` param set to `false` turns the package off. Kept entries are cleared after each yii2-queue job.

=== "Any other"

    Nothing to register. In a long-running worker, call `Lerd::flush()` after each job.

## In tests

`Lerd::enable(true)` turns capture on in a test suite, `Lerd::enable(false)` off, and `Lerd::enable(null)` goes back to asking lerd. The package keeps the last 500 entries for `Lerd::entries()`, so a test can assert what the app wrote; `Lerd::flush()` clears them.

## Bringing your own objects

What `Lerd` accepts is described by interfaces in `Lerd\Debug\Contracts`, and no class is final, so a report object of your own can implement `Chart` and be passed to `chart()`, or a `Block` of your own be added to a tab with `Tab::add()`.

| Contract | Describes |
|---|---|
| `Trackable` | Anything `Lerd::track()` accepts |
| `TimelineEntry` | A row on the timeline |
| `LogEntry` | A log line |
| `TabEntry` | A block added to a tab |
| `Tab` | A tab |
| `Block` | A block of a tab |
| `Chart`, `Series` | A chart and its series |

??? note "How lerd reads it"

    Every entry passes `Lerd::track()`, which lerd's framework store declares as a seam. lerd's extension observes the call and its collector reads the entry, so the package calls no lerd function and editors and static analysis know every method.

    The collector reads the entry through a renderer in `Lerd\Debug\Rendering`, one per version of lerd's schema. lerd names the version it reads (1 today) and gets the newest renderer the package has that is no newer, so either side can upgrade without breaking the other.
