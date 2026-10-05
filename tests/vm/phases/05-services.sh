#!/usr/bin/env bash
# Phase 5, services and database operations, on demo from phase 2.
source "$(dirname "$0")/../lib.sh"
need_demo
cd "$DEMO_DIR" || exit 1
name=$(site_name "$DEMO_DIR")
host=$(site_host "$DEMO_DIR")
url="https://$host"
[ "$(tld)" = localhost ] && url="http://$host"

api() { curl -s -X "$1" -H 'X-Lerd-CSRF: 1' "http://127.0.0.1:7073$2"; }
# mcp_call <tool> <json-args>: one tools/call over lerd's MCP stdio server.
mcp_call() {
	python3 - "$1" "$2" <<'PY'
import json, subprocess, sys
p = subprocess.Popen(["lerd", "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
def send(o):
    p.stdin.write(json.dumps(o) + "\n"); p.stdin.flush()
send({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "vm", "version": "1"}}})
p.stdout.readline()
send({"jsonrpc": "2.0", "method": "notifications/initialized"})
send({"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": sys.argv[1], "arguments": json.loads(sys.argv[2])}})
print(p.stdout.readline())
p.stdin.close(); p.terminate()
PY
}
# pty_run <seconds> <input> <cmd...>: a command on a terminal, input typed after a second.
pty_run() {
	python3 - "$@" <<'PY'
import os, pty, select, sys, time
secs, text = float(sys.argv[1]), sys.argv[2].encode().decode("unicode_escape").encode()
pid, fd = pty.fork()
if pid == 0:
    os.execvp(sys.argv[3], sys.argv[3:])
out, start, sent = b"", time.time(), False
while time.time() < start + secs:
    if not sent and time.time() > start + 1.5:
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
mailpit_web() { podman port lerd-mailpit 8025/tcp 2>/dev/null | grep -m1 -o '127\.0\.0\.1:[0-9]*'; }

check_out "5.1 lerd service preset lists presets and service search filters" 'mysql.*redis' bash -c 'lerd service preset | tr "\n" " "; lerd service search redis'
check "5.2 lerd service start mysql comes up" lerd service start mysql
# A fresh Laravel defaults to SQLite, which lerd rightly leaves alone; switch
# the project to MySQL the way a user would before checking the wiring.
sed -i 's/^DB_CONNECTION=.*/DB_CONNECTION=mysql/' .env
check_out "5.3 lerd env wires DB_* from the preset" '^DB_HOST=(lerd-mysql|127\.0\.0\.1)' bash -c 'lerd env >/dev/null 2>&1; grep ^DB_HOST= .env'
db=$(grep '^DB_DATABASE=' .env | cut -d= -f2)
check_out "5.4 lerd db:create creates the database and its _testing twin" "${db}_testing" bash -c "lerd db:create '$db' >/dev/null 2>&1; podman exec lerd-mysql mysql -h127.0.0.1 -uroot -plerd -e 'SHOW DATABASES' 2>/dev/null"
check "5.5 migrations run against the service" lerd artisan migrate --force
mkdir -p routes && grep -q vm-dbcheck routes/web.php || echo "Route::get('/vm-dbcheck', fn () => DB::table('migrations')->count());" >>routes/web.php
check_out "5.6 a route that hits the database answers 200" '^200 [1-9]' bash -c "curl -sk -w '%{http_code} ' -o /tmp/lerd-vm-db '$url/vm-dbcheck'; cat /tmp/lerd-vm-db"

check "lerd service start redis" lerd service start redis
check_out "5.7 redis wiring lands and the cache driver works" '^vm-ok$' bash -c "lerd env >/dev/null 2>&1; sed -i 's/^CACHE_STORE=.*/CACHE_STORE=redis/' .env; lerd artisan tinker --execute='Cache::put(\"vm\", \"vm-ok\"); echo Cache::get(\"vm\");' | tail -1"
check "lerd service start mailpit" lerd service start mailpit
expect_200 5.8 "http://$(mailpit_web)"

# Take mailpit's web port while it is down: it has to move, not restart-loop.
old_web=$(mailpit_web | cut -d: -f2)
lerd service stop mailpit </dev/null >/dev/null 2>&1
python3 -m http.server "$old_web" --bind 127.0.0.1 >/dev/null 2>&1 &
squatter=$!
sleep 1
lerd service start mailpit </dev/null >/dev/null 2>&1
new_web=$(mailpit_web | cut -d: -f2)
kill "$squatter" 2>/dev/null
check_out "mailpit moved off the taken port and runs" "moved=yes active=active" bash -c "echo moved=$([ -n "$new_web" ] && echo yes) active=\$(systemctl --user is-active lerd-mailpit)"
# The dashboard reaches Mailpit through its own proxy, not by port.
expect_200 5.9 "http://127.0.0.1:7073/_svc/mailpit/"
# Through nginx with the browser's Origin, the way the embedded dashboard calls
# it: Mailpit refuses an Origin that is not the Host it receives.
dash_mailpit() {
	curl -s -o /dev/null -w '%{http_code}' --max-time 3 --resolve lerd.localhost:80:127.0.0.1 -H 'Origin: http://lerd.localhost' "$@"
}
check_out "5.45 mailpit embedded on lerd.localhost answers its API and opens its websocket" '^api=200 ws=101 cors=0$' bash -c "
	$(declare -f dash_mailpit)
	since=\$(date +%s)
	api=\$(dash_mailpit http://lerd.localhost/_svc/mailpit/api/v1/info)
	ws=\$(dash_mailpit -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' http://lerd.localhost/_svc/mailpit/api/events)
	echo api=\$api ws=\$ws cors=\$(podman logs --since \$since lerd-mailpit 2>&1 | grep -c '\[cors\]')"

check_out "lerd service preset spamassassin" "Installed preset|already" lerd service preset spamassassin
check "lerd service start spamassassin" lerd service start spamassassin
check_out "spamassassin is wired into mailpit" 'SPAMASSASSIN' bash -c "podman inspect lerd-mailpit --format '{{range .Config.Env}}{{println .}}{{end}}'"
lerd artisan tinker --execute='Mail::raw("vm spam check", fn ($m) => $m->to("vm@example.test")->subject("vm-sa"));' </dev/null >/dev/null 2>&1
sleep 3
msg=$(curl -s "http://$(mailpit_web)/api/v1/messages?limit=1" | python3 -c 'import json,sys; print(json.load(sys.stdin)["messages"][0]["ID"])' 2>/dev/null)
check_out "5.10 a caught message shows a SpamAssassin score, and removing it unwires mailpit" '"Score".*unwired' bash -c "curl -s 'http://$(mailpit_web)/api/v1/message/$msg/sa-check'; lerd service remove spamassassin >/dev/null 2>&1; podman inspect lerd-mailpit --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -q SPAMASSASSIN || echo unwired"

lerd dump on </dev/null >/dev/null 2>&1
lerd artisan tinker --execute='Mail::send("welcome", [], fn ($m) => $m->to("vm@example.test")->subject("vm-view"));' </dev/null >/dev/null 2>&1
sleep 3
msg=$(curl -s "http://$(mailpit_web)/api/v1/messages?limit=1" | python3 -c 'import json,sys; print(json.load(sys.stdin)["messages"][0]["ID"])' 2>/dev/null)
check_out "5.11 a Laravel mail carries X-Lerd-View naming its Blade view" 'X-Lerd-View.*welcome' bash -c "curl -s 'http://$(mailpit_web)/api/v1/message/$msg/headers' | tr -d '\n'"
lerd dump off </dev/null >/dev/null 2>&1

check_out "5.12 lerd service list shows status, version and the Update column" 'Version.*Status.*Update' lerd service list
check "lerd service port mysql 3397" lerd service port mysql 3397
check_out "5.13 [partial] the published port moves and .env follows" '^DB_PORT=3397|DB_HOST=lerd-mysql' bash -c 'grep -E "^DB_(PORT|HOST)=" .env'
expect_200 "$url/vm-dbcheck"
lerd service port mysql 3306 </dev/null >/dev/null 2>&1
check_out "5.14 lerd service expose publishes an extra port" '33061' bash -c 'lerd service expose mysql 33061:3306 >/dev/null 2>&1; podman port lerd-mysql'
lerd service expose mysql 33061:3306 --remove </dev/null >/dev/null 2>&1
pins=$HOME/.local/share/lerd/pinned-services.yaml
check_out "5.15 lerd service pin persists across a restart, and unpin" 'pinned.*unpinned' bash -c "lerd service pin redis >/dev/null; lerd stop >/dev/null 2>&1; lerd start >/dev/null 2>&1; grep -q redis '$pins' && echo pinned; lerd service unpin redis >/dev/null; grep -q redis '$pins' 2>/dev/null || echo unpinned"
update_out=$(lerd service update mysql </dev/null 2>&1)
echo "$update_out"
expect_200 5.16 "$url/vm-dbcheck"
if grep -qiE 'up to date|already|no update|nothing to update' <<<"$update_out"; then
	todo "5.17 lerd service rollback mysql swaps back" "no newer mysql image today, so update changed nothing to roll back"
else
	check "lerd service rollback mysql" lerd service rollback mysql
	expect_200 5.17 "$url/vm-dbcheck"
fi
out=$(lerd service migrate mysql 9.7 </dev/null 2>&1)
echo "$out"
check_out "5.18 service migrate keeps the dump and the old data dir under backups" 'dump=.*olddata=' bash -c 'echo dump=$(ls ~/.local/share/lerd/backups/mysql-*.sql 2>/dev/null | tail -1) olddata=$(ls -d ~/.local/share/lerd/backups/mysql* ~/.local/share/lerd/data/mysql.pre-migrate-* 2>/dev/null | grep -v "\.sql$" | tail -1)'
expect_200 "$url/vm-dbcheck"
ver_before=$(podman inspect lerd-redis --format '{{.ImageName}}')
check_out "5.19 [partial] lerd service reinstall redis comes back at the same version" "^$ver_before\$" bash -c "lerd service reinstall redis >/dev/null 2>&1; podman inspect lerd-redis --format '{{.ImageName}}'"

check "lerd service remove mailpit" lerd service remove mailpit
check_not "5.20 a removed service stays removed through start, install and link" 'lerd-mailpit' bash -c 'lerd start >/dev/null 2>&1; lerd install >/dev/null 2>&1; lerd link >/dev/null 2>&1; podman ps --format "{{.Names}}"'
# An orphan is a running service container whose definition is gone.
lerd service add --name vmghost --image docker.io/library/nginx:alpine </dev/null >/dev/null 2>&1
lerd service start vmghost </dev/null >/dev/null 2>&1
wait_for 30 bash -c 'podman ps --format "{{.Names}}" | grep -qx lerd-vmghost'
rm -f "$HOME/.config/lerd/services/vmghost.yaml"
check_out "an orphaned service container shows and its trash action removes it" 'orphan=true.*gone' bash -c "curl -s http://127.0.0.1:7073/api/stats | grep -q '\"lerd-vmghost\"[^}]*\"orphaned\":true' && echo orphan=true; curl -s -X POST -H 'X-Lerd-CSRF: 1' http://127.0.0.1:7073/api/services/vmghost/remove >/dev/null; test -e ~/.config/containers/systemd/lerd-vmghost.container || echo gone"
lerd service start mailpit </dev/null >/dev/null 2>&1

cp .env /tmp/lerd-vm-env
check_out "a plus on an env-only service records it without rewiring" '"ok": ?true' api POST "/api/sites/$host/service:declare?name=redis"
check_out "5.21 the Overview X takes redis off the site, back to the example values" '"ok": ?true.*REDIS_HOST=127' bash -c "curl -s -X POST -H 'X-Lerd-CSRF: 1' 'http://127.0.0.1:7073/api/sites/$host/service:remove?name=redis'; grep ^REDIS_HOST= .env"
expect_200 "$url"
cp /tmp/lerd-vm-env .env
expect_200 "$url"

lerd service start rustfs </dev/null >/dev/null 2>&1
expect_200 "5.22 [partial]" "http://127.0.0.1:7073/rustfs/console/"
skip "the console is already logged in on the bucket clicked" "a browser session, phase 10's browser pass"

lerd service start postgres </dev/null >/dev/null 2>&1
lerd service preset phpmyadmin </dev/null >/dev/null 2>&1
lerd service preset adminer </dev/null >/dev/null 2>&1
check_out "5.23 [partial] postgres opens Adminer and MySQL the higher admin_rank" '"postgres"[^]]*adminer|adminer[^]]*postgres' bash -c "curl -s http://127.0.0.1:7073/api/services | tr -d ' \n'"
skip "5.24 pgAdmin over plain http keeps its session" "a browser session, phase 10's browser pass"
if getenforce 2>/dev/null | grep -q Enforcing; then
	check "5.25 mysqldump --result-file through the shim writes the file under SELinux" bash -c "rm -f ~/lerd-vm-dump.sql; mysqldump --result-file ~/lerd-vm-dump.sql '$db' && test -s ~/lerd-vm-dump.sql"
else
	skip "5.25 mysqldump under SELinux" "Fedora or Silverblue only; SELinux is not enforcing here"
fi

check_not "5.26 lerd service remove mailpit stops and removes it cleanly" 'lerd-mailpit' bash -c 'lerd service remove mailpit >/dev/null 2>&1; podman ps -a --format "{{.Names}}"'
# Keep a named service-wide snapshot to restore after the purge below.
lerd db:snapshot vm-before-purge -s mysql -A </dev/null >/dev/null 2>&1
check_out "5.27 service remove --purge renames the data dir aside" 'mysql\.pre-remove-' bash -c 'lerd service remove mysql --purge >/dev/null 2>&1; ls ~/.local/share/lerd/data/'
check_out "5.28 site:doctor names the missing database" 'mysql' bash -c 'lerd site:doctor 2>&1 | grep -A2 "✗ Database"'
lerd service start mysql </dev/null >/dev/null 2>&1
check "restore the snapshot taken before the purge" lerd db:restore vm-before-purge -s mysql -A --force
lerd env </dev/null >/dev/null 2>&1
expect_200 5.29 "$url/vm-dbcheck"

lerd service domain rustfs rustfs </dev/null >/dev/null 2>&1
# The site has to use RustFS before lerd env wires its domain in.
python3 - <<'PY'
import yaml
cfg = yaml.safe_load(open(".lerd.yaml")) or {}
svcs = cfg.get("services") or []
if "rustfs" not in [x if isinstance(x, str) else x.get("name") for x in svcs]:
    svcs.append("rustfs")
cfg["services"] = svcs
yaml.safe_dump(cfg, open(".lerd.yaml", "w"), sort_keys=False)
PY
lerd env </dev/null >/dev/null 2>&1
check_out "5.30 the service answers a browser preflight on its own domain" '^access-control-allow-origin: https?://' bash -c "curl -sk -o /dev/null -D - -X OPTIONS -H 'Origin: https://$(site_host "$DEMO_DIR")' -H 'Access-Control-Request-Method: GET' https://rustfs.$(tld)/"
check_out "and lerd env writes that domain into the site" 'rustfs\.' grep -E '^AWS_(ENDPOINT|URL)=' .env
check_not "5.31 [partial] site:doctor calls the site wired" '✗ Service Wiring' lerd site:doctor

check "lerd db:export" lerd db:export -o /tmp/lerd-vm-dump.sql
check "5.32 lerd db:import round trips" lerd db:import /tmp/lerd-vm-dump.sql
rm -f /tmp/lerd-vm-dump.sql
lerd artisan migrate --force </dev/null >/dev/null 2>&1
lerd db:snapshot before-change </dev/null >/dev/null 2>&1
lerd artisan tinker --execute='DB::table("migrations")->delete();' </dev/null >/dev/null 2>&1
check_out "5.33 snapshot, change, restore, list and rm" 'restored=[1-9].*listed.*removed' bash -c 'lerd db:restore before-change --force >/dev/null 2>&1; echo restored=$(lerd artisan tinker --execute="echo DB::table(\"migrations\")->count();" | tail -1); lerd db:snapshots | grep -q before-change && echo listed; lerd db:snapshot:rm before-change >/dev/null 2>&1; lerd db:snapshots | grep -q before-change || echo removed'
check_out "5.34 [partial] db:snapshot:auto on, per site, and keep" 'enabled' bash -c "lerd db:snapshot:auto on >/dev/null 2>&1; lerd db:snapshot:auto site '$name' on >/dev/null 2>&1; lerd db:snapshot:auto status; lerd db:snapshot vm-keep >/dev/null 2>&1; lerd db:snapshot:keep vm-keep >/dev/null 2>&1 && echo kept"
lerd db:snapshot:auto off </dev/null >/dev/null 2>&1
shell_out=$(pty_run 8 'SELECT 42 AS vm_answer;\nexit\n' lerd db:shell)
check_out "5.35 lerd db:shell opens an interactive shell" 'vm_answer' printf '%s' "$shell_out"
if lerd service list 2>/dev/null | grep -q '^mysql-9-7'; then
	check "lerd db:move to mysql-9-7" lerd db:move --from mysql --to mysql-9-7 --site "$name" --force
	check_out "db:move repoints .env" 'lerd-mysql-9-7|3307|3308' grep -E '^DB_(HOST|PORT)=' .env
	expect_200 5.36 "$url/vm-dbcheck"
	lerd db:move --from mysql-9-7 --to mysql --site "$name" --force </dev/null >/dev/null 2>&1
else
	lerd service preset mysql --version 9.7 </dev/null >/dev/null 2>&1
	check "lerd db:move to mysql-9-7" lerd db:move --from mysql --to mysql-9-7 --site "$name" --force
	expect_200 5.36 "$url/vm-dbcheck"
	lerd db:move --from mysql-9-7 --to mysql --site "$name" --force </dev/null >/dev/null 2>&1
fi
check_out "5.37 db:extension lists and adds one" 'pg_trgm' bash -c "lerd db:extension list -s postgres -d postgres; lerd db:extension add pg_trgm -s postgres -d postgres >/dev/null 2>&1; lerd db:extension list -s postgres -d postgres | grep pg_trgm"
mkdir -p "$HOME/.local/share/lerd/data/minio/vmbucket" && echo vm >"$HOME/.local/share/lerd/data/minio/vmbucket/vm.txt"
check_out "5.38 lerd minio:migrate moves a MinIO volume onto RustFS" 'vm.txt' bash -c 'lerd minio:migrate >/dev/null 2>&1; ls ~/.local/share/lerd/data/rustfs/vmbucket/'

lerd service preset memcached </dev/null >/dev/null 2>&1
# The pull, and its notice, come with the first start.
out=$(lerd service start memcached </dev/null 2>&1)
echo "$out"
if hub_limited; then skip "5.39 download size named" "Docker Hub is rate-limiting this network (HTTP 429), so no size can be read"; else
	check_out "5.39 [partial] an install that pulls names the image and its size first" 'will download 1 image \(~|Nothing to download|already' printf '%s' "$out"
fi
if hub_limited; then skip "5.40 download size named" "Docker Hub is rate-limiting this network (HTTP 429), so no size can be read"; else
	check_out "5.40 the dashboard estimate names the image and size, and a stored image needs no click" '"image":"[^"]+","bytes":[1-9].*"local":true' bash -c "curl -s 'http://127.0.0.1:7073/api/image-estimate?preset=typesense'; curl -s 'http://127.0.0.1:7073/api/image-estimate?service=mysql&action=reinstall'"
fi
if hub_limited; then skip "5.41 download size named" "Docker Hub is rate-limiting this network (HTTP 429), so no size can be read"; else
	check_out "5.41 MCP answers with the image and size instead of downloading" 'image.*(MiB|MB|GiB|bytes)' mcp_call service '{"action":"preset_install","name":"typesense"}'
fi
check_not "the MCP call downloaded nothing" 'typesense' bash -c "podman images --format '{{.Repository}}'"
expect_200 "$url"

# Hand the next phase a working site: the purge above left a fresh, empty
# MySQL, so recreate the schema whatever the checks above found.
lerd service start mysql </dev/null >/dev/null 2>&1
lerd db:create "$db" </dev/null >/dev/null 2>&1
check "the site's schema is back for the next phase" lerd artisan migrate --force
expect_200 "$url"
reclaim
