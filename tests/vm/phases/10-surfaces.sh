#!/usr/bin/env bash
# Phase 10, surfaces: the TUI part, driven through a pty and read back as the
# screen a terminal would show. The dashboard, tray and other surfaces are
# signed off by hand. Runs after phase 6, whose queue worker and Horizon
# project it uses.
source "$(dirname "$0")/../lib.sh"
need_demo
cd "$DEMO_DIR" || exit 1
host=$(site_host "$DEMO_DIR")
name=$(site_name "$DEMO_DIR")
wk=$PROJECTS/vmworkers
queue=lerd-queue-$name
unit_active() { [ "$(systemctl --user is-active "$1" 2>/dev/null)" = active ]; }
# crash_queue: puts the queue worker in the failed state, the only state the
# TUI calls crashed. Its unit restarts on any exit, so a drop-in turns that off.
dropin=$HOME/.config/systemd/user/$queue.service.d/zz-vm-crash.conf
crash_queue() {
	mkdir -p "$(dirname "$dropin")"
	printf '[Service]\nRestart=no\nSuccessExitStatus=\n' >"$dropin"
	systemctl --user daemon-reload
	systemctl --user start "$queue"
	wait_for 20 unit_active "$queue"
	systemctl --user kill -s KILL "$queue"
	wait_for 20 bash -c "[ \"\$(systemctl --user is-active $queue)\" = failed ]"
}
uncrash_queue() {
	rm -f "$dropin"
	systemctl --user daemon-reload
}

if ! need_pyte; then
	todo "10.36 the TUI items" "python3 pyte is not installable here"
	exit 0
fi

# palette <query>: ctrl+p with a query typed, as the screen shows it.
palette() { tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:$1" "sleep:1"; }
export -f tui_screen palette

check_out "10.36 [partial] the sidebar lists the pages" 'Dashboard.*Databases.*PHP & Node.*Settings' \
	bash -c 'tui_screen 140 45 "wait:SERVICES" | tr "\n" " "'
check_out "10.36 [partial] sites and services carry running/total counts" 'SITES +[0-9]+/[0-9]+.*SERVICES +[0-9]+/[0-9]+' \
	bash -c 'tui_screen 140 45 "wait:SERVICES" | tr "\n" " "'
check_out "10.36 [partial] dns, nginx and the watcher sit at the foot with their state" 'dns +resolving.*nginx +running.*watcher +running' \
	bash -c 'tui_screen 140 45 "wait:watcher" | tr "\n" " "'
lerd workspace add vmws </dev/null >/dev/null 2>&1
check "demo joins a workspace" lerd workspace assign "$name" vmws
crash_queue
check_out "10.37 a folded workspace still shows its crashed worker" '▸ vmws +✖ 1' \
	tui_screen 140 45 "wait:vmws" "until:▌ *▾ vmws:j" "keys:\\r" "wait:▸ vmws"

systemctl --user stop lerd-watcher
check_out "10.38 a stopped watcher is listed under NEEDS ATTENTION" 'The watcher is stopped' \
	tui_screen 140 45 "wait:NEEDS ATTENTION"
tui_screen 140 45 "wait:The watcher is stopped" "keys:\\t" "keys:r" "wait:Everything is running" >/dev/null
check "10.38 r starts lerd and the watcher runs again" wait_for 30 systemctl --user is-active --quiet lerd-watcher
check_out "10.38 the dashboard returns to everything running" 'Everything is running' \
	tui_screen 140 45 "wait:Everything is running"
# The watcher's self-heal restarts a failed worker, so crash it right before looking.
crash_queue
check_out "10.39 a crashed worker gets a card naming it" "$host.*worker queue crashed" \
	bash -c 'tui_screen 140 45 "wait:worker queue crashed" | tr "\n" " "'
# The unit stays failed once the drop-in is gone, and a restart then sticks.
uncrash_queue
tui_screen 140 45 "wait:worker queue crashed" "keys:\\t" "keys:r" "sleep:3" >/dev/null
check "10.39 r on the card restarts the worker" wait_for 30 unit_active "$queue"
crash_queue
uncrash_queue
tui_screen 140 45 "wait:worker queue crashed" "keys:H" "sleep:3" >/dev/null
check "10.39 H heals every crashed worker" wait_for 30 unit_active "$queue"
rmdir "$(dirname "$dropin")" 2>/dev/null
lerd workspace assign "$name" none </dev/null >/dev/null 2>&1
lerd workspace rm vmws </dev/null >/dev/null 2>&1
unit_file() { echo "--- $1: $queue unit file $(test -f "$HOME/.config/systemd/user/$queue.service" && echo present || echo MISSING), $(systemctl --user is-active "$queue")"; }
unit_file "after 10.39"

site=$(tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:$host" "sleep:1" "keys:\\r" "wait:Doctor")
echo "$site"
check_out "10.40 [partial] the site header carries its URL and PHP version" "https?://$host.*php [0-9]" bash -c "tr '\n' ' ' <<<\"\$1\"" _ "$site"
check_out "10.40 [partial] the site has its five tabs" 'Overview +Logs +Env +Debug +Doctor' echo "$site"
check_out "10.40 [partial] 3 switches to the Env tab" 'APP_URL' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:$host" "sleep:1" "keys:\\r" "wait:Doctor" "keys:3" "wait:APP_URL"
# Each control through its ctrl+p entry, which runs the same action as the row.
run_entry() { tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:$1" "sleep:1" "keys:\\r" "sleep:${2:-4}" >/dev/null; }
# switch_runtime: the switch runs as the TUI's child and can build an image for
# minutes, so the TUI stays open until it reports the command done; quitting
# earlier would kill it halfway.
switch_runtime() { TUI_WAIT=1200 tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:switch runtime $host" "sleep:1" "keys:\\r" "wait:[✓✖] lerd runtime" >/dev/null; }
run_entry "toggle keep awake $host"
check_out "10.41 [partial] keep awake pins the site" "$name.*pin" bash -c 'lerd idle status | tr "\n" " "'
run_entry "toggle keep awake $host"
check_not "10.41 [partial] and unpins it again" "$name.*pin" bash -c 'lerd idle status | tr "\n" " "'
whost=$(site_host "$wk")
# site_view <domain>: the site's own view, as the TUI draws it.
site_view() { tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:$1" "sleep:1" "keys:\\r" "wait:Doctor" "sleep:2"; }
export -f site_view
run_entry "toggle horizon reload $whost"
check_out "10.41 [partial] Horizon reload turns on" '● Reload horizon +on' site_view "$whost"
run_entry "toggle horizon reload $whost"
check_out "10.41 [partial] and off again" '○ Reload horizon +off' site_view "$whost"
switch_runtime
check_out "10.41 [partial] the runtime switches to FrankenPHP" 'runtime frankenphp' site_view "$host"
expect_200 "10.41 [partial]" "https://$host"
switch_runtime
check_not "10.41 [partial] and back to php-fpm" 'runtime frankenphp' site_view "$host"
expect_200 "10.41 [partial]" "https://$host"
# xdg-open stand-in: records what the TUI asked to open.
fake=$HOME/lerd-vm-fakebin
mkdir -p "$fake" && printf '#!/bin/sh\necho "$@" >>%s/opened\n' "$fake" >"$fake/xdg-open" && chmod +x "$fake/xdg-open" && rm -f "$HOME/lerd-vm-fakebin/opened"
PATH=$fake:$PATH tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:open folder $host" "sleep:1" "keys:\\r" "sleep:2" >/dev/null
check_out "10.41 [partial] open folder hands the site's path to the desktop" "$DEMO_DIR" cat "$fake/opened"
check_out "10.41 [partial] new worktree opens the command prompt prefilled" 'lerd worktree add' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:new worktree $host" "sleep:1" "keys:\\r" "wait:worktree add"
unit_file "after 10.41"
check_out "10.41 [partial] each worker on the Overview names its unit" "lerd-queue-$name" site_view "$host"
todo "10.41 the Stripe listener toggle" "needs a Stripe secret, which the test guests do not have"

db=$(grep -m1 '^DB_DATABASE=' .env | cut -d= -f2)
dbs=$(tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:go to databases" "sleep:1" "keys:\\r" "wait:$db")
echo "$dbs"
check_out "10.42 [partial] Databases lists the site's database" "$db" echo "$dbs"
# A list row carries a size; the twin may only appear in the detail panel.
check_not "10.42 [partial] its _testing twin is folded into the same row" "(▸ |  )${db}_testing +[0-9.]+ ?[KMG]?B" echo "$dbs"

check_out "10.42 [partial] a service names its systemd unit" 'unit lerd-mysql' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:open service mysql" "sleep:1" "keys:\\r" "wait:unit lerd-mysql"
check_out "10.42 [partial] Update service from ctrl+p runs" 'lerd service update redis' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:update service redis" "sleep:1" "keys:\\r" "wait:lerd service update redis"
check_out "10.43 [partial] words match in any order" "Show Logs.*$host" bash -c "palette 'logs $host' | tr '\n' ' '"
check_out "10.43 [partial] the palette hands over to the lerd command prompt" 'Run lerd command' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:run a lerd command" "sleep:1" "keys:\\r" "wait:Run lerd command"
# Control: the same read finds a verb the palette does offer.
check_out "10.44 [partial] the palette read finds an action it offers" '\bRestart\b' bash -c 'palette restart | grep -v "›"'
for verb in remove drop unlink restore uninstall purge reset; do
	# The input line echoes the query, so only the entries below it are read.
	check_not "10.44 [partial] ctrl+p offers nothing for $verb" '\b(Remove|Drop|Unlink|Restore|Uninstall|Purge|Reset)\b' bash -c 'palette "$1" | grep -v "›"' _ "$verb"
done

check_not "10.45 below 96 columns the sidebar folds away" 'PHP & Node' tui_screen 90 40 "wait:Everything is running|NEEDS ATTENTION"
check_out "10.45 \\ opens the sidebar over the main area" 'PHP & Node' tui_screen 90 40 "wait:Everything is running|NEEDS ATTENTION" "keys:\\\\" "wait:PHP & Node"
check_out "10.45 tab on a narrow terminal shows the sidebar" 'PHP & Node' tui_screen 90 40 "wait:Everything is running|NEEDS ATTENTION" "keys:\\t" "wait:PHP & Node"
check_out "10.45 below 60x12 only terminal too small is drawn" 'terminal too small' tui_screen 50 10 "sleep:2"
todo "10.46 colours follow the terminal's palette" "needs a real terminal with a light and a dark profile"
check_out "10.47 [partial] an action raises a toast" 'lerd service restart redis' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:restart service redis" "sleep:1" "keys:\\r" "wait:lerd service restart redis"
# A request that logs, for the debug lenses.
cp routes/web.php /tmp/lerd-vm-web.php
cat >>routes/web.php <<'PHP'
Route::get('/vm-log', function () { \Log::warning('vm-log-probe'); return 'ok'; });
PHP
lerd dump on </dev/null >/dev/null 2>&1
wait_for 30 bash -c "[ \"\$(curl -ks -o /dev/null -w '%{http_code}' 'https://$host/vm-log')\" = 200 ]"
dbg() { tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:go to debug window" "sleep:1" "keys:\\r" "wait:Dumps" "sleep:1" "keys:$1" "sleep:2"; }
export -f dbg
( sleep 6; curl -ks -o /dev/null "https://$host/vm-log" ) &
check_out "10.47 [partial] the Logs lens shows a request's log entry" 'vm-log-probe' dbg '[[['
todo "10.47 the Exceptions lens" "needs Sentry wired into the app's exception handler (sentry:publish), the debug bridge's own setup"
wait
cp /tmp/lerd-vm-web.php routes/web.php
lerd dump off </dev/null >/dev/null 2>&1
todo "10.47 the Messages lens and the SPX hot function" "needs a notification channel and a slow profiled route"

# --- request linking: 10.91-10.93
# Each request names the id its debug events carry, nginx logs it beside the
# request, SPX writes it on the capture, and lerd-ui keeps it over a restart.
ui() { curl -s --max-time 10 -H 'X-Lerd-CSRF: 1' "$@"; }
rid_of() { curl -ks -D - -o /dev/null --max-time 30 "https://$host/" "$@" | tr -d '\r' | awk 'tolower($1)=="x-lerd-rid:" {print $2}'; }
# recent_since <ms> <python test on r>: a recent row newer than ms passes the test.
recent_since() { ui "http://127.0.0.1:7073/api/sites/$host/analytics?range=1h" | python3 -c "import json,sys; rows=[r for r in json.load(sys.stdin)['recent'] if r['at_millis']>$1]; sys.exit(0 if rows and $2 else 1)"; }
fpms() { podman ps --format '{{.Names}}' | grep -E '^lerd-php[0-9]+-fpm$'; }
spx_dir=$HOME/.local/share/lerd/spx
started=$(($(date +%s) * 1000))

lerd dump on </dev/null >/dev/null 2>&1
rid=$(rid_of)
check_out "10.91 a PHP response names its request in X-Lerd-Rid" '^[0-9a-f]{8,64}$' echo "$rid"
check "10.91 the request's captured events carry that id" wait_for 15 bash -c "curl -s 'http://127.0.0.1:7073/api/dumps?rid=$rid' | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin) else 1)'"
check "10.91 its Recent requests row carries the id and a clean path" wait_for 30 recent_since "$started" "any(r.get('rid')=='$rid' and r['uri']=='/' for r in rows)"
lerd dump off </dev/null >/dev/null 2>&1
off=$(($(date +%s) * 1000))
curl -ks -o /dev/null --max-time 30 "https://$host/"
check "10.91 with capture off the path stays clean" wait_for 30 recent_since "$off" "all('|' not in r['uri'] and not r.get('rid') for r in rows)"

lerd dump on </dev/null >/dev/null 2>&1
# SPX ships in the FPM image only, so the site has to be served by FPM here.
lerd runtime fpm </dev/null >/dev/null 2>&1
key=$(for c in $(fpms); do podman exec "$c" php -r 'echo ini_get("spx.http_key");' 2>/dev/null && break; done)
since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
lerd profile on </dev/null >/dev/null 2>&1
# Arming reloads nginx, which drains the old workers rather than swapping in
# place, so the first requests can still miss the profiler: ask until one lands.
profiled_rid() {
	local r
	for _ in $(seq 10); do
		r=$(rid_of)
		wait_for 5 bash -c "grep -lq 'lerd-rid:$r' $spx_dir/*.json" && { echo "$r"; return 0; }
	done
	return 1
}
rid2=$(profiled_rid)
check_out "10.92 its SPX capture carries lerd-rid:<id>" '^[0-9a-f]{8,64}$' echo "$rid2"
check_out "10.92 a wrong SPX key still answers 200" '^200$' curl -ks -o /dev/null -w '%{http_code}' --max-time 30 -H 'Cookie: SPX_ENABLED=1; SPX_KEY=wrong' "https://$host/"
lerd profile off </dev/null >/dev/null 2>&1
check_out "10.92 SPX cookies with the profiler off still answer 200" '^200$' curl -ks -o /dev/null -w '%{http_code}' --max-time 30 -H "Cookie: SPX_ENABLED=1; SPX_KEY=$key; SPX_REPORT=full" "https://$host/"
check_not "10.92 the FPM log shows no segfault" 'signal 11|SIGSEGV|segfault' bash -c "for c in \$(podman ps --format '{{.Names}}' | grep -E '^lerd-php[0-9]+-fpm$'); do podman logs --since '$since' \"\$c\" 2>&1; done"

systemctl --user restart lerd-ui
wait_for 30 curl -sf -o /dev/null http://127.0.0.1:7073/api/dumps/status
check_out "10.93 the debug buffer is saved owner-only" '^600$' stat -c %a "$HOME/.local/share/lerd/dumps-buffer.json"
check "10.93 a request's debug events survive a lerd-ui restart" bash -c "curl -s 'http://127.0.0.1:7073/api/dumps?rid=$rid2' | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin) else 1)'"
# The watcher writes in batches, so the row has to be on disk before removing it can find it.
wait_for 30 recent_since "$started" "any(r.get('rid')=='$rid2' for r in rows)"
ui -X POST -H 'Content-Type: application/json' -d '{"route":"GET /"}' "http://127.0.0.1:7073/api/sites/$host/analytics/remove" >/dev/null
check "10.93 removing the route drops its debug events" bash -c "curl -s 'http://127.0.0.1:7073/api/dumps?rid=$rid2' | python3 -c 'import json,sys; sys.exit(1 if json.load(sys.stdin) else 0)'"
check "10.93 removing the route drops its SPX capture" bash -c "! grep -lq 'lerd-rid:$rid2' $spx_dir/*.json"
lerd dump off </dev/null >/dev/null 2>&1
# --- end request linking

check_out "10.48 the shell drop-in runs in the site's container and comes back" 'shell .* exited' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:open shell $host" "sleep:1" "keys:\\r" "sleep:4" "keys:echo VMSHELL\$((40+2))\\r" "wait:VMSHELL42" "keys:exit\\r" "wait:exited"
check_out "10.48 a service with a dashboard shows its URL" 'http://localhost:8025' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:open service mailpit" "sleep:1" "keys:\\r" "wait:localhost:8025"
rm -f "$HOME/lerd-vm-fakebin/opened"
PATH=$fake:$PATH tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:open service dashboard mailpit" "sleep:1" "keys:\\r" "sleep:3" >/dev/null
check_out "10.48 opening it hands the dashboard URL to the browser" 'http://localhost:8025' cat "$fake/opened"
rm -rf "$HOME/lerd-vm-fakebin"
