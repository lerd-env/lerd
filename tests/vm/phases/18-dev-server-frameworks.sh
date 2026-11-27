#!/usr/bin/env bash
# Phase 18, frameworks served by their own dev server. Stands on its own: needs
# only lerd, and writes the definitions it uses as user definitions.
source "$(dirname "$0")/../lib.sh"
need_lerd
mkdir -p "$PROJECTS" "$HOME/.config/lerd/frameworks"
api=http://127.0.0.1:7073/api
# Another user's lerd holding the ports would fail every check below.
serving() { curl -s "$api/sites" | grep -q '"' && systemctl --user is-active -q lerd-nginx lerd-ui; }
wait_for 60 serving || {
	_fail "setup: this user's lerd is not serving (ss -ltnp, another user's lerd?)"
	exit 1
}
DEFS=$HOME/.config/lerd/frameworks
SITES=${XDG_DATA_HOME:-$HOME/.local/share}/lerd/sites.yaml
CONFD=${XDG_DATA_HOME:-$HOME/.local/share}/lerd/nginx/conf.d
UNITS=${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user
# npm 10 crashes resolving Nest's optional peers without this.
export npm_config_legacy_peer_deps=true
# ssh runs a non-interactive shell, and Debian's .bashrc returns before lerd's PATH line.
export PATH=${XDG_DATA_HOME:-$HOME/.local/share}/lerd/bin:$PATH
idle_was=$(lerd idle status </dev/null 2>/dev/null | grep -oE 'timeout [0-9hms]+' | cut -d' ' -f2)
idle_on=$(lerd idle status </dev/null 2>/dev/null | grep -c 'Idle-suspend: enabled')

# define <name> <manifest> <package> <dev command> <port> <worker> <check> [extra yaml]
# Detects on <package>-vmlane, which only detects() writes: the real package may
# belong to another definition. The apps name their framework themselves.
define() {
	cat >"$DEFS/vm$1.yaml" <<YAML
name: vm$1
label: VM $1
public_dir: .
composer: "false"
detect:
  - package: {file: $2, name: "$3-vmlane"}
dev_server:
  command: $4
  port: $5
  move_port: true
${8:+  $8}
env:
  file: .env
  format: dotenv
  url_key: APP_URL
  services:
    redis:
      detect:
        - key: REDIS_URL
      vars:
        - REDIS_URL=redis://lerd-redis:6379/0
workers:
  tick:
    label: Tick
    command: $6
    host: true
  boxed:
    label: Boxed
    command: $6
commands:
  - name: check
    label: Check
    command: $7
doctor:
  checks:
    - name: app_boots
      type: command
      command: $7
      timeout_seconds: 120
YAML
}

# body <dir>: what the site answers, past the waking page and a booting server.
body() {
	local r
	for _ in $(seq 60); do
		r=$(curl -s --max-time 20 "http://$(site_host "$1")/")
		case $r in *"Waking up"* | *"502 Bad"* | "") sleep 1 ;; *) break ;; esac
	done
	printf '%s\n' "$r"
}
answers() { body "$1" | grep -q -- "$2"; }
unit() { printf 'lerd-app-%s' "$(site_name "$1")"; }
declared_port() { grep -oE 'port: [0-9]+' "$1/.lerd.yaml" | grep -oE '[0-9]+'; }
registry_port() { awk -v n="$(site_name "$1")" '$2 == "name:" { hit = ($3 == n) } hit && $1 == "host_port:" { print $2; exit }' "$SITES"; }
# on_port <dir> <port>: the registry, nginx and the server's unit all say port.
on_port() {
	[ "$(registry_port "$1")" = "$2" ] &&
		grep -Eq "proxy_pass [^;]*:$2;" "$CONFD/$(site_host "$1").conf" &&
		grep -q "PORT=$2 " "$UNITS/$(unit "$1").service"
}
# moved <dir>: all three agree on a port other than the committed one.
moved() {
	local p
	p=$(registry_port "$1")
	echo "declared $(declared_port "$1"), registry $p"
	[ "$p" != "$(declared_port "$1")" ] && on_port "$1" "$p"
}
BLOCKER=
hold_port() {
	python3 -c "
import http.server
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(s): s.send_response(200); s.end_headers(); s.wfile.write(b'WRONG APP')
    def log_message(*a): pass
http.server.ThreadingHTTPServer(('0.0.0.0', $1), H).serve_forever()" &
	BLOCKER=$!
	wait_for 10 curl -s "http://127.0.0.1:$1/"
}
free_port() { [ -n "$BLOCKER" ] && kill "$BLOCKER" 2>/dev/null && wait "$BLOCKER" 2>/dev/null; BLOCKER=; }
trap 'free_port; _summary' EXIT

# detects <kind> <manifest> <content> <port>: a manifest alone is recognised and
# offered the declared port.
detects() {
	local dir=$PROJECTS/vmdetect-$1
	mkdir -p "$dir"
	printf '%s' "$3" >"$dir/$2"
	check_out "18.2 $1 is detected from $2 and offered its declared port" "\"kind\":\"proxy\".*\"proxy_port\":$4" curl -s "$api/project/questions?dir=$dir"
	rm -rf "${PROJECTS:?}/vmdetect-$1"
}

# serve <dir> <kind>: refusal, then a link with the block init saves.
serve() {
	local dir=$1 q
	printf 'REDIS_URL=redis://localhost:6379/0\n' >"$dir/.env"
	echo "framework: vm$2" >"$dir/.lerd.yaml"
	check_out "18.2 $2 is refused by a plain link" 'lerd init' bash -c "cd $dir && lerd link --yes"
	check_not "$2 is not in the registry after the refusal" "path: $dir\$" cat "$SITES"
	q=$(curl -s "$api/project/questions?dir=$dir")
	echo "$q"
	check_out "$2 is offered its declared dev command" '"kind":"proxy"' echo "$q"
	python3 - "$dir" "$q" "$2" <<'PY' || _fail "$2: no questions to answer"
import json, sys
d, q = sys.argv[1], json.loads(sys.argv[2])
open(d + "/.lerd.yaml", "w").write("framework: vm%s\nservices: [redis]\nproxy:\n  command: %s\n  port: %d\n" % (sys.argv[3], json.dumps(q["proxy_command"]), q["proxy_port"]))
PY
	(cd "$dir" && lerd link --yes) </dev/null
	(cd "$dir" && lerd env) </dev/null
}

# features <id> <dir> <kind> <marker>: what a framework gives any site.
features() {
	local dir=$2 kind=$3
	check "$1 $kind answers with its own page" answers "$dir" "$4"
	check_out "$kind keeps its framework on the registry" "framework: vm$kind" awk -v n="$(site_name "$dir")" '$2 == "name:" { hit = ($3 == n) } hit' "$SITES"
	check "$kind has redis wired over loopback" grep -Eq '^REDIS_URL=redis://127\.0\.0\.1:[0-9]+' "$dir/.env"
	check "$kind has its URL" grep -q "^APP_URL=http://$(site_host "$dir")" "$dir/.env"
	rm -f "/tmp/lerd-vm-tick-$kind"
	(cd "$dir" && lerd worker start tick) </dev/null
	check "$kind host worker really runs" wait_for 60 test -s "/tmp/lerd-vm-tick-$kind"
	(cd "$dir" && lerd worker stop tick) </dev/null >/dev/null 2>&1
	check_out "$kind refuses a worker with no container" 'host: true' bash -c "cd $dir && lerd worker start boxed"
	check "$kind wrote no unit for it" test ! -e "$UNITS/lerd-boxed-$(site_name "$dir").service"
	check "$kind lerd run check" bash -c "cd $dir && lerd run check"
	check_out "$kind doctor has no failures" '0 failing|all checks pass' bash -c "cd $dir && lerd site:doctor"
}

# reloads <id> <dir> <kind> <file> <from> <to>
reloads() {
	body "$2" >/dev/null
	sed -i "s/$5/$6/" "$4"
	check "$1 $3 serves a save without a restart" wait_for 45 answers "$2" "$6"
}

# --- Node: the real NestJS scaffold, through lerd new.
tick='setInterval(function(){require("fs").writeFileSync("/tmp/lerd-vm-tick-nest","1")},500)'
define nest package.json @nestjs/core 'npm run start:dev' 43100 "node -e '$tick'" 'npm run build' \
	'create: npx --yes @nestjs/cli new --skip-git --package-manager npm'
detects nest package.json '{"dependencies":{"@nestjs/core-vmlane":"1"}}' 43100
nest=$PROJECTS/vmnestapp
(cd "$nest" 2>/dev/null && lerd unlink) </dev/null >/dev/null 2>&1
rm -rf "${PROJECTS:?}/vmnestapp"
check_out "18.1 lerd new scaffolds through a host create command" 'created vmnestapp' bash -c "cd $PROJECTS && lerd new vmnestapp --framework=vmnest"
check "the project is where lerd said" test -f "$nest/nest-cli.json"
check "npm ci" bash -c "cd $nest && npm ci"
serve "$nest" nest
features 18.5 "$nest" nest 'Hello World'
reloads 18.9 "$nest" nest "$nest/src/app.service.ts" 'Hello World!' 'Hello SAVED'

# A PHP project that uses the same package stays a PHP project.
mixed=$PROJECTS/vmmixed
mkdir -p "$mixed"
echo '{"require":{"php":"*"}}' >"$mixed/composer.json"
echo '{"dependencies":{"@nestjs/core-vmlane":"^11"}}' >"$mixed/package.json"
check_out "18.3 a composer project with the package stays PHP" '"kind":"php"' curl -s "$api/project/questions?dir=$mixed"
rm -rf "${PROJECTS:?}/vmmixed"

# A parked folder never starts a project's host command by itself.
park=$PROJECTS/vmpark
mkdir -p "$park/vmparked"
echo '{"dependencies":{"@nestjs/core-vmlane":"^11"}}' >"$park/vmparked/package.json"
lerd park "$park" </dev/null
sleep 5
check_not "18.4 a parked dev-server project is not registered" 'vmparked' cat "$SITES"
lerd unpark "$park" </dev/null
rm -rf "${PROJECTS:?}/vmpark"

# --- Python: Django in a venv.
define django requirements.txt django "sh -c 'exec .venv/bin/python manage.py runserver \"\$HOST:\$PORT\"'" 43200 \
	".venv/bin/python -c \"import time; open('/tmp/lerd-vm-tick-django','w').write('1'); time.sleep(600)\"" '.venv/bin/python manage.py check'
detects django requirements.txt 'Django_vmlane>=1' 43200
dj=$PROJECTS/vmdjango
if python3 -c 'import ensurepip, venv' 2>/dev/null; then
	(cd "$dj" 2>/dev/null && lerd unlink) </dev/null >/dev/null 2>&1
	rm -rf "${PROJECTS:?}/vmdjango"
	mkdir -p "$dj"
	check "install Django" bash -c "cd $dj && python3 -m venv .venv && .venv/bin/pip -q install django && echo 'Django>=5' >requirements.txt && .venv/bin/django-admin startproject mysite ."
	sed -i "s/ALLOWED_HOSTS = \[\]/ALLOWED_HOSTS = ['*']/" "$dj/mysite/settings.py"
	printf 'from django.http import HttpResponse\ndef home(r): return HttpResponse("django first")\n' >"$dj/mysite/views.py"
	sed -i 's|^urlpatterns = \[|from mysite.views import home\nurlpatterns = [path("", home),|' "$dj/mysite/urls.py"
	serve "$dj" django
	features 18.6 "$dj" django 'django first'
	reloads 18.9 "$dj" django "$dj/mysite/views.py" 'django first' 'django SAVED'
else
	skip "18.6 django" "the guest has no python3 venv"
fi

# --- Rust and Ruby need a C compiler, which an immutable guest has not.
define axum Cargo.toml axum 'cargo run --bin vmaxum' 43300 'cargo run --bin tick' 'cargo check'
define rails Gemfile rails "sh -c 'exec bin/rails server --binding \"\$HOST\"'" 43400 \
	"bin/rails runner \"File.write('/tmp/lerd-vm-tick-rails', '1'); sleep 600\"" 'bin/rails runner "exit 0"'
detects axum Cargo.toml $'[dependencies]\naxum-vmlane = "1"\n' 43300
detects rails Gemfile "gem 'rails-vmlane'" 43400
rs=$PROJECTS/vmaxum
if have cargo && have cc; then
	(cd "$rs" 2>/dev/null && lerd unlink) </dev/null >/dev/null 2>&1
	rm -rf "${PROJECTS:?}/vmaxum"
	mkdir -p "$rs/src/bin"
	cat >"$rs/Cargo.toml" <<'TOML'
[package]
name = "vmaxum"
version = "0.1.0"
edition = "2021"
default-run = "vmaxum"
[dependencies]
axum = "0.8"
tokio = { version = "1", features = ["rt-multi-thread", "macros", "net"] }
TOML
	cat >"$rs/src/main.rs" <<'RS'
use axum::{routing::get, Router};
#[tokio::main]
async fn main() {
    let var = |k: &str, d: &str| std::env::var(k).unwrap_or(d.into());
    let addr = format!("{}:{}", var("HOST", "0.0.0.0"), var("PORT", "3000"));
    let app = Router::new().route("/", get(|| async { "axum first" }));
    axum::serve(tokio::net::TcpListener::bind(addr).await.unwrap(), app).await.unwrap();
}
RS
	echo 'fn main() { std::fs::write("/tmp/lerd-vm-tick-axum", "1").unwrap(); std::thread::sleep(std::time::Duration::from_secs(600)); }' >"$rs/src/bin/tick.rs"
	check "cargo build" bash -c "cd $rs && cargo build"
	serve "$rs" axum
	features 18.7 "$rs" axum 'axum first'
	# cargo run does not watch; the catalog's rust plugin does.
	lerd plugin list </dev/null 2>/dev/null | grep -q '^rust ' || check "install the rust plugin" lerd plugin install rust
	sed -i 's/command: .*/command: lerd rust:dev/' "$rs/.lerd.yaml"
	(cd "$rs" && lerd link --yes) </dev/null
	reloads 18.10 "$rs" axum "$rs/src/main.rs" 'axum first' 'axum SAVED'
	sed -i 's/"axum SAVED"/"axum BROKEN/' "$rs/src/main.rs"
	sleep 10
	check "18.10 a failed build leaves the last one serving" answers "$rs" 'axum SAVED'
else
	skip "18.7 axum" "the guest has no cargo and C compiler"
	skip "18.10 rust plugin" "the guest has no cargo and C compiler"
fi
rb=$PROJECTS/vmrails
if have rails && have bundle && have cc; then
	(cd "$rb" 2>/dev/null && lerd unlink) </dev/null >/dev/null 2>&1
	rm -rf "${PROJECTS:?}/vmrails"
	check "rails new" bash -c "cd $PROJECTS && rails new vmrails --skip-git --minimal --skip-bundle && cd vmrails && bundle config set --local path vendor/bundle && bundle install && bin/rails generate controller home index"
	sed -i 's|^end|  root "home#index"\nend|' "$rb/config/routes.rb"
	serve "$rb" rails
	features 18.8 "$rb" rails 'Home#index'
	reloads 18.9 "$rb" rails "$rb/app/views/home/index.html.erb" 'Home#index' 'Home SAVED'
else
	skip "18.8 rails" "the guest has no rails, bundler and C compiler"
fi

# --- The port, on the Nest site: its server stays alive when it cannot bind.
port=$(declared_port "$nest")
body "$nest" >/dev/null
hash=$(sha256sum "$nest/.lerd.yaml")
(cd "$nest" && lerd pause) </dev/null
wait_for 15 bash -c "! ss -ltnH | grep -q ':$port '"
hold_port "$port"
check_out "18.11 unpause says the port is taken and where the site went" "port $port is taken" bash -c "cd $nest && lerd unpause"
check "18.11 the site is its own app, not the one on its port" answers "$nest" 'Hello SAVED'
check "registry, nginx and the server moved together" moved "$nest"
was=$(registry_port "$nest")
(cd "$nest" && lerd restart) </dev/null
check "18.12 a restart keeps the moved port" on_port "$nest" "$was"
check "18.12 and the site answers" answers "$nest" 'Hello SAVED'
(cd "$nest" && lerd link --yes) </dev/null
check "a re-link keeps it too" on_port "$nest" "$was"
check_out "doctor agrees the port is fine" '✓ Dev Server Port' bash -c "cd $nest && lerd site:doctor"
check "18.13 .lerd.yaml is untouched" bash -c "[ '$hash' = \"\$(sha256sum $nest/.lerd.yaml)\" ]"
free_port
(cd "$nest" && lerd restart) </dev/null
check "18.14 the declared port comes back once free" on_port "$nest" "$port"
check "18.14 and the site answers" answers "$nest" 'Hello SAVED'
# A stop right after a start used to leave a node past the SIGTERM for systemd's 90s.
u=$(unit "$nest")
check "18.21 a dev server stopped right after it starts stops within 20s" bash -c "systemctl --user start $u; sleep 1; s=\$(date +%s); systemctl --user stop $u; [ \$((\$(date +%s) - s)) -le 20 ]"
systemctl --user start "$u"
check "and it serves again" wait_for 60 answers "$nest" 'Hello SAVED'

# Phase 6 leaves idle-suspend off, so turn it on for the sleep and put it back after.
lerd idle on </dev/null >/dev/null 2>&1
lerd idle timeout 1m </dev/null
check "the site falls asleep" wait_for 240 bash -c "! systemctl --user is-active -q $(unit "$nest")"
hold_port "$port"
check "18.15 a sleeping site wakes on another port" answers "$nest" 'Hello SAVED'
check "18.15 with nginx following" wait_for 30 moved "$nest"
free_port
(cd "$nest" && lerd restart) </dev/null
lerd idle timeout "${idle_was:-30m}" </dev/null
[ "$idle_on" = 1 ] || lerd idle off </dev/null >/dev/null 2>&1

(cd "$nest" && lerd stop) </dev/null >/dev/null 2>&1
hold_port "$port"
lerd start </dev/null
check "18.16 lerd start moves it as well" answers "$nest" 'Hello SAVED'
check "18.16 with nginx following" moved "$nest"
free_port
(cd "$nest" && lerd restart) </dev/null

sed -i "s/port: $port/port: 43155/" "$nest/.lerd.yaml"
(cd "$nest" && lerd link --yes) </dev/null
check "18.17 an edited port reaches nginx and the server" on_port "$nest" 43155
check "18.17 and the site answers" answers "$nest" 'Hello SAVED'

# A command that names its port is never moved, and doctor says why it is down.
fixed=$PROJECTS/vmfixed
mkdir -p "$fixed"
echo '{"name":"vmfixed","dependencies":{"@nestjs/core-vmlane":"*"}}' >"$fixed/package.json"
echo "require('http').createServer((q,r)=>r.end('fixed ok')).listen(+process.argv[2],'0.0.0.0')" >"$fixed/server.js"
printf 'proxy:\n  command: node server.js 43500\n  port: 43500\n' >"$fixed/.lerd.yaml"
hold_port 43500
(cd "$fixed" && lerd link --yes) </dev/null
sleep 4
check "18.18 a command naming its port is not moved" bash -c "[ '$(registry_port "$fixed")' = 43500 ]"
check_out "18.18 doctor names the taken port" 'another process has port 43500' bash -c "cd $fixed && lerd site:doctor"
free_port
check "the server recovers once the port is free" wait_for 60 answers "$fixed" 'fixed ok'
(cd "$fixed" && lerd unlink) </dev/null >/dev/null 2>&1
rm -rf "${PROJECTS:?}/vmfixed"

# A worktree of a dev-server site runs its own server on its own port.
(cd "$nest" && printf 'node_modules\ndist\n' >.gitignore && rm -f .env.before_lerd && git init -q && git add . && git -c user.email=t@t -c user.name=t commit -qm nest) </dev/null
check "lerd worktree add" bash -c "cd $nest && lerd worktree add -b vmbranch"
wt=$(git -C "$nest" worktree list --porcelain | awk '/^worktree/ { p = $2 } END { print p }')
check "the worktree is a checkout of its own" bash -c "[ -n '$wt' ] && [ '$wt' != '$nest' ]"
[ -d "$wt/node_modules" ] || (cd "$wt" && npm ci) </dev/null >/dev/null 2>&1
[ "$wt" != "$nest" ] && sed -i 's/Hello SAVED/Hello BRANCH/' "$wt/src/app.service.ts"
# The site's wildcard answers 200 for any branch name, so only the branch's own text counts.
check "18.19 the worktree serves its own code" wait_for 120 bash -c "curl -s --max-time 20 http://vmbranch.$(site_host "$nest")/ | grep -q 'Hello BRANCH'"
expect_200 18.19 "http://vmbranch.$(site_host "$nest")/"
check "and the site still serves its own" answers "$nest" 'Hello SAVED'
(cd "$nest" && lerd worktree remove vmbranch) </dev/null

expect_200 18.20 "http://$(site_host "$nest")/"
