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

if (!defined('LERD_DEVTOOLS_ON') || !\LERD_DEVTOOLS_ON) {
    return;
}
if (defined(__NAMESPACE__ . '\\REGISTERED')) {
    return;
}
const REGISTERED = 1;

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

try {
    $app = \app();
    $events = $app['events'] ?? null;

    if ($events) {
        // Reset the request id per job so each queued job is its own group.
        $events->listen(\Illuminate\Queue\Events\JobProcessing::class, static function () {
            $GLOBALS['__lerd_rid'] = new_id();
        });

        // Jobs — terminal states.
        $events->listen(\Illuminate\Queue\Events\JobProcessed::class, static function ($e) {
            emit('job', [
                'class'      => job_name($e->job ?? null),
                'status'     => 'processed',
                'connection' => (string) ($e->connectionName ?? ''),
            ]);
        });
        $events->listen(\Illuminate\Queue\Events\JobFailed::class, static function ($e) {
            emit('job', [
                'class'      => job_name($e->job ?? null),
                'status'     => 'failed',
                'connection' => (string) ($e->connectionName ?? ''),
                'exception'  => isset($e->exception) ? $e->exception->getMessage() : '',
            ]);
        });

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
