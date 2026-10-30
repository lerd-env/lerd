#!/usr/bin/env bash
# Phase 4, PHP versions. The per-version checks run on phpsite, a plain PHP
# site, so a framework's composer platform constraints do not decide which
# PHP versions it can be isolated to. Legacy 7.4 and prerelease 8.6 are tier 3.
source "$(dirname "$0")/../lib.sh"
need_demo

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
driver=$(ls /usr/lib/x86_64-linux-gnu/odbc/libsqlite3odbc.so /usr/lib64/libsqlite3odbc.so /usr/lib/libsqlite3odbc.so 2>/dev/null | head -1)
# lerd refuses a driver in /usr/lib (Arch's path), as mounting it would shadow the container's own.
if [[ $driver == /usr/lib/*.so && $driver != /usr/lib/*/* ]]; then
	mkdir -p ~/odbc-drivers && cp -L "$driver" ~/odbc-drivers/ && driver=$HOME/odbc-drivers/$(basename "$driver")
fi
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
check_out "4.19 tinker and logs run on the version the site is served with" "(^| )$served\\..*logs=ok" bash -c "cd '$DEMO_DIR' && lerd artisan tinker --execute='echo PHP_VERSION;' | tail -1; lerd logs -n 5 >/dev/null 2>&1 && echo logs=ok"
cp /tmp/lerd-vm-demo-lerd.yaml "$DEMO_DIR/.lerd.yaml"
(cd "$DEMO_DIR" && lerd link </dev/null >/dev/null 2>&1)

base=$(podman images --format '{{.Repository}}:{{.Tag}}' | grep -E 'php.*(base|fpm).*8\.3' | grep -v localhost | head -1)
[ -n "$base" ] && podman rmi -f "$base" >/dev/null 2>&1
check_out "4.20 php:rebuild names the base image and its size before pulling" 'will download 1 image \(~' lerd php:rebuild 8.3
check_out "4.21 [partial] lerd fetch names built versions nothing serves and points at php:rebuild" 'php:rebuild|runtime|up to date|already' lerd fetch
lerd isolate 8.4 </dev/null >/dev/null 2>&1
expect_200 "$url"

# env_provider. phpsite and envsite, both on 8.4 so one FPM container serves
# them, give LERD_VM_SHARED different values; demo's queue worker covers the
# worker channel. The provider is a script, so rotating it needs no re-approval.
provided() { printf '%s/lerd/env/%s.env' "$XDG_RUNTIME_DIR" "$1"; }
env_probe='<?php echo getenv("LERD_VM_SHARED"), "|", getenv("LERD_VM_ONLY_ENVSITE"), "|", substr_count((string) getenv("LERD_VM_PEM"), "\n");'
cat >"$site/lerd-vm-provider.sh" <<'SH'
cat <<'EOF'
LERD_VM_SHARED=from-phpsite
LERD_VM_PEM="-----BEGIN KEY-----
abc
-----END KEY-----"
EOF
SH
echo "$env_probe" >"$site/public/env.php"
echo 'env_provider: sh lerd-vm-provider.sh' >>"$site/.lerd.yaml"
envsite="$PROJECTS/envsite"
mkdir -p "$envsite/public"
printf '%s\n' 'printf "LERD_VM_SHARED=from-envsite\nLERD_VM_ONLY_ENVSITE=yes\n"' >"$envsite/lerd-vm-provider.sh"
# index.php makes lerd serve public/, where the probe is.
echo "$env_probe" >"$envsite/public/env.php"
cp "$envsite/public/env.php" "$envsite/public/index.php"
echo 'env_provider: sh lerd-vm-provider.sh' >"$envsite/.lerd.yaml"
(cd "$envsite" && lerd link && lerd isolate 8.4) </dev/null >/dev/null 2>&1
ename=$(site_name "$envsite")
lerd secure "$ename" </dev/null >/dev/null 2>&1
eurl="$scheme://$(site_host "$envsite")"

# A plain PHP site has no framework, so lerd env ends in an error after the
# provider has run; the checks read the file, not the exit status. The refusal
# is checked on envsite, linked fresh each run, since approvals are remembered.
check_out "4.22 lerd env with no terminal refuses the provider and names --yes" 'lerd env --yes' bash -c "cd '$envsite' && lerd env"
(cd "$envsite" && lerd env --yes) </dev/null >/dev/null 2>&1
lerd env --yes </dev/null >/dev/null 2>&1
check_out "4.22 lerd env --yes writes the provided file 0600 and nothing reaches .env" '^600 .*clean' bash -c "stat -c '%a %n' '$(provided "$name")' && ! grep -rqs from-phpsite '$site/.env' && echo clean"
check_out "4.23 https serves the provided values and a multi-line value whole" '^from-phpsite\|\|2$' curl -sk "$url/env.php"
check_out "4.23 lerd php in the site sees them" '^from-phpsite\|\|2$' lerd php public/env.php
check_out "4.23 an exec naming another site loads nothing" '^\|\|0$' podman exec -w "$site" --env "LERD_SITE=$ename" lerd-php84-fpm php public/env.php

# One echo per response naming the site nginx routed it to: concurrent curls
# share the output file, so a body written in pieces would interleave.
bleed_probe='<?php echo $_SERVER["LERD_SITE"], "|", getenv("LERD_VM_SHARED"), "|", getenv("LERD_VM_ONLY_ENVSITE"), "\n";'
echo "$bleed_probe" >"$site/public/bleed.php"
echo "$bleed_probe" >"$envsite/public/bleed.php"
for _ in $(seq 200); do echo "$url/bleed.php"; echo "$eurl/bleed.php"; done | xargs -P 16 -n 1 curl -sk >/tmp/lerd-vm-bleed.txt
check_out "4.24 400 alternating requests across two sites never cross" '^crossed=0 total=400$' bash -c "echo crossed=\$(grep -Evxc '$name\|from-phpsite\||$ename\|from-envsite\|yes' /tmp/lerd-vm-bleed.txt) total=\$(wc -l </tmp/lerd-vm-bleed.txt)"
rm -f /tmp/lerd-vm-bleed.txt "$site/public/bleed.php"

sed -i 's/^LERD_VM_SHARED=from-phpsite$/LERD_VM_SHARED=rotated/' "$site/lerd-vm-provider.sh"
lerd env --yes </dev/null >/dev/null 2>&1
check_out "4.25 a rotated value reaches the next request" '^rotated\|' curl -sk "$url/env.php"

# A command of its own for the worker to run, since a queued closure cannot be
# serialised from tinker's eval'd code. The worker loads the provided env when
# it boots, so it is restarted after lerd env.
dname=$(site_name "$DEMO_DIR")
queue_was=$(systemctl --user is-active "lerd-queue-$dname" 2>/dev/null)
cp -f "$DEMO_DIR/.lerd.yaml" /tmp/lerd-vm-demo-lerd.yaml
echo "env_provider: printf 'LERD_VM_DEMO=from-demo\\n'" >>"$DEMO_DIR/.lerd.yaml"
mkdir -p "$DEMO_DIR/app/Console/Commands"
cat >"$DEMO_DIR/app/Console/Commands/LerdVmEnv.php" <<'PHP'
<?php

namespace App\Console\Commands;

use Illuminate\Console\Command;

class LerdVmEnv extends Command
{
    protected $signature = 'lerd-vm:env';

    public function handle(): void
    {
        file_put_contents(storage_path('lerd-vm-env'), (string) getenv('LERD_VM_DEMO'));
    }
}
PHP
rm -f "$DEMO_DIR/storage/lerd-vm-env"
(cd "$DEMO_DIR" && lerd env --yes; lerd queue:stop; lerd queue:start) </dev/null >/dev/null 2>&1
(cd "$DEMO_DIR" && lerd artisan tinker --execute='Illuminate\Support\Facades\Artisan::queue("lerd-vm:env");') </dev/null >/dev/null 2>&1
wait_for 60 test -s "$DEMO_DIR/storage/lerd-vm-env"
check_out "4.26 the queue worker's job sees the demo's provided value" '^from-demo$' cat "$DEMO_DIR/storage/lerd-vm-env"
rm -f "$DEMO_DIR/storage/lerd-vm-env" "$DEMO_DIR/app/Console/Commands/LerdVmEnv.php"
cp /tmp/lerd-vm-demo-lerd.yaml "$DEMO_DIR/.lerd.yaml"
(cd "$DEMO_DIR" && lerd env; lerd queue:stop; [ "$queue_was" = active ] && lerd queue:start) </dev/null >/dev/null 2>&1

(cd "$envsite" && lerd unlink) </dev/null >/dev/null 2>&1
check "4.27 unlinking a site removes its provided file" test ! -e "$(provided "$ename")"
rm -rf "$envsite"
sed -i '/^env_provider:/d' "$site/.lerd.yaml"
lerd env </dev/null >/dev/null 2>&1
rm -f "$site/lerd-vm-provider.sh" "$site/public/env.php"
expect_200 "$url"

# 4.28-4.33: a command cut short takes what it started with it. Each payload
# runs `sleep <n>` under a number of its own, so a count of those is the check.
eg_ver=$(cd "$DEMO_DIR" && lerd php -r 'echo PHP_MAJOR_VERSION.PHP_MINOR_VERSION;' 2>/dev/null)
eg_c=lerd-php${eg_ver}-fpm
eg_n=900
eg_next() { eg_n=$((eg_n + 1)); }
eg_cnt() { podman exec "$eg_c" sh -c "ps -eo args | grep -c '^sleep $1\$'"; }
eg_gone() { [ "$(eg_cnt "$1")" = 0 ]; }
eg_has() { [ "$(eg_cnt "$1")" -ge "$2" ]; }
eg_up() { wait_for 40 eg_has "$1" "$2"; }
eg_shape() { # eg_shape <n> own|ignore|detach: a PHP file running that tree
	local cmd
	case $2 in
	own) cmd="setsid sleep $1 & sleep $1" ;;
	ignore) cmd="setsid sh -c 'trap \"\" TERM; exec sleep $1' & sh -c 'trap \"\" TERM; exec sleep $1'" ;;
	detach) cmd="setsid sleep $1 >/dev/null 2>&1 &" ;;
	esac
	printf "<?php passthru(<<<'SH'\n%s\nSH);\n" "$cmd" >"$DEMO_DIR/storage/lerd-vm-eg-$1.php"
}
eg_start() { # eg_start php|artisan|vendor <n>: the lerd pid
	local rel="storage/lerd-vm-eg-$2.php"
	cd "$DEMO_DIR" || return
	case $1 in
	php) setsid lerd php "$rel" </dev/null >/dev/null 2>&1 & ;;
	artisan) setsid lerd artisan tinker --execute="require base_path('$rel');" </dev/null >/dev/null 2>&1 & ;;
	vendor) setsid lerd lerd-vm-egrun "$rel" </dev/null >/dev/null 2>&1 & ;;
	esac
	echo $!
	cd - >/dev/null || return
}
printf '#!/bin/sh\nexec php "$@"\n' >"$DEMO_DIR/vendor/bin/lerd-vm-egrun"
chmod +x "$DEMO_DIR/vendor/bin/lerd-vm-egrun"

eg_ok=1
for e in php artisan vendor; do
	for s in TERM INT HUP; do
		for k in own ignore; do
			eg_next; eg_shape $eg_n $k
			pid=$(eg_start $e $eg_n)
			eg_up $eg_n 2 || { eg_ok=0; echo "$e $s $k never started"; continue; }
			kill -$s "$pid"
			wait_for 20 eval "! kill -0 $pid 2>/dev/null"
			wait_for 15 eg_gone $eg_n || { eg_ok=0; echo "$e SIG$s $k left $(eg_cnt $eg_n)"; }
		done
	done
done
check "4.28 php, artisan and vendor/bin stopped with TERM, INT or HUP leave nothing in the container" test $eg_ok = 1

eg_next; eg_a=$eg_n; eg_shape $eg_a own; eg_next; eg_b=$eg_n; eg_shape $eg_b own
pa=$(eg_start php $eg_a); pb=$(eg_start php $eg_b)
eg_up $eg_a 2 && eg_up $eg_b 2
kill -TERM "$pa"
wait_for 15 eg_gone $eg_a
check_out "4.29 stopping one of two runs leaves the other running" '^2$' eg_cnt $eg_b
kill -TERM "$pb"
eg_next; eg_k=$eg_n; eg_shape $eg_k own
pk=$(eg_start artisan $eg_k); eg_up $eg_k 2; kill -9 "$pk"
eg_next; eg_d=$eg_n; eg_shape $eg_d detach
(cd "$DEMO_DIR" && lerd php "storage/lerd-vm-eg-$eg_d.php") </dev/null >/dev/null 2>&1
check_out "4.29 a run killed with kill -9 is still running right after" '^2$' eg_cnt $eg_k
check "4.29 lerd-watcher sweeps a run killed with kill -9 within a minute" wait_for 80 eg_gone $eg_k
check "4.29 lerd-watcher sweeps a background child a finished run left within a minute" wait_for 80 eg_gone $eg_d

for how in ctrlc close; do
	eg_next; eg_shape $eg_n own
	python3 - "$DEMO_DIR" "$eg_n" "$how" <<'PY' &
import os, pty, sys, time
site, n, how = sys.argv[1:4]
pid, fd = pty.fork()
if pid == 0:
    os.chdir(site)
    os.execvp("lerd", ["lerd", "php", f"storage/lerd-vm-eg-{n}.php"])
time.sleep(6)
if how == "ctrlc":
    os.write(fd, b"\x03")
    time.sleep(3)
os.close(fd)
os.waitpid(pid, 0)
PY
	eg_up $eg_n 1
	wait
	check "4.30 $how on a terminal stops the run in the container" wait_for 80 eg_gone $eg_n
done

eg_next; eg_shape $eg_n own
eg_fifo=$(mktemp -u); mkfifo "$eg_fifo"
(cd "$DEMO_DIR" && setsid lerd mcp <"$eg_fifo" >/dev/null 2>&1) &
exec 7>"$eg_fifo"
wait_for 10 pgrep -f '^lerd mcp'
eg_mcp=$(pgrep -nf '^lerd mcp')
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"lerd-vm","version":"1"}}}' \
	"{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"exec\",\"arguments\":{\"action\":\"artisan\",\"path\":\"$DEMO_DIR\",\"args\":[\"tinker\",\"--execute=require base_path('storage/lerd-vm-eg-$eg_n.php');\"]}}}" >&7
eg_up $eg_n 2
printf '%s\n' '{"jsonrpc":"2.0","id":3,"method":"tools/list"}' >&7
exec 7>&-
check "4.31 closing an MCP session mid-call stops lerd mcp" wait_for 20 eval "! kill -0 $eg_mcp 2>/dev/null"
check "4.31 closing an MCP session mid-call stops the call in the container" wait_for 15 eg_gone $eg_n
rm -f "$eg_fifo"

# The dashboard refuses a request without its CSRF header, so a check that the
# run is gone only means something once the run is seen starting.
eg_tinker() { curl -s --max-time "$2" -X POST -H 'Content-Type: application/json' -H 'X-Lerd-CSRF: 1' \
	-d "{\"code\":\"require base_path('storage/lerd-vm-eg-$1.php');\"}" "http://127.0.0.1:7073/api/sites/$(site_host "$DEMO_DIR")/tinker" >/dev/null 2>&1 & }
eg_next; eg_shape $eg_n own; eg_tinker $eg_n 6
check "4.32 the dashboard's tinker starts the run" eg_up $eg_n 2
check "4.32 a browser leaving the dashboard's tinker stops the run" wait_for 20 eg_gone $eg_n
eg_next; eg_shape $eg_n own; eg_tinker $eg_n 60
check "4.32 the dashboard's tinker starts the run it will time out" eg_up $eg_n 2
check "4.32 the dashboard's tinker timeout stops the run" wait_for 45 eg_gone $eg_n
wait

# lerd sail runs vendor/bin/sail on the host, the one host path Linux reaches.
eg_sail="$DEMO_DIR/vendor/bin/sail"
[ -e "$eg_sail" ] && mv "$eg_sail" "$eg_sail.lerd-vm"
printf '#!/bin/sh\nsleep "$1" & sleep "$1"\n' >"$eg_sail"
chmod +x "$eg_sail"
eg_hcnt() { pgrep -fxc "sleep $1"; }
eg_hgone() { [ "$(eg_hcnt "$1")" = 0 ]; }
eg_hup() { [ "$(eg_hcnt "$1")" -ge 2 ]; }
eg_next; (cd "$DEMO_DIR" && setsid lerd sail $eg_n </dev/null >/dev/null 2>&1 &)
wait_for 20 eg_hup $eg_n
pkill -TERM -f "^lerd sail $eg_n\$"
check "4.33 a host command stopped with TERM takes its group with it" wait_for 15 eg_hgone $eg_n
eg_next; (cd "$DEMO_DIR" && setsid lerd sail $eg_n </dev/null >/dev/null 2>&1 &)
wait_for 20 eg_hup $eg_n
pkill -KILL -f "^lerd sail $eg_n\$"
check "4.33 lerd-watcher sweeps the group of a host command killed with kill -9" wait_for 80 eg_hgone $eg_n
eg_next
eg_job=$(python3 - "$DEMO_DIR" "$eg_n" <<'PY'
import os, pty, select, subprocess, sys, time
site, n = sys.argv[1:3]
out = b""
def drain(fd, secs):
    global out
    end = time.time() + secs
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.2)
        if r:
            try:
                out += os.read(fd, 4096)
            except OSError:
                return
def states():
    r = subprocess.run(["ps", "-eo", "stat=,args="], capture_output=True, text=True).stdout
    return "".join(l.split()[0][0] for l in r.splitlines() if l.split(None, 1)[1:] == [f"sleep {n}"])
pid, fd = pty.fork()
if pid == 0:
    os.chdir(site)
    os.execvp("bash", ["bash", "--norc", "-i"])
drain(fd, 1); os.write(fd, f"lerd sail {n}\n".encode()); drain(fd, 5)
os.write(fd, b"\x1a"); drain(fd, 3)
stopped = states()
os.write(fd, f"echo prompt-{n}\n".encode()); drain(fd, 2)
prompt = out.count(f"prompt-{n}".encode()) >= 2
os.write(fd, b"fg\n"); drain(fd, 3)
resumed = states()
os.write(fd, b"\x03"); drain(fd, 3)
os.write(fd, f"echo alive-{n}\n".encode()); drain(fd, 2)
alive = out.count(f"alive-{n}".encode()) >= 2
os.write(fd, b"exit\n"); drain(fd, 2)
print(f"stopped={stopped} prompt={prompt} resumed={resumed} alive={alive}")
PY
)
echo "--- job control: $eg_job"
check_out "4.33 Ctrl+Z stops a host command and gives the prompt back" 'stopped=TT prompt=True' echo "$eg_job"
check_out "4.33 fg resumes it and Ctrl+C ends it with the shell still usable" 'resumed=SS alive=True' echo "$eg_job"
check "4.33 nothing of the host command is left after Ctrl+C" wait_for 15 eg_hgone $eg_n
rm -f "$eg_sail"
[ -e "$eg_sail.lerd-vm" ] && mv "$eg_sail.lerd-vm" "$eg_sail"
rm -f "$DEMO_DIR"/storage/lerd-vm-eg-*.php "$DEMO_DIR/vendor/bin/lerd-vm-egrun"
reclaim
