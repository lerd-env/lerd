<?php
// Drops OPcache entries in the php-fpm pool of this container, where the cache
// lives; the CLI has a cache of its own. Run from the CLI with a mode, it asks
// php-fpm over FastCGI to run this same file, which then does the work:
//   app    every cached script outside a vendor/ directory, so edited code,
//          compiled templates and generated caches are compiled again while
//          the dependencies stay cached
//   reset  the whole cache, after the dependencies themselves changed

if (PHP_SAPI !== 'cli') {
    $mode = isset($_SERVER['LERD_OPCACHE']) ? $_SERVER['LERD_OPCACHE'] : '';
    if (!function_exists('opcache_get_status')) {
        echo 'ok';
        return;
    }
    if ($mode === 'reset') {
        opcache_reset();
    } elseif ($mode === 'app') {
        $status = opcache_get_status(true);
        $scripts = is_array($status) && isset($status['scripts']) ? $status['scripts'] : array();
        foreach (array_keys($scripts) as $path) {
            if (strpos($path, '/vendor/') === false) {
                opcache_invalidate($path, true);
            }
        }
    }
    echo 'ok';
    return;
}

$mode = isset($argv[1]) ? $argv[1] : 'app';
$params = array(
    'GATEWAY_INTERFACE' => 'FastCGI/1.0',
    'REQUEST_METHOD' => 'GET',
    'SCRIPT_FILENAME' => __FILE__,
    'SCRIPT_NAME' => '/' . basename(__FILE__),
    'SERVER_PROTOCOL' => 'HTTP/1.1',
    'LERD_OPCACHE' => $mode,
);

function lerd_fcgi_record($type, $content)
{
    return pack('CCnnCx', 1, $type, 1, strlen($content), 0) . $content;
}

function lerd_fcgi_length($n)
{
    return $n < 128 ? chr($n) : pack('N', $n | 0x80000000);
}

$body = '';
foreach ($params as $k => $v) {
    $body .= lerd_fcgi_length(strlen($k)) . lerd_fcgi_length(strlen($v)) . $k . $v;
}
$request = lerd_fcgi_record(1, pack('nCx5', 1, 0))
    . lerd_fcgi_record(4, $body) . lerd_fcgi_record(4, '')
    . lerd_fcgi_record(5, '');

$sock = @stream_socket_client('tcp://127.0.0.1:9000', $errno, $errstr, 5);
if (!$sock) {
    fwrite(STDERR, "opcache-invalidate: cannot reach php-fpm: $errstr\n");
    exit(1);
}
stream_set_timeout($sock, 10);
fwrite($sock, $request);

$out = '';
while (!feof($sock)) {
    $header = fread($sock, 8);
    if (strlen($header) < 8) {
        break;
    }
    $h = unpack('Cversion/Ctype/nid/nlength/Cpadding', $header);
    $content = $h['length'] > 0 ? stream_get_contents($sock, $h['length']) : '';
    if ($h['padding'] > 0) {
        fread($sock, $h['padding']);
    }
    if ($h['type'] === 6) {
        $out .= $content;
    } elseif ($h['type'] === 3) {
        break;
    }
}
fclose($sock);

if (substr($out, -2) !== 'ok') {
    fwrite(STDERR, "opcache-invalidate: php-fpm did not confirm: " . trim($out) . "\n");
    exit(1);
}
