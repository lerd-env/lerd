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

// DEBUG_SCHEMA is the version of lerd/debug's entry format this collector
// reads; the package renders its entries for it.
const DEBUG_SCHEMA = 1;
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
            ? $_SERVER['REQUEST_METHOD'] . ' ' . mask_url((string) ($_SERVER['REQUEST_URI'] ?? ''), 'input')
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
        emit_with($kind, $data, $bt['src'], $bt['trace']);
    } catch (\Throwable $_) {
    }
}

// emit_with is emit for an event whose origin is not the call that captured it:
// a throwable carries the frames it was thrown from, and reporting the line
// that handed it to Sentry instead would name the reporting, not the fault.
function emit_with(string $kind, array $data, array $src, array $trace): void
{
    // An excluded command's own loop is left out; a job it runs has switched
    // to an id of its own and comes through.
    if (!empty($GLOBALS['__lerd_excluded']) && defined('LERD_DEVTOOLS_RID') && rid() === (string) \LERD_DEVTOOLS_RID) {
        return;
    }
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
    }
}

// render_var renders one variable to the text a dump shows, through Symfony's
// cloner when the project has it and through print_r when it does not. Every
// capture that ships a whole value goes through here, so dump(), dd() and a
// ray() all read the same way.
function render_var($var): string
{
    if (!class_exists(\Symfony\Component\VarDumper\Cloner\VarCloner::class, true)
        || !class_exists(\Symfony\Component\VarDumper\Dumper\CliDumper::class, true)) {
        return is_scalar($var) ? (string) $var : print_r($var, true);
    }
    $cloner = new \Symfony\Component\VarDumper\Cloner\VarCloner();
    $maxItems = (int) (getenv('LERD_DUMP_MAX_ITEMS') ?: 2500);
    $cloner->setMaxItems($maxItems > 0 ? $maxItems : 2500);
    $cloner->setMaxString(4096);
    $dumper = new \Symfony\Component\VarDumper\Dumper\CliDumper();
    $dumper->setColors(false);
    $rendered = $dumper->dump($cloner->cloneVar($var), true);
    return is_string($rendered) ? $rendered : '';
}

// send_dump ships one variable as a dump event. The dump envelope (top-level
// label and text) differs from the structured kinds, so it is built here rather
// than through emit(); everything else is the same transport.
function send_dump($var, $label = null): void
{
    send_text(render_var($var), $label);
}

// send_text is send_dump for a value that is already the text to show.
function send_text(string $text, $label = null): void
{
    try {
        $bt = backtrace();
        send([
            'v'     => 1,
            'id'    => new_id(),
            'ts'    => ts(),
            'kind'  => 'dump',
            'ctx'   => context(),
            'src'   => $bt['src'],
            'label' => $label,
            'text'  => $text,
        ]);
    } catch (\Throwable $_) {
        // never throw out of a debug bridge
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
            // Reflection reads private properties on its own since PHP 8.1, and 8.5
            // deprecates asking it to.
            if (\PHP_VERSION_ID < 80100) {
                $prop->setAccessible(true);
            }
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

// asset_path locates a file lerd writes next to this collector: the container
// mounts the assets at a fixed path, while a PHP on the host finds them through
// the ini the debug bridge reads.
function asset_path(string $name): string
{
    $assets = \get_cfg_var('lerd.assets_dir');
    if (!is_string($assets) || $assets === '') {
        $assets = '/usr/local/etc/lerd';
    }
    return $assets . '/' . $name;
}

// excluded_commands are the console commands, by name or class, whose own work
// this site leaves out: the store's for every site and the site's .lerd.yaml.
function excluded_commands(): array
{
    static $list = null;
    if ($list !== null) {
        return $list;
    }
    $list = [];
    $path = getenv('LERD_DEVTOOLS_EXCLUDE');
    $lines = @file(is_string($path) && $path !== '' ? $path : asset_path('devtools-exclude.conf'), \FILE_IGNORE_NEW_LINES | \FILE_SKIP_EMPTY_LINES);
    $site = detect_site();
    foreach (is_array($lines) ? $lines : [] as $line) {
        $f = explode('|', $line, 2);
        if (count($f) === 2 && $line[0] !== '#' && ($f[0] === '*' || $f[0] === $site)) {
            $list[strtolower(ltrim($f[1], '\\'))] = true;
        }
    }
    return $list;
}

// exclude_command marks this process as one whose own work is left out when
// the command it runs, by name or by class, is on the list. The jobs it runs
// carry ids of their own and are still reported.
function exclude_command(string $name, string $class = ''): void
{
    $list = excluded_commands();
    if (($name !== '' && isset($list[strtolower($name)])) || ($class !== '' && isset($list[strtolower(ltrim($class, '\\'))]))) {
        $GLOBALS['__lerd_excluded'] = true;
    }
}

// seams returns the capture seams the store declared, parsed once per process
// from the file lerd writes next to this collector. One line each:
// kind|match|target|method|name[|label]. Keyed by method, since that is what the
// extension matched on; the target settles which one applies and the kind says
// what the call means.
function seams(): array
{
    static $parsed = null;
    if ($parsed !== null) {
        return $parsed;
    }
    $parsed = [];
    // The container mounts the assets at a fixed path; a PHP running on the
    // host has no such directory, so the location comes from the ini the way
    // the debug bridge reads it, and only then falls back to the mount. The
    // env override exists so the parsing and extraction can be exercised
    // without either.
    $path = getenv('LERD_DEVTOOLS_SEAMS');
    if (!is_string($path) || $path === '') {
        $path = asset_path('devtools-seams.conf');
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
        if (count($f) < 5 || $f[0] === '') {
            continue;
        }
        $parsed[strtolower($f[3])][] = ['kind' => $f[0], 'target' => $f[2], 'name' => $f[4], 'label' => isset($f[5]) ? $f[5] : ''];
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
    // A closure is named by where it was written, which is what a reader
    // looks for, relative to the project when it lives inside it.
    if (is_array($v) && count($v) === 2 && is_string($v[1] ?? null) && (is_object($v[0] ?? null) || is_string($v[0] ?? null))) {
        return (is_object($v[0]) ? get_class($v[0]) : $v[0]) . '@' . $v[1];
    }
    if ($v instanceof \Closure) {
        try {
            $fn = new \ReflectionFunction($v);
            // A first-class callable of a method names that method.
            $bound = $fn->getClosureThis();
            if ($bound !== null && strpos($fn->getName(), '{closure') === false) {
                return get_class($bound) . '@' . $fn->getName();
            }
            $root = !empty($_SERVER['DOCUMENT_ROOT']) ? dirname((string) $_SERVER['DOCUMENT_ROOT']) . '/' : '';
            $file = (string) $fn->getFileName();
            if ($root !== '' && $root !== '/' && strncmp($file, $root, strlen($root)) === 0) {
                $file = substr($file, strlen($root));
            }
            return 'Closure ' . $file . ':' . $fn->getStartLine();
        } catch (\Throwable $_) {
            return 'Closure';
        }
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
    // Alternatives separated by commas, the first that yields something wins.
    if (strpos($expr, ',') !== false) {
        foreach (explode(',', $expr) as $alt) {
            $v = seam_value(trim($alt), $self, $args);
            if ($v !== '') {
                return $v;
            }
        }
        return '';
    }
    if (strpos($expr, '.') === false) {
        $subject = seam_raw($expr, $self, $args);
        return is_object($subject) && !$subject instanceof \Closure ? get_class($subject) : scalar_string($subject);
    }
    $value = seam_raw($expr, $self, $args);
    return $value === null || is_array($value) ? '' : scalar_string($value);
}

// seam_raw resolves an expression to the value itself: "this" or "arg:N",
// then steps separated by dots, each a method:name to call, a prop:name to
// read, or a bare name that reads on into an array or object, so
// prop:action.uses and method:getRequest.prop:attributes.method:all._route
// both walk. Null when a step finds nothing.
function seam_raw(string $expr, $self, array $args)
{
    $steps = explode('.', $expr);
    $head = array_shift($steps);
    if ($head === 'this') {
        $value = $self;
    } elseif (strncmp($head, 'arg:', 4) === 0) {
        $n = (int) substr($head, 4);
        $value = isset($args[$n]) ? $args[$n] : null;
    } else {
        return null;
    }
    try {
        foreach ($steps as $step) {
            if (strncmp($step, 'method:', 7) === 0) {
                $m = substr($step, 7);
                if (!is_object($value) || !method_exists($value, $m)) {
                    return null;
                }
                $value = $value->$m();
                continue;
            }
            $key = strncmp($step, 'prop:', 5) === 0 ? substr($step, 5) : $step;
            if (is_array($value) && array_key_exists($key, $value)) {
                $value = $value[$key];
            } elseif (is_object($value) && isset($value->$key)) {
                $value = $value->$key;
            } else {
                return null;
            }
        }
    } catch (\Throwable $_) {
        return null;
    }
    return $value;
}

// seam_begin reports a store-declared job starting, and remembers it so the end
// observer can close it. A frame is pushed either way, so a call the extension
// observed but no seam claims cannot pop somebody else's.
function seam_begin($class, $method, $self, $args): void
{
    $seam = seam_for((string) $class, (string) $method, $self);
    $stack = isset($GLOBALS['__lerd_seam_stack']) ? $GLOBALS['__lerd_seam_stack'] : [];
    // A frame is pushed for a capture seam too: the extension observes the way
    // out of every seam it claimed, and an unbalanced stack would let that end
    // close a job that is still running.
    if ($seam && $seam['kind'] === 'component') {
        $args = is_array($args) ? $args : [];
        $name = seam_value($seam['name'], $self, $args);
        $stack[] = [
            'timed'   => 'component',
            'subject' => seam_subject($seam['name'], $self, $args),
            'data'    => [
                'name'  => $name !== '' ? $name : (is_object($self) ? get_class($self) : (string) $class),
                'phase' => (string) $method,
            ],
            'start'   => microtime(true),
            'details' => component_details($args),
        ];
        $GLOBALS['__lerd_seam_stack'] = $stack;
        return;
    }
    // A Redis command or a filesystem operation, timed. A call into the same
    // kind from inside one (a driver handing to its parent) is the same call.
    // A response read as it ends, so a lazy one is complete and the client's
    // own status check has already run, never moved later by holding it.
    // A route's parameters are read once the call returns, when the framework
    // has put them where the expression looks; the first route wins.
    if ($seam && $seam['kind'] === 'route_params') {
        $stack[] = ['timed' => 'route_params', 'subject' => $self, 'args' => is_array($args) ? $args : [], 'expr' => $seam['name']];
        $GLOBALS['__lerd_seam_stack'] = $stack;
        return;
    }
    if ($seam && $seam['kind'] === 'http_response') {
        $stack[] = ['timed' => 'http_response', 'subject' => $self];
        $GLOBALS['__lerd_seam_stack'] = $stack;
        return;
    }
    if ($seam && ($seam['kind'] === 'redis' || $seam['kind'] === 'filesystem')) {
        $top = end($stack);
        if (is_array($top) && ($top['timed'] ?? '') === $seam['kind']) {
            $stack[] = ['skip' => true];
            $GLOBALS['__lerd_seam_stack'] = $stack;
            return;
        }
        $args = is_array($args) ? $args : [];
        $name = seam_value($seam['name'], $self, $args);
        if ($seam['kind'] === 'redis') {
            $data = ['command' => strtoupper(scalar_string($args[1] ?? ''))];
            if (isset($args[2]) && is_array($args[2]) && $args[2]) {
                $json = json_encode(array_values($args[2]), \JSON_PARTIAL_OUTPUT_ON_ERROR | \JSON_UNESCAPED_SLASHES | \JSON_UNESCAPED_UNICODE);
                $data['args'] = is_string($json) ? (strlen($json) > 300 ? substr($json, 0, 297) . '...' : $json) : '';
            }
            if ($name !== '') {
                $data['connection'] = $name;
            }
        } else {
            $data = ['op' => (string) $method, 'path' => scalar_string($args[1] ?? '')];
            // A framework adapter that knows its storage disks names the one
            // this instance is; Flysystem itself has no name for it.
            if ($name === '' && \function_exists('Lerd\\LaravelAdapter\\disk_for')) {
                $name = \Lerd\LaravelAdapter\disk_for($self);
            }
            if ($name !== '') {
                $data['disk'] = $name;
            }
        }
        $stack[] = ['timed' => $seam['kind'], 'data' => $data, 'start' => microtime(true)];
        $GLOBALS['__lerd_seam_stack'] = $stack;
        return;
    }
    // A span is one phase of the app's own work, Bootstrap or Controller, shown
    // by the store's label with the name expression saying which one it was.
    if ($seam && $seam['kind'] === 'span') {
        $data = ['label' => $seam['label'] !== '' ? $seam['label'] : (string) $method];
        $name = seam_value($seam['name'], $self, is_array($args) ? $args : []);
        // A template named by its file reads better from the project root.
        $root = !empty($_SERVER['DOCUMENT_ROOT']) ? dirname((string) $_SERVER['DOCUMENT_ROOT']) . '/' : '';
        if ($root !== '/' && $root !== '' && strncmp($name, $root, strlen($root)) === 0) {
            $name = substr($name, strlen($root));
        }
        if ($name !== '') {
            $data['name'] = $name;
            $data += code_location($name);
        }
        $stack[] = ['timed' => 'span', 'data' => $data, 'start' => microtime(true)];
        $GLOBALS['__lerd_seam_stack'] = $stack;
        return;
    }
    if (!$seam || $seam['kind'] !== 'job') {
        $stack[] = ['skip' => true];
        $GLOBALS['__lerd_seam_stack'] = $stack;
        if ($seam) {
            capture($seam['kind'], (string) $method, $self, is_array($args) ? $args : [], $seam['name']);
        }
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
    if (($frame['timed'] ?? '') === 'route_params') {
        if (empty($GLOBALS['__lerd_route_params'])) {
            $params = seam_raw((string) $frame['expr'], $frame['subject'], $frame['args']);
            if (is_array($params)) {
                route_params(array_filter($params, 'is_scalar'));
            }
        }
        return;
    }
    if (($frame['timed'] ?? '') === 'http_response') {
        http_done($frame['subject'] ?? null, (bool) $failed);
        return;
    }
    if (isset($frame['timed'])) {
        $data = $frame['data'] + [
            'status'  => $failed ? 'failed' : 'ok',
            'time_ms' => round((microtime(true) - $frame['start']) * 1000, 3),
        ];
        if (!empty($frame['details'])) {
            $data['details'] = $frame['details'];
        }
        if ($failed && is_string($error) && $error !== '') {
            $data['exception'] = $error;
        }
        if ($frame['timed'] === 'component') {
            $data = component_identity($data, $frame['subject'] ?? null);
        }
        if ($frame['timed'] === 'span') {
            $GLOBALS['__lerd_spans'][$data['label']] = ($GLOBALS['__lerd_spans'][$data['label']] ?? 0) + $data['time_ms'];
        }
        emit($frame['timed'], $data);
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

// component_details previews what a component seam was handed, leaving out the
// objects (the component itself, the framework's context), so a property
// update shows its path and value and a method call shows which methods ran.
// code_location finds where a resolved name is written, so the lens can open
// it: Class@method by the method, a class by its declaration, and a closure by
// the file and line its name already carries.
function code_location(string $name): array
{
    try {
        if (preg_match('/^Closure (.+):(\d+)$/', $name, $m)) {
            $file = $m[1];
            if ($file !== '' && $file[0] !== '/' && !empty($_SERVER['DOCUMENT_ROOT'])) {
                $file = dirname((string) $_SERVER['DOCUMENT_ROOT']) . '/' . $file;
            }
            return ['file' => $file, 'line' => (int) $m[2]];
        }
        if (strpos($name, '@') !== false) {
            [$class, $method] = explode('@', $name, 2);
            if (class_exists($class, false) && method_exists($class, $method)) {
                $ref = new \ReflectionMethod($class, $method);
                return ['file' => (string) $ref->getFileName(), 'line' => (int) $ref->getStartLine()];
            }
            return [];
        }
        if (class_exists($name, false)) {
            $ref = new \ReflectionClass($name);
            return $ref->getFileName() ? ['file' => (string) $ref->getFileName(), 'line' => (int) $ref->getStartLine()] : [];
        }
    } catch (\Throwable $_) {
    }
    return [];
}

// seam_subject is the object a name expression starts from, "this" or "arg:N",
// which for a component seam is the component itself once it exists.
function seam_subject(string $expr, $self, array $args)
{
    $base = explode('.', explode(',', $expr)[0])[0];
    if ($base === 'this') {
        return is_object($self) ? $self : null;
    }
    if (strncmp($base, 'arg:', 4) === 0) {
        $v = $args[(int) substr($base, 4)] ?? null;
        return is_object($v) ? $v : null;
    }
    return null;
}

// component_identity names a component by its class when it has one of its
// own, remembering the name it goes by so a phase seen before the instance
// existed (a mount) is named the same, and adds its public state.
function component_identity(array $data, $subject): array
{
    $known = $GLOBALS['__lerd_component_class'] ?? [];
    if (!is_object($subject)) {
        if (isset($known[$data['name']])) {
            $data['name'] = $known[$data['name']];
        }
        return $data;
    }
    $ref = new \ReflectionObject($subject);
    if ($ref->getFileName()) {
        $data['file'] = (string) $ref->getFileName();
        $data['line'] = (int) $ref->getStartLine();
    }
    if (!$ref->isAnonymous()) {
        $GLOBALS['__lerd_component_class'][$data['name']] = $ref->getName();
        $data['name'] = $ref->getName();
    }
    $state = input_map(get_object_vars($subject));
    if ($state) {
        $data['state'] = $state;
    }
    return $data;
}

function component_details(array $args): array
{
    $plain = [];
    $lists = [];
    foreach ($args as $i => $v) {
        if (is_array($v)) {
            // A list of calls or updates says more as JSON than as its size.
            $json = json_encode($v, JSON_UNESCAPED_SLASHES | JSON_PARTIAL_OUTPUT_ON_ERROR);
            if (is_string($json)) {
                $lists['arg'.$i] = strlen($json) > 300 ? substr($json, 0, 300).'…' : $json;
            }
        } elseif (!is_object($v) && $v !== null) {
            $plain['arg'.$i] = $v;
        }
    }
    return preview_payload($plain) + $lists;
}

// request_end reports how a web request ended: its method and URI, the status
// it answered with, how long PHP spent on it and its peak memory. It runs at
// shutdown, after the response, so it costs the request nothing.
function request_end(): void
{
    try {
        $start = isset($_SERVER['REQUEST_TIME_FLOAT']) ? (float) $_SERVER['REQUEST_TIME_FLOAT'] : 0.0;
        $code = http_response_code();
        $data = [
            'method'      => isset($_SERVER['REQUEST_METHOD']) ? (string) $_SERVER['REQUEST_METHOD'] : '',
            'uri'         => isset($_SERVER['REQUEST_URI']) ? mask_url((string) $_SERVER['REQUEST_URI'], 'input') : '',
            'status'      => is_int($code) ? $code : 0,
            'time_ms'     => $start > 0 ? round((microtime(true) - $start) * 1000, 3) : 0,
            'memory_peak' => memory_get_peak_usage(true),
        ];
        if (!empty($_SERVER['HTTP_ORIGIN'])) {
            $data['origin'] = (string) $_SERVER['HTTP_ORIGIN'];
        }
        $data += request_input();
        // nginx says how long it held the request and when it handed it on, so
        // the time it waited for a free FPM worker is the gap to PHP's start.
        if (isset($_SERVER['LERD_NGINX_SENT']) && $start > 0) {
            $data['nginx_ms'] = round((float) ($_SERVER['LERD_NGINX_ELAPSED'] ?? 0) * 1000, 3);
            $data['queue_ms'] = max(0.0, round(($start - (float) $_SERVER['LERD_NGINX_SENT']) * 1000, 3));
        }
        if (!empty($GLOBALS['__lerd_route'])) {
            $data['route'] = (string) $GLOBALS['__lerd_route'];
        }
        // A framework's own session store reported itself on save; a plain PHP
        // session has no such call, so it is read here on the way out.
        if (empty($GLOBALS['__lerd_session_sent']) && isset($_SESSION) && is_array($_SESSION) && $_SESSION) {
            session_report($_SESSION, (string) session_name());
        }
        emit('request', $data);
    } catch (\Throwable $_) {
    }
}

// server_timing adds a Server-Timing header with what was measured before the
// headers went out: the FPM queue and each framework phase that had finished,
// a phase seen more than once (every view) summed under its label.
function server_timing(): void
{
    try {
        $parts = [];
        if (isset($_SERVER['LERD_NGINX_SENT'], $_SERVER['REQUEST_TIME_FLOAT'])) {
            $queue = max(0.0, ((float) $_SERVER['REQUEST_TIME_FLOAT'] - (float) $_SERVER['LERD_NGINX_SENT']) * 1000);
            $parts[] = sprintf('queue;dur=%.2f;desc="FPM queue"', $queue);
        }
        foreach ($GLOBALS['__lerd_spans'] ?? [] as $label => $ms) {
            $token = trim((string) preg_replace('/[^a-z0-9]+/', '-', strtolower((string) $label)), '-');
            if ($token !== '') {
                $parts[] = sprintf('%s;dur=%.2f;desc="%s"', $token, $ms, str_replace('"', '', (string) $label));
            }
        }
        if ($parts && !headers_sent()) {
            header('Server-Timing: ' . implode(', ', $parts), false);
        }
    } catch (\Throwable $_) {
    }
}

// request_input is what the request carried and what the response sent back,
// for the Requests lens: headers, query, body, cookies and response headers,
// each value cut short and anything that reads as a credential masked.
function request_input(): array
{
    $headers = [];
    foreach ($_SERVER as $k => $v) {
        if (strncmp($k, 'HTTP_', 5) === 0 && is_string($v)) {
            $headers[ucwords(strtolower(str_replace('_', '-', substr($k, 5))), '-')] = $v;
        }
    }
    unset($headers['Cookie']);
    if (isset($_SERVER['CONTENT_TYPE'])) {
        $headers['Content-Type'] = (string) $_SERVER['CONTENT_TYPE'];
    }
    $response = [];
    foreach (headers_list() as $line) {
        $parts = explode(':', $line, 2);
        if (count($parts) === 2 && strcasecmp($parts[0], 'Set-Cookie') !== 0) {
            $response[trim($parts[0])] = trim($parts[1]);
        }
    }
    $out = [];
    $scopes = ['headers' => 'in_request', 'query' => 'input', 'body' => 'input', 'cookies' => 'input', 'response_headers' => 'in_response'];
    foreach (['headers' => $headers, 'query' => $_GET, 'body' => $_POST, 'cookies' => $_COOKIE, 'response_headers' => $response] as $name => $values) {
        $map = input_map(is_array($values) ? $values : [], $scopes[$name]);
        if ($map) {
            $out[$name] = $map;
        }
    }
    return $out;
}

function input_map(array $values, string $scope = ''): array
{
    $rules = redaction();
    $out = [];
    foreach ($values as $k => $v) {
        if (count($out) >= PAYLOAD_KEYS) {
            break;
        }
        $k = (string) $k;
        $v = is_scalar($v) || $v === null ? (string) $v : (string) json_encode($v, \JSON_PARTIAL_OUTPUT_ON_ERROR | \JSON_UNESCAPED_SLASHES | \JSON_UNESCAPED_UNICODE);
        $mask = redact_rule($scope, $k) ?? (preg_match('/pass|secret|token|sess|cookie|authorization|api[-_]?key/i', $k) ? $rules['mask'] : null);
        if ($mask !== null) {
            $out[$k] = mask_value($v, $mask);
            continue;
        }
        $out[$k] = strlen($v) > 500 ? substr($v, 0, 497) . '...' : $v;
    }
    return $out;
}

// mask_url masks the query values of a URL the way input_map masks a name,
// so a secret in a query string never reaches the list or a request's header.
function mask_url(string $url, string $scope = ''): string
{
    // A route parameter the rules mask is masked where it sits in the path.
    if ($scope === 'input' && !empty($GLOBALS['__lerd_route_params'])) {
        $q = strpos($url, '?');
        $path = $q === false ? $url : substr($url, 0, $q);
        $segments = explode('/', $path);
        foreach ($GLOBALS['__lerd_route_params'] as $name => $value) {
            if (!is_scalar($value) || (string) $value === '') {
                continue;
            }
            // A route_params rule wins; input names and the defaults apply otherwise.
            $rule = redact_rule('route', (string) $name);
            if ($rule !== null) {
                $masked = mask_value((string) $value, $rule);
            } else {
                $mask = input_map([(string) $name => (string) $value], 'input');
                $masked = (string) reset($mask);
            }
            if ($masked === (string) $value) {
                continue;
            }
            foreach ($segments as $i => $segment) {
                if (rawurldecode($segment) === (string) $value) {
                    $segments[$i] = $masked;
                }
            }
        }
        $url = implode('/', $segments) . ($q === false ? '' : substr($url, $q));
    }
    $q = strpos($url, '?');
    if ($q === false) {
        return $url;
    }
    $parts = [];
    foreach (explode('&', substr($url, $q + 1)) as $pair) {
        $kv = explode('=', $pair, 2);
        if (count($kv) === 2) {
            $masked = input_map([urldecode($kv[0]) => urldecode($kv[1])], $scope);
            $value = (string) reset($masked);
            if ($value !== urldecode($kv[1])) {
                $pair = $kv[0] . '=' . $value;
            }
        }
        $parts[] = $pair;
    }
    return substr($url, 0, $q + 1) . implode('&', $parts);
}

// route_params keeps the matched route's raw parameters, as the request
// asked for them before any binding, so its path can be masked by name.
function route_params(array $params): void
{
    $GLOBALS['__lerd_route_params'] = $params;
}

// redact_rule is how the site's .lerd.yaml masks a name in a scope, or null
// when it says nothing about it. Names match case-insensitively, * as a glob.
function redact_rule(string $scope, string $name): ?array
{
    if ($scope === '') {
        return null;
    }
    foreach (redaction()[$scope] ?? [] as $pattern => $mask) {
        if (preg_match('/^' . str_replace('\\*', '.*', preg_quote((string) $pattern, '/')) . '$/i', $name)) {
            return $mask;
        }
    }
    return null;
}

// mask_value hides a value as [redacted], or as its first few characters and
// then the mask character, one per hidden character or a fixed run when cropped.
function mask_value(string $v, array $mask): string
{
    if (($mask['style'] ?? '') !== 'masked') {
        return '[redacted]';
    }
    $visible = (int) ($mask['visible'] ?? 4);
    $char = (string) ($mask['char'] ?? '*');
    $len = function_exists('mb_strlen') ? mb_strlen($v) : strlen($v);
    // A value no longer than twice what would show keeps nothing, so most of a
    // short secret is never on screen.
    $keep = $visible > 0 && $len > $visible * 2 ? $visible : 0;
    $head = function_exists('mb_substr') ? mb_substr($v, 0, $keep) : substr($v, 0, $keep);
    return $head . str_repeat($char, !empty($mask['crop']) ? 8 : $len - $keep);
}

// redaction reads what this site's .lerd.yaml masks on top of the defaults,
// once per process: the style the defaults are shown in, and header names
// per direction of an outgoing request with the style each is shown in.
function redaction(): array
{
    static $rules = null;
    if ($rules !== null) {
        return $rules;
    }
    $rules = ['mask' => ['style' => 'redacted', 'visible' => 4, 'crop' => false, 'char' => '*'], 'http_request' => [], 'http_response' => [], 'in_request' => [], 'in_response' => [], 'input' => [], 'route' => []];
    $read = static function (array $f): array {
        return ['style' => $f[0], 'visible' => (int) ($f[1] ?? 4), 'crop' => ($f[2] ?? '0') === '1', 'char' => isset($f[3]) && $f[3] !== '' ? $f[3] : '*'];
    };
    $path = getenv('LERD_DEVTOOLS_REDACT');
    $lines = @file(is_string($path) && $path !== '' ? $path : asset_path('devtools-redact.conf'), \FILE_IGNORE_NEW_LINES | \FILE_SKIP_EMPTY_LINES);
    $site = detect_site();
    foreach (is_array($lines) ? $lines : [] as $line) {
        $f = explode('|', $line);
        if ($line[0] === '#' || $f[0] !== $site || count($f) < 3) {
            continue;
        }
        if ($f[1] === 'style' && in_array($f[2], ['redacted', 'masked'], true)) {
            $rules['mask'] = $read(array_slice($f, 2));
        } elseif (in_array($f[1], ['http_request', 'http_response', 'in_request', 'in_response', 'input', 'route'], true) && count($f) >= 4 && in_array($f[3], ['redacted', 'masked'], true)) {
            $rules[$f[1]][$f[2]] = $read(array_slice($f, 3));
        }
    }
    return $rules;
}

// capture reports a call a store-declared capture seam claimed, where the whole
// event is the call itself rather than a span with a beginning and an end. The
// kind names the library; each one's extraction is its own function below.
function capture(string $kind, string $method, $self, array $args, string $name = ''): void
{
    if ($kind === 'ray') {
        ray($args);
        return;
    }
    if ($kind === 'log') {
        log_record($self, $args);
        return;
    }
    if ($kind === 'exception') {
        error_report($args, $name);
        return;
    }
    if ($kind === 'message') {
        notifier_message(isset($args[1]) ? $args[1] : null);
        return;
    }
    // The name the app gave the route it matched, kept for the request's own
    // event at shutdown. Only the first route bound counts: a framework may bind
    // another one later, the page a component update came from, say.
    if ($kind === 'route') {
        if (!array_key_exists('__lerd_route', $GLOBALS)) {
            $GLOBALS['__lerd_route'] = seam_value($name, $self, $args);
        }
        return;
    }
    // Everything an app writes through lerd/debug passes one method, and the
    // entry says which it is: a timeline row or a block of a tab.
    // lerd/debug renders its entries for the schema version this collector
    // reads, so a change on either side is a new renderer, not a break. A
    // package without renderers describes its entries as JSON in the same form.
    if ($kind === 'lerd') {
        $entry = $args[1] ?? null;
        if (!$entry instanceof \JsonSerializable) {
            return;
        }
        try {
            $data = \class_exists('Lerd\\Debug\\Rendering\\Renderers')
                ? \Lerd\Debug\Rendering\Renderers::for(DEBUG_SCHEMA)->render($entry)
                : json_decode((string) json_encode($entry, \JSON_PARTIAL_OUTPUT_ON_ERROR), true);
            $data = json_decode((string) json_encode($data, \JSON_PARTIAL_OUTPUT_ON_ERROR), true);
        } catch (\Throwable $_) {
            return;
        }
        $type = is_array($data) ? ($data['type'] ?? '') : '';
        if ($type === 'timeline') {
            timeline_entry($data);
        } elseif ($type === 'log') {
            lerd_log_line($data);
        } elseif ($type === 'auth') {
            auth_user(['id' => $data['id'] ?? '', 'email' => $data['email'] ?? null, 'name' => $data['name'] ?? null, 'guard' => $data['guard'] ?? null]);
        } elseif ($type === 'tab') {
            custom_tab_block([1 => $data['tab'] ?? '', 2 => $data['title'] ?? '', 3 => $data['block'] ?? null, 4 => $data['columns'] ?? 1, 5 => $data['placement'] ?? null]);
        }
        return;
    }
    // A security token being stored is who the request runs as, on a
    // framework whose store declares where that happens. Without a token the
    // name expression resolves the user's id itself.
    if ($kind === 'auth') {
        $token = $args[1] ?? null;
        $user = is_object($token) && method_exists($token, 'getUser') ? $token->getUser() : null;
        if (is_object($user)) {
            $id = method_exists($user, 'getUserIdentifier') ? (string) $user->getUserIdentifier() : (method_exists($user, 'getId') ? scalar_string($user->getId()) : '');
            auth_user(['id' => $id, 'email' => method_exists($user, 'getEmail') ? scalar_string($user->getEmail()) : null, 'name' => null, 'guard' => null]);
        } elseif ($name !== '') {
            auth_user(['id' => seam_value($name, $self, $args), 'email' => null, 'name' => null, 'guard' => null]);
        }
        return;
    }
    if ($kind === 'command') {
        if (is_object($self)) {
            exclude_command(method_exists($self, 'getName') ? (string) $self->getName() : '', get_class($self));
        }
        return;
    }
    if ($kind === 'session' && is_object($self) && method_exists($self, 'all')) {
        try {
            $all = $self->all();
        } catch (\Throwable $_) {
            return;
        }
        if (is_array($all)) {
            session_report($all, seam_value($name, $self, $args));
        }
    }
}

// timeline_entry reports a row an app put on its own timeline: a label, the
// category it filters under and the colour it is drawn in, when it started and
// how long it took, and what to show in its popover.
function timeline_entry(array $entry): void
{
    if (!isset($entry['label'])) {
        return;
    }
    $data = [
        'label'    => scalar_string($entry['label']),
        'category' => scalar_string($entry['category'] ?? 'app'),
        'color'    => scalar_string($entry['color'] ?? ''),
        'start'    => (float) ($entry['start'] ?? 0),
    ];
    if (isset($entry['duration_ms'])) {
        $data['duration_ms'] = round((float) $entry['duration_ms'], 3);
    }
    $details = isset($entry['details']) && is_array($entry['details']) ? input_map($entry['details']) : [];
    if ($details) {
        $data['details'] = $details;
    }
    emit('timeline', $data);
}

// auth_user reports who the request runs as, once per request: the first
// user resolved is the one shown, so a guard checked twice does not repeat.
function auth_user(array $user): void
{
    $id = scalar_string($user['id'] ?? '');
    $rid = rid();
    if ($id === '' || isset($GLOBALS['__lerd_auth'][$rid])) {
        return;
    }
    $GLOBALS['__lerd_auth'][$rid] = true;
    $data = ['id' => $id];
    foreach (['email', 'name', 'guard'] as $key) {
        if (!empty($user[$key])) {
            $data[$key] = scalar_string($user[$key]);
        }
    }
    emit('auth', $data);
}

// lerd_log_line reports a line an app wrote through lerd/debug as a log entry
// like any other, on the "lerd" channel, carrying whether to show its stack
// trace and whether it belongs on the Performance tab.
function lerd_log_line(array $line): void
{
    $message = scalar_string($line['message'] ?? '');
    if ($message === '') {
        return;
    }
    $data = ['level' => strtolower(scalar_string($line['level'] ?? 'info')), 'message' => $message, 'channel' => 'lerd'];
    if (!empty($line['context']) && is_array($line['context'])) {
        $data['context'] = rtrim(render_var($line['context']));
    }
    if (!empty($line['trace'])) {
        $data['show_trace'] = true;
    }
    if (!empty($line['performance'])) {
        $data['performance'] = true;
    }
    emit('log', $data);
}

// custom_tab_block reports one block an app added to a tab of its own: the
// tab's id and title, which put its blocks together in the lens, and the block
// as plain data, cut off once it grows past what a lens can show.
function custom_tab_block(array $args): void
{
    $id = scalar_string($args[1] ?? '');
    $block = $args[3] ?? null;
    if ($id === '' || !is_array($block)) {
        return;
    }
    $json = json_encode($block, \JSON_PARTIAL_OUTPUT_ON_ERROR | \JSON_UNESCAPED_SLASHES | \JSON_UNESCAPED_UNICODE);
    if (!is_string($json) || strlen($json) > 65536) {
        $json = '{"type":"text","text":"This block is larger than 64 KB and was left out."}';
    }
    // Blocks added in the same millisecond keep their order by this sequence.
    static $seq = 0;
    $data = ['id' => $id, 'title' => scalar_string($args[2] ?? $id), 'columns' => max(1, min(4, (int) ($args[4] ?? 1))), 'seq' => ++$seq, 'block' => json_decode($json, true) ?: []];
    $placement = $args[5] ?? null;
    if (is_array($placement) && in_array($placement['position'] ?? '', ['before', 'after'], true) && is_string($placement['tab'] ?? null)) {
        $data['placement'] = ['position' => $placement['position'], 'tab' => $placement['tab']];
    }
    emit('tab', $data);
}

// session_report sends what the session held when the request finished, one
// entry per top-level key with nested values as JSON. A key naming a password
// or a secret is masked, since a login stores the password hash there.
function session_report(array $all, string $name): void
{
    $GLOBALS['__lerd_session_sent'] = true;
    $out = [];
    foreach ($all as $k => $v) {
        if (count($out) >= PAYLOAD_KEYS) {
            break;
        }
        $k = (string) $k;
        $rule = redact_rule('input', $k);
        if ($rule !== null || preg_match('/password|secret|token|authkey|csrf/i', $k)) {
            $out[$k] = mask_value(is_scalar($v) ? (string) $v : '', $rule ?? redaction()['mask']);
        } elseif (is_array($v) || (is_object($v) && !$v instanceof \Closure && !$v instanceof \UnitEnum)) {
            $json = json_encode($v, \JSON_PARTIAL_OUTPUT_ON_ERROR | \JSON_UNESCAPED_SLASHES | \JSON_UNESCAPED_UNICODE);
            $out[$k] = is_string($json) ? (strlen($json) > 500 ? substr($json, 0, 497) . '...' : $json) : preview_value($v);
        } else {
            $out[$k] = preview_value($v);
        }
    }
    $data = ['keys' => count($all), 'data' => $out];
    if ($name !== '') {
        $data['name'] = $name;
    }
    emit('session', $data);
}

// notifier_message reports one message an app sent to somebody: an SMS, a chat
// post, a push. Mail has had a lens since the window was built and everything
// else a site sends had none, even though it is the same question, did it go
// out and what did it say, asked of a different channel.
//
// Symfony's Notifier is where to take it: every transport it ships, Twilio,
// Vonage, Slack and the rest, is reached through the same two entry points, and
// each message answers the same small interface whatever channel it is for.
function notifier_message($message): void
{
    if (!is_object($message) || !method_exists($message, 'getSubject')) {
        return;
    }
    $data = ['channel' => message_channel(get_class($message))];
    $body = (string) $message->getSubject();
    if ($body !== '') {
        $data['body'] = $body;
    }
    // An SMS names the phone it is going to, everything else a recipient id.
    if (method_exists($message, 'getPhone')) {
        $data['to'] = (string) $message->getPhone();
    } elseif (method_exists($message, 'getRecipientId')) {
        $data['to'] = (string) $message->getRecipientId();
    }
    if (method_exists($message, 'getFrom')) {
        $from = (string) $message->getFrom();
        if ($from !== '') {
            $data['from'] = $from;
        }
    }
    if (method_exists($message, 'getTransport')) {
        $transport = (string) $message->getTransport();
        if ($transport !== '') {
            $data['transport'] = $transport;
        }
    }
    emit('message', $data);
}

// message_channel names the kind of message from its class, so SmsMessage is an
// sms and ChatMessage a chat, whatever namespace it came from.
function message_channel(string $class): string
{
    $short = strtolower(substr($class, strrpos($class, '\\') === false ? 0 : strrpos($class, '\\') + 1));
    if (substr($short, -7) === 'message') {
        $short = substr($short, 0, -7);
    }
    return $short !== '' ? $short : 'message';
}

// error_report reports what an app was about to send to an error monitor.
// Locally the report usually goes nowhere, there being no key configured, and
// where one is set it goes to a project nobody watches for a developer's own
// laptop, so the report that matters is the one in front of them.
//
// The two shapes a reporter hands over are a throwable, which is everything
// needed, and Sentry's event plus hint, which is where that SDK keeps the
// throwable while the event itself is still empty.
function error_report(array $args, string $source = ''): void
{
    $first = isset($args[1]) ? $args[1] : null;
    if ($first instanceof \Throwable) {
        throwable_event($first, 'error', $source);
        return;
    }
    sentry_event($args, $source);
}

// log_record reports one record written to a logger. Monolog is the seam every
// framework's logging ends up going through, and its entry point has carried
// the same three arguments since Monolog 1, so one capture covers the field.
// The level arrives as an integer on the older majors and as an enum on the
// newest, and the channel is the logger's own name.
function log_record($self, array $args): void
{
    $message = isset($args[2]) ? $args[2] : '';
    if (!is_string($message) || $message === '') {
        return;
    }
    $data = ['level' => log_level(isset($args[1]) ? $args[1] : null), 'message' => $message];
    if (is_object($self) && method_exists($self, 'getName')) {
        $channel = (string) $self->getName();
        if ($channel !== '') {
            $data['channel'] = $channel;
        }
    }
    // The context is what the developer chose to attach to the line, so it is
    // rendered in full the way a dump is, rather than reduced to its shape.
    if (isset($args[3]) && is_array($args[3]) && $args[3] !== []) {
        $data['context'] = rtrim(render_var($args[3]));
    }
    emit('log', $data);
}

// log_level names the severity a record was written at. Monolog 3 passes an
// enum, Monolog 1 and 2 an integer of their own scale, and either major accepts
// an RFC 5424 severity in place of one, which is the same range as the low end
// of theirs; the enum is asked first and the two scales are told apart by size,
// since Monolog's own start at 100.
function log_level($level): string
{
    if (is_object($level)) {
        if (method_exists($level, 'getName')) {
            return strtolower((string) $level->getName());
        }
        return isset($level->name) ? strtolower((string) $level->name) : '';
    }
    if (!is_int($level)) {
        return is_string($level) ? strtolower($level) : '';
    }
    static $monolog = [
        100 => 'debug', 200 => 'info', 250 => 'notice', 300 => 'warning',
        400 => 'error', 500 => 'critical', 550 => 'alert', 600 => 'emergency',
    ];
    static $rfc = [
        0 => 'emergency', 1 => 'alert', 2 => 'critical', 3 => 'error',
        4 => 'warning', 5 => 'notice', 6 => 'info', 7 => 'debug',
    ];
    if (isset($monolog[$level])) {
        return $monolog[$level];
    }
    return isset($rfc[$level]) ? $rfc[$level] : (string) $level;
}

// ray_silent reports a payload the Ray app draws rather than reads: a colour, a
// screen switch, a lock. There is nothing in one to show in a window that is
// not Ray, so they are dropped instead of arriving as empty rows.
function ray_silent(string $type): bool
{
    static $silent = [
        'clear_all' => 1, 'color' => 1, 'confetti' => 1, 'create_lock' => 1,
        'expand' => 1, 'hide' => 1, 'hide_app' => 1, 'label' => 1,
        'new_screen' => 1, 'remove' => 1, 'screen_color' => 1, 'separator' => 1,
        'show_app' => 1, 'size' => 1,
    ];
    return isset($silent[$type]);
}

// ray reports the payloads of one ray() call as dumps. Every call funnels into
// the one method this seam observes, whatever built it, so a plain ray($user)
// and a ray()->table() both arrive here; the payload has already been converted
// for the Ray app by the time it does, so it is unwrapped from that form rather
// than rendered. A plain call is labelled `ray` and everything else by the kind
// of payload it is.
function ray(array $args): void
{
    $payloads = isset($args[1]) ? $args[1] : null;
    if (!is_array($payloads)) {
        $payloads = [$payloads];
    }
    foreach ($payloads as $payload) {
        $type = ray_type($payload);
        if ($type === '' || ray_silent($type)) {
            continue;
        }
        send_text(ray_content_text($payload), $type === 'log' ? 'ray' : 'ray:' . $type);
    }
}

// ray_type reads a payload's type, from the object or from the array form a
// payload turns into on the wire.
function ray_type($payload): string
{
    if (is_object($payload) && method_exists($payload, 'getType')) {
        return (string) $payload->getType();
    }
    if (is_array($payload) && isset($payload['type'])) {
        return (string) $payload['type'];
    }
    return '';
}

// ray_content_text renders a payload's content as the lines the dumps lens
// shows. Meta is the package's own bookkeeping (versions, clipboard copies) and
// is dropped; a single unnamed value is its own text, and anything else is
// listed by the name the payload gave it.
function ray_content_text($payload): string
{
    $content = [];
    if (is_object($payload) && method_exists($payload, 'getContent')) {
        $content = $payload->getContent();
    } elseif (is_array($payload) && isset($payload['content'])) {
        $content = $payload['content'];
    }
    if (!is_array($content)) {
        $content = [$content];
    }
    unset($content['meta']);
    // A payload keeps what it is showing under 'values' and its own settings
    // beside it, so the values are lifted out and listed first.
    $values = [];
    if (isset($content['values']) && is_array($content['values'])) {
        $values = $content['values'];
        unset($content['values']);
    }
    $lines = [];
    foreach (array_merge($values, $content) as $key => $value) {
        if ($value === null || $value === '' || $value === []) {
            continue;
        }
        // A rendered value ends in its own newline, which between the lines of
        // a listed payload reads as a blank row.
        $text = rtrim(is_string($value) ? dump_html_to_text($value) : render_var($value));
        $lines[] = is_int($key) ? $text : $key . ': ' . $text;
    }
    return implode("\n", $lines);
}

// dump_html_to_text turns the HTML a package rendered a value into back into
// text. Ray converts every non-scalar argument with Symfony's HtmlDumper and
// escapes every plain one before either leaves the process, and neither reads
// as anything in a terminal or a text lens, so the markup is taken back off.
// A value carrying neither is returned as it came, since a string a developer
// dumped may well contain an ampersand it is meant to keep.
function dump_html_to_text(string $html): string
{
    $markup = strpos($html, '<') !== false;
    if (!$markup && strpos($html, '&') === false) {
        return $html;
    }
    $out = $html;
    if ($markup) {
        $out = preg_replace('~<(script|style)\b[^>]*>.*?</\1>~is', '', $out);
        $out = preg_replace('~<br\s*/?>~i', "\n", (string) $out);
        $out = strip_tags((string) $out);
    }
    $out = html_entity_decode((string) $out, \ENT_QUOTES, 'UTF-8');
    return trim(str_replace("\xc2\xa0", ' ', $out));
}

// sentry_event reports one event on its way to Sentry. The throwable is handed
// in the hint rather than on the event, capturing an exception building an
// empty event and letting the pipeline attach the frames later, so the hint is
// read first, the event's own exceptions second, and a captured message last.
function sentry_event(array $args, string $source = ''): void
{
    $event = isset($args[1]) ? $args[1] : null;
    $hint = isset($args[2]) ? $args[2] : null;
    $level = sentry_level($event);
    if (is_object($hint) && isset($hint->exception) && $hint->exception instanceof \Throwable) {
        throwable_event($hint->exception, $level, $source);
        return;
    }
    $thrown = sentry_exception_bag($event);
    if ($thrown) {
        emit('exception', with_source(array_merge($thrown, ['level' => $level]), $source));
        return;
    }
    $message = is_object($event) && method_exists($event, 'getMessage') ? $event->getMessage() : null;
    if (is_string($message) && $message !== '') {
        emit('exception', with_source(['type' => 'message', 'message' => $message, 'level' => $level], $source));
    }
}

// throwable_event reports one throwable with its own origin: the frames it was
// thrown from rather than the ones that reported it, resolved by the same rule
// the collector resolves a caller with, so the line named is the developer's.
function throwable_event(\Throwable $t, string $level, string $source = ''): void
{
    $frames = [['file' => $t->getFile(), 'line' => $t->getLine(), 'func' => '']];
    foreach ($t->getTrace() as $f) {
        if (!isset($f['file'])) {
            continue;
        }
        $func = (isset($f['class']) ? $f['class'] . (isset($f['type']) ? $f['type'] : '::') : '') . (isset($f['function']) ? $f['function'] : '');
        $frames[] = ['file' => $f['file'], 'line' => isset($f['line']) ? $f['line'] : 0, 'func' => $func];
    }
    $src = null;
    foreach ($frames as $f) {
        if ($f['file'] !== '' && !is_dependency($f['file'])) {
            $src = ['file' => $f['file'], 'line' => $f['line']];
            break;
        }
    }
    if ($src === null) {
        $src = ['file' => $frames[0]['file'], 'line' => $frames[0]['line']];
    }
    $data = ['type' => get_class($t), 'message' => $t->getMessage(), 'level' => $level];
    $previous = $t->getPrevious();
    if ($previous instanceof \Throwable) {
        $data['previous'] = get_class($previous) . ': ' . $previous->getMessage();
    }
    emit_with('exception', with_source($data, $source), $src, $frames);
}

// with_source names the reporter an event was taken from, which the store's
// seam declares. An app running one reporter sees the same word on every row,
// and one running two can tell which saw what.
function with_source(array $data, string $source): array
{
    if ($source !== '') {
        $data['source'] = $source;
    }
    return $data;
}

// sentry_level reads the severity off an event, defaulting to the one Sentry
// itself defaults to. The level is an object that renders as its own name.
function sentry_level($event): string
{
    if (!is_object($event) || !method_exists($event, 'getLevel')) {
        return 'error';
    }
    $level = $event->getLevel();
    if ($level === null) {
        return 'error';
    }
    return strtolower((string) $level);
}

// sentry_exception_bag reads the first exception off an event that was built
// with one already attached, which is how an event reaches the client when the
// app assembled it itself rather than handing over a throwable.
function sentry_exception_bag($event): array
{
    if (!is_object($event) || !method_exists($event, 'getExceptions')) {
        return [];
    }
    foreach ($event->getExceptions() as $bag) {
        if (!is_object($bag) || !method_exists($bag, 'getType')) {
            continue;
        }
        return [
            'type'    => (string) $bag->getType(),
            'message' => method_exists($bag, 'getValue') ? (string) $bag->getValue() : '',
        ];
    }
    return [];
}

// http captures one outgoing Symfony HttpClient request at call time. The
// response is lazy (not sent until read), so no status code is available here;
// the UI shows the request as "sent". Method and url are read at the begin
// observer because request() rewrites its $url argument internally.
function http($method, $url, $options = null): void
{
    $u = is_string($url) ? $url : '';
    if ($u === '') {
        return;
    }
    $headers = [];
    $options = is_array($options) ? $options : [];
    foreach ((array) ($options['headers'] ?? []) as $k => $v) {
        if (is_int($k) && is_string($v) && strpos($v, ':') !== false) {
            [$k, $v] = array_map('trim', explode(':', $v, 2));
        }
        $headers[(string) $k] = is_array($v) ? implode(', ', array_map('strval', $v)) : (string) $v;
    }
    if (!empty($options['auth_bearer']) || !empty($options['auth_basic'])) {
        $headers['Authorization'] = '[redacted]';
    }
    if (empty($GLOBALS['__lerd_http_flush'])) {
        $GLOBALS['__lerd_http_flush'] = true;
        register_shutdown_function(__NAMESPACE__ . '\\http_flush');
    }
    $bt = backtrace();
    $GLOBALS['__lerd_http_open'][] = [
        'method' => strtoupper(is_string($method) ? $method : ''),
        'url' => $u,
        'request_headers' => input_map($headers, 'http_request'),
        'src' => $bt['src'],
        'trace' => $bt['trace'],
    ];
}

// http_done reports an outgoing request once its response has ended, read off
// the response the way the client recorded it, and paired with the request it
// answers by method and the URL it was sent to.
function http_done($response, bool $failed): void
{
    if (!is_object($response) || !method_exists($response, 'getInfo')) {
        return;
    }
    try {
        $info = $response->getInfo();
    } catch (\Throwable $_) {
        return;
    }
    $method = strtoupper((string) ($info['http_method'] ?? ''));
    $url = (string) ($info['original_url'] ?? $info['url'] ?? '');
    $open = $GLOBALS['__lerd_http_open'] ?? [];
    $sent = null;
    foreach ($open as $i => $o) {
        if ($o['url'] === $url && ($method === '' || $o['method'] === $method)) {
            $sent = $o;
            unset($open[$i]);
            break;
        }
    }
    $GLOBALS['__lerd_http_open'] = array_values($open);
    $sent = $sent ?? ['method' => $method, 'url' => $url, 'request_headers' => [], 'src' => [], 'trace' => []];
    $headers = [];
    $reason = '';
    foreach ((array) ($info['response_headers'] ?? []) as $line) {
        // The status line names the reason; after a redirect the last one counts.
        if (preg_match('#^HTTP/\S+\s+\d{3}\s*(.*)$#', (string) $line, $m)) {
            $reason = trim($m[1]);
            continue;
        }
        $parts = explode(':', (string) $line, 2);
        if (count($parts) === 2) {
            $headers[trim($parts[0])] = trim($parts[1]);
        }
    }
    $status = (int) ($info['http_code'] ?? 0);
    http_report($sent, $status, $headers, $info, $failed || $status === 0 || !empty($info['error']), $reason);
}

// http_report emits one outgoing request with what both ends said and how long
// each phase took, from the cumulative seconds curl and the clients report.
function http_report(array $sent, int $status, array $responseHeaders, array $stats, bool $failed, string $reason = ''): void
{
    $data = ['method' => $sent['method'], 'url' => mask_url((string) $sent['url']), 'status' => $status];
    if ($reason !== '') {
        $data['reason'] = $reason;
    }
    if ($failed) {
        $data['failed'] = true;
    }
    if (isset($stats['total_time']) && is_numeric($stats['total_time'])) {
        $data['time_ms'] = round((float) $stats['total_time'] * 1000, 2);
    }
    $timing = [];
    foreach (['dns' => 'namelookup_time', 'connect' => 'connect_time', 'tls' => 'appconnect_time', 'sent' => 'pretransfer_time', 'first_byte' => 'starttransfer_time'] as $label => $key) {
        if (isset($stats[$key]) && is_numeric($stats[$key]) && (float) $stats[$key] > 0) {
            $timing[$label] = round((float) $stats[$key] * 1000, 2);
        }
    }
    if ($timing) {
        $data['timing'] = $timing;
    }
    // Body sizes as the client counted them, else as the response declared.
    $len = null;
    foreach ($responseHeaders as $k => $v) {
        if (strcasecmp((string) $k, 'Content-Length') === 0 && is_numeric($v)) {
            $len = (int) $v;
        }
    }
    foreach (['request_size' => ['size_upload', null], 'response_size' => ['size_download', $len]] as $key => [$stat, $fallback]) {
        $size = isset($stats[$stat]) && is_numeric($stats[$stat]) && (float) $stats[$stat] > 0 ? (int) $stats[$stat] : $fallback;
        if ($size !== null) {
            $data[$key] = $size;
        }
    }
    if (!empty($sent['request_headers'])) {
        $data['request_headers'] = $sent['request_headers'];
    }
    if ($responseHeaders) {
        $data['response_headers'] = input_map($responseHeaders, 'http_response');
    }
    if ($sent['src']) {
        emit_with('http', $data, $sent['src'], $sent['trace']);
    } else {
        emit('http', $data);
    }
}

// http_flush reports the requests whose response never ended in the process,
// so a request is never lost when the response seam is not declared.
function http_flush(): void
{
    foreach ($GLOBALS['__lerd_http_open'] ?? [] as $o) {
        http_report($o, 0, [], [], false);
    }
    $GLOBALS['__lerd_http_open'] = [];
}

// A console command named as typed (artisan queue:work, bin/cake queue
// worker) is known from argv before the framework boots, so even the boot of
// an excluded command is left out. A name may span its first few words.
if (\PHP_SAPI === 'cli' && !empty($_SERVER['argv']) && is_array($_SERVER['argv'])) {
    $typed = '';
    foreach (array_slice($_SERVER['argv'], 1, 3) as $arg) {
        if (!is_string($arg) || $arg === '' || $arg[0] === '-') {
            break;
        }
        $typed = ltrim($typed . ' ' . $arg);
        exclude_command($typed);
    }
}
