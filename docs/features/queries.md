# Debug window

The Debug window shows what each request did: its queries, views, cache calls, jobs, logs, exceptions and more, with a timeline of where the time went. It works for any PHP app, and every supported framework adds its own phases on top.

It is **off by default**. Turn it on from **System → Debug** or with `lerd dump on`. It shares the switch and the receiver with the [dump viewer](dumps.md), so there is nothing else to set up.

## Requests

The **Requests** lens comes first, in the Debug window and on each site's Debug tab. It lists every request, job and CLI run, newest first:

- A job is listed under the request that queued it, and the calls a page made later are listed under the page.
- Each row shows the method, status, time and the clock time it ran, plus what went wrong: an exception, an error log, a failed call, an N+1 or a slow query. A GraphQL request carries a GraphQL icon and is named by the operations it ran, the first one and `+N` for the rest.
- Filter to pages, API calls or CLI runs, or to requests with problems.
- lerd keeps the last 60,000 events in memory, every query, log line and cache call counting as one, which is around a thousand requests across all sites. The oldest go first, and nothing is kept once lerd-ui restarts.

![The Requests lens on a site's Debug tab, with jobs indented under the request that queued them](/assets/screenshots/requests-lens.png)

Open a request to see it in full. The header names the route, the controller and who the request ran as. Below it, one tab per kind of work, shown only when the request has something for it. The same view opens from the [debug bar](debug-bar.md) on the site's own pages.

A class is shown by its own name, with the full name on hover, and the app's own classes open in the editor.

**Performance** leads with response time, memory, app and database time, the FPM queue and the page load. Under them, one timeline puts everything in order:

- the browser's DNS, connect, wait, download and DOM phases
- nginx and the FPM queue
- the framework's phases: bootstrap, routing, middleware, controller, views
- queries, cache calls, Redis, storage, components, logs and your own rows

Each layer has a colour and a filter, and hovering a bar shows its duration. Views have a layer of their own. **Condense** folds rows of one kind that follow each other, such as queries, cache calls, logs or the browser's phases, into one row; the framework phases and your own rows always keep theirs.

![A request's Performance tab: headline numbers above one timeline of nginx, the FPM queue, the framework's phases, queries, cache calls and the app's own events](/assets/screenshots/request-performance.png)

**Request** holds the route, controller, middleware, route parameters, request and response headers, query string, body, cookies and session. A JSON request body is shown as a tree, and so is a JSON response body. Each route parameter is named as the route wrote it, `user:id` for one bound by a field, and shows its raw value and, where it resolved to a model, the model's class and key, which is never masked; the app's own middleware and models open in the editor. Anything that reads as a credential is masked.

![A request's Request tab with its route, controller and middleware](/assets/screenshots/request-detail.png)

The other tabs are GraphQL, Database, Models, Views, Components, Cache, Redis, Filesystem, Events, Log, Dumps, Mail & messages, HTTP, Jobs, Exceptions, Browser and Child requests, plus any tabs the app adds through the [lerd/debug package](debug-package.md).

Nothing a request recorded is dropped, however much it did. A long list shows 200 rows at a time with a button for the next 200, and on the timeline an opened group does the same, so a page that runs two thousand queries or renders two thousand views opens as quickly as a small one.

### How requests are linked

- Every PHP request gets an id, and every event it emits carries it. The response sends it back as `X-Lerd-Rid`.
- With [browser capture](browser-capture.md) on, the page view takes the id of the request that served it, and every `fetch` and XHR the page makes is linked to the request it reached, even on another site.
- Every PHP response carries a `Server-Timing` header with the FPM queue, each framework phase's total and then every timed event that ended before the headers went out, each query, cache call, Redis command, storage operation, outgoing request, component phase and row of your own on its own entry, so the browser's network panel shows the whole picture too. It is kept within what nginx buffers for headers, the rest summed up as one entry, and it is left out of the Request tab's response headers.
- CORS preflights and static files served without PHP are not linked.

## What each lens shows

Most of it is captured at the library every framework shares, so it is not tied to one framework. Laravel gets a little more through lerd's built-in adapter.

| Lens | Captured from | Works on |
|---|---|---|
| Queries | PDO | every PHP app |
| Views | Blade, Twig, and each framework's templates | Laravel, Symfony, every supported framework |
| Mail | Symfony Mailer | Laravel, Symfony, anything using it |
| Cache | Laravel's cache events | Laravel |
| Events | Laravel's dispatcher, Symfony EventDispatcher | Laravel, Symfony, Drupal |
| HTTP, with headers and timing | Laravel's HTTP client, Symfony HttpClient | Laravel, Symfony |
| Jobs | Laravel queue, Symfony Messenger, each framework's queue | every supported framework |
| App log | Monolog | every app using it |
| Exceptions | Sentry, Inspector | every app using them |
| Messages | Laravel notifications, Symfony Notifier | Laravel, Symfony |
| Timeline phases, route, user | framework definitions | every supported framework |
| Session | the framework's session store, or `$_SESSION` | every PHP app |
| Redis | Laravel's Redis connection | Laravel |
| Filesystem | Flysystem | Laravel, Symfony |
| Components | Livewire | Laravel |
| Models | Eloquent's model events | Laravel |
| GraphQL | the request body, and the schema where a framework definition hands it over | every PHP app |

The [debug package page](debug-package.md#what-lerd-records-on-its-own) lists what lerd records on each framework.

### Queries

- Grouped per request, with the count and total time.
- **N+1:** queries with the same shape (literals collapsed) three or more times in one request.
- **Slow:** any single query at 100 ms or more.
- Each query is numbered by its place in the run, and keeps that number while the list is filtered.
- **Show queries** on an N+1 finding narrows the list to the queries it repeats.
- **Format** lays each query out over several lines; the SQL is highlighted either way.
- Expand a row for the line that ran it, the bindings, and on Laravel the connection and read/write type.
- **Copy SQL** copies the query with its bindings filled in, ready to paste into a SQL editor.

![A request's Database tab: its counts by kind, two N+1 findings with a runnable example each, and the numbered, highlighted queries with the line that ran each one](/assets/screenshots/request-database.png)

Bindings are captured whether they are passed to `execute([...])` or bound one by one with `bindValue()` and `bindParam()`.

### Models

How many of each model the request retrieved, created, updated, deleted and restored. A model the app defines opens in the editor.

### Views

Every template rendered, numbered in the order it rendered, with its file and the data keys it was given. Each value is labelled by what it holds (`user User`, `page array(4)`, `title "Dashboard"`) rather than shipped whole, since a view's data often holds the logged-in user. A value whose name reads as a credential is masked, and so are such fields inside a JSON string, so an error page that renders the request body shows it masked. Blade's own compiled components are left out.

### GraphQL

A request to a GraphQL endpoint, whatever server answers it, gets a GraphQL tab next to Request and a GraphQL icon in the list. Each operation shows its type and name, the query, the variables, and for each root field it called the arguments, the response it got and any errors. Where a framework definition hands over the server's schema, each field is named by the type it returns, both open in the editor, and the schema is browsable up to eight levels deep. Arguments, variables and the response are masked like the request body; see [the limit on secrets written into the query itself](#masking-values).

![A GraphQL request batching a query and a mutation: each root field with the type it returns, its schema unfolded, the input and the response, and the mutation's password masked](/assets/screenshots/request-graphql.png)

### Mail and messages

- **Mail:** subject, recipients and an HTML preview, captured before it is sent.
- **Messages:** everything sent that is not mail, SMS, chat and the rest, with the channel, transport, both ends and the text. Laravel notifications show the channel, class and recipient. A message also raises a desktop notification.

Messages are reported, not caught: an SMS is really sent, and billed.

### Cache, events and HTTP

- **Cache:** hits, misses, writes and forgets with the key, the store, and the file or Redis connection behind it. Framework-internal keys (queue signals, scheduler mutexes, Horizon and Reverb) are left out.
- **Events:** your app's and packages' events; Laravel's own `Illuminate\*` events are left out.
- **HTTP:** outgoing calls with method, URL, status and time. Expand one for how long each phase took (DNS, connect, TLS, waiting for the first byte, download) and both ends' headers, with credentials and cookies masked. On Symfony the response is read as it ends, so a lazy response is complete and the client's own status check runs where the app expects it.

### Jobs

Each job is listed as a process of its own, with its class, how it ended and how long it took. On Laravel a job goes through `queued`, `processing` and `processed` or `failed`; a sync job is listed under the request that ran it. Other frameworks' queues come from the [framework definitions](../usage/framework-definitions.md).

Expand a job to see what it was handed:

```
program        Program #17
program.id     17
program.name   "Spring 2026"
items          array(12)
```

- Values are labelled the way views are, and a model is named by its key with its stored attributes one level down. Hidden attributes, like a password, are left out, and reading them never runs a query.
- On Laravel the payload is on the `queued` row only, since reading it in the worker would load every model again.
- A payload shows at most 60 entries, 20 per object.

### App log and exceptions

- **App log:** every Monolog record, with its level, channel, message and context. A record is shown even when your handlers drop it.
- **Exceptions:** anything reported to Sentry or Inspector, with the frames it was thrown from. No DSN is needed.

### Timeline, session, Redis, storage and components

- **Phases:** each framework's bootstrap, routing, middleware, controller and views, with the route that matched and who the request ran as.
- **Session:** what it held when the request finished. Keys that look like credentials are masked.
- **Redis:** every command, timed, with its connection.
- **Filesystem:** every Flysystem operation, timed, with its path and, on Laravel, the Storage disk.
- **Components:** each Livewire component's mount, update, method call and render, with what it was given and its state after rendering. The component's name opens its class, or the source file of a single-file component.

### Dumps, browser and child requests

- **Dumps:** each `dump()` the request made, in order.
- **Browser:** with [browser capture](browser-capture.md) on, the page's console lines, errors and network calls, each at the time it happened.
- **Child requests:** the calls the page sent after it loaded and the jobs it queued, each with its status, how it was sent, its time and the clock time it went out. A click opens it in the same view.

![A Drupal request in the Requests lens, with the kernel's bootstrap, handle and routing phases, its queries and its views on the timeline](/assets/screenshots/request-drupal.png)

## Keeping the noise down

- **Queue workers** (`queue:work`, `horizon`, `messenger:consume`) are hidden by default. Tick **Show worker queries** to see them, grouped per worker. Jobs always show.
- **Test runs** (PHPUnit and Pest) are not recorded by default. Tick **Show test runs** and rerun the test to see them.
- **Commands that only loop** are left out entirely, while the jobs they run still show. The framework definitions list them, and a project adds its own in `.lerd.yaml`, by name or by class:

    ```yaml
    devtools:
      exclude_commands:
        - inventory:watch
        - App\Console\Commands\PollFeeds
    ```

## Masking values

Anything that reads as a credential is masked wherever it is shown: headers, query string, body, cookies, session, outgoing requests, GraphQL variables, arguments and responses, and the values views, jobs and events are given. Names with password, secret, token, session, cookie, authorization or an API key in them are always masked, and a project cannot turn that off.

A project can mask more, and choose how a masked value looks, in `.lerd.yaml`:

```yaml
devtools:
  redact:
    style: masked              # how the built-in masks show: redacted (default) or masked
    visible: 4                 # characters a masked value keeps, 0 to 32
    crop: false                # true shortens the rest to a fixed run of 8
    char: "*"                  # the character it is masked with
    outgoing_http:
      headers:
        request:               # headers sent on outgoing requests
          X-Shop-Signature: masked
          X-Api-Secret:        # a style alone, or any of the masked settings too
            style: masked
            visible: 8
            crop: true
            char: "•"
        response:              # headers on the responses they get back
          X-Upstream-Token: redacted
        "*":                   # both directions
          X-Trace-*: masked    # globs work, case does not matter
    inbound_http:              # the site's own requests and responses
      headers:
        request:
          X-Signature: masked
        response:
          X-Internal-Id: redacted
      input:                   # names in the query string, body, cookies and session
        customer_ssn: { style: masked, visible: 0 }
      route_params:            # route parameters by their name in the route, masked in the path
        customer: { style: masked, visible: 2 }   # /customers/{customer:ssn}/details
```

Query values and route parameters are masked in the request's URL too, a route parameter by its `route_params` rule or else by an `input` rule of the same name, so a secret in `/customers/0101901234/details` or `?api_key=…` never shows in the list or a request's header. Route parameters are read before binding, by their name in the route, on Laravel, Symfony, Drupal, Yii, CakePHP and WordPress.

| Style | Shows |
|---|---|
| `redacted` | `[redacted]` |
| `masked` | The first characters, then the mask character for each one hidden, `sk_l************` for a 16-character value, so it keeps its length; with `crop: true` the hidden part is a fixed run of 8, `sk_l********`, so the length is not given away. `visible` sets how many characters show (4 by default), and a value no longer than twice that shows only the mask character. `char` picks that character, `*` by default. |

A header's settings fall back to the ones set directly under `redact`. A direction, style, `visible` outside 0 to 32 or a `char` longer than one character is refused with a warning naming it. The rules apply as soon as the file is saved.

A secret written straight into a GraphQL query or mutation, `login(password: "…")` rather than passed as a variable, is masked in the query text only when the argument's name is one of the always-masked ones above: password, secret, token, session, cookie, authorization or an API key. The project's own `input` rules apply to the field's arguments and to variables, but not to the query text, so pass anything sensitive as a variable.

## N+1 warnings

When a query shape repeats three times in one request, lerd sends one desktop notification, once per route or command per session. The warning names the route or command, and the request gets an **N+1** badge. It follows the global `lerd notify` setting.

## Open in editor

Every file path and every class the app defines opens in your editor with a click:

- the line that ran a query, a cache call, a Redis command or a storage operation, and each line of its stack trace
- the controller, the app's middleware, and the model a route parameter resolved to
- models on the Models tab, templates on the Views tab, Livewire components, single-file ones at their source
- a GraphQL field's resolver and the type it returns, in the operation and in the schema
- a log line's source and an exception's frames
- on the timeline, the controller and each row's source

A clickable name is underlined with dots; the copy button beside a path puts it on the clipboard instead. In the [debug bar](debug-bar.md) the same links work from a page on this machine.

The line that triggered a query, a cache call, a Redis command or a storage operation has a stack button beside it that opens the stack trace as a path from that line back to where the request began. The app's own lines are stops on the path, each named by the function it sits in, and runs of vendor frames fold into one that opens on a click. The code around the picked line shows beside the path, highlighted, and a click on any of its line numbers opens the editor at that line; **Code** hides it, and the choice is remembered. The trace opens on the app's own line, leaves out lerd's own frames and those of the lerd/debug package, and **Copy** puts it on the clipboard in PHP's own `#0 file(line): function()` form.

- Pick the editor on the System page, or per site in its controls. VS Code, Cursor, VSCodium, Windsurf, Sublime Text, Zed, PhpStorm, IntelliJ IDEA and WebStorm are listed, the installed ones first.
- An editor not on the PATH opens through its URL scheme, so a JetBrains IDE from Toolbox works too.
- **Custom…** takes a template: `myeditor --line {line} {file}` or `myeditor://open?file={file}&line={line}`.
- Once an editor is picked, the site header gets a button that opens the site, or the selected worktree, as a project in it.

![A site's controls with its editor picker](/assets/screenshots/site-editor-picker.png)

## For AI assistants

An assistant gets the same data through lerd's MCP server:

- The `request` tool reads one request by the id a response sends back as `X-Lerd-Rid`, everything the Debug window and the debug bar show. `summary` gives the timing, the phases, the N+1 and slow findings with runnable examples, exceptions, GraphQL operations and the requests it sent; `tabs` lists the tabs it has, the app's own included; `tab` reads one of them a page at a time; and `trace` reads a stack trace a row points at by its `trace_ref`. `list` gives a site's recent requests with their problems.
- `analyze_queries` reports N+1 and slow queries per request, each with the line that ran it and a runnable example.
- `dumps_recent` with a `kind` pulls raw events of any kind.

The same data is at `/api/requests` and `/api/requests/{rid}`.

## Caveats

- Queries need PDO. WordPress uses mysqli, which is not captured yet.
- Capture has a cost. Turn it off when you are not debugging.
- Nothing is kept on disk; the buffer resets when `lerd-ui` restarts.

??? note "How it works"

    lerd compiles a small PHP extension, `lerd_devtools`, into every PHP image. It uses PHP's observer API (PHP 8.0 and later) to watch PDO, the shared libraries above and the methods the framework definitions name, and hands each call to a small collector written in PHP. On Laravel, an adapter loaded at boot listens to the framework's own events instead.

    Capture follows the same runtime switch as the dump viewer, so turning it on or off never restarts PHP. Events travel over the same socket to `lerd-ui`, which keeps the last 60,000 in memory and streams them to the dashboard. A stack trace is sent once per request, however many events share it, and stored once.

    An event looks like this:

    ```json
    {
      "v": 1,
      "kind": "query",
      "ctx": { "type": "fpm", "site": "acme", "request": "GET /users", "rid": "..." },
      "src": { "file": "/home/u/Code/acme/app/Models/User.php", "line": 30 },
      "data": { "sql": "select * from users where id = ?", "bindings": [42], "time_ms": 1.42 }
    }
    ```
