<?php
// /usr/local/etc/lerd/dump-bridge.php
//
// Always-mounted auto_prepend_file. The runtime sentinel
// `/usr/local/etc/lerd/enabled.flag` controls whether this file installs
// the dump()/dd() override or short-circuits and lets Symfony's stock
// helpers stay in charge. Flipping the bridge on or off is a single
// touch/rm of that file — no FPM restart, no worker cascade.
//
// Transport (socket send, request context, ids, source frame) and the rendering
// of a variable to text are shared with the lerd_devtools collector so there is
// one implementation, not two: this file only owns the dump()/dd() override.
//
// This file must never throw, never block, and never emit output. It is an
// auto_prepend_file for every PHP lerd builds, down to the 7.2 legacy tier, so
// it (and the collector it requires) must parse and run on all of them: no
// `mixed`/`never` hints, no `match`, no arrow functions, no nullsafe.

namespace {
    // env_provider output for this site (see `env_provider` in .lerd.yaml),
    // kept on tmpfs by lerd so secrets never touch disk. Loaded before any
    // framework boots; a variable the process already has is left alone.
    // $_SERVER first: under FPM it is the per-request fastcgi param nginx sets
    // for the vhost, never state a reused worker carries over.
    $lerdSite = isset($_SERVER['LERD_SITE']) ? $_SERVER['LERD_SITE'] : \getenv('LERD_SITE');
    if (\is_string($lerdSite) && $lerdSite !== '' && \preg_replace('/[A-Za-z0-9._-]/', '', $lerdSite) === '' && \strpos($lerdSite, '..') === false) {
        $lerdEnvDir = \get_cfg_var('lerd.provided_env_dir');
        if (!\is_string($lerdEnvDir) || $lerdEnvDir === '') {
            $lerdEnvDir = '/run/lerd/env';
        }
        $lerdEnv = @\file_get_contents($lerdEnvDir.'/'.$lerdSite.'.env');
        if (\is_string($lerdEnv)) {
            // The file names the directories it belongs to; a script outside
            // them gets nothing, so a wrong LERD_SITE cannot leak another site's.
            $lerdScript = isset($_SERVER['SCRIPT_FILENAME']) ? @\realpath($_SERVER['SCRIPT_FILENAME']) : false;
            $lerdAllowed = false;
            // The header lerd writes comes first; root lines further down are
            // provider output and widen nothing.
            \preg_match('/\A(?:#lerd-root=[^\r\n]*\r?\n)*/', $lerdEnv, $lerdHead);
            foreach (\preg_split('/\r?\n/', $lerdHead[0]) as $lerdLine) {
                $root = \rtrim((string) \substr($lerdLine, 11), '/');
                if ($root !== '' && \is_string($lerdScript) && \strpos($lerdScript, $root.'/') === 0) {
                    $lerdAllowed = true;
                }
            }
            // Quoted values may span lines (PEM keys); unquoted ones end at
            // the line and drop a ` # comment`.
            $lerdPairs = array();
            \preg_match_all('/^[ \t]*(?:export[ \t]+)?([A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*("(?:[^"\\\\]|\\\\.)*"|\'[^\']*\'|[^\r\n]*)/m', $lerdEnv, $lerdMatches, \PREG_SET_ORDER);
            foreach ($lerdMatches as $lerdMatch) {
                $v = $lerdMatch[2];
                $q = $v === '' ? '' : $v[0];
                if ($q === '"' && \strlen($v) > 1 && \substr($v, -1) === '"') {
                    $v = \strtr(\substr($v, 1, -1), array('\\n' => "\n", '\\"' => '"', '\\\\' => '\\'));
                } elseif ($q === "'" && \strlen($v) > 1 && \substr($v, -1) === "'") {
                    $v = \substr($v, 1, -1);
                } else {
                    $v = \trim(\preg_replace('/(?:^|[ \t]+)#.*$/', '', $v));
                }
                $lerdPairs[$lerdMatch[1]] = $v;
            }
            if ($lerdAllowed) {
                foreach ($lerdPairs as $k => $v) {
                    if (\getenv($k) !== false || isset($_SERVER[$k])) {
                        continue;
                    }
                    \putenv($k.'='.$v);
                    $_ENV[$k] = $v;
                    $_SERVER[$k] = $v;
                }
            }
        }
        unset($lerdEnvDir, $lerdEnv, $lerdScript, $lerdAllowed, $lerdHead, $lerdPairs, $lerdMatches, $lerdMatch, $lerdLine, $root, $k, $v, $q);
    }
    unset($lerdSite);

    // Where the bridge's own assets live. The container mounts them at a fixed
    // path; a PHP running on the host has no such directory, so the location is
    // read from the ini and only falls back to the container path. get_cfg_var
    // rather than ini_get: the directive belongs to no extension, and ini_get
    // returns false for those.
    $lerdAssets = \get_cfg_var('lerd.assets_dir');
    if (!\is_string($lerdAssets) || $lerdAssets === '') {
        $lerdAssets = '/usr/local/etc/lerd';
    }
    // Fast no-op when the toggle file is absent. One stat() per request in the
    // disabled case; the return stops the whole prepend so nothing below loads.
    if (!@file_exists($lerdAssets.'/enabled.flag')) {
        return;
    }
    // The shared transport lives in the collector. Pull it in if some other
    // seam hasn't already; without it we can't ship, so stand down and let
    // Symfony's stock dump()/dd() stay in charge rather than half-capture.
    if (!\function_exists('Lerd\\Collector\\send')) {
        @include_once $lerdAssets.'/devtools-collector.php';
    }
    if (!\function_exists('Lerd\\Collector\\send')) {
        return;
    }
    // While capture is on, the response names the request its events were
    // grouped under, so nginx's access log and the browser can link to it.
    if (\PHP_SAPI !== 'cli' && \defined('LERD_DEVTOOLS_RID')) {
        if (!\headers_sent()) {
            \header('X-Lerd-Rid: '.\LERD_DEVTOOLS_RID);
        }
        // The id goes on the SPX profile too, linking the request to its flame
        // graph. SPX segfaults FPM when this is called on a request it is not
        // profiling, so only when its cookie and key say it is.
        $lerdSpxKey = \ini_get('spx.http_key');
        if (\function_exists('spx_profiler_full_report_set_custom_metadata_str')
            && isset($_COOKIE['SPX_ENABLED'], $_COOKIE['SPX_KEY'])
            && $_COOKIE['SPX_ENABLED'] === '1'
            && \is_string($lerdSpxKey) && $lerdSpxKey !== '' && $_COOKIE['SPX_KEY'] === $lerdSpxKey
            && (!isset($_COOKIE['SPX_REPORT']) || $_COOKIE['SPX_REPORT'] === 'full')
            && (!isset($_COOKIE['SPX_AUTO_START']) || $_COOKIE['SPX_AUTO_START'] !== '0')) {
            \spx_profiler_full_report_set_custom_metadata_str('lerd-rid:'.\LERD_DEVTOOLS_RID);
        }
        unset($lerdSpxKey);
    }
}

namespace Lerd\DumpBridge {
    if (defined(__NAMESPACE__.'\\LOADED')) {
        return;
    }
    const LOADED = 1;

    // passthrough_enabled reports whether the dashboard capture should ALSO
    // emit the dump to the response via Symfony's stock VarDumper handler.
    // Default false (capture-only) — same behaviour as Herd's dumps window;
    // override per-install with `dumps.passthrough: true` in config.yaml or
    // via the LERD_DUMP_PASSTHROUGH env var.
    function passthrough_enabled(): bool
    {
        $env = getenv('LERD_DUMP_PASSTHROUGH');
        if ($env !== false && $env !== '') {
            return $env === '1' || strcasecmp($env, 'true') === 0;
        }
        $cfg = get_cfg_var('lerd.dump_passthrough');
        return is_string($cfg) && ($cfg === '1' || strcasecmp($cfg, 'true') === 0);
    }

    // emit ships one variable as a dump event. Rendering and transport both
    // live in the collector, so a dump() and a ray() land as the same text on
    // the same socket; this file only owns which calls get captured.
    function emit($var, ?string $label = null): void
    {
        \Lerd\Collector\send_dump($var, $label);
    }
}

namespace {
    // Define dump()/dd() in auto_prepend_file before composer's var-dumper
    // functions.php gets a chance to. Both Symfony helpers are gated on
    // `if (!function_exists(...))`, so ours wins. With passthrough on we forward
    // through Symfony's VarDumper so existing display pipelines (Whoops,
    // Ignition) keep working in the response.
    if (!function_exists('dump')) {
        function dump(...$vars)
        {
            $passthrough = \Lerd\DumpBridge\passthrough_enabled();
            foreach ($vars as $label => $var) {
                \Lerd\DumpBridge\emit($var, is_string($label) ? $label : null);
                if ($passthrough && class_exists(\Symfony\Component\VarDumper\VarDumper::class)) {
                    \Symfony\Component\VarDumper\VarDumper::dump($var);
                }
            }
            if (count($vars) === 0) {
                return null;
            }
            if (count($vars) === 1) {
                return reset($vars);
            }
            return $vars;
        }
    }
    if (!function_exists('dd')) {
        function dd(...$vars)
        {
            dump(...$vars);
            exit(1);
        }
    }
}
