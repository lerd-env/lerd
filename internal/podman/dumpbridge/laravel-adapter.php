<?php
// /usr/local/etc/lerd/laravel-adapter.php
//
// Loaded by the lerd_devtools Zend extension when Illuminate\Foundation\
// Application::boot() returns, so the framework is up and app() is usable.
// It registers Laravel event listeners and ships structured events to the same
// socket the debug bridge uses, where lerd-ui buffers and fans them out.
//
// While this adapter is active the extension's engine-level PDO observer stops
// emitting queries, because QueryExecuted gives us richer data: real bindings
// (Laravel binds via bindValue, invisible to the PDO hook), the connection
// name, and a per-job request id so each queued job is its own group.
//
// Like the debug bridge, this file must never throw, block, or emit output.

namespace Lerd\LaravelAdapter;

// LERD_DEVTOOLS_ON is capture with the worker policy already applied;
// LERD_DEVTOOLS_JOBS is the same decision without it, so a queue or scheduler
// worker lands here with only the second set and reports nothing but the jobs
// it runs. An extension older than this file sets only the first, and then
// there is no such case to be in.
if (!(defined('LERD_DEVTOOLS_ON') && \LERD_DEVTOOLS_ON)
    && !(defined('LERD_DEVTOOLS_JOBS') && \LERD_DEVTOOLS_JOBS)) {
    return;
}
if (defined(__NAMESPACE__ . '\\REGISTERED')) {
    return;
}
const REGISTERED = 1;

// PAYLOAD_KEYS caps a whole payload; PAYLOAD_NESTED caps how much of one
// object inside it is described.
const PAYLOAD_KEYS = 60;
const PAYLOAD_NESTED = 20;

// full reports whether this process captures every kind. A worker is not one of
// them unless the user opted into full worker capture: everything it does
// besides running jobs is background polling that would bury the request being
// debugged.
function full(): bool
{
    return defined('LERD_DEVTOOLS_ON') && \LERD_DEVTOOLS_ON;
}

function target(): string
{
    $h = \get_cfg_var('lerd.devtools_host');
    return (is_string($h) && $h !== '') ? $h : '';
}

function send(array $payload): void
{
    $t = target();
    if ($t === '') {
        return;
    }
    if (strpos($t, '://') === false) {
        $t = 'tcp://' . $t;
    }
    $sock = @\stream_socket_client($t, $errno, $errstr, 0.05, \STREAM_CLIENT_CONNECT);
    if (!$sock) {
        return;
    }
    @\stream_set_blocking($sock, false);
    $line = \json_encode($payload, \JSON_UNESCAPED_SLASHES | \JSON_PARTIAL_OUTPUT_ON_ERROR);
    if ($line !== false) {
        @\fwrite($sock, $line . "\n");
    }
    @\fclose($sock);
}

function lerd_var(string $key): string
{
    if (!empty($_SERVER[$key])) {
        return (string) $_SERVER[$key];
    }
    $env = getenv($key);
    return $env === false ? '' : $env;
}

function new_id(): string
{
    try {
        return bin2hex(random_bytes(12));
    } catch (\Throwable $_) {
        return (string) (microtime(true) * 1000) . '-' . mt_rand();
    }
}

// One request id per HTTP request / per job. Reset on JobProcessing so a
// queue worker's jobs each form their own group instead of lumping together.
$GLOBALS['__lerd_rid'] = new_id();
function rid(): string
{
    return $GLOBALS['__lerd_rid'] ?? '';
}

function ts(): string
{
    $now = microtime(true);
    $ms = (int) (($now - floor($now)) * 1000);
    return gmdate('Y-m-d\TH:i:s.', (int) $now) . sprintf('%03dZ', $ms);
}

// in_test mirrors the collector's signal: PHPUnit's bootstrap defines
// PHPUNIT_COMPOSER_INSTALL on every run, Pest included.
function in_test(): bool
{
    return defined('PHPUNIT_COMPOSER_INSTALL') || class_exists('PHPUnit\\Framework\\TestCase', false);
}

// condense_arg mirrors the collector's: short arguments survive intact, long
// values are elided ("--execute=..."), since the command names the run and the
// event's src/data carry the exact detail.
function condense_arg(string $arg): string
{
    if (strlen($arg) <= 32) {
        return $arg;
    }
    $eq = strpos($arg, '=');
    if ($eq !== false && $eq < 32) {
        return substr($arg, 0, $eq + 1) . '...';
    }
    return substr($arg, 0, 29) . '...';
}

// command_line mirrors the collector's: the CLI invocation an event came from,
// e.g. "artisan queue:work --queue=high", condensed per argument and capped so
// a long invocation can't bloat the context of every event the process emits.
function command_line(): string
{
    $argv = $_SERVER['argv'] ?? null;
    if (!is_array($argv) || !isset($argv[0])) {
        return '';
    }
    $parts = [basename((string) $argv[0])];
    foreach (array_slice($argv, 1) as $arg) {
        $parts[] = condense_arg((string) $arg);
    }
    $line = trim(implode(' ', $parts));
    return strlen($line) > 120 ? substr($line, 0, 117) . '...' : $line;
}

function context(): array
{
    $ctx = [
        'type'   => \PHP_SAPI === 'cli' ? 'cli' : 'fpm',
        'site'   => lerd_var('LERD_SITE'),
        'branch' => lerd_var('LERD_BRANCH'),
        'rid'    => rid(),
    ];
    if (\PHP_SAPI !== 'cli') {
        $ctx['domain']  = isset($_SERVER['HTTP_HOST']) ? (string) $_SERVER['HTTP_HOST'] : '';
        $ctx['request'] = isset($_SERVER['REQUEST_METHOD'])
            ? $_SERVER['REQUEST_METHOD'] . ' ' . ($_SERVER['REQUEST_URI'] ?? '')
            : '';
    } else {
        // The CLI counterpart of request: what a console event points at.
        $ctx['command'] = command_line();
    }
    $worker = defined('LERD_DEVTOOLS_WORKER') ? (string) \LERD_DEVTOOLS_WORKER : '';
    if ($worker !== '') {
        $ctx['worker'] = $worker;
    }
    if (in_test()) {
        $ctx['test'] = true;
    }
    return array_filter($ctx, static fn ($v) => $v !== '' && $v !== null);
}

// backtrace builds the call chain and picks the first application frame
// (outside vendor/) as the primary src, like the engine-level path. The
// QueryExecuted event fires synchronously inside Connection::run, so the live
// stack still contains the controller/model that issued the query.
function backtrace(): array
{
    $bt = debug_backtrace(\DEBUG_BACKTRACE_IGNORE_ARGS, 50);
    $trace = [];
    $src = null;
    $fallback = null;
    foreach ($bt as $f) {
        if (!isset($f['file'])) {
            continue;
        }
        $file = $f['file'];
        if (strpos($file, 'laravel-adapter.php') !== false) {
            continue;
        }
        $line = $f['line'] ?? 0;
        $func = (isset($f['class']) ? $f['class'] . ($f['type'] ?? '::') : '') . ($f['function'] ?? '');
        $trace[] = ['file' => $file, 'line' => $line, 'func' => $func];
        if ($fallback === null) {
            $fallback = ['file' => $file, 'line' => $line];
        }
        if ($src === null && strpos($file, '/vendor/') === false) {
            $src = ['file' => $file, 'line' => $line];
        }
    }
    return ['src' => $src ?? $fallback ?? ['file' => '', 'line' => 0], 'trace' => $trace];
}

// emit_with ships one event using an explicit src + trace (e.g. an exception's
// own origin rather than the log call site).
function emit_with(string $kind, array $data, array $src, array $trace): void
{
    try {
        $data['trace'] = $trace;
        send([
            'v'    => 1,
            'id'   => new_id(),
            'ts'   => ts(),
            'kind' => $kind,
            'ctx'  => context(),
            'src'  => $src,
            'data' => $data,
        ]);
    } catch (\Throwable $_) {
        // never throw out of a listener
    }
}

// emit ships one event of the given kind, attaching the current caller src and
// full trace automatically.
function emit(string $kind, array $data): void
{
    $bt = backtrace();
    emit_with($kind, $data, $bt['src'], $bt['trace']);
}

// compiled_view_dir returns where Blade writes compiled templates, read from
// the app's own config so a project that moves the cache is still understood.
function compiled_view_dir($app): string
{
    try {
        $dir = $app['config']['view.compiled'] ?? '';
    } catch (\Throwable $_) {
        return '';
    }
    if (!is_string($dir) || $dir === '') {
        return '';
    }
    return rtrim($dir, DIRECTORY_SEPARATOR) . DIRECTORY_SEPARATOR;
}

// is_synthetic_view reports whether a render is one Blade made for itself
// rather than a template in the project.
//
// An inline or anonymous component has no source file, so Blade registers it
// under a hashed name and its path is the compiled artefact. Reporting those
// fills the view lens with md5 names and cache paths, none of which is
// something the developer wrote or can open.
function is_synthetic_view(string $path, string $compiledDir): bool
{
    if ($path === '' || $compiledDir === '') {
        return false;
    }
    return strncmp($path, $compiledDir, strlen($compiledDir)) === 0;
}

// preview_value renders one view variable as a short label, enough to tell a
// string from a 40-item collection from a model.
//
// The values themselves are not shipped: view data is a whole page payload on
// an Inertia app and routinely holds the authenticated user, so the shape is
// both the useful part and the safe one.
function preview_value($v): string
{
    if ($v === null) {
        return 'null';
    }
    if (is_bool($v)) {
        return $v ? 'true' : 'false';
    }
    if (is_int($v) || is_float($v)) {
        return (string) $v;
    }
    if (is_string($v)) {
        return strlen($v) > 160 ? '"' . substr($v, 0, 157) . '..."' : '"' . $v . '"';
    }
    if (is_array($v)) {
        return 'array(' . count($v) . ')';
    }
    if ($v instanceof \Closure) {
        return 'Closure';
    }
    if (is_object($v)) {
        $class = get_class($v);
        $pos = strrpos($class, '\\');
        $short = $pos === false ? $class : substr($class, $pos + 1);
        // An enum is its case, not its class: "ProgramStatus::Published" says
        // what a bare class name cannot. The backing value is added only when
        // it differs from the case name, which is usually is not the case.
        if ($v instanceof \UnitEnum) {
            $label = $short . '::' . $v->name;
            if ($v instanceof \BackedEnum && (string) $v->value !== $v->name) {
                $label .= ' (' . $v->value . ')';
            }
            return $label;
        }
        // A model says which record it is. The key is what a queued job is
        // really carrying, and it costs nothing to read.
        if (method_exists($v, 'getKey')) {
            try {
                $key = $v->getKey();
                if (is_scalar($key) && (string) $key !== '') {
                    return $short . ' #' . $key;
                }
            } catch (\Throwable $_) {
            }
        }
        if ($v instanceof \Countable) {
            try {
                return $short . '(' . count($v) . ')';
            } catch (\Throwable $_) {
                return $short;
            }
        }
        return $short;
    }
    return gettype($v);
}

// preview_data labels the variables a template was actually given, capped so
// one view with a large context cannot dominate the buffer.
//
// getData() merges three things: what the developer passed, what the factory
// shares with every view, and what Blade adds while rendering. Only the first
// says anything about this template, so the shared keys are subtracted and the
// double-underscore ones Blade reserves for itself (__env, __laravel_slots,
// __currentLoopData) are dropped.
function preview_data($data, array $shared = []): array
{
    $out = [];
    if (!is_array($data)) {
        return $out;
    }
    // What Blade hands a component view to render it, alongside the props the
    // developer wrote. Not shared, not underscored, and the same on every
    // component, so neither rule above reaches them.
    static $machinery = ['componentName', 'attributes', 'slot', 'component', 'constructor', 'ignoredParameterNames'];
    foreach ($data as $k => $v) {
        if (count($out) >= 40) {
            break;
        }
        $key = (string) $k;
        if (strncmp($key, '__', 2) === 0 || array_key_exists($key, $shared) || in_array($key, $machinery, true)) {
            continue;
        }
        try {
            $out[$key] = preview_value($v);
        } catch (\Throwable $_) {
            $out[$key] = '?';
        }
    }
    return $out;
}

// shared_view_data is what the factory injects into every view, so it can be
// told apart from what this one was passed. A factory that cannot be asked
// yields nothing, and every variable is reported as the developer's.
function shared_view_data($v): array
{
    try {
        if (!method_exists($v, 'getFactory')) {
            return [];
        }
        $factory = $v->getFactory();
        if (!is_object($factory) || !method_exists($factory, 'getShared')) {
            return [];
        }
        $shared = $factory->getShared();
        return is_array($shared) ? $shared : [];
    } catch (\Throwable $_) {
        return [];
    }
}

// addrs flattens a Symfony Mime address list to plain "name@host" strings.
function addrs($list): array
{
    $out = [];
    if (is_iterable($list)) {
        foreach ($list as $a) {
            $out[] = is_object($a) && method_exists($a, 'getAddress') ? $a->getAddress() : (string) $a;
        }
    }
    return $out;
}

function job_name($job): string
{
    if (is_object($job) && method_exists($job, 'resolveName')) {
        try {
            return (string) $job->resolveName();
        } catch (\Throwable $_) {
        }
    }
    return is_object($job) ? get_class($job) : '';
}

// trait_property_names collects what the traits a class uses contribute. A
// trait's properties are declared on the using class, so without this they read
// as the job's own: Laravel's Queueable alone would put thirteen of them in
// front of the two arguments the job was actually given.
function trait_property_names($ref): array
{
    $names = [];
    $pending = $ref->getTraits();
    while ($pending) {
        $trait = array_pop($pending);
        foreach ($trait->getProperties() as $prop) {
            $names[$prop->getName()] = true;
        }
        foreach ($trait->getTraits() as $nested) {
            $pending[] = $nested;
        }
    }
    return $names;
}

// preview_payload labels what a job was handed: the properties it declares
// itself, rendered by preview_value so an id stays an id and a model is named
// rather than dumped. Same rule as the view lens, for the same reason: a queued
// job routinely carries the authenticated user.
//
// Only properties declared on the job's own class are read, so a job extending
// a framework base class reports what its author passed rather than the base
// class's plumbing.
function preview_payload($subject): array
{
    if (!is_object($subject) || $subject instanceof \Closure) {
        return [];
    }
    $out = [];
    foreach (own_properties($subject) as $k => $v) {
        append_preview($out, $k, $v);
    }
    return $out;
}

// own_properties returns the values a class declares itself, skipping what a
// trait contributed and what a parent class owns: both are framework plumbing
// rather than what this job was given.
function own_properties($subject): array
{
    $out = [];
    try {
        $ref = new \ReflectionObject($subject);
        $own = get_class($subject);
        $fromTrait = trait_property_names($ref);
        foreach ($ref->getProperties() as $prop) {
            if ($prop->isStatic() || $prop->getDeclaringClass()->getName() !== $own) {
                continue;
            }
            if (isset($fromTrait[$prop->getName()])) {
                continue;
            }
            if (method_exists($prop, 'isInitialized') && !$prop->isInitialized($subject)) {
                continue;
            }
            $prop->setAccessible(true);
            $out[$prop->getName()] = $prop->getValue($subject);
        }
    } catch (\Throwable $_) {
    }
    return $out;
}

// object_detail describes what one object inside a payload holds: an Eloquent
// model's stored attributes, otherwise the properties its class declares
// itself. Without this a job carrying a model reports only its type, which does
// not say which record the job is about.
//
// Attributes come from what the model already has loaded, never through an
// accessor or a relation, so describing a payload cannot put a query on the
// database. Whatever the model hides, a password or a remember token, is left
// out on its own say-so.
function object_detail($subject): array
{
    $out = [];
    try {
        if (method_exists($subject, 'getAttributes')) {
            $attributes = $subject->getAttributes();
            if (!is_array($attributes)) {
                return [];
            }
            $hidden = method_exists($subject, 'getHidden') ? $subject->getHidden() : [];
            $hidden = is_array($hidden) ? array_flip($hidden) : [];
            foreach ($attributes as $k => $v) {
                if (count($out) >= PAYLOAD_NESTED) {
                    break;
                }
                if (isset($hidden[$k])) {
                    continue;
                }
                $out[(string) $k] = preview_value($v);
            }
            return $out;
        }
        foreach (own_properties($subject) as $k => $v) {
            if (count($out) >= PAYLOAD_NESTED) {
                break;
            }
            $out[$k] = preview_value($v);
        }
    } catch (\Throwable $_) {
    }
    return $out;
}

// append_preview writes one payload entry: the value's own label, and for an
// object the data one level down under dotted keys ("program.name"). One level
// only, since those nested values are labelled by preview_value rather than
// expanded again, so an object graph can never be walked.
function append_preview(array &$out, string $name, $value): void
{
    if (count($out) >= PAYLOAD_KEYS) {
        return;
    }
    try {
        $out[$name] = preview_value($value);
    } catch (\Throwable $_) {
        $out[$name] = '?';
        return;
    }
    // An enum is already fully described by its label; its name and value
    // properties would only repeat it.
    if (!is_object($value) || $value instanceof \Closure || $value instanceof \UnitEnum) {
        return;
    }
    foreach (object_detail($value) as $k => $v) {
        if (count($out) >= PAYLOAD_KEYS) {
            return;
        }
        $out[$name . '.' . $k] = $v;
    }
}

// queued_uuid reads the id the worker will later report the same job under.
// JobQueued carries its payload as the raw JSON string that goes on the queue,
// so it is decoded through the event's own accessor where there is one.
function queued_uuid($e): string
{
    try {
        if (method_exists($e, 'payload')) {
            $decoded = $e->payload();
        } else {
            $raw = $e->payload ?? null;
            $decoded = is_string($raw) ? json_decode($raw, true) : null;
        }
        if (is_array($decoded) && !empty($decoded['uuid'])) {
            return (string) $decoded['uuid'];
        }
    } catch (\Throwable $_) {
    }
    return '';
}

// queued_name names the job a JobQueued event carries. That event holds the job
// object itself (or a class string when the job was pushed by name), not the
// queue's Job wrapper the worker events carry.
function queued_name($e): string
{
    $job = $e->job ?? null;
    if (is_string($job)) {
        return $job;
    }
    return is_object($job) ? get_class($job) : '';
}

// job_fields is what every worker-side queue event says about the job it names:
// what ran, where it ran, and how many times it has been tried.
function job_fields($e): array
{
    $job = $e->job ?? null;
    $out = ['class' => job_name($job), 'connection' => (string) ($e->connectionName ?? '')];
    if (!is_object($job)) {
        return $out;
    }
    foreach (['queue' => 'getQueue', 'uuid' => 'uuid'] as $key => $method) {
        if (method_exists($job, $method)) {
            try {
                $v = (string) $job->$method();
            } catch (\Throwable $_) {
                continue;
            }
            if ($v !== '') {
                $out[$key] = $v;
            }
        }
    }
    if (method_exists($job, 'attempts')) {
        try {
            $out['attempts'] = (int) $job->attempts();
        } catch (\Throwable $_) {
        }
    }
    return $out;
}

// job_elapsed measures the job that just finished. A queue worker runs one job
// at a time, so a single start stamp is all the bookkeeping this needs.
function job_elapsed(): float
{
    $start = $GLOBALS['__lerd_job_start'] ?? 0.0;
    return $start > 0 ? round((microtime(true) - $start) * 1000, 3) : 0.0;
}

try {
    $app = \app();
    $events = $app['events'] ?? null;

    if ($events) {
        // Jobs, the full lifecycle: queued in the request that dispatched it,
        // then picked up and finished in the worker. Reported from workers even
        // when the rest of worker capture is off, so a queue being drained is
        // visible without a dashboard like Horizon.
        $events->listen(\Illuminate\Queue\Events\JobQueued::class, static function ($e) {
            $data = [
                'class'      => queued_name($e),
                'status'     => 'queued',
                'connection' => (string) ($e->connectionName ?? ''),
            ];
            if (!empty($e->queue)) {
                $data['queue'] = (string) $e->queue;
            }
            // The dispatching side is the only place the job is a live object.
            // Once it is on the queue it is a serialized blob, and reading that
            // back would re-hydrate every model the job holds, firing the
            // queries a debug lens must not cause. So the payload is reported
            // here, and the worker's rows carry the same uuid to tie them to it.
            $uuid = queued_uuid($e);
            if ($uuid !== '') {
                $data['uuid'] = $uuid;
            }
            $payload = preview_payload(is_object($e->job ?? null) ? $e->job : null);
            if ($payload) {
                $data['payload'] = $payload;
            }
            emit('job', $data);
        });
        $events->listen(\Illuminate\Queue\Events\JobProcessing::class, static function ($e) {
            // Reset the request id per job so each one is its own group.
            $GLOBALS['__lerd_rid'] = new_id();
            $GLOBALS['__lerd_job_start'] = microtime(true);
            emit('job', ['status' => 'processing'] + job_fields($e));
        });
        $events->listen(\Illuminate\Queue\Events\JobProcessed::class, static function ($e) {
            emit('job', ['status' => 'processed', 'time_ms' => job_elapsed()] + job_fields($e));
        });
        $events->listen(\Illuminate\Queue\Events\JobFailed::class, static function ($e) {
            emit('job', [
                'status'    => 'failed',
                'time_ms'   => job_elapsed(),
                'exception' => isset($e->exception) ? $e->exception->getMessage() : '',
            ] + job_fields($e));
        });
    }

    // Everything below is per-request detail that a long-running worker would
    // only flood the buffer with, so it stays behind the worker opt-in.
    if (!full()) {
        return;
    }

    if ($events) {
        // Mail — captured before send so a failed send is still recorded.
        $events->listen(\Illuminate\Mail\Events\MessageSending::class, static function ($e) {
            $m = $e->message ?? null;
            if (!$m) {
                return;
            }
            $html = method_exists($m, 'getHtmlBody') ? (string) $m->getHtmlBody() : '';
            if ($html === '' && method_exists($m, 'getTextBody')) {
                $html = (string) $m->getTextBody();
            }
            emit('mail', [
                'subject' => method_exists($m, 'getSubject') ? (string) $m->getSubject() : '',
                'to'      => addrs(method_exists($m, 'getTo') ? $m->getTo() : []),
                'from'    => addrs(method_exists($m, 'getFrom') ? $m->getFrom() : []),
                'cc'      => addrs(method_exists($m, 'getCc') ? $m->getCc() : []),
                'html'    => substr($html, 0, 20000),
            ]);
        });

        // Cache. Skip framework-internal keys (queue restart signals, the
        // scheduler's overlap mutexes, reverb/horizon/pulse/telescope pub-sub)
        // so the Cache tab shows the application's own cache use, not the
        // machinery that polls the cache constantly in the background.
        static $cacheNoise = [
            'illuminate:',
            'laravel:reverb:',
            'laravel:horizon:',
            'laravel:pulse:',
            'laravel:telescope:',
            'framework/schedule',
        ];
        $cacheEmit = static function ($op, $e) use ($cacheNoise) {
            $key = (string) $e->key;
            foreach ($cacheNoise as $prefix) {
                if (strncmp($key, $prefix, strlen($prefix)) === 0) {
                    return;
                }
            }
            emit('cache', ['op' => $op, 'key' => $key, 'store' => (string) ($e->storeName ?? '')]);
        };
        $events->listen(\Illuminate\Cache\Events\CacheHit::class, static fn ($e) => $cacheEmit('hit', $e));
        $events->listen(\Illuminate\Cache\Events\CacheMissed::class, static fn ($e) => $cacheEmit('miss', $e));
        $events->listen(\Illuminate\Cache\Events\KeyWritten::class, static fn ($e) => $cacheEmit('write', $e));
        $events->listen(\Illuminate\Cache\Events\KeyForgotten::class, static fn ($e) => $cacheEmit('forget', $e));

        // Dispatched events — application/package class events only. Skip
        // framework internals (Illuminate\*), and the noisy string-keyed
        // lifecycle/model events (eloquent.retrieved:, composing:, bootstrapped:,
        // …) which all carry a ':' — keeping only namespaced app/package events.
        static $eventNoise = ['Illuminate\\', 'Laravel\\Horizon\\', 'Laravel\\Reverb\\', 'Laravel\\Octane\\', 'Laravel\\Telescope\\'];
        $events->listen('*', static function ($name, $payload = []) use ($eventNoise) {
            if (!is_string($name) || strpos($name, '\\') === false || strpos($name, ':') !== false) {
                return;
            }
            foreach ($eventNoise as $prefix) {
                if (strncmp($name, $prefix, strlen($prefix)) === 0) {
                    return;
                }
            }
            emit('event', ['name' => $name]);
        });

        // Outgoing HTTP client requests.
        $events->listen(\Illuminate\Http\Client\Events\ResponseReceived::class, static function ($e) {
            emit('http', [
                'method' => method_exists($e->request, 'method') ? $e->request->method() : '',
                'url'    => method_exists($e->request, 'url') ? $e->request->url() : '',
                'status' => method_exists($e->response, 'status') ? $e->response->status() : 0,
            ]);
        });
        $events->listen(\Illuminate\Http\Client\Events\ConnectionFailed::class, static function ($e) {
            emit('http', [
                'method' => method_exists($e->request, 'method') ? $e->request->method() : '',
                'url'    => method_exists($e->request, 'url') ? $e->request->url() : '',
                'status' => 0,
                'failed' => true,
            ]);
        });
    }

    // Views — name + path + the top-level data keys passed in, skipping the
    // ones Blade compiled for itself.
    $view = $app['view'] ?? null;
    if ($view) {
        $compiled = compiled_view_dir($app);
        $view->composer('*', static function ($v) use ($compiled) {
            $path = method_exists($v, 'getPath') ? (string) $v->getPath() : '';
            if (is_synthetic_view($path, $compiled)) {
                return;
            }
            $vars = method_exists($v, 'getData') ? $v->getData() : [];
            $preview = preview_data($vars, shared_view_data($v));
            emit('view', [
                'name'         => method_exists($v, 'getName') ? (string) $v->getName() : '',
                'path'         => $path,
                'data_keys'    => array_keys($preview),
                'data_preview' => $preview,
            ]);
        });
    }

    $db = $app['db'] ?? null;
    if ($db) {
        $db->listen(static function ($query) {
            try {
                $sql = (string) ($query->sql ?? '');
                if ($sql === '') {
                    return;
                }
                $bindings = $query->bindings ?? [];
                if (isset($query->connection) && method_exists($query->connection, 'prepareBindings')) {
                    $bindings = $query->connection->prepareBindings($bindings);
                }
                $scalar = [];
                foreach ($bindings as $b) {
                    if (is_scalar($b) || $b === null) {
                        $scalar[] = $b;
                    } elseif ($b instanceof \DateTimeInterface) {
                        $scalar[] = $b->format('Y-m-d H:i:s');
                    } else {
                        $scalar[] = '(object)';
                    }
                }
                emit('query', [
                    'sql'        => $sql,
                    'bindings'   => array_values($scalar),
                    'time_ms'    => (float) ($query->time ?? 0),
                    'connection' => (string) ($query->connectionName ?? ''),
                ]);
            } catch (\Throwable $_) {
            }
        });
    }
} catch (\Throwable $_) {
    // app() not ready / unexpected container shape — stay silent.
}
