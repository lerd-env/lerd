#!/usr/bin/env bash
# Phase 8, site lifecycle and sharing.
source "$(dirname "$0")/../lib.sh"
cd "$DEMO_DIR" || exit 1
# site_url <dir>: the site's URL on the scheme it is actually served on.
site_url() {
	local n tls
	n=$(site_name "$1")
	tls=$(lerd sites 2>/dev/null | awk -v n="$n" '$1 == n { print $5; exit }')
	if [ "$tls" = Yes ]; then printf 'https://%s' "$(site_host "$1")"; else printf 'http://%s' "$(site_host "$1")"; fi
}
name=$(site_name "$DEMO_DIR")
url=$(site_url "$DEMO_DIR")
scheme=${url%%://*}
lan_ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "src") print $(i + 1)}')
api=http://127.0.0.1:7073/api

# site_entry <name>: the site's registry entry as JSON, for before/after diffs.
site_entry() {
	python3 - "$1" <<'EOF'
import json, os, sys, yaml
path = os.path.expanduser("~/.local/share/lerd/sites.yaml")
for s in (yaml.safe_load(open(path)) or {}).get("sites", []):
    if s.get("name") == sys.argv[1]:
        print(json.dumps(s, sort_keys=True))
EOF
}

# nginx_with <site> <text>: runs lerd nginx edit --location with an editor
# that appends text to the override, the way a user would type it.
nginx_with() {
	local site=$1 text=$2 ed=/tmp/lerd-vm-editor.sh
	printf '#!/bin/sh\nprintf "%%s\\n" %q >> "$1"\n' "$text" >"$ed"
	chmod +x "$ed"
	EDITOR=$ed VISUAL=$ed lerd nginx edit --location "$site"
}

lerd queue:start </dev/null >/dev/null 2>&1
check_out "8.1 [partial] lerd pause swaps in the landing page" 'paused' bash -c "lerd pause '$name' && curl -sk '$url'"
expect_200 "$url"
check_not "pause stops the site's workers" '^active$' systemctl --user is-active "lerd-queue-$name"
check "lerd unpause" lerd unpause "$name"
expect_200 8.2 "$url"
check "unpause restarts the workers" wait_for 30 systemctl --user is-active "lerd-queue-$name"
check "lerd restart" lerd restart "$name"
expect_200 8.3 "$url"

check "lerd domain add foo" lerd domain add foo
expect_200 8.4 "$scheme://$(host foo)"
check "lerd domain add bar.$(tld) (already carries the TLD)" lerd domain add "bar.$(tld)"
expect_200 "$scheme://bar.$(tld)"
check_not "8.16 the TLD is not doubled" "\.$(tld)\.$(tld)" bash -c "lerd domain list; lerd sites"
check "lerd domain remove foo" lerd domain remove foo
check "lerd domain remove bar.$(tld)" lerd domain remove "bar.$(tld)"

cp .lerd.yaml /tmp/lerd-vm-lerd.yaml 2>/dev/null || : >/tmp/lerd-vm-lerd.yaml
python3 - <<'EOF'
import yaml
cfg = yaml.safe_load(open(".lerd.yaml")) or {}
cfg["domains"] = list(dict.fromkeys((cfg.get("domains") or []) + ["baz.test"]))
yaml.safe_dump(cfg, open(".lerd.yaml", "w"), sort_keys=False)
EOF
lerd link </dev/null >/dev/null 2>&1
check_out "8.17 a domain declared in .lerd.yaml is registered once, TLD applied once" '^1$' bash -c "lerd domain list | grep -c 'baz\.test\b'; lerd domain list | grep -q 'baz\.test\.test' && echo doubled"
cp /tmp/lerd-vm-lerd.yaml .lerd.yaml
rm -f /tmp/lerd-vm-lerd.yaml
lerd domain remove baz.test </dev/null >/dev/null 2>&1
lerd link </dev/null >/dev/null 2>&1

check "lerd unlink" lerd unlink
check_not "8.5 unlink stops serving it (no stale 200)" '^200$' code "$url"
check "lerd link again" lerd link
expect_200 "$url"

park=$HOME/ParkVM
mkparked() { mkdir -p "$park/$1/public" && echo '{}' >"$park/$1/composer.json" && echo "<?php echo 'ok';" >"$park/$1/public/index.php"; }
mkparked parkone
check "lerd park picks up existing projects" bash -c "lerd park $park && lerd sites | grep -q parkone"
# lerd park only edits the config; the watcher takes the new directory up on
# its next pass, every 5 seconds, so a project made sooner raises no event.
sleep 6
mkparked parktwo
check "8.6 [partial] a project created in a parked directory is picked up" wait_for 90 bash -c 'lerd sites | grep -q parktwo'
check "unlink a site inside the parked directory" bash -c "cd $park/parkone && lerd unlink"
check "link it again" bash -c "cd $park/parkone && lerd link"
check "it is back in lerd sites" bash -c 'lerd sites | grep -q parkone'
check "and in the dashboard" bash -c "curl -s $api/sites | grep -q parkone"
expect_200 8.8 "$(site_url "$park/parkone")"
check "lerd unpark" lerd unpark "$park"
check_not "8.7 unpark unlinks them" 'parkone|parktwo' lerd sites

# Groups need a second site on the same database engine as demo.
grp=$PROJECTS/grp
if [ ! -d "$grp/vendor" ]; then
	rm -rf "${PROJECTS:?}/grp"
	check "scaffold the group's secondary" bash -c "cd '$PROJECTS' && lerd new grp && cd grp && lerd setup --all --skip-open"
fi
(cd "$grp" && lerd link </dev/null >/dev/null 2>&1)
# Sharing needs both sites on a database server; a file database is refused.
sed -i 's/^DB_CONNECTION=.*/DB_CONNECTION=mysql/' "$DEMO_DIR/.env"
(cd "$DEMO_DIR" && lerd env </dev/null >/dev/null 2>&1)
(cd "$grp" && sed -i 's/^DB_CONNECTION=.*/DB_CONNECTION=mysql/' .env && lerd env </dev/null >/dev/null 2>&1)
# A switch to MySQL needs that database created and migrated, as a user would;
# phase 5 has done it for demo already, a phase 8 run on its own has not.
(cd "$DEMO_DIR" && lerd db:create "$(grep '^DB_DATABASE=' .env | cut -d= -f2)" </dev/null >/dev/null 2>&1; lerd artisan migrate --force </dev/null >/dev/null 2>&1)
(cd "$grp" && lerd db:create "$(grep '^DB_DATABASE=' .env | cut -d= -f2)" </dev/null >/dev/null 2>&1 && lerd artisan migrate --force </dev/null >/dev/null 2>&1)
check "lerd group add $name admin" bash -c "cd $grp && lerd group add '$name' admin"
expect_200 "$scheme://admin.$(site_host "$DEMO_DIR")"
check "lerd group db share" bash -c "cd $grp && lerd group db share"
check "lerd group db separate" bash -c "cd $grp && lerd group db separate"
check_out "lerd group list" "admin" lerd group list
check "lerd group remove" bash -c "cd $grp && lerd group remove"
check_out "8.9 [partial] group remove restores a standalone domain" 'grp' bash -c "cd $grp && lerd which"

ws=vmws$$
check "lerd workspace add" lerd workspace add "$ws"
check "lerd workspace assign" lerd workspace assign "$name" "$ws"
check "the dashboard groups the site under it" bash -c "curl -s $api/workspaces | grep -q '$ws'"
check "lerd workspace move" lerd workspace move "$ws" 0
check "lerd workspace rename" lerd workspace rename "$ws" "${ws}b"
check_out "lerd workspace list shows it" "${ws}b" lerd workspace list
check "lerd workspace rm" lerd workspace rm "${ws}b"
check_not "8.10 [partial] the dashboard no longer has it" "${ws}" curl -s "$api/workspaces"

share_out=$(lerd lan:share </dev/null 2>&1)
echo "$share_out"
lan_url=$(grep -Eo 'http://[0-9.]+:[0-9]+' <<<"$share_out" | head -1)
check_out "lan:share prints a URL and a QR" '█|▀|▄' echo "$share_out"
lan_port=${lan_url##*:}
expect_200 "http://$lan_ip:$lan_port"
check_not "8.11 assets load from the LAN address (URL rewriting)" "https?://$(site_host "$DEMO_DIR")" curl -s "http://$lan_ip:$lan_port"
check "lerd lan:unshare" lerd lan:unshare
check_out "8.12 lan:unshare releases the port" '^000$' code "http://$lan_ip:$lan_port"

# A secured Inertia starter with Ziggy, shared on the LAN over plain http.
inert=$PROJECTS/inert
# Each step runs on its own and into the log, so one that fails is named
# rather than silently skipping the link after it.
if [ ! -d "$inert" ]; then
	(cd "$PROJECTS" && lerd composer create-project laravel/react-starter-kit inert --no-interaction </dev/null 2>&1 | tail -5)
	(cd "$inert" && lerd composer require tightenco/ziggy --no-interaction </dev/null 2>&1 | tail -5)
	grep -q '@routes' "$inert/resources/views/app.blade.php" || sed -i 's|<head>|<head>\n        @routes|' "$inert/resources/views/app.blade.php"
	(cd "$inert" && lerd link </dev/null 2>&1 | tail -5)
	(cd "$inert" && lerd setup --all --skip-open </dev/null 2>&1 | tail -5)
fi
(cd "$inert" && lerd link </dev/null >/dev/null 2>&1)
(cd "$inert" && lerd secure </dev/null >/dev/null 2>&1)
ishare=$(cd "$inert" && lerd lan:share </dev/null 2>&1 | tee /dev/stderr | grep -Eo 'http://[0-9.]+:[0-9]+' | head -1)
echo "inert share -> ${ishare:-none}"
iport=${ishare##*:}
jar=/tmp/lerd-vm-jar
rm -f "$jar"
page=$(curl -s -c "$jar" "http://$lan_ip:$iport/login")
check_not "Ziggy's routes point at the LAN address, not https" '"url":"https://|^no page$' echo "${page:-no page}"
xsrf=$(awk '$6 == "XSRF-TOKEN" {print $7}' "$jar" | python3 -c 'import sys, urllib.parse; print(urllib.parse.unquote(sys.stdin.read().strip()))')
post=$(curl -s -o /dev/null -w '%{http_code}' -b "$jar" -c "$jar" -H "X-XSRF-TOKEN: $xsrf" -H 'X-Inertia: true' -H 'Accept: text/html' \
	--data 'email=nobody%40example.com&password=wrong' "http://$lan_ip:$iport/login")
echo "login post -> $post"
check_not "8.13 a form post over the LAN share is not refused with 419" '^(419|000)$' echo "$post"
(cd "$inert" && lerd lan:unshare </dev/null >/dev/null 2>&1)
rm -f "$jar"

check "lerd lan:expose" lerd lan:expose
check_out "lan:status reports exposed" 'expos' lerd lan:status
check "lan:services on" lerd lan:services on
check "lan:services off" lerd lan:services off
check "lerd lan:unexpose" lerd lan:unexpose
check_out "8.14 [partial] lan:status reports loopback only" 'loopback|not exposed|unexposed' lerd lan:status

check_out "lerd remote-setup prints a one-time code" '[A-Za-z0-9]{8}' lerd remote-setup
check "lerd remote-setup --revoke" lerd remote-setup --revoke
check_out "remote-control full-access on" 'on|enabled' lerd remote-control full-access on
check_out "remote-control full-access status" 'on|enabled' lerd remote-control full-access status
check "remote-control full-access off" lerd remote-control full-access off
check_out "8.15 [partial] full-access status reports off" 'off|disabled' lerd remote-control full-access status
lerd lan:unexpose </dev/null >/dev/null 2>&1

check "lerd share:tool pinggy" lerd share:tool pinggy
check_out "share:tool records it" 'pinggy' lerd share:tool
check_out "share:token reports the providers" 'ngrok|pinggy' lerd share:token
check_out "share:domain reports the base domain" '.' lerd share:domain
share_log=/tmp/lerd-vm-share.log
(setsid lerd share </dev/null >"$share_log" 2>&1 &)
public=""
for _ in $(seq 60); do
	public=$(grep -Eo 'https://[a-zA-Z0-9.-]+pinggy[a-zA-Z0-9.-]*' "$share_log" | head -1)
	[ -n "$public" ] && break
	sleep 1
done
check_not "8.18 lerd share uses the recorded tool without asking" '\[y/N\]|\[Y/n\]|choose a' cat "$share_log"
expect_200 8.19 "${public:-https://no-public-url.invalid}"
pkill -f 'lerd share' 2>/dev/null
lerd share:tool auto </dev/null >/dev/null 2>&1
rm -f "$share_log"

check "lerd stripe:config" lerd stripe:config --path /stripe/webhook --secret-env-key STRIPE_SECRET
grep -q '^STRIPE_SECRET=' .env || echo 'STRIPE_SECRET=sk_test_vmdummy' >>.env
check "lerd stripe:listen starts the listener" lerd stripe:listen
sleep 5
check_not "the listener does not fail on a missing flag" 'must specify|--events' bash -c "journalctl --user -u 'lerd-stripe-$name*' --since '-1min'"
check_out "the unit is the listener, running" '^(active|activating)$' bash -c "systemctl --user is-active \$(systemctl --user list-units 'lerd-stripe-$name*' --no-legend --plain | awk '{print \$1; exit}')"
check "8.20 lerd stripe:listen stop" lerd stripe:listen stop
skip "stripe webhooks reaching the app" "needs a real Stripe test key"

check "lerd nginx edit adds a location-scope block" nginx_with "$name" 'add_header X-Lerd-VM location always;'
# nginx edit returns a moment before the reload serves, so allow it a few seconds.
has_header() { curl -skI "$url" | grep -qi 'x-lerd-vm: location'; }
check "the block is served" wait_for 10 has_header
check "lerd restart" lerd restart "$name"
check_out "the block survives a restart" 'x-lerd-vm: location' curl -skI "$url"
check "lerd nginx reset --location" lerd nginx reset --location "$name"
check "reset puts it back" wait_for 10 bash -c "! curl -skI '$url' | grep -qi x-lerd-vm"
expect_200 "8.21 [partial]" "$url"
broken=$(nginx_with "$name" 'bogus_vm_directive on;' 2>&1)
echo "$broken"
check_out "the error names the directive and its line" 'bogus_vm_directive.*:[0-9]+' echo "$broken"
check_out "8.22 a broken directive is rolled back" 'rolled back' echo "$broken"
check_not "the broken override did not stay" 'bogus_vm_directive' lerd nginx show --location "$name"
expect_200 "$url"
lerd nginx reset --location "$name" </dev/null >/dev/null 2>&1
rm -f /tmp/lerd-vm-editor.sh

# Registry-only state survives a re-link: share a LAN port and pause first.
lerd lan:share </dev/null >/dev/null 2>&1
lerd pause "$name" </dev/null >/dev/null 2>&1
before=$(site_entry "$name")
lerd link </dev/null >/dev/null 2>&1
after=$(site_entry "$name")
echo "before: $before"
echo "after:  $after"
check "8.23 a re-link keeps everything only the registry knew" python3 -c "
import json, sys
b, a = json.loads(sys.argv[1]), json.loads(sys.argv[2])
lost = {k: v for k, v in b.items() if k not in ('framework', 'framework_version') and a.get(k) != v}
print('changed:', lost)
sys.exit(1 if lost else 0)" "$before" "$after"
lerd unpause "$name" </dev/null >/dev/null 2>&1
lerd lan:unshare </dev/null >/dev/null 2>&1
expect_200 "$url"

# A Sail-shaped project imported into lerd's services.
sail=$PROJECTS/sailapp
if [ ! -d "$sail" ]; then
	(cd "$PROJECTS" && lerd new sailapp </dev/null >/dev/null 2>&1)
fi
(cd "$sail" && lerd link </dev/null >/dev/null 2>&1)
if ! ls "$sail"/*compose.y*ml >/dev/null 2>&1; then
	(cd "$sail" && lerd composer require laravel/sail --dev --no-interaction </dev/null >/dev/null 2>&1
		lerd artisan sail:install --with=mysql --no-interaction </dev/null >/dev/null 2>&1)
fi
# The import needs a compose provider, as its documented prerequisites say.
if ! podman compose version >/dev/null 2>&1; then
	if have dnf; then sudo dnf install -y -q podman-compose >/dev/null 2>&1; elif have apt-get; then sudo apt-get install -y -qq podman-compose >/dev/null 2>&1; fi
fi
check "a compose provider is available" podman compose version
skip "8.24 lerd sail / lerd import sail" "skipped by the maintainer for this release"

# A non-PHP project served from its own Containerfile.
cc=$PROJECTS/ccapp
mkdir -p "$cc/site"
echo 'vm custom container' >"$cc/site/index.html"
printf 'FROM docker.io/library/busybox:latest\nCOPY site /srv\nCMD ["httpd", "-f", "-p", "8000", "-h", "/srv"]\n' >"$cc/Containerfile.lerd"
printf 'container:\n  port: 8000\n' >"$cc/.lerd.yaml"
check "a Containerfile.lerd project links" bash -c "cd $cc && lerd link"
expect_200 "$(site_url "$cc")"
check "lerd rebuild" bash -c "cd $cc && lerd rebuild"
# The rebuilt container takes a moment to answer.
wait_for 60 bash -c "[ \"\$(curl -ks -o /dev/null -w '%{http_code}' '$(eval echo "$(site_url "$cc")")')\" = 200 ]"
expect_200 8.25 "$(site_url "$cc")"
