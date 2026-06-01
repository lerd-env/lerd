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
    }
    $worker = defined('LERD_DEVTOOLS_WORKER') ? (string) \LERD_DEVTOOLS_WORKER : '';
    if ($worker !== '') {
        $ctx['worker'] = $worker;
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

    // Views — name + path + the top-level data keys passed in.
    $view = $app['view'] ?? null;
    if ($view) {
        $view->composer('*', static function ($v) {
            emit('view', [
                'name'      => method_exists($v, 'getName') ? (string) $v->getName() : '',
                'path'      => method_exists($v, 'getPath') ? (string) $v->getPath() : '',
                'data_keys' => method_exists($v, 'getData') ? array_keys($v->getData()) : [],
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
