#!/usr/bin/env bash
# Phase 6, workers.
source "$(dirname "$0")/../lib.sh"
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

check_out "6.11 [partial] queue:start's flags match its tune_command placeholders" '--tries' lerd queue:start --help
check_out "6.12 [partial] a worker with a reload variant gets its reload toggle" 'reload' bash -c "cd '$wk' && lerd horizon:reload --help"
clone=$HOME/lerd-vm-unlinked
rm -rf "$clone" && git clone -q "$DEMO_DIR" "$clone" 2>/dev/null
# A committed project carries its .lerd.yaml; this demo's is untracked.
cp "$DEMO_DIR/.lerd.yaml" "$clone/" 2>/dev/null
# The plan's case is a .lerd.yaml that names its framework.
grep -q '^framework:' "$clone/.lerd.yaml" 2>/dev/null || echo 'framework: laravel' >>"$clone/.lerd.yaml"
check_out "6.13 an unlinked clone gets its worker commands with the link hint" 'lerd link' bash -c "cd '$clone' && lerd queue:start"
rm -rf "$clone"
todo "6.14 requires_service refuses and orders after the service" "needs a store worker that declares requires_service"
todo "6.15 a dev_server worker serves through its proxy" "needs a framework whose definition declares dev_server"
todo "6.16 dev_server_port held and released" "as 6.15"
todo "6.17 two dev_server sites do not collide" "as 6.15"
check "vite starts again" lerd worker start vite
# Vite answers once it has booted; give it the seconds a user would.
wait_for 60 bash -c "[ \"\$(curl -ks -o /dev/null -w '%{http_code}' '$url/@lerd-vite/@vite/client')\" = 200 ]"
expect_200 "6.18 [partial]" "$url/@lerd-vite/@vite/client"
lerd worker stop vite </dev/null >/dev/null 2>&1
todo "6.19 a Vite+ starter kit serves /@vite/client on the site domain" "needs a vp dev starter kit scaffold"

check "lerd queue:start --tries 5" lerd queue:start --tries 5
check_out "6.20 a passed flag is written under worker_options" 'tries' bash -c "sed -n '/worker_options/,/^[a-z]/p' .lerd.yaml"
lerd queue:start --tries 3 </dev/null >/dev/null 2>&1
check_not "6.21 a value equal to the default is not stored" 'tries' bash -c "sed -n '/worker_options/,/^[a-z]/p' .lerd.yaml"
check_out "6.22 a value carrying whitespace is refused" 'whitespace|space|invalid' lerd queue:start --queue "a b"
todo "6.23 the dashboard gear offers one field per option" "dashboard; the API route for worker options is not wired into this script yet"
skip "6.24 lerd workers mode" "macOS only"
check_out "6.25 [partial] lerd framework list prints the store's packages" 'horizon' bash -c "cd '$wk' && lerd framework list"

mkdir -p "$HOME/.config/lerd/frameworks"
cat >"$HOME/.config/lerd/frameworks/laravel.yaml" <<'YAML'
name: laravel
workers:
  vmprobe:
    label: VM probe
    command: php artisan about
YAML
check_out "6.26 an overlay that only adds a worker keeps detection and lists it" 'vmprobe' bash -c 'lerd worker list; lerd sites | grep -i laravel'
todo "6.27 package vs version file vs overlay precedence" "needs a package declaration colliding with a version file"
rm -f "$HOME/.config/lerd/frameworks/laravel.yaml"
if [ "${RUN_TIER3:-}" = 1 ]; then
	gw=$(ip route | awk '/^default/ {print $3; exit}')
	dev=$(ip route | awk '/^default/ {print $5; exit}')
	sudo ip route del default
	check_out "6.28 offline, a never-fetched version falls back to a cached file" 'queue' lerd worker list
	sudo ip route add default via "$gw" dev "$dev"
else
	skip "6.28 offline fallback to a cached definition" "tier 3, set RUN_TIER3=1"
fi
check "queue back on" lerd queue:start
expect_200 "$url"
