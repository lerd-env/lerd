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

// PAYLOAD_KEYS caps a whole payload; PAYLOAD_NESTED caps how much of one
// object inside it is described.
const PAYLOAD_KEYS = 60;
const PAYLOAD_NESTED = 20;

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

// rid groups an event with the others from the same run. The extension stamps
// one id per request (or per CLI process); a long-running worker overrides it
// per message so each job is its own group instead of a whole shift's work
// landing in one.
function rid(): string
{
    if (!empty($GLOBALS['__lerd_rid'])) {
        return (string) $GLOBALS['__lerd_rid'];
    }
    return defined('LERD_DEVTOOLS_RID') ? (string) \LERD_DEVTOOLS_RID : new_id();
}

// full reports whether this process captures every kind. A queue or scheduler
// worker reports only its jobs unless the user opted into full worker capture:
// everything else it does is background polling that would bury the request
// being debugged. Without LERD_DEVTOOLS_JOBS there is no jobs-only mode to be
// in, either because the extension predates it or because it isn't loaded at
// all, as when the debug bridge requires this file for its transport.
function full(): bool
{
    if (!defined('LERD_DEVTOOLS_JOBS')) {
        return true;
    }
    return defined('LERD_DEVTOOLS_ON') && \LERD_DEVTOOLS_ON;
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

// condense_arg keeps short arguments intact and elides long values, so
// "--queue=high" survives but tinker's "--execute=<code>" reads "--execute=...":
// the command names the run, the event's src/data carry the exact detail.
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

// command_line names the CLI invocation an event came from, e.g.
// "artisan queue:work --queue=high". Condensed per argument and capped so a
// long invocation can't bloat the context of every event the process emits.
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
    return array_filter($ctx, static function ($v) {
        return $v !== '' && $v !== null;
    });
}

// installed_dirs returns every directory Composer put a package in, read once
// per process from the project's installed map. The root package is excluded:
// it is the project itself, and everything sits under it.
function installed_dirs(): array
{
    static $dirs = null;
    if ($dirs !== null) {
        return $dirs;
    }
    $dirs = [];
    $dir = $_SERVER['DOCUMENT_ROOT'] ?? '';
    if ($dir === '') {
        $dir = getcwd() ?: '';
    }
    for ($up = 0; $up < 8 && $dir !== '' && $dir !== DIRECTORY_SEPARATOR; $up++) {
        $map = $dir . '/vendor/composer/installed.php';
        if (is_file($map)) {
            $installed = @include $map;
            $root = isset($installed['root']['install_path']) ? realpath($installed['root']['install_path']) : false;
            foreach ($installed['versions'] ?? [] as $pkg) {
                $path = isset($pkg['install_path']) ? realpath($pkg['install_path']) : false;
                if ($path !== false && $path !== $root) {
                    $dirs[] = $path . DIRECTORY_SEPARATOR;
                }
            }
            break;
        }
        $dir = dirname($dir);
    }
    return $dirs;
}

// is_dependency reports whether a file belongs to something the project
// installed rather than to code its developer wrote.
//
// The test used to be the literal path /vendor/, which misses a framework whose
// own core is a package placed elsewhere: Drupal core installs at web/core, so
// every query resolved to the database layer inside it and nothing lerd
// reported about a query said which code had run it.
function is_dependency(string $file): bool
{
    if (strpos($file, '/vendor/') !== false) {
        return true;
    }
    foreach (installed_dirs() as $dir) {
        if (strpos($file, $dir) === 0) {
            return true;
        }
    }
    return false;
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
        if ($src === null && !is_dependency($file)) {
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
// one view with a large context cannot dominate the buffer. Whatever the
// environment injects into every template is subtracted, along with the
// underscore-prefixed names the engine reserves, so what is left is what the
// developer passed to this one.
function preview_data($data, array $globals = []): array
{
    $out = [];
    if (!is_array($data)) {
        return $out;
    }
    foreach ($data as $k => $v) {
        if (count($out) >= 40) {
            break;
        }
        $key = (string) $k;
        if (strncmp($key, '_', 1) === 0 || array_key_exists($key, $globals)) {
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

// twig_globals is what the environment adds to every template, so it can be
// told apart from this template's own context.
function twig_globals($env): array
{
    try {
        if (is_object($env) && method_exists($env, 'getGlobals')) {
            $g = $env->getGlobals();
            return is_array($g) ? $g : [];
        }
    } catch (\Throwable $_) {
    }
    return [];
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
    $preview = preview_data($context, twig_globals($env));
    emit('view', ['name' => $tpl, 'path' => $path, 'data_keys' => array_keys($preview), 'data_preview' => $preview]);
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
    if (worker_job($event, $cls)) {
        return;
    }
    if (!full()) {
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

// preview_payload labels what a job was handed: an array's entries, or the
// properties an object declares itself. Values go through preview_value, so an
// id stays an id and a model is named rather than dumped, which is the same
// rule the view lens follows and for the same reason: a queued job routinely
// carries the authenticated user.
//
// Only properties declared on the object's own class are read. A job extending
// a framework base class would otherwise report that base class's plumbing
// alongside the two arguments its author actually passed.
function preview_payload($subject): array
{
    $out = [];
    if (is_array($subject)) {
        foreach ($subject as $k => $v) {
            append_preview($out, (string) $k, $v);
        }
        return $out;
    }
    if (!is_object($subject) || $subject instanceof \Closure) {
        return [];
    }
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

// object_detail describes what one object inside a payload holds: a model's
// stored attributes where it has them, otherwise the properties its class
// declares itself. Without this a job carrying a model reports only its type,
// which does not say which record the job is about.
//
// Attributes are read from what the model already has loaded, never through an
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

// envelope_class names the message inside a Messenger envelope, which is what
// the developer wrote; the envelope itself is transport bookkeeping.
function envelope_class($envelope): string
{
    $inner = envelope_message($envelope);
    return is_object($inner) ? get_class($inner) : '';
}

// envelope_message unwraps a Messenger envelope to the message inside it, and
// hands back anything that is not one unchanged.
function envelope_message($envelope)
{
    if (is_object($envelope) && method_exists($envelope, 'getMessage')) {
        $inner = $envelope->getMessage();
        if (is_object($inner)) {
            return $inner;
        }
    }
    return $envelope;
}

// job_elapsed measures the job that just finished. A worker runs one message at
// a time, so a single start stamp is all the bookkeeping this needs.
function job_elapsed(): float
{
    $start = $GLOBALS['__lerd_job_start'] ?? 0.0;
    return $start > 0 ? round((microtime(true) - $start) * 1000, 3) : 0.0;
}

// worker_job turns Symfony Messenger's worker lifecycle into job events. The
// bus seam only says a message was dispatched; these three say what the worker
// then did with it, which is the half that is invisible without a dashboard.
// Reports whether the event was one of them, so the caller stops there.
function worker_job($event, string $cls): bool
{
    static $states = [
        'Symfony\\Component\\Messenger\\Event\\WorkerMessageReceivedEvent' => 'processing',
        'Symfony\\Component\\Messenger\\Event\\WorkerMessageHandledEvent'  => 'processed',
        'Symfony\\Component\\Messenger\\Event\\WorkerMessageFailedEvent'   => 'failed',
    ];
    if (!isset($states[$cls]) || !is_object($event) || !method_exists($event, 'getEnvelope')) {
        return false;
    }
    $status = $states[$cls];
    if ($status === 'processing') {
        // Each message is its own group, the way the Laravel adapter does it.
        $GLOBALS['__lerd_rid'] = new_id();
        $GLOBALS['__lerd_job_start'] = microtime(true);
    }
    $envelope = $event->getEnvelope();
    $data = ['class' => envelope_class($envelope), 'status' => $status];
    $payload = preview_payload(envelope_message($envelope));
    if ($payload) {
        $data['payload'] = $payload;
    }
    if (method_exists($event, 'getReceiverName')) {
        $queue = (string) $event->getReceiverName();
        if ($queue !== '') {
            $data['queue'] = $queue;
        }
    }
    if ($status !== 'processing') {
        $data['time_ms'] = job_elapsed();
    }
    if ($status === 'failed' && method_exists($event, 'getThrowable')) {
        $t = $event->getThrowable();
        $data['exception'] = $t instanceof \Throwable ? $t->getMessage() : '';
    }
    emit('job', $data);
    return true;
}

// job captures one message dispatched to the Symfony Messenger bus. A message
// can be dispatched raw or already wrapped in an Envelope, so we unwrap to the
// real message class. Status is "dispatched" since the bus only tells us a
// message was sent; what the worker then does with it comes from worker_job.
function job($message): void
{
    if (!is_object($message)) {
        return;
    }
    if ($message instanceof \Symfony\Component\Messenger\Envelope) {
        // A worker hands the envelope it received back to the bus to run the
        // handler, so a ReceivedStamp marks that redelivery rather than a new
        // dispatch: the worker lifecycle events already report it.
        if (method_exists($message, 'last')
            && $message->last('Symfony\\Component\\Messenger\\Stamp\\ReceivedStamp') !== null) {
            return;
        }
        $subject = envelope_message($message);
    } else {
        $subject = $message;
    }
    $data = ['class' => get_class($subject), 'status' => 'dispatched'];
    $payload = preview_payload($subject);
    if ($payload) {
        $data['payload'] = $payload;
    }
    emit('job', $data);
}

// seams returns the capture seams the framework store declared, parsed once per
// process from the file lerd writes next to this collector. One line each:
// kind|match|target|method|name. Keyed by method, since that is what the
// extension matched on; the target settles which one applies.
function seams(): array
{
    static $parsed = null;
    if ($parsed !== null) {
        return $parsed;
    }
    $parsed = [];
    // The mounted path is where lerd writes it; the override exists so the
    // parsing and extraction can be exercised without that mount.
    $path = getenv('LERD_DEVTOOLS_SEAMS');
    if (!is_string($path) || $path === '') {
        $path = '/usr/local/etc/lerd/devtools-seams.conf';
    }
    $lines = @file($path, \FILE_IGNORE_NEW_LINES | \FILE_SKIP_EMPTY_LINES);
    if (!is_array($lines)) {
        return $parsed;
    }
    foreach ($lines as $line) {
        if ($line === '' || $line[0] === '#') {
            continue;
        }
        $f = explode('|', $line);
        if (count($f) < 5 || $f[0] !== 'job') {
            continue;
        }
        $parsed[strtolower($f[3])][] = ['target' => $f[2], 'name' => $f[4]];
    }
    return $parsed;
}

// seam_for picks the seam that covers this call. is_a() with a string subject
// resolves a class, an interface and a parent alike, which is exactly the three
// ways a seam can be declared.
function seam_for(string $class, string $method, $self): array
{
    $candidates = seams();
    $key = strtolower($method);
    if (!isset($candidates[$key])) {
        return [];
    }
    foreach ($candidates[$key] as $seam) {
        if (strcasecmp($class, $seam['target']) === 0) {
            return $seam;
        }
        if (is_object($self) && is_a($self, $seam['target'])) {
            return $seam;
        }
    }
    return [];
}

// scalar_string renders a resolved seam value as the label the lens shows.
function scalar_string($v): string
{
    if (is_string($v)) {
        return $v;
    }
    if (is_int($v) || is_float($v)) {
        return (string) $v;
    }
    if (is_bool($v)) {
        return $v ? 'true' : 'false';
    }
    return is_object($v) ? get_class($v) : '';
}

// seam_value resolves a store-declared expression against the observed call:
// "this" or "arg:N", either optionally followed by ".method:getHook" or
// ".prop:queue". An object with no accessor yields its class, which is the name
// a queued job goes by.
function seam_value(string $expr, $self, array $args): string
{
    if ($expr === '') {
        return '';
    }
    $accessor = '';
    $dot = strpos($expr, '.');
    if ($dot !== false) {
        $accessor = substr($expr, $dot + 1);
        $expr = substr($expr, 0, $dot);
    }
    if ($expr === 'this') {
        $subject = $self;
    } elseif (strncmp($expr, 'arg:', 4) === 0) {
        $n = (int) substr($expr, 4);
        $subject = isset($args[$n]) ? $args[$n] : null;
    } else {
        return '';
    }
    if ($accessor === '') {
        return is_object($subject) ? get_class($subject) : scalar_string($subject);
    }
    if (!is_object($subject)) {
        return '';
    }
    try {
        if (strncmp($accessor, 'method:', 7) === 0) {
            $m = substr($accessor, 7);
            return method_exists($subject, $m) ? scalar_string($subject->$m()) : '';
        }
        if (strncmp($accessor, 'prop:', 5) === 0) {
            $prop = substr($accessor, 5);
            return isset($subject->$prop) ? scalar_string($subject->$prop) : '';
        }
    } catch (\Throwable $_) {
    }
    return '';
}

// seam_begin reports a store-declared job starting, and remembers it so the end
// observer can close it. A frame is pushed either way, so a call the extension
// observed but no seam claims cannot pop somebody else's.
function seam_begin($class, $method, $self, $args): void
{
    $seam = seam_for((string) $class, (string) $method, $self);
    $stack = isset($GLOBALS['__lerd_seam_stack']) ? $GLOBALS['__lerd_seam_stack'] : [];
    if (!$seam) {
        $stack[] = ['skip' => true];
        $GLOBALS['__lerd_seam_stack'] = $stack;
        return;
    }
    $args = is_array($args) ? $args : [];
    $name = seam_value($seam['name'], $self, $args);
    if ($name === '') {
        $name = is_object($self) ? get_class($self) : (string) $class;
    }
    // What the job was handed: the first argument where the method takes one,
    // which is how a queue passes an item, and otherwise the job object's own
    // properties, which is where a job with a no-argument entry point keeps it.
    $payload = preview_payload(isset($args[1]) ? $args[1] : $self);
    // Each job is its own group, the way the Laravel adapter and the Messenger
    // worker events do it. The previous id is restored when this one ends, so a
    // job that runs inside another leaves the outer grouping intact.
    $previous = isset($GLOBALS['__lerd_rid']) ? $GLOBALS['__lerd_rid'] : '';
    $GLOBALS['__lerd_rid'] = new_id();
    $stack[] = ['skip' => false, 'class' => $name, 'start' => microtime(true), 'previous' => $previous, 'payload' => $payload];
    $GLOBALS['__lerd_seam_stack'] = $stack;
    $data = ['class' => $name, 'status' => 'processing'];
    if ($payload) {
        $data['payload'] = $payload;
    }
    emit('job', $data);
}

// seam_end closes the job seam_begin opened, as failed when the call is on its
// way out with an exception in flight, carrying that throwable's message.
function seam_end($class, $method, $failed, $error = ''): void
{
    $stack = isset($GLOBALS['__lerd_seam_stack']) ? $GLOBALS['__lerd_seam_stack'] : [];
    $frame = array_pop($stack);
    $GLOBALS['__lerd_seam_stack'] = $stack;
    if (!is_array($frame) || !empty($frame['skip'])) {
        return;
    }
    $data = [
        'class'   => $frame['class'],
        'status'  => $failed ? 'failed' : 'processed',
        'time_ms' => round((microtime(true) - $frame['start']) * 1000, 3),
    ];
    if (!empty($frame['payload'])) {
        $data['payload'] = $frame['payload'];
    }
    if ($failed && is_string($error) && $error !== '') {
        $data['exception'] = $error;
    }
    emit('job', $data);
    if ($frame['previous'] !== '') {
        $GLOBALS['__lerd_rid'] = $frame['previous'];
    }
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
