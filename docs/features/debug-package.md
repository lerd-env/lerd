# Debug package (lerd/debug)

`lerd/debug` is a small Composer package for writing your own data into lerd's [Requests lens](queries.md#request-linking): rows on a request's timeline, log lines, and tabs of your own built from tables, figures and charts. It has no dependency on lerd and does nothing outside it, so it can stay in an app's dependencies everywhere.

```bash
composer require lerd/debug --dev
```

Everything goes through the `Lerd\Debug\Lerd` class.

## When it does anything

`Lerd::enabled()` reports whether lerd is capturing the current process, read once from the constants lerd's PHP extension defines (`LERD_DEVTOOLS_ON`, or `LERD_DEVTOOLS_JOBS` in a worker). When it is not, in production, on a machine without lerd, or with the Debug window switched off, every call returns at once and nothing is kept, not even in memory. `Lerd::enable(true)` forces capture on, which is what a test suite wants; `Lerd::enable(false)` switches it off by hand, and `Lerd::enable(null)` goes back to asking lerd.

## Timeline

![A request's timeline in the Requests lens, where an app's own rows such as Price calculation sit among the framework's phases](/assets/screenshots/request-performance.png)

```php
use Lerd\Debug\{Lerd, Color};

$pdf = Lerd::timeline()->measure('Render invoice PDF', fn () => $invoice->render(), 'billing', Color::Amber, ['order' => $order->id]);

Lerd::timeline()->event('Payment authorised', 'billing', Color::Emerald)->stop();
```

`measure()` runs a callback, returns what it returns, and puts a span on the timeline even when the callback throws. `event()` hands back an event to time by hand, below; stopped without being started, it is a moment. Each row has a label, a category, a colour and details:

- **Category** is free text, `billing` or `imports`, and becomes a filter beside lerd's own layers.
- **Colour** comes from the `Color` enum: Blue, Indigo, Violet, Pink, Rose, Orange, Amber, Lime, Emerald, Teal, Cyan, Slate.
- **Details** show in the row's popover, nested values as a tree.

### Events timed by hand

`Lerd::timeline()->event()` hands back an event to time yourself. Nothing reaches lerd until it is stopped.

```php
$sync = Lerd::timeline()->event('Stock sync', 'inventory', Color::Violet)->start();
// ... the work ...
$sync->with(['skus' => count($skus)])->stop();

// The same running event, by name, from somewhere else:
Lerd::timeline()->event('Stock sync')->stop();

// Or around a callback:
$rows = Lerd::timeline()->event('Import feed')->run(fn () => $importer->run());

// Or for work that already happened:
Lerd::timeline()->event('Queue wait')->startAt($job->queuedAt)->duration($waitedMs)->stop();
```

An event stopped after `start()` is a span, one stopped without it a moment. Stopping twice does nothing, and `color()`, `category()` and `with()` can be set at any point before it stops. `event()` gives back the same event for the same name until it is stopped, so one part of an app can start it and another stop it.

## Logging

```php
Lerd::info('Cache warmed', ['keys' => 120]);
Lerd::warning('Slow upstream', ['ms' => 2300, 'trace' => true]);
Lerd::notice('Hit the fast path', ['performance' => true]);
Lerd::log('error', 'Import failed', ['file' => $path]);
```

A line written this way lands in the request's Log tab and on its timeline like any other log line, on the `lerd` channel, at one of the PSR-3 levels (`emergency`, `alert`, `critical`, `error`, `warning`, `notice`, `info`, `debug`). Two context keys are flags rather than context: `trace` keeps the call's stack trace with the line, and `performance` also shows the line at the top of the Performance tab.

## Who the request runs as

```php
Lerd::auth($user->id, $user->email, $user->name, 'api');
```

The request's header shows who it ran as, below the controller. On Laravel the adapter sets this itself from the first user a guard resolves, with the guard's name, and on Symfony the store declares the security token storage, so `Lerd::auth()` is for an app that authenticates its own way. Only the first user a request reports is shown.

## Tabs of your own

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

`Lerd::tab()` gives back the same tab for the same name, and each block reaches lerd as it is added, so several parts of an app can write to one tab. The blocks:

| Method | Block |
|---|---|
| `table($columns, $rows)` | Rows under named columns; a row is a list in column order or keyed by column. |
| `keyValue($values)` | Names and values; nested values render as a tree to fold open. |
| `counters($counters)` | Headline numbers in a row, a name above each value. |
| `code($code, $language)` | Code or other preformatted text. |
| `text($text)` | A paragraph. |
| `chart($chart)` | A line, bar or pie chart. |

Every block takes an optional heading and a `span`. `columns(n)` lays the tab out in up to four columns on a wide screen, a block spanning one unless told otherwise; a narrow screen shows one column.

### Charts

`Chart::line()`, `Chart::bar()` and `Chart::pie()` take `Series`, each a name, a value per label and an optional colour; a series without one takes the next colour in the palette. The chart draws over every series' labels in the order first seen, and a label a series has no value for draws as nothing. A pie shows its one series' labels as slices.

```php
Chart::line(
    new Series('p50', ['10:00' => 120, '10:05' => 135]),
    Series::make('p95', Color::Rose)->point('10:00', 310)->point('10:05', 420),
)->add(new Series('p99', ['10:05' => 980]));
```

## Laravel

The package's `Lerd\Debug\Frameworks\Laravel\DebugServiceProvider` is discovered on its own. `php artisan vendor:publish --tag=lerd-config` publishes `config/lerd.php`, whose `enabled` (`LERD_ENABLED`) switches the package off. It forgets the entries `Lerd` keeps once a queued job or an Octane request, task or tick is done, so a long-running worker does not carry one job's entries into the next. Nothing else needs registering: `Lerd` is a plain class with static methods.

## Symfony

Register `Lerd\Debug\Frameworks\Symfony\LerdDebugBundle` in `config/bundles.php`. It forgets the entries `Lerd` keeps when Symfony resets its services between requests in a worker runtime (FrankenPHP, RoadRunner, through `kernel.reset`) and after each Messenger message, handled or failed. `lerd.enabled: false` in `config/packages/lerd.yaml` switches the package off.

## Yii

List `Lerd\Debug\Frameworks\Yii\Bootstrap` under `bootstrap` in the app's config. It forgets the entries `Lerd` keeps after each yii2-queue job, run or failed, and a `lerd.enabled` param set to `false` switches the package off.

```php
// config/web.php and config/console.php
'bootstrap' => ['log', Lerd\Debug\Frameworks\Yii\Bootstrap::class],
```

## Other frameworks

`Lerd` needs nothing registered, so it works the same in CakePHP, CodeIgniter, Drupal, TYPO3, Magento, Tempest, WordPress or plain PHP. What an integration adds is a switch in the app's own config and forgetting kept entries between the jobs of a long-running worker; without one, `Lerd::flush()` at the end of each job does the second. The framework's own phases, route and user come from lerd's framework definitions either way, not from this package.

## Kept entries

Every entry passes `Lerd::track()`, which keeps the last 500 for `Lerd::entries()`, handy in a test that asserts what an app wrote. `Lerd::flush()` forgets them, along with any named events not yet stopped.

## Objects of your own

What `Lerd` takes is described by contracts in `Lerd\Debug\Contracts`, and no class in the package is final, so an app can hand over its own objects:

| Contract | Describes |
|---|---|
| `Trackable` | Anything `Lerd::track()` accepts. |
| `TimelineEntry` | A row on the timeline. |
| `LogEntry` | A log line. |
| `TabEntry` | A block added to a tab, with the tab it belongs to. |
| `Tab` | A tab: its id, title, columns, and adding a block. |
| `Block` | A block of a tab. |
| `Chart`, `Series` | A chart block and its series. |

A report object can implement `Chart` and be passed to `chart()` as it is, or a `Block` of your own be passed to `Tab::add()`.

## How lerd reads it

lerd's framework store declares a `lerd` seam on `Lerd::track()` for the `lerd/debug` package. lerd's extension observes the call and its collector reads the entry, so the package needs no lerd function and editors and static analysis know every call.

What the collector reads is produced by a renderer in `Lerd\Debug\Rendering`, one per version of lerd's schema. The collector names the version it reads (`DEBUG_SCHEMA`, 1 today) and gets the newest renderer the package has that is no newer, so either side can move to a new schema without breaking the other. A block or entry of a type the renderer does not know describes itself through its own `jsonSerialize()`.
