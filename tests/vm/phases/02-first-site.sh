#!/usr/bin/env bash
# Phase 2, a real framework on a real site, and the HTTP/HTTPS toggle.
source "$(dirname "$0")/../lib.sh"

api() { curl -s -X "$1" -H 'X-Lerd-CSRF: 1' "http://127.0.0.1:7073$2"; }
# site_json <domain> <field>: one field of the site from the dashboard's API.
site_json() { api GET /api/sites | python3 -c "import json,sys; d=[s for s in json.load(sys.stdin) if s.get('domain')==sys.argv[1]]; print(json.dumps(d[0].get(sys.argv[2])) if d else 'missing')" "$1" "$2"; }

mkdir -p "$PROJECTS"
cd "$PROJECTS" || exit 1
created=0
if [ ! -d "$DEMO_DIR" ]; then
	check "2.1 lerd new scaffolds through the framework's own create command" lerd new demo
	created=1
else
	_fail "2.1 lerd new scaffolds through the framework's own create command" "demo already exists; vm.sh reset the guest for a fresh run"
fi
cd "$DEMO_DIR" || exit 1
if [ "$created" = 1 ]; then
	check_not "2.2 lerd setup --all runs every step without prompting" 'Continue with remaining steps|✗' lerd setup --all --skip-open
else
	_fail "2.2 lerd setup --all runs every step without prompting" "only judged on a fresh scaffold"
fi
if [ "$created" = 1 ]; then
	# lerd backs up only an env file the user had before lerd touched it; a fresh
	# scaffold has none, so any backup present must sit beside .env by its name.
	check "2.3 .env exists, and any pre-lerd backup is .env.before_lerd beside it" bash -c 'test -f .env && ! find . -maxdepth 3 -name "*.before_lerd" ! -path "./.env.before_lerd" -print | grep -q .'
else
	_fail "2.3 .env exists with its .env.before_lerd backup beside it" "only judged on a fresh scaffold"
fi
name=$(site_name "$DEMO_DIR")
host=$(site_host "$DEMO_DIR")
check_out "2.4 lerd sites lists the site with its PHP version and doc root" "$name +$host +8\\.[0-9].*$DEMO_DIR" lerd sites
check_out "2.5 lerd which resolves PHP, Node, doc root and nginx config" 'PHP.*[0-9].*Node.*Document root.*Nginx config' bash -c 'lerd which | tr "\n" " "'


if [ "$(tld)" = localhost ]; then
	expect_200 2.6 "http://$host"
	for id in 2.7 2.17 2.18 2.19 2.20 2.21 2.22 2.23 2.24 2.25 2.26 2.27 2.28 2.29 2.30 2.31 2.32 2.33 2.34 2.35 2.36 2.37; do
		skip "$id the HTTPS toggle" "a .localhost install has no HTTPS; phase 3 covers it"
	done
else
	lerd secure "$name" </dev/null >/dev/null 2>&1
	expect_200 2.6 "https://$host"
	check "2.7 the welcome page is served with a valid certificate" curl -sf -o /dev/null "https://$host"
fi
check_not "2.8 lerd site:doctor is clean" '✗' lerd site:doctor

check_out "2.9 [partial] lerd new refuses a name with spaces and suggests the slug" 'try "my-demo"' lerd new "My Demo"
mkdir -p "$PROJECTS/space dir/public" && echo '<?php echo "ok";' >"$PROJECTS/space dir/public/index.php"
(cd "$PROJECTS/space dir" && lerd link </dev/null >/dev/null 2>&1)
space_host=$(site_host "$PROJECTS/space dir")
check_out "a folder with spaces links with dashes in the domain" '^space-dir\.' echo "$space_host"
expect_code "http://$space_host" 200

# nvm with a prefix in ~/.npmrc: setup runs its npm steps and leaves the file alone.
if [ ! -s "$HOME/.nvm/nvm.sh" ]; then
	curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh | PROFILE=/dev/null bash >/dev/null 2>&1
fi
manager_before=$(lerd node:manager 2>/dev/null | awk '/manager:/ {print $NF}')
cp -f "$HOME/.npmrc" /tmp/lerd-vm-npmrc 2>/dev/null || : >/tmp/lerd-vm-npmrc
echo "prefix=$HOME/.npm-global" >>"$HOME/.npmrc"
cp "$HOME/.npmrc" /tmp/lerd-vm-npmrc-with-prefix
lerd node:manager nvm </dev/null >/dev/null 2>&1
out=$(lerd setup --all --skip-open --step "npm install" </dev/null 2>&1)
echo "$out"
if ! grep -q '✗' <<<"$out" && cmp -s "$HOME/.npmrc" /tmp/lerd-vm-npmrc-with-prefix; then
	_pass "2.10 with nvm and an npm prefix, setup runs its npm steps and leaves ~/.npmrc alone"
else
	_fail "2.10 with nvm and an npm prefix, setup runs its npm steps and leaves ~/.npmrc alone" "see log"
fi
cp /tmp/lerd-vm-npmrc "$HOME/.npmrc"
lerd node:manager "${manager_before:-mise}" </dev/null >/dev/null 2>&1

# A package that wants a service gets it suggested, by package, and Add wires it.
grep -q 'meilisearch/meilisearch-php' composer.json || lerd composer require meilisearch/meilisearch-php --no-interaction </dev/null >/dev/null 2>&1
# A guest that cannot be reset still has it wired from the last run, and a
# service the site already uses is never suggested.
api POST "/api/sites/$host/service:remove?name=meilisearch" >/dev/null 2>&1
check_out "2.11 [partial] the linked site suggests Meilisearch, naming the package" 'meilisearch.*meilisearch-php' site_json "$host" suggested_services
check_out "dashboard Add wires the suggestion into .env" '"ok": ?true' api POST "/api/sites/$host/service:add?name=meilisearch"
check_out "the Meilisearch key is in .env" '^MEILISEARCH_HOST=' grep ^MEILISEARCH_HOST= .env
skip "2.12 a suggested card is drawn dashed and faded" "dashboard styling, phase 10's browser pass"

# Local overrides: the local key wins and never reaches the committed file.
git rev-parse --git-dir >/dev/null 2>&1 || git init -q
git add -A >/dev/null 2>&1 && git -c user.email=t@t -c user.name=t commit -qm "before the local overrides" >/dev/null 2>&1
cp -f .lerd.yaml /tmp/lerd-vm-lerd.yaml 2>/dev/null || : >/tmp/lerd-vm-lerd.yaml
committed_php=$(awk -F'"' '/^php_version:/ {print $2}' .lerd.yaml)
echo 'php_version: "8.4"' >.lerd.local.yaml
check_not "2.13 a save from another command keeps the local key out of .lerd.yaml" '^php_version: "8\.4"' bash -c "lerd secure '$name' >/dev/null 2>&1; lerd link >/dev/null 2>&1; if [ -n '$committed_php' ] && [ '$committed_php' != 8.4 ]; then cat .lerd.yaml; fi"
check_out "2.14 a PHP version pinned only locally is the one the link applies and lerd which reports" 'PHP +8\.4' bash -c 'lerd link >/dev/null 2>&1; lerd which'
check_out "2.15 changing a key the local file owns says so" 'lerd.local.yaml sets php_version' lerd isolate 8.5
rm -f "$DEMO_DIR/.lerd.local.yaml"
lerd link </dev/null >/dev/null 2>&1
check_not "2.16 git status in the project is clean after all of it" '.' git status --porcelain

[ "$(tld)" = localhost ] && exit 0

check "lerd secure" lerd secure "$name"
expect_200 2.17 "https://$host"
check_out "2.18 http redirects to https" '^30[12] https://' bash -c "curl -s -o /dev/null -w '%{http_code} %{redirect_url}' http://$host"
check "2.19 https answers 200 without -k (mkcert CA trusted)" curl -sf -o /dev/null "https://$host"
check_out "2.20 APP_URL is https" "^APP_URL=https://$host" grep ^APP_URL= .env
check_not "2.21 [partial] no mixed content: the page loads no http:// asset" "(src|href)=\"http://$host" curl -sk "https://$host"
check_out "2.22 asset and route URLs in the page source are https" "https://$host" curl -sk "https://$host"

cp .env /tmp/lerd-vm-env-secured
check "2.23 lerd unsecure" lerd unsecure "$name"
expect_200 2.24 "http://$host"
check_not "2.25 https refuses cleanly: no redirect loop, no stale 200" '^(200|301|302)$' code "https://$host"
check_out "2.26 APP_URL is http" "^APP_URL=http://$host" grep ^APP_URL= .env
check_out "2.27 page source emits http:// URLs" "http://$host" curl -s "http://$host"
tls_now=$(site_json "$host" tls)
check_out "2.28 lerd sites and the dashboard both show the site as HTTP" "^$name .* No .*dashboard=false" bash -c "echo \$(lerd sites | grep -E '^$name ') dashboard=$tls_now"

check "2.29 lerd secure reissues the cert" lerd secure "$name"
expect_200 2.30 "https://$host"
expect_code "http://$host" 301 302
check_not "2.31 APP_URL is back to https and nothing else in .env was rewritten" '^[<>]' bash -c 'diff /tmp/lerd-vm-env-secured .env'
serial_before=$(echo | openssl s_client -connect "$host:443" -servername "$host" 2>/dev/null | openssl x509 -noout -serial)
lerd secure --renew "$name" </dev/null
served_serial() { echo | openssl s_client -connect "$host:443" -servername "$host" 2>/dev/null | openssl x509 -noout -serial; }
renewed() { [ "$(served_serial)" != "$serial_before" ]; }
check "2.32 lerd secure --renew reissues on demand" wait_for 15 renewed
expect_200 "https://$host"

check_out "dashboard HTTPS off" '"ok": ?true' api POST "/api/sites/$host/unsecure"
expect_200 "http://$host"
check_out "dashboard HTTPS on" '"ok": ?true' api POST "/api/sites/$host/secure"
expect_200 "2.33 [partial]" "https://$host"
# The TUI's toggle is the "Toggle HTTPS" entry under ctrl+p.
tui_https() { tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:toggle https $host" "sleep:1" "keys:\\r" "sleep:3" >/dev/null; }
if need_pyte; then
	tui_https
	expect_200 "2.34 [partial]" "http://$host"
	tui_https
	expect_200 "2.34" "https://$host"
else
	todo "2.34 the TUI inline toggle" "python3 pyte is not installable here"
fi

ok=1
for flip in unsecure secure unsecure; do
	lerd "$flip" "$name" </dev/null >/dev/null 2>&1
	if [ "$flip" = secure ]; then want=https; else want=http; fi
	[ "$(code "$want://$host")" = 200 ] || { ok=0; echo "flip $flip: $want not 200"; }
done
if [ "$ok" = 1 ]; then _pass "2.35 three flips without a restart, 200 after each"; else _fail "2.35 three flips without a restart, 200 after each" "see log"; fi

lerd secure "$space_host" >/dev/null 2>&1 || lerd secure "$(site_name "$PROJECTS/space dir")" </dev/null >/dev/null 2>&1
expect_200 "https://$space_host"
expect_200 2.36 "http://$host"
check "stop and start" bash -c 'lerd stop && lerd start'
expect_200 "https://$space_host"
expect_200 2.37 "http://$host"
lerd secure "$name" </dev/null >/dev/null 2>&1
lerd unsecure "$(site_name "$PROJECTS/space dir")" </dev/null >/dev/null 2>&1
expect_200 "https://$host"
