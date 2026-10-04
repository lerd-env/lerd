# Query viewer

The dump viewer catches `dump()` / `dd()`, but most of what slows a request down never passes through either: the database queries. lerd's query viewer records every SQL statement a request (or an artisan command) runs, with its bindings, duration, and the exact line that fired it, and streams them to the same **Debug** view as dumps, grouped per request with N+1 detection and slow-query flags.

The feature is **off by default**. Enable it from **System → Debug → Queries** with the Enable button. It shares the dump receiver, so there is nothing else to wire up.

## How it works

Unlike the debug bridge, which works by redefining `dump()` from an `auto_prepend_file`, queries live inside the database layer where a prepend can't reach. lerd ships a small first-party Zend extension, **`lerd_devtools`**, compiled into every PHP-FPM image. It uses PHP's `zend_observer` API (PHP 8.0+) to observe `PDOStatement::execute`, `PDO::query`, and `PDO::exec`, capturing the SQL, the bound parameters, the wall-clock duration, and the calling `file:line` from the backtrace. Because it hooks at the engine level it works for any PDO app, framework or not. On PHP 7.x the extension still loads but captures nothing (no `zend_observer`); query capture needs 8.0+.

Capture is gated by the **same** runtime sentinel as the debug bridge, so the whole Debug window is one switch and toggling never restarts FPM:

- The extension and its config ini (`/usr/local/etc/php/conf.d/96-lerd-devtools.ini`) are always present in the image / mounted.
- `/usr/local/etc/lerd/enabled.flag` is the shared runtime sentinel. Both the debug bridge and the extension stat it once per request; present = capture, absent = no-op. There is no separate devtools enable flag, `lerd dump on/off` (or the dashboard Debug toggle) arms both at once. Worker capture has its own `devtools-workers.flag`, which lerd keeps set, so the dashboard's worker checkbox only hides worker rows and never loses events.

Events ship over the **same** Unix socket (Linux) or TCP loopback (macOS) the debug bridge uses, so `lerd-ui` buffers them in the same 500-event ring and fans them out through the same SSE stream. The web client filters by `kind` to render the Queries lens.

## What you get

- **Per-request grouping.** Every query is bucketed by the request (or a 5-second window for CLI), with a rollup header showing the query count and total time.
- **N+1 detection.** Queries whose SQL normalizes to the same fingerprint (literals collapsed) are flagged as duplicates; a request with three or more repeats of one shape is flagged **N+1**.
- **Slow-query tagging.** Any single query at or over 100ms is tagged **slow**.
- **Caller, bindings, connection.** Expand a row for the originating `file:line`, the bound parameters, and (when a framework adapter is present) the connection name and read/write type.
- **Copy the caller.** Every path in the debug views, the primary frame, each frame of an expanded stack trace, a rendered template, and a dump's header, opens in your editor when clicked and carries a faint copy button beside it that copies the whole `file:line` reference, even where the display is shortened.
- **Copy runnable SQL.** A copy button next to each query's duration copies the statement with its bindings resolved: positional `?` placeholders are replaced with properly escaped SQL literals (strings quoted, single quotes doubled, `NULL` for nulls, `1`/`0` for booleans), while question marks inside quoted strings are left untouched. Queries with no bindings copy verbatim. Paste it straight into a SQL editor without hand-substituting each parameter.

## Wire format

Query events reuse the dump event envelope; the kind-specific fields live under `data`:

```json
{
  "v": 1,
  "id": "...",
  "ts": "2026-06-01T12:34:56.123Z",
  "kind": "query",
  "ctx": { "type": "fpm", "site": "acme", "request": "GET /users", "pid": 1234 },
  "src": { "file": "/home/u/Code/acme/app/Models/User.php", "line": 30 },
  "data": {
    "sql": "select * from users where id = ?",
    "bindings": [42],
    "time_ms": 1.42,
    "connection": "mysql",
    "rw_type": "read"
  }
}
```

`connection` and `rw_type` are filled by the Laravel adapter; the engine-level capture always provides `sql`, `bindings`, and `time_ms`. Bindings are captured whether the app passes them to `execute([...])` or binds them one at a time with `bindValue()` / `bindParam()` (the extension buffers per-statement bound values and attaches them on execute), so Doctrine and other libraries that bind individually still show their parameters.

## Laravel adapter (richer capture)

For Laravel apps, the extension loads a small in-app adapter at `Application::boot` (observed at the engine level) that listens to `QueryExecuted`. While it's active the engine-level PDO capture stands down, so Laravel queries come through with data the raw PDO hook can't see:

- **Real bindings**: Laravel binds via `bindValue()`, invisible to the PDO observer; the adapter reads them from the event (formatted with `prepareBindings()`).
- **Connection name**: e.g. `pgsql`, `mysql`.
- **Per-job grouping**: every job runs under a request id of its own, whatever connection runs it, so it shows up in the Requests lens as a process of its own with its class, how it ended and how long it took. The id it interrupted comes back when the job ends, and a `sync` job is listed as a child request of the request it ran in. A job that threw with attempts to spare is reported as `errored`, one with none left as `failed`.

Non-Laravel apps (and queries that run before the framework boots) still fall back to the engine-level PDO capture. The adapter respects the same on/off policy as the engine path, never throws, and emits to the same socket.

Beyond queries, the same adapter feeds additional Debug sub-tabs:

- **Jobs** *(Laravel)*: the whole life of a queued job, `queued` where it was dispatched, then `processing`, then `processed` or `failed` in the worker, with the queue, the connection, the attempt count, how long the job took, and the exception on failure. The `queued` row also carries what the job was handed, see [job payloads](#job-payloads).
- **Views**: every template rendered, with its source path and the top-level data keys passed in. Each variable is labelled with what it holds, `page array(4)`, `user User`, `title "Dashboard"`, so the shape is visible without opening anything, with a string kept whole up to 160 characters and truncated past that. The values are not shipped: a view's data is the whole page payload on an Inertia app and routinely holds the authenticated user, so a label is both the useful part and the safe one. A render Blade compiled for itself is left out: an inline or anonymous component has no source file, so it appears under a hashed `__components::` name pointing into the compiled cache, and a template a developer wrote never lives there. The cache location comes from the app's own `view.compiled` config, so a project that moves it is still understood.
- **Mail**: outgoing messages captured before send, with subject, recipients, and a sandboxed HTML preview.
- **Cache** *(Laravel)*: hit / miss / write / forget events with the key and store, timed from the event the store fires before it goes to its backend, with the file a file store keeps the key in or the Redis connection a Redis store went through. On a request's timeline a timed call takes in the queries, Redis commands and filesystem operations that ran inside it, a database store's own `select from cache`, so they show in its popover instead of as rows of their own. Framework-internal keys (the queue restart/pause signals, scheduler overlap mutexes, and reverb/horizon/pulse/telescope pub-sub) are filtered out so the tab shows the application's own cache use rather than background machinery, this matters most with worker capture on, where those keys are polled constantly.
- **Events** *(Laravel)*: application and package events dispatched (framework-internal `Illuminate\*` events are filtered out).
- **HTTP** *(Laravel)*: outgoing requests made via Laravel's HTTP client (method, URL, status), so third-party API calls are visible the way queries are.

Each lens groups per request/job, shows the originating app frame with the stack trace and editor links, and is filterable by site and worker command. All of them render the newest 100 rows and load the next 100 as you reach the end, so a worker that has been running all afternoon doesn't put thousands of rows on screen at once.

## Framework coverage (agnostic seams)

Queries, Mail and Views are captured **agnostically** at the shared library every framework uses, so they are not Laravel-only. Where the Laravel adapter is active it claims these kinds (richer, event-sourced data) and the agnostic seams stand down to avoid double capture; everywhere else the extension observes the library directly and a small framework-neutral collector (`devtools-collector.php`, mounted next to the adapter) extracts the event in PHP:

- **Queries**: observed at the PDO layer, so any PDO app (Symfony/Doctrine, raw PDO, …) gets them with real bindings.
- **Mail**: observed at `Symfony\Component\Mailer\Mailer::send`, the de-facto mail library used directly by Symfony and wrapped by Laravel. The collector reads the `Symfony\Component\Mime\Email` for subject, recipients and body.
- **Views**: observed at `Twig\Environment::render` / `display`, the de-facto Symfony view layer. The collector resolves the on-disk `.twig` source path through Twig's loader the same way Blade's `getPath()` does, and skips the dev-only `@WebProfiler` toolbar.
- **Events**: observed at `Symfony\Component\EventDispatcher\EventDispatcher::dispatch` (the debug `TraceableEventDispatcher` delegates to it, so each dispatch is seen once). The collector keeps application events and drops the framework lifecycle noise (`kernel.*`, `console.*`, and the `Symfony\`/`Twig\`/`Doctrine\` component internals), mirroring the Laravel `Illuminate\*` filter.
- **Jobs**: observed at `Symfony\Component\Messenger\MessageBus::dispatch` (the debug `TraceableMessageBus` delegates to it). Each message dispatched to the bus is recorded with its class; an `Envelope` is unwrapped to the real message, and one carrying a `ReceivedStamp` is skipped because that is the worker handing a message it already received back to the bus, not a new dispatch. Status is `dispatched`, since the bus only signals that a message was queued, and the message's own properties ride along as the payload. What the worker then does with it comes from `WorkerMessageReceivedEvent`, `WorkerMessageHandledEvent` and `WorkerMessageFailedEvent`, which arrive through the same dispatcher seam and are reported as `processing`, `processed` and `failed` with the receiver name as the queue and the time the message took.
- **HTTP**: observed at `Symfony\Component\HttpClient\CurlHttpClient::request` / `NativeHttpClient::request` (the default factory yields one of these; decorators delegate down to them). Captured at the *begin* of the call because `request()` rewrites its own `$url` argument internally. Symfony responses are lazy, so no status code is known at call time, the row shows the method and URL with a `sent` marker rather than a status, whereas Laravel's adapter (which listens after the response arrives) shows the real code.

- **App log**: observed at `Monolog\Logger::addRecord`, declared by the `monolog/monolog` package in the store rather than by any framework, since the class ships with the package. Every logging facade a framework puts in front of it, Laravel's `Log`, Symfony's injected PSR logger, a library writing to its own channel, ends up there, and the arguments have not changed since Monolog 1. The row carries the level, the channel the logger was named after, and the message; expand it for the context array, rendered in full the way a dump is, since the context is what the developer chose to attach. The level is read whichever way it arrived, as Monolog 3's enum or as either of the integer scales the older majors accept, and the lens, named App log so it is not taken for the site's own Logs tab, gains a level filter when a request logged at more than one. Records are taken where they are written rather than where they are handled, so a line your handlers drop at this level still shows.

- **Exceptions**: observed at `Sentry\Client::captureEvent`, declared by the `sentry/sentry` package in the store. Every capture the SDK makes funnels through it, an exception, an event the app assembled itself, a captured message, so one seam covers them all. Locally a report usually goes nowhere, there being no DSN configured, and where one is set it goes to a project nobody watches for a developer's own laptop, so the window shows it instead; nothing about what Sentry then does changes. An exception carries the frames it was thrown from rather than the line that reported it, resolved to the developer's own code the way every other capture is, and a wrapped cause is named under the row. Capture does not depend on the app throwing: anything it hands the SDK, including a `captureMessage`, arrives here.

  Inspector reports the same way, by handing over the throwable, so `inspector-apm/inspector-php` declares the same kind of seam at `Inspector\Inspector::reportException` and its faults arrive in the same lens, read the same way, each row naming the reporter it was taken from since an app can run both. Its segments are deliberately not captured: a segment is recorded before it has a duration, and the spans they describe (queries, outgoing HTTP, views, jobs) already have lenses of their own that time them.

- **Messages**: everything a site sends that is not mail. Symfony's Notifier is the seam, declared by `symfony/notifier` in the store at `Texter::send` and `Chatter::send`: every transport the component ships, Twilio, Vonage, Slack and the rest, is reached through those two, and each message answers the same interface whatever channel it is on, so the row carries the channel, the transport that would carry it, both ends and the text. Laravel routes its notifications through its own channels rather than Notifier, so the adapter reports those instead, on every channel but mail, which the Mail lens already shows in full. What a Laravel notification *says* is built inside the channel by a `to*` method on the app's own code, and calling it a second time to read the text would run the app's queries again, so that row names the channel, the notification class and where it was routed rather than the body. The lens narrows by channel.

  A message also raises a notification, on by default, for the same reason: nothing catches it, so without one the only sign it went out is a tab you were not looking at.

  Worth knowing: this reports, it does not catch. A caught mail never leaves, because a local catcher answers for it; an SMS has no such catcher here, so it is really sent, and really billed, while you watch it go.

- **Spans**: the framework's own phases, timed and drawn as the Framework layer on a request's timeline in the Requests lens. The store declares them as `span` seams with a label: `laravel/framework` times Bootstrap, Routing, the Middleware stack, the Controller (named by its action), each View and Terminate, `symfony/http-kernel` times Bootstrap, Routing, Handle and Terminate, `twig/twig` each template, and `symfony/http-foundation` sending the response, which Laravel shares. Every other supported framework declares its own the same way: Yii 2, CakePHP, CodeIgniter 4, Drupal, TYPO3, Magento, Tempest, and WordPress and Bedrock, which have fewer classes to time, so their main query and REST requests. Each also names the matched route and, where the framework keeps one, who the request runs as.

    ![A Drupal request in the Requests lens, with the kernel's phases, routing and the controller on the timeline](/assets/screenshots/request-drupal.png)

- **Session**: what the session held when the request finished, one row per top-level key with nested values as JSON. A framework's session store is read as it saves, declared in the store for `laravel/framework` and `symfony/http-foundation`, and a plain PHP session is read from `$_SESSION` at the end of the request. A key naming a password or a secret is masked.
- **Redis**: every command, timed, with its arguments and the connection it ran on, declared by `laravel/framework` at the connection's `command` method, which both the phpredis and the Predis driver go through.
- **Filesystem**: storage operations through Flysystem, declared by `league/flysystem` at its `Filesystem` methods (read, write, delete, fileExists, listContents and the rest), timed with the path they touched and, on Laravel, the name of the Storage disk. Laravel's `Storage` facade and its S3 disks go through it, and so does Symfony's Flysystem bundle.
- **Components**: a UI component's lifecycle, declared by `livewire/livewire` in the store at Livewire 3 and 4's `HandleComponents` mechanism: `mount` on the first render, then `updateProperty` and `callMethods` for each round trip, each followed by `render`. Every phase is a span with its time, the component's own name and what it was handed, a property's path and value or the methods called with their parameters, so a slow component and the update that made it slow show up in the request they ran in. The kind is generic: a store entry naming another library's lifecycle methods and where its component name comes from gets the same capture.

Cache still comes solely from the Laravel adapter. Symfony spreads cache across many adapter classes with the read path living in a trait, so there's no single canonical seam to observe; the only single-class option is the dev-only `TraceableAdapter`, which is also extremely noisy (the framework hammers its system pools every request). It's deferred rather than captured half-complete. The Debug sub-tabs reflect this: a Symfony site shows Dumps, Queries, Mail, Views, Events, Jobs, HTTP, App log, Exceptions and Messages; a Laravel site shows all of them.

## Job payloads

A job row expands to show what the job was given:

```
program        Program #17
program.id     17
program.name   "Spring 2026"
program.status "active"
email          "client@example.test"
items          array(12)
```

Scalars are kept whole and everything else is labelled by what it is, the same rule the Views lens follows and for the same reason: a queued job routinely holds the authenticated user, and the shape is the useful half without the tokens. Only properties declared on the job's own class are read, so a job extending a framework base class, or using a trait like Laravel's `Queueable`, reports what its author passed rather than the thirteen properties the trait brings.

An enum reads as its case, `ProgramStatus::Published`, with the backing value appended only when it differs from the case name (`Priority::High (9)`). A bare class name would say nothing, and the case is what the code works with.

A model is the case that made this worth doing. `program Program` says nothing about which program, so an object is named by its key where it has one and then described one level down under dotted keys. For a model those are its stored attributes, read from what is already loaded and never through an accessor, an appended attribute or a relation, so opening a job row cannot put a query on the database. That also means an attribute is reported as it is stored: a column cast to an enum shows the string in the database rather than the enum instance, which is what the queue is actually carrying. Whatever the model hides, a password or a remember token, is left out on its own say-so. For anything else they are the properties its class declares. Only one level: a model inside a model reads as `owner User #3` and stops there, so an object graph can never be walked. A payload is capped at 60 entries and any one object at 20.

Where the payload comes from depends on what there is to read:

- **Laravel** reports it on the `queued` row only. Once a job is on the queue it is a serialized blob, and reading that back in the worker would re-hydrate every model the job holds through `SerializesModels`, firing the queries a debug lens must never cause. The `queued` row therefore carries the payload and the job's uuid, and the worker's rows carry the same uuid, so one job's four states tie together.
- **Symfony** reports it on every state, since Messenger hands the worker the message object itself.
- **Store seams** report the first argument where the observed method takes one, which is how a queue passes an item (Drupal's `processItem($data)` shows `id 999999`), and the job object's own properties where it does not.

## Queued jobs (store-declared seams)

Laravel and Symfony aside, a framework's queue is reported from data rather than code. A framework definition in the store declares which method runs a queued job:

```yaml
devtools:
  jobs:
    - implements: Drupal\Core\Queue\QueueWorkerInterface
      method: processItem
```

lerd renders every framework's declarations into `devtools-seams.conf` next to the collector, and the extension reads it at startup. It observes the declared method, reporting `processing` on the way in and `processed` on the way out, or `failed` (carrying the throwable's message) when the call is unwinding with an exception, timed and grouped per job the same way Laravel's and Symfony's are. Because nothing is compiled in, a new framework's queue is a store change that reaches every install within a day, with no binary release. A running FPM container picks up a new seam on its next restart, like the ini beside it.

A seam matches on `class`, `implements` or `extends`, and the optional `name` says where the job's label comes from: `this` (the default) or `arg:N`, either followed by `.method:getHook` or `.prop:queue`. An object with no accessor yields its class, which is what a job is usually called. Ship the seams today: CakePHP (`Cake\Queue\Job\JobInterface::execute`), CodeIgniter (`CodeIgniter\Queue\Interfaces\JobInterface::process`), Drupal (`Drupal\Core\Queue\QueueWorkerInterface::processItem`), TYPO3 (`TYPO3\CMS\Scheduler\Task\AbstractTask::execute`) and WordPress (Action Scheduler, labelled by the hook it runs). Magento and Tempest are not covered yet: neither has a per-message seam that can be named with confidence without checking it against a real install.

## Queue workers (hidden by default)

Long-running queue and scheduler workers (`queue:work`, `horizon`, `schedule:work`, `messenger:consume`) poll the database constantly, so showing them by default would bury the web-request queries you're actually debugging. Worker events are therefore **hidden by default**. They are still recorded, so a worker's queries from before you ticked the box are there when you do.

Jobs are the exception, and are shown whatever this toggle says. A worker's jobs are the queue's own feedback rather than noise about it, and hiding them is what left a queue being drained looking like nothing was happening at all. The Jobs lens therefore carries no worker checkbox; it offers a status filter instead, because one job now reports every state it passes through.

Show them with the **Show worker queries** checkbox in the Debug window toolbar (present on every lens but Jobs: Queries, Views, Mail, Cache, Events, HTTP). Each worker invocation is grouped on its own, labelled by the worker command, and a per-command filter dropdown appears so you can narrow to one worker. The Laravel adapter resets the request id on every `JobProcessing`, so each queued job is its own group rather than a worker's jobs lumping together. Unchecking it hides the worker rows again. The setting is saved in `devtools.workers`, is shared with the TUI's `w` key, and is independent of the main Debug on/off switch.

## Your own timeline rows and tabs

An app or a package can put its own work on a request's timeline, write log lines, and add tabs of its own with tables, figures and charts through the `lerd/debug` Composer package, which does nothing outside lerd. See [Debug package](debug-package.md).

## Commands left out

A console command that only loops, a queue worker, Horizon or a websocket server, is left out of the Debug window entirely, while each job it runs is reported as a process of its own. The store lists them under `devtools.exclude_commands` (`laravel/framework`: `queue:work`, `queue:listen`, `schedule:work`; `laravel/horizon`: its processes; `laravel/reverb`: `reverb:start`), and a project adds its own in `.lerd.yaml`, as typed or by class:

```yaml
debug:
  exclude_commands:
    - inventory:watch
    - App\Console\Commands\PollFeeds
```

A command named as typed is recognised from its arguments before the framework boots, so even its boot is left out; one named by class is recognised as it starts to run, through the `command` seam `symfony/console` declares on `Command::run`, which Laravel's commands run through as well. The list reaches the collector through `devtools-exclude.conf` next to it, rewritten when `.lerd.yaml` changes.

## Test runs (not recorded by default)

A test suite is the other kind of flood: it is CLI, high volume, and a feature suite fires hundreds of simulated requests, so one run can clear the buffer of everything you were looking at. Every event captured inside a PHPUnit or Pest run therefore carries `ctx.test`, and lerd does not record those events by default.

Unlike worker events, test events are not recorded while this is off: the receiver drops test events before they reach the buffer, so a suite cannot evict anything else and fires no N+1 warnings. Tick **Show test runs** in any lens toolbar and rerun the test when you need to inspect a dump or a query from inside it. Unticking it also removes the test events already buffered, freeing the space they held. The setting is saved in `devtools.tests` in the global config, so it holds across restarts and for every open dashboard.

The signal is PHPUnit's own `PHPUNIT_COMPOSER_INSTALL` bootstrap constant, which Pest inherits, so it is ecosystem-level rather than tied to a framework. Filtering on `ctx.type` would not do: artisan, tinker and queue workers are all CLI too.

## N+1 warnings

When a query shape repeats past a threshold (3×) within a single request or worker invocation, lerd fires one OS notification, **once per route/script per session**: so it warns you without nagging on every subsequent hit of the same endpoint. The dashboard also flags the request group with an **N+1** badge and tints the duplicate rows. Notifications respect the global `lerd notify` toggle.

Every warning names the run the queries came from: the worker command if the capture came from an opted-in worker, otherwise the CLI invocation (`artisan sync:users --all`, with long argument values elided) or the request route (`GET /orders`). The label locates the run, not the query, whose exact origin and SQL are already on the events in the Debug lens. The same value separates the warnings, so one noisy artisan command does not silence the next one for the rest of the session.

## Debugging over MCP

The same capture is available to an AI assistant through lerd's MCP server, so an agent can debug and fix performance issues end to end. The loop: `dumps_toggle` to arm capture, `dumps_clear` for a clean slate, trigger the page or job, then `analyze_queries` for a per-request N+1 and slow-query report, each finding carries the originating `file:line`, so the agent can open the offending code and add a `with()` eager-load, an index, or a cache, then re-run to confirm the count dropped. Each N+1 and slow-query finding also carries `example_sql`, the query with its bindings in place, ready to run or `EXPLAIN` locally. `request` gives the whole of one request in one call, see [Request linking](#request-linking). `dumps_recent` with a `kind` filter (`query`, `mail`, `view`, `log`, `exception`, `message`, …) pulls the raw events for anything the report doesn't cover. The analysis is server-side, so it uses the same fingerprinting as the dashboard badge and the N+1 notification.

## Request linking

Every PHP request gets an id from the devtools extension, and every event it emits carries it. lerd sends that id with the response as `X-Lerd-Rid`, whether capture is on or not, and at the end of each web request the bridge reports how it ended: its method, URI, the route name when the app gave the matched route one (declared in the store, for `laravel/framework` at `Route::bind`), status, how long PHP spent on it and its peak memory. Together they make a request a thing of its own rather than a group of events that happen to share a URL.

With [browser capture](browser-capture.md) on, the id reaches the page too. nginx puts it on the injected script tag, so the page view takes the id of the request that served it, and the script reads the header off every `fetch` and XHR response and reports the call as a linked request, whatever its status. Responses expose the header to other origins, so a single-page app on one site calling an API on another links to the request it reached there: the API request names the page view that sent it, and the page lists every request it made. CORS preflights are not linked, since the browser does not hand them to scripts.

The **Requests** lens, first in the Debug window and on each site's Debug tab, lists every request lerd saw, newest first, with the child requests a page sent indented under it. Each row shows the method, status, time and what went wrong (an exception, an error log, a failed call, an N+1 or a slow query), and the list narrows to pages, API calls or CLI runs, or to requests with problems. Opening one shows it in a dialog filling the window, its header naming who the request ran as when the app or its framework said (Laravel's guards and Symfony's security token are read on their own), with a tab per kind of work, a tab only when the request has something for it. Performance leads with the headline numbers, response time, memory, app and database time, the FPM queue and the page load, above a timeline that puts everything on one chronological list: the browser's DNS, connect, wait, download and DOM phases, nginx and the FPM queue, the request itself, the framework's phases, queries, components, views, logs and the page's own events up to its load. Each layer has a colour and an icon to filter on, a run of close events folds into one row, and hovering a bar shows its duration and when it started. Request has the route, controller, request and response headers, query string, body, cookies and session, with anything that reads as a credential masked. Database, Views, Components, Cache, Redis, Filesystem, Events, Log, Dumps, Mail & messages, HTTP, Jobs, Exceptions and Browser each list their own, file paths open in the editor, and Child requests lists the calls a page made later, which stay off the timeline since minutes can pass before one.

![The Requests lens on a site's Debug tab, with jobs indented under the request that queued them](/assets/screenshots/requests-lens.png)

![A request's Performance tab: headline numbers above one timeline of nginx, the FPM queue, the framework's phases, queries, cache calls and the app's own events](/assets/screenshots/request-performance.png)

![A request's Request tab with its route, controller and middleware](/assets/screenshots/request-detail.png)

Every PHP response also carries a `Server-Timing` header with the FPM queue and the framework phases that finished before the headers went out, so the browser's own network panel shows them under the request's timing.

Over MCP, `diag` with `action: "request"` lists a site's recent requests with what went wrong in each, and with a `rid` returns one request and everything that carried its id: its queries with runnable examples, exceptions, logs, dumps, components, spans, the session, mail, events, the browser events of its page view, the page view that sent it and the requests it sent. The same comes from `/api/requests` and `/api/requests/{rid}`.

## Open in editor

Every file path the Debug window shows is a link: a query's caller, a view's template, a cached file, a log line's origin, and on a request's timeline the controller (by its method, or a closure by the file and line it was written on) and every row's source. Expand a Queries row to see the originating application frame (`Class::method — file:line`) and a **Details** button for the full stack trace; click any `file:line` to open it.

The editor comes from a curated list, VS Code, Cursor, VSCodium, Windsurf, Sublime Text, Zed, PhpStorm, IntelliJ IDEA and WebStorm, with the ones found on this machine (on PATH, through a desktop entry claiming the editor's URL scheme, which is how JetBrains Toolbox and Flatpak install them, or as an app bundle on macOS) listed first. Choose it globally on the System page, and per site in the site's controls, where the default follows the global choice; a site's choice is kept in lerd's own site registry rather than in `.lerd.yaml`, since it is personal. An editor whose binary is on PATH is started directly, and one that is not is opened through its URL scheme (`phpstorm://open?file=…&line=…`, `vscode://file/…:…`), which the browser hands to the desktop. An editor not on the list is set as **Custom…** with a template naming where the path and line go, `myeditor --line {line} {file}` to run a command or `myeditor://open?file={file}&line={line}` to open a URL. The global choice is the `editor` key in `~/.config/lerd/config.yaml`, which also still takes a command template written by hand. Once an editor is chosen, globally or for the site, the site header also gets a button that opens the site's folder, or the selected worktree's, as a project in it; with only an editor found by probing there is no button, since a project should open where you said. The site's editor picker shows the global choice by name while it is following it. The endpoint requires dashboard-control authority, which authenticated remote sessions receive.

![A site's controls with its editor picker](/assets/screenshots/site-editor-picker.png)

## Caveats

- **PDO-backed databases.** Queries run on a PDO driver (including Doctrine over PDO) are captured; raw `mysqli` (WordPress) lands in follow-up work.
- **Bindings cover both styles.** Both `PDOStatement::execute([...])` arrays and individually bound `bindValue()` / `bindParam()` values are captured. Very large bound values (e.g. a serialized Messenger envelope) are shown verbatim.
- **Capture has overhead.** Like the debug bridge, leave it off when you're not actively debugging; it's a development tool, not something to run under load.
- **No persistence.** The buffer is in-memory and resets when `lerd-ui` restarts.
