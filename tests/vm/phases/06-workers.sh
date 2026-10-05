#!/usr/bin/env bash
# Phase 6, workers.
source "$(dirname "$0")/../lib.sh"
need_demo
cd "$DEMO_DIR" || exit 1
name=$(site_name "$DEMO_DIR")
scheme=https
[ "$(tld)" = localhost ] && scheme=http
url="$scheme://$(site_host "$DEMO_DIR")"
unit_active() { [ "$(systemctl --user is-active "$1" 2>/dev/null)" = active ]; }
# A phase starts from a working site: bring the schema up to date first.
lerd artisan migrate --force </dev/null >/dev/null 2>&1
jobs_left() { [ "$(lerd artisan tinker --execute='echo DB::table("jobs")->count();' 2>/dev/null | tail -1)" = 0 ]; }

check "lerd queue:start" lerd queue:start
check "a job is dispatched" lerd artisan tinker --execute='Illuminate\Support\Facades\Artisan::queue("inspire");'
check "the queue worker runs it" wait_for 60 jobs_left
check "lerd queue:stop" lerd queue:stop
check "6.1 queue:start runs a dispatched job and queue:stop stops it" bash -c "! systemctl --user is-active --quiet lerd-queue-$name"
check "lerd schedule:start" lerd schedule:start
check "lerd schedule:stop" lerd schedule:stop
check "6.2 schedule:start / stop" bash -c "! systemctl --user is-active --quiet lerd-schedule-$name"
check_out "6.3 lerd worker list shows the framework's workers" 'queue.*schedule|schedule.*queue' bash -c 'lerd worker list | tr "\n" " "'
check "lerd worker start vite" lerd worker start vite
check "vite is running" wait_for 30 unit_active "lerd-vite-$name"
check "lerd worker stop vite" lerd worker stop vite
check "6.4 worker start/stop for a non-queue worker" bash -c "! systemctl --user is-active --quiet lerd-vite-$name"

# Horizon and Reverb commands come from the package definitions once a project
# requires the package. Horizon supersedes the plain queue worker, so they get
# a throwaway project of their own rather than changing demo.
wk=$PROJECTS/vmworkers
if [ ! -d "$wk/vendor" ]; then
	rm -rf "${PROJECTS:?}/vmworkers"
	check "scaffold the throwaway project" bash -c "cd '$PROJECTS' && lerd new vmworkers && cd vmworkers && lerd setup --all --skip-open"
	check "require laravel/horizon" bash -c "cd '$wk' && lerd composer require laravel/horizon -W --no-interaction && lerd artisan horizon:install --no-interaction"
	check "require laravel/reverb" bash -c "cd '$wk' && lerd composer require laravel/reverb -W --no-interaction"
	# reverb:install asks a question no flag answers off a terminal, so do what
	# it does: publish the config and give the app its keys.
	(cd "$wk" && lerd artisan vendor:publish --tag=reverb-config --no-interaction </dev/null >/dev/null 2>&1)
	for kv in REVERB_APP_ID=vm REVERB_APP_KEY=vmkey REVERB_APP_SECRET=vmsecret REVERB_HOST=localhost REVERB_PORT=8080 REVERB_SCHEME=http BROADCAST_CONNECTION=reverb; do
		grep -q "^${kv%%=*}=" "$wk/.env" && sed -i "s|^${kv%%=*}=.*|$kv|" "$wk/.env" || echo "$kv" >>"$wk/.env"
	done
fi
wurl="$scheme://$(site_host "$wk")"
sed -i 's/^QUEUE_CONNECTION=.*/QUEUE_CONNECTION=redis/' "$wk/.env"
check "lerd horizon:start" bash -c "cd '$wk' && lerd horizon:start"
expect_200 "$wurl/horizon"
# Watch mode needs chokidar, as lerd says when it is missing.
(cd "$wk" && lerd npm install -D chokidar </dev/null >/dev/null 2>&1)
check "lerd horizon:reload on" bash -c "cd '$wk' && lerd horizon:reload on"
check "lerd horizon:reload off" bash -c "cd '$wk' && lerd horizon:reload off"
check "6.5 Horizon starts, its dashboard answers, reload toggles, stop" bash -c "cd '$wk' && lerd horizon:stop"
check "lerd reverb:start" bash -c "cd '$wk' && lerd reverb:start"
port=$(grep -m1 '^REVERB_PORT=' "$wk/.env" | cut -d= -f2)
key=$(grep -m1 '^REVERB_APP_KEY=' "$wk/.env" | cut -d= -f2)
# Reverb runs inside the PHP container and is reached through the site's own
# domain, which is where a browser's WebSocket goes too.
ws_up() { [ "$(curl -sk -o /dev/null -w '%{http_code}' --max-time 5 --http1.1 -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' "$wurl/app/$key")" = 101 ]; }
wait_for 30 ws_up
check_out "6.6 a WebSocket client connects to Reverb" '^HTTP/1.1 101' bash -c "curl -sk -i -N --max-time 5 --http1.1 -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' '$wurl/app/$key' | head -1"
check "lerd reverb:stop" bash -c "cd '$wk' && lerd reverb:stop"

lerd queue:start </dev/null >/dev/null 2>&1
pid=$(systemctl --user show -p MainPID --value "lerd-queue-$name")
[ -n "$pid" ] && [ "$pid" != 0 ] && kill "$pid"
check "6.7 self-heal restarts a killed worker" wait_for 60 unit_active "lerd-queue-$name"

check "lerd idle on" lerd idle on
check "lerd idle timeout 1m" lerd idle timeout 1m
check "workers suspend after the timeout" wait_for 150 bash -c "! systemctl --user is-active --quiet lerd-queue-$name"
curl -ks -o /dev/null "$url"
check "6.8 hitting the site resumes them" wait_for 60 unit_active "lerd-queue-$name"
expect_200 "$url"
check "lerd idle pin" lerd idle pin "$name"
check_out "6.9 [partial] idle status reports the pinned site" "$name.*pin" bash -c 'lerd idle status | tr "\n" " "'
check "lerd idle unpin" lerd idle unpin "$name"
check "lerd idle off" lerd idle off
check "6.10 lerd idle off resumes everything" wait_for 30 unit_active "lerd-queue-$name"

# Sleeping services. The plan wants a mysql site on https; demo is moved to
# mysql here as phase 5 does, in case this phase runs without it.
api() { curl -s -X "$1" -H 'X-Lerd-CSRF: 1' "http://127.0.0.1:7073$2"; }
asleep() { lerd idle status 2>/dev/null | grep -Eq "sleeping:.*\b$1\b"; }
vhost=$HOME/.local/share/lerd/nginx/conf.d/$(site_host "$DEMO_DIR").conf
watcher_log() { journalctl --user -u lerd-watcher --since "$1" --no-pager -o cat 2>/dev/null; }
# cold <args...>: one request to the sleeping site, never following a redirect.
cold() { curl -sk -o /dev/null -w '%{http_code} %{time_total}' --max-time 90 "$@"; }
if ! grep -q '^DB_CONNECTION=mysql' .env; then
	lerd service start mysql </dev/null >/dev/null 2>&1
	sed -i 's/^DB_CONNECTION=.*/DB_CONNECTION=mysql/' .env
	lerd env </dev/null >/dev/null 2>&1
	lerd artisan migrate --force </dev/null >/dev/null 2>&1
fi
expect_200 "$url"
check "lerd idle on" lerd idle on
check "lerd idle timeout 1m" lerd idle timeout 1m
check "lerd idle services on" lerd idle services on
# Services go to sleep one after another, so wait for the last one.
only_core() { ! podman ps --format '{{.Names}}' | grep -Ev '^lerd-(nginx|dns|php[0-9]+-fpm)$'; }
check "mysql goes to sleep" wait_for 240 asleep mysql
check "6.11 [partial] only nginx, dns and php-fpm keep running" wait_for 60 only_core
svc_json() { api GET /api/services | python3 -c 'import json,sys; print([s.get(sys.argv[2]) for s in json.load(sys.stdin) if s["name"]==sys.argv[1]])' "$1" "$2"; }
check_out "6.11 [partial] the dashboard API marks mysql idle_suspended" '\[True\]' svc_json mysql idle_suspended
check_out "6.11 [partial] the vhost is switched to the waking one" '_lerd/wake' cat "$vhost"
r=$(cold "$url")
echo "--- cold GET: $r"
[ "${r%% *}" = 200 ] && _pass "6.12 a cold GET answers 200 with no redirect in ${r#* }s" || _fail "6.12 a cold GET answers 200 with no redirect" "got $r"

wait_for 240 asleep mysql
r=$(cold -X POST -d a=1 "$url")
echo "--- cold POST: $r"
case ${r%% *} in 200 | 405 | 419) _pass "6.13 a cold POST reaches the app (${r%% *})" ;; *) _fail "6.13 a cold POST reaches the app" "got $r" ;; esac

# 6.14 races the stop: the request goes out the moment mysql is listed asleep.
lerd artisan migrate:status </dev/null >/dev/null 2>&1
for _ in $(seq 1500); do asleep mysql && break; sleep 0.2; done
since=$(date '+%F %T')
r=$(cold "$url")
echo "--- request during the stop: $r"
[ "${r%% *}" = 200 ] && _pass "6.14 a request while mysql is stopping answers 200" || _fail "6.14 a request while mysql is stopping answers 200" "got $r"
check_out "6.14 the watcher woke mysql straight away" 'resumed services:.*mysql' watcher_log "$since"

wait_for 240 asleep mysql
check "lerd stop" lerd stop
check "lerd start" lerd start
check "6.15 services stay asleep across lerd stop && start" asleep mysql
expect_200 "6.15" "$url"

wait_for 240 asleep mysql
lerd install </dev/null >/dev/null 2>&1
check_out "6.16 lerd install keeps the waking vhost" '_lerd/wake' cat "$vhost"
expect_200 "6.16" "$url"

wait_for 240 asleep mysql
check "6.17 the framework console wakes its database" lerd artisan migrate:status
r=$(cold "$url")
[ "${r%% *}" = 200 ] && _pass "6.17 the next page load is warm (${r#* }s)" || _fail "6.17 the next page load is warm" "got $r"

# adminer administers mysql, so opening it has to wake both.
lerd service preset adminer </dev/null >/dev/null 2>&1
lerd service start adminer </dev/null >/dev/null 2>&1
dash() { curl -sL -H 'Accept: text/html' --max-time 90 "http://127.0.0.1:7073/_svc/adminer/"; }
dash_up() { dash | grep -qi 'adminer' && ! dash | grep -q '<title>Waking up'; }
check "adminer and mysql go to sleep" wait_for 300 bash -c "lerd idle status | grep -Eq 'sleeping:.*adminer' && lerd idle status | grep -Eq 'sleeping:.*mysql'"
check_out "6.18 [partial] a sleeping dashboard answers the waking page first" '<title>Waking up' dash
check "6.18 [partial] then the dashboard itself" wait_for 90 dash_up
check "6.18 [partial] adminer wakes its databases with it" unit_active lerd-mysql

# The dashboard overlay's heartbeat is a keepalive every few seconds.
wait_for 240 asleep mysql
api POST "/api/dashboard/keepalive?name=mysql" >/dev/null
check "a keepalive wakes mysql" wait_for 60 unit_active lerd-mysql
end=$((SECONDS + 100))
while [ $SECONDS -lt $end ]; do
	api POST "/api/dashboard/keepalive?name=mysql" >/dev/null
	sleep 10
done
check "6.19 an open dashboard keeps mysql awake past the timeout" bash -c '! lerd idle status | grep -Eq "sleeping:.*mysql"'
check "6.19 mysql sleeps once the dashboard closes" wait_for 240 asleep mysql

api POST "/api/dashboard/keepalive?name=mysql" >/dev/null
wait_for 60 bash -c "! lerd idle status | grep -Eq 'sleeping:.*mysql'"
db_count() { api GET /api/databases | python3 -c 'import json,sys; print([len(e.get("databases") or []) for e in json.load(sys.stdin) if e["service"]=="mysql"])'; }
check_out "6.20 [partial] a sleeping engine's databases list once it wakes" '\[[1-9]' db_count

wait_for 240 asleep mysql
check "lerd service pin mysql" lerd service pin mysql
lerd artisan migrate:status </dev/null >/dev/null 2>&1
sleep 150
check "6.22 [partial] a pinned service stays awake past the timeout" unit_active lerd-mysql
check "lerd service unpin mysql" lerd service unpin mysql

check "mysql sleeps after the unpin" wait_for 240 asleep mysql
check "lerd service stop mysql" lerd service stop mysql
curl -sk -o /dev/null --max-time 30 "$url"
sleep 5
check "6.23 a sleeping service stopped by hand stays stopped" bash -c '! systemctl --user is-active --quiet lerd-mysql'
lerd service start mysql </dev/null >/dev/null 2>&1

wait_for 240 asleep mysql
check "lerd idle services off" lerd idle services off
check "6.24 idle services off wakes mysql" wait_for 60 unit_active lerd-mysql
check "6.24 the real vhost is back" wait_for 30 bash -c "! grep -q _lerd/wake '$vhost'"
expect_200 "6.24" "$url"

# 6.21 last: it turns snapshots on and restarts the watcher.
# The empty listing is two lines too, so count the snapshot rows.
snap_count() { lerd db snapshots </dev/null 2>/dev/null | grep -c '^auto-'; }
export -f snap_count
check "lerd idle services on" lerd idle services on
lerd artisan tinker --execute='DB::table("migrations")->insert(["migration" => "vm_snap_'"$RANDOM"'", "batch" => 99]);' </dev/null >/dev/null 2>&1
check "mysql sleeps after the change" wait_for 240 asleep mysql
check "lerd db snapshot:auto on, covering every site" lerd db snapshot:auto on --every 1m --selection opt-out
before=$(snap_count)
since=$(date '+%F %T')
systemctl --user restart lerd-watcher
wait_for 120 bash -c "[ \$(snap_count) -gt $before ]"
check "6.21 a sleeping database with changes gets its snapshot" bash -c "[ \$(snap_count) -gt $before ]"
check "6.21 mysql goes back to sleep after the dump" wait_for 60 bash -c "! systemctl --user is-active --quiet lerd-mysql"
watcher_log "$since"
before=$(snap_count)
systemctl --user restart lerd-watcher
sleep 30
check "6.21 a second pass with nothing changed takes no snapshot" bash -c "[ \$(snap_count) -eq $before ]"
check "lerd db snapshot:auto off" lerd db snapshot:auto off
lerd idle services off </dev/null >/dev/null 2>&1
check "lerd idle off" lerd idle off

check_out "6.25 [partial] queue:start's flags match its tune_command placeholders" '--tries' lerd queue:start --help
check_out "6.26 [partial] a worker with a reload variant gets its reload toggle" 'reload' bash -c "cd '$wk' && lerd horizon:reload --help"
clone=$HOME/lerd-vm-unlinked
rm -rf "$clone" && git clone -q "$DEMO_DIR" "$clone" 2>/dev/null
# A committed project carries its .lerd.yaml; this demo's is untracked.
cp "$DEMO_DIR/.lerd.yaml" "$clone/" 2>/dev/null
# The plan's case is a .lerd.yaml that names its framework.
grep -q '^framework:' "$clone/.lerd.yaml" 2>/dev/null || echo 'framework: laravel' >>"$clone/.lerd.yaml"
check_out "6.27 an unlinked clone gets its worker commands with the link hint" 'lerd link' bash -c "cd '$clone' && lerd queue:start"
rm -rf "$clone"
todo "6.28 requires_service refuses and orders after the service" "needs a store worker that declares requires_service"
todo "6.29 a dev_server worker serves through its proxy" "needs a framework whose definition declares dev_server"
todo "6.30 dev_server_port held and released" "as 6.15"
todo "6.31 two dev_server sites do not collide" "as 6.15"
check "vite starts again" lerd worker start vite
# Vite answers once it has booted; give it the seconds a user would.
wait_for 60 bash -c "[ \"\$(curl -ks -o /dev/null -w '%{http_code}' '$url/@lerd-vite/@vite/client')\" = 200 ]"
expect_200 "6.32 [partial]" "$url/@lerd-vite/@vite/client"
lerd worker stop vite </dev/null >/dev/null 2>&1
todo "6.33 a Vite+ starter kit serves /@vite/client on the site domain" "needs a vp dev starter kit scaffold"

check "lerd queue:start --tries 5" lerd queue:start --tries 5
check_out "6.34 a passed flag is written under worker_options" 'tries' bash -c "sed -n '/worker_options/,/^[a-z]/p' .lerd.yaml"
lerd queue:start --tries 3 </dev/null >/dev/null 2>&1
check_not "6.35 a value equal to the default is not stored" 'tries' bash -c "sed -n '/worker_options/,/^[a-z]/p' .lerd.yaml"
check_out "6.36 a value carrying whitespace is refused" 'whitespace|space|invalid' lerd queue:start --queue "a b"
todo "6.37 the dashboard gear offers one field per option" "dashboard; the API route for worker options is not wired into this script yet"
skip "6.38 lerd workers mode" "macOS only"
check_out "6.39 [partial] lerd framework list prints the store's packages" 'horizon' bash -c "cd '$wk' && lerd framework list"

mkdir -p "$HOME/.config/lerd/frameworks"
cat >"$HOME/.config/lerd/frameworks/laravel.yaml" <<'YAML'
name: laravel
workers:
  vmprobe:
    label: VM probe
    command: php artisan about
YAML
check_out "6.40 an overlay that only adds a worker keeps detection and lists it" 'vmprobe' bash -c 'lerd worker list; lerd sites | grep -i laravel'
todo "6.41 package vs version file vs overlay precedence" "needs a package declaration colliding with a version file"
rm -f "$HOME/.config/lerd/frameworks/laravel.yaml"
if [ "${RUN_TIER3:-}" = 1 ]; then
	gw=$(ip route | awk '/^default/ {print $3; exit}')
	dev=$(ip route | awk '/^default/ {print $5; exit}')
	sudo ip route del default
	check_out "6.42 offline, a never-fetched version falls back to a cached file" 'queue' lerd worker list
	sudo ip route add default via "$gw" dev "$dev"
else
	skip "6.42 offline fallback to a cached definition" "tier 3, set RUN_TIER3=1"
fi
check "queue back on" lerd queue:start
expect_200 "$url"
