<?php
// /usr/local/etc/lerd/dump-bridge.php
//
// Always-mounted auto_prepend_file. The runtime sentinel
// `/usr/local/etc/lerd/enabled.flag` controls whether this file installs
// the dump()/dd() override or short-circuits and lets Symfony's stock
// helpers stay in charge. Flipping the bridge on or off is a single
// touch/rm of that file — no FPM restart, no worker cascade.
//
// Transport (socket send, request context, ids, source frame) is shared with
// the lerd_devtools collector so there is one implementation, not two: this
// file only owns the dump()/dd() override and rendering the variable to text.
//
// This file must never throw, never block, and never emit output. It is an
// auto_prepend_file for every PHP lerd builds, down to the 7.2 legacy tier, so
// it (and the collector it requires) must parse and run on all of them: no
// `mixed`/`never` hints, no `match`, no arrow functions, no nullsafe.

namespace {
    // Fast no-op when the toggle file is absent. One stat() per request in the
    // disabled case; the return stops the whole prepend so nothing below loads.
    if (!@file_exists('/usr/local/etc/lerd/enabled.flag')) {
        return;
    }
    // The shared transport lives in the collector. Pull it in if some other
    // seam hasn't already; without it we can't ship, so stand down and let
    // Symfony's stock dump()/dd() stay in charge rather than half-capture.
    if (!\function_exists('Lerd\\Collector\\send')) {
        @include_once '/usr/local/etc/lerd/devtools-collector.php';
    }
    if (!\function_exists('Lerd\\Collector\\send')) {
        return;
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

    // emit renders one variable to text and ships it on the shared collector
    // transport. The dump envelope (top-level label + text) differs from the
    // structured kinds, so it's built here rather than via the collector's
    // emit(); everything else (id, ts, context, source frame, socket) is reused.
    function emit($var, ?string $label = null): void
    {
        try {
            if (!class_exists(\Symfony\Component\VarDumper\Cloner\VarCloner::class, true)
                || !class_exists(\Symfony\Component\VarDumper\Dumper\CliDumper::class, true)) {
                $text = is_scalar($var) ? (string) $var : print_r($var, true);
            } else {
                $cloner = new \Symfony\Component\VarDumper\Cloner\VarCloner();
                $maxItems = (int) (getenv('LERD_DUMP_MAX_ITEMS') ?: 2500);
                $cloner->setMaxItems($maxItems > 0 ? $maxItems : 2500);
                $cloner->setMaxString(4096);
                $data = $cloner->cloneVar($var);
                $dumper = new \Symfony\Component\VarDumper\Dumper\CliDumper();
                $dumper->setColors(false);
                $rendered = $dumper->dump($data, true);
                $text = is_string($rendered) ? $rendered : '';
            }
            $bt = \Lerd\Collector\backtrace();
            \Lerd\Collector\send([
                'v'     => 1,
                'id'    => \Lerd\Collector\new_id(),
                'ts'    => \Lerd\Collector\ts(),
                'kind'  => 'dump',
                'ctx'   => \Lerd\Collector\context(),
                'src'   => $bt['src'],
                'label' => $label,
                'text'  => $text,
            ]);
        } catch (\Throwable $_) {
            // never throw out of a debug bridge
        }
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
