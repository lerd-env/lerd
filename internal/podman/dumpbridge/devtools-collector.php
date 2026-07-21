<?php
// /usr/local/etc/lerd/devtools-collector.php
//
// Framework-neutral collector loaded lazily by the lerd_devtools extension
// when it observes a shared library (Symfony Mailer today). It extracts the
// event data in PHP and ships it to the same socket as everything else, so a
// single engine seam covers every framework that uses that library. The
// extension only invokes this for kinds no framework adapter has claimed, so
// there's no double capture. Must never throw or emit output.

namespace Lerd\Collector;

if (defined(__NAMESPACE__ . '\\LOADED')) {
    return;
}
const LOADED = 1;

// host resolves the capture socket. Both the devtools ini (lerd.devtools_host)
// and the dump ini (lerd.dump_host) point at the same socket, and either may be
// the one present, so we accept both keys plus their env-var overrides for
// CLI/tinker. This is the single transport target for every captured kind.
// Kept 7.2-parse-safe: this file is also required by the debug bridge's
// auto_prepend, which runs on every PHP version lerd builds down to 7.2.
function host(): string
{
    foreach (['LERD_DEVTOOLS_HOST', 'LERD_DUMP_HOST'] as $envKey) {
        $v = getenv($envKey);
        if ($v !== false && $v !== '') {
            return $v;
        }
    }
    foreach (['lerd.devtools_host', 'lerd.dump_host'] as $cfgKey) {
        $v = \get_cfg_var($cfgKey);
        if (is_string($v) && $v !== '') {
            return $v;
        }
    }
    return '';
}

function send(array $payload): void
{
    $t = host();
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

function rid(): string
{
    return defined('LERD_DEVTOOLS_RID') ? (string) \LERD_DEVTOOLS_RID : new_id();
}

function ts(): string
{
    $now = microtime(true);
    $ms = (int) (($now - floor($now)) * 1000);
    return gmdate('Y-m-d\TH:i:s.', (int) $now) . sprintf('%03dZ', $ms);
}

// in_test reports whether this process is a test run. PHPUnit's own bootstrap
// defines PHPUNIT_COMPOSER_INSTALL and Pest runs on PHPUnit, so the signal is
// ecosystem-level rather than tied to any framework.
function in_test(): bool
{
    return defined('PHPUNIT_COMPOSER_INSTALL') || class_exists('PHPUnit\\Framework\\TestCase', false);
}

// detect_site names the site an event belongs to: the lerd-injected LERD_SITE
// wins, then the working-directory basename for CLI, then the parent of the
// document root for web requests that didn't get the param.
function detect_site(): string
{
    $v = lerd_var('LERD_SITE');
    if ($v !== '') {
        return $v;
    }
    if (\PHP_SAPI === 'cli') {
        $cwd = @getcwd();
        return $cwd ? basename($cwd) : '';
    }
    if (!empty($_SERVER['DOCUMENT_ROOT'])) {
        return basename(dirname($_SERVER['DOCUMENT_ROOT']));
    }
    return '';
}

function context(): array
{
    $ctx = [
        'type'   => \PHP_SAPI === 'cli' ? 'cli' : 'fpm',
        'site'   => detect_site(),
        'branch' => lerd_var('LERD_BRANCH'),
        'rid'    => rid(),
        'pid'    => getmypid() ?: 0,
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
    if (in_test()) {
        $ctx['test'] = true;
    }
    return array_filter($ctx, static function ($v) {
        return $v !== '' && $v !== null;
    });
}

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
        // Skip our own plumbing and the dumper internals so the resolved
        // src/trace points at the caller's code, not the capture machinery.
        if (strpos($file, 'devtools-collector.php') !== false
            || strpos($file, 'dump-bridge.php') !== false
            || strpos($file, 'symfony/var-dumper') !== false) {
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

function emit(string $kind, array $data): void
{
    try {
        $bt = backtrace();
        $data['trace'] = $bt['trace'];
        send([
            'v'    => 1,
            'id'   => new_id(),
            'ts'   => ts(),
            'kind' => $kind,
            'ctx'  => context(),
            'src'  => $bt['src'],
            'data' => $data,
        ]);
    } catch (\Throwable $_) {
    }
}

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

// mail extracts a Symfony\Component\Mime\Email passed to Mailer::send. A raw
// RawMessage without these accessors is skipped.
function mail($message): void
{
    if (!is_object($message) || !method_exists($message, 'getSubject')) {
        return;
    }
    $html = method_exists($message, 'getHtmlBody') ? (string) $message->getHtmlBody() : '';
    if ($html === '' && method_exists($message, 'getTextBody')) {
        $html = (string) $message->getTextBody();
    }
    emit('mail', [
        'subject' => (string) $message->getSubject(),
        'to'      => addrs(method_exists($message, 'getTo') ? $message->getTo() : []),
        'from'    => addrs(method_exists($message, 'getFrom') ? $message->getFrom() : []),
        'cc'      => addrs(method_exists($message, 'getCc') ? $message->getCc() : []),
        'html'    => substr($html, 0, 20000),
    ]);
}

// view extracts one Twig render. $env is the Twig\Environment, $name the
// template (string or TemplateWrapper), $context the variables passed in. The
// loader resolves the on-disk .twig source path so the UI can link to it, the
// same as Blade getPath() does for Laravel.
function view($env, $name, $context): void
{
    $tpl = is_object($name) && method_exists($name, 'getTemplateName')
        ? (string) $name->getTemplateName()
        : (string) $name;
    if ($tpl === '' || strncmp($tpl, '@WebProfiler', 12) === 0) {
        return;
    }
    $path = '';
    try {
        if (is_object($env) && method_exists($env, 'getLoader')) {
            $source = $env->getLoader()->getSourceContext($tpl);
            if (is_object($source) && method_exists($source, 'getPath')) {
                $path = (string) $source->getPath();
            }
        }
    } catch (\Throwable $_) {
    }
    $keys = is_array($context) ? array_map('strval', array_keys($context)) : [];
    emit('view', ['name' => $tpl, 'path' => $path, 'data_keys' => $keys]);
}

// event captures one Symfony event dispatch. $event is the event object, $name
// the explicit event name (Symfony falls back to the class when null). We keep
// application events and drop the framework lifecycle noise (kernel.*, console.*
// and the component/library internal events), mirroring the Laravel filter.
function event($event, $name): void
{
    $cls = is_object($event) ? get_class($event) : '';
    $evt = (is_string($name) && $name !== '') ? $name : $cls;
    if ($evt === '') {
        return;
    }
    static $noise = ['kernel.', 'console.', 'Symfony\\', 'Twig\\', 'Doctrine\\'];
    foreach ($noise as $prefix) {
        if (strncmp($evt, $prefix, strlen($prefix)) === 0) {
            return;
        }
    }
    emit('event', ['name' => $evt]);
}

// job captures one message dispatched to the Symfony Messenger bus. A message
// can be dispatched raw or already wrapped in an Envelope, so we unwrap to the
// real message class. Status is "dispatched" since the bus only tells us a
// message was sent (handled/failed live in the worker, captured separately).
function job($message): void
{
    if (!is_object($message)) {
        return;
    }
    if ($message instanceof \Symfony\Component\Messenger\Envelope) {
        $inner = $message->getMessage();
        $cls = is_object($inner) ? get_class($inner) : get_class($message);
    } else {
        $cls = get_class($message);
    }
    emit('job', ['class' => $cls, 'status' => 'dispatched']);
}

// http captures one outgoing Symfony HttpClient request at call time. The
// response is lazy (not sent until read), so no status code is available here;
// the UI shows the request as "sent". Method and url are read at the begin
// observer because request() rewrites its $url argument internally.
function http($method, $url): void
{
    $u = is_string($url) ? $url : '';
    if ($u === '') {
        return;
    }
    emit('http', ['method' => is_string($method) ? $method : '', 'url' => $u]);
}
