#!/usr/bin/env bash
# Phase 4, PHP versions. The per-version checks run on phpsite, a plain PHP
# site, so a framework's composer platform constraints do not decide which
# PHP versions it can be isolated to. Legacy 7.4 and prerelease 8.6 are tier 3.
source "$(dirname "$0")/../lib.sh"

# pty_run <seconds> <input> <cmd...>: runs a command on a terminal, types the
# input after a second, and prints what the terminal showed.
pty_run() {
	python3 - "$@" <<'PY'
import os, pty, select, sys, time
secs, text = float(sys.argv[1]), sys.argv[2].encode().decode("unicode_escape").encode()
pid, fd = pty.fork()
if pid == 0:
    os.execvp(sys.argv[3], sys.argv[3:])
out, end, sent = b"", time.time() + secs, False
while time.time() < end:
    if not sent and time.time() > end - secs + 1.5:
        os.write(fd, text); sent = True
    r, _, _ = select.select([fd], [], [], 0.3)
    if r:
        try: out += os.read(fd, 65536)
        except OSError: break
try: os.kill(pid, 9)
except ProcessLookupError: pass
sys.stdout.write(out.decode(errors="replace"))
PY
}

site="$PROJECTS/phpsite"
mkdir -p "$site/public"
echo '<?php echo PHP_VERSION;' >"$site/public/index.php"
cd "$site" || exit 1
lerd link </dev/null >/dev/null 2>&1
host=$(site_host "$site")
name=$(site_name "$site")
lerd secure "$name" </dev/null >/dev/null 2>&1
scheme=https
[ "$(tld)" = localhost ] && scheme=http
url="$scheme://$host"

check_out "4.1 lerd php:list shows the installed versions" '8\.[0-9]' lerd php:list
check_out "4.2 lerd use 8.5 switches the global version" '\* *8\.5' bash -c 'lerd use 8.5 >/dev/null 2>&1; lerd php:list'
if lerd php:list 2>/dev/null | grep -q '8\.3'; then
	check_out "4.5 [partial] lerd fetch pulls a prebuilt base" '8\.3|up to date|already' lerd fetch 8.3
else
	check_out "4.5 [partial] lerd fetch pulls a prebuilt base for a version not yet installed" '8\.3' lerd fetch 8.3
fi
check "lerd isolate 8.3" lerd isolate 8.3
check_out "4.3 isolate writes .php-version and .lerd.yaml, re-links, lerd which reports 8.3" 'PHP +8\.3' bash -c 'grep -q "^8.3" .php-version && grep -q "php_version: \"8.3\"" .lerd.yaml && lerd which'
check_out "4.4 https answers 200 on the isolated version" '^8\.3\.' curl -sk "$url"
check "lerd php:rebuild 8.3" lerd php:rebuild 8.3
expect_200 4.6 "$url"
check "lerd xdebug on 8.3 --mode debug" lerd xdebug on 8.3 --mode debug
check_out "lerd xdebug status reflects it" '8\.3.*(on|enabled|debug)' lerd xdebug status
check_out "Xdebug is loaded on 8.3" '^xdebug$' podman exec lerd-php83-fpm php -m
expect_200 4.7 "$url"
check "lerd xdebug off 8.3" lerd xdebug off 8.3
expect_200 4.8 "$url"
check "lerd php:ext add ds" lerd php:ext add ds
check_out "4.9 ds is loaded, and adding redis is refused" '(^| )ds .*already ships' bash -c 'podman exec lerd-php83-fpm php -m | grep -x ds; lerd php:ext add redis 2>&1'
check_not "4.10 lerd php:ext remove ds rebuilds cleanly" '^ds$' bash -c 'lerd php:ext remove ds >/dev/null && podman exec lerd-php83-fpm php -m'

# php:ini opens in $EDITOR; an editor that appends a line stands in for a person.
printf '#!/bin/sh\necho "memory_limit = 321M" >>"$1"\n' >/tmp/lerd-vm-editor.sh
chmod +x /tmp/lerd-vm-editor.sh
EDITOR=/tmp/lerd-vm-editor.sh lerd php:ini shared </dev/null >/dev/null 2>&1
lerd php:rebuild 8.3 </dev/null >/dev/null 2>&1
check_out "4.11 [partial] a php:ini shared edit survives a rebuild" '321M' podman exec lerd-php83-fpm php -r 'echo ini_get("memory_limit");'
printf '#!/bin/sh\nsed -i "/memory_limit = 321M/d" "$1"\n' >/tmp/lerd-vm-editor.sh
EDITOR=/tmp/lerd-vm-editor.sh lerd php:ini shared </dev/null >/dev/null 2>&1
rm -f /tmp/lerd-vm-editor.sh

if [ "${RUN_TIER3:-}" = 1 ]; then
	lerd isolate 7.4 </dev/null >/dev/null 2>&1
	check_out "4.12 a legacy 7.4 site serves on the frozen image" '^7\.4\.' curl -sk "$url"
	lerd isolate 8.4 </dev/null >/dev/null 2>&1
	check_not "a bare lerd fetch leaves 8.6 alone" '8\.6' lerd fetch
	lerd use 8.6 </dev/null >/dev/null 2>&1
	lerd isolate 8.6 </dev/null >/dev/null 2>&1
	check_out "4.13 8.6 is marked a prerelease and serves" 'prerelease.*8\.6\.' bash -c "lerd php:list | tr '\n' ' '; curl -sk '$url'"
	check_out "4.14 8.6 ships redis, imagick and mongodb, no igbinary, pcov or xdebug" '^ok$' bash -c 'm=$(podman exec lerd-php86-fpm php -m); for e in redis imagick mongodb; do grep -qx $e <<<"$m" || exit 1; done; for e in igbinary pcov xdebug; do grep -qx $e <<<"$m" && exit 1; done; echo ok'
	lerd use 8.5 </dev/null >/dev/null 2>&1
else
	skip "4.12 legacy 7.4" "tier 3, set RUN_TIER3=1"
	skip "4.13 prerelease 8.6" "tier 3, set RUN_TIER3=1"
	skip "4.14 the 8.6 extension set" "tier 3, set RUN_TIER3=1"
fi

shell_out=$(cd / && pty_run 8 'php -v\nexit\n' lerd shell 8.5)
check_out "4.15 [partial] lerd shell 8.5 drops into that version's container from anywhere" 'PHP 8\.5' printf '%s' "$shell_out"
check_out "4.16 [partial] lerd php:ports and php:pkg report the version's ports and packages" '.' bash -c 'lerd php:ports 8.5 && lerd php:pkg 8.5'
check "lerd php:ports add 5199" lerd php:ports add 5199
# The FPM container the site is served from right now.
export fpm; fpm=$(lerd which 2>/dev/null | awk '$1 == "PHP" {v=$2; gsub(/\./, "", v); print "lerd-php" v "-fpm"; exit}')
check_out "4.17 the port is published and php:ports remove takes it off" 'added=.*5199.*removed= *$' bash -c 'echo added=$(podman port $fpm | grep 5199); lerd php:ports remove 5199 >/dev/null 2>&1; echo removed=$(podman port $fpm | grep 5199)'

ok=1
for c in $(podman ps --format '{{.Names}}' | grep -E '^lerd-php[0-9]+-fpm$'); do
	podman exec "$c" php -m | grep -qx PDO_ODBC || { ok=0; echo "$c has no PDO_ODBC"; }
done
dpkg -s libsqliteodbc >/dev/null 2>&1 || sudo apt-get install -y -qq libsqliteodbc >/dev/null 2>&1 || sudo dnf install -y -q sqliteodbc >/dev/null 2>&1
driver=$(ls /usr/lib/x86_64-linux-gnu/odbc/libsqlite3odbc.so /usr/lib64/libsqlite3odbc.so 2>/dev/null | head -1)
if [ "$ok" = 1 ] && [ -n "$driver" ] && lerd php:odbc add vmsqlite "$driver" </dev/null && lerd php:odbc list | grep -q vmsqlite && lerd php:odbc remove vmsqlite </dev/null; then
	_pass "4.18 every version has odbc and PDO_ODBC, and a vendor driver registers, lists and unregisters"
else
	_fail "4.18 every version has odbc and PDO_ODBC, and a vendor driver registers, lists and unregisters" "ok=$ok driver=$driver"
fi
expect_200 "$url"

# A site whose .lerd.yaml pins a version this machine lacks is served on what
# it runs with, and tinker and logs use that one too.
cp -f "$DEMO_DIR/.lerd.yaml" /tmp/lerd-vm-demo-lerd.yaml
sed -i '/^php_version:/d' "$DEMO_DIR/.lerd.yaml" && echo 'php_version: "8.1"' >>"$DEMO_DIR/.lerd.yaml"
(cd "$DEMO_DIR" && LERD_OFFLINE=1 lerd link </dev/null >/dev/null 2>&1)
served=$(cd "$DEMO_DIR" && lerd which | awk '$1=="PHP" {print $2}')
check_out "4.19 tinker and logs run on the version the site is served with" "^$served\\..*logs=ok" bash -c "cd '$DEMO_DIR' && lerd artisan tinker --execute='echo PHP_VERSION;' | tail -1; lerd logs -n 5 >/dev/null 2>&1 && echo logs=ok"
cp /tmp/lerd-vm-demo-lerd.yaml "$DEMO_DIR/.lerd.yaml"
(cd "$DEMO_DIR" && lerd link </dev/null >/dev/null 2>&1)

base=$(podman images --format '{{.Repository}}:{{.Tag}}' | grep -E 'php.*(base|fpm).*8\.3' | grep -v localhost | head -1)
[ -n "$base" ] && podman rmi -f "$base" >/dev/null 2>&1
check_out "4.20 php:rebuild names the base image and its size before pulling" 'will download 1 image \(~' lerd php:rebuild 8.3
check_out "4.21 [partial] lerd fetch names built versions nothing serves and points at php:rebuild" 'php:rebuild|runtime|up to date|already' lerd fetch
lerd isolate 8.4 </dev/null >/dev/null 2>&1
expect_200 "$url"
reclaim
