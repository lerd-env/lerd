#!/usr/bin/env bash
# Phase 17, a project that came from another tool gets its setup back. Stands on
# its own: it uninstalls lerd (keeping data) and installs the build in ~/rc.
source "$(dirname "$0")/../lib.sh"
need_demo
if [ ! -x "$HOME/rc/lerd" ] && [ ! -f "$HOME/lerd-install.sh" ]; then
	_fail "setup: no installer saved by phase 1 to reinstall from (vm.sh push, or rerun from phase 1)"
	exit 1
fi

DIR=$PROJECTS/herdshop
ORIG=$HOME/lerd-vm/herdshop.env
RESOLVER=/etc/resolver/test

# old_values_back: every key of the original has its old value in .env.
old_values_back() {
	local line rc=0
	while IFS= read -r line; do
		grep -Fxq -- "$line" "$DIR/.env" || { echo "not back: $line, now: $(grep "^${line%%=*}=" "$DIR/.env")"; rc=1; }
	done < <(grep -E '^[A-Z]' "$ORIG")
	return $rc
}
# put_on_lerd: the project is on lerd's values again, with a key of its own.
put_on_lerd() {
	(cd "$DIR" && lerd env) </dev/null
	grep -q '^ADDED_SINCE=' "$DIR/.env" || echo 'ADDED_SINCE=kept' >>"$DIR/.env"
}
on_lerd() { grep -q '^DB_HOST=lerd-' "$DIR/.env"; }
uninstall_answering() {
	pty_answers 'Continue\?.*\]=y' 'Remove all config.*\]=n' 'Remove MCP.*\]=n' 'lerd CA.*\]=n' 'Purge lerd.*\]=n' \
		'desktop app.*\]=n' "Put back the env values.*\\]=$1" 'press Enter to skip:=' -- lerd uninstall
}

(cd "$DIR" 2>/dev/null && lerd unlink) </dev/null >/dev/null 2>&1
rm -rf "$HOME/Projects/herdshop"
lerd uninstall --keep-data </dev/null >/dev/null 2>&1
[ -e "$HOME/.local/bin/lerd" ] && _fail "setup: lerd is still installed"
saved=$(mktemp -d)
for d in "$PROJECTS"/*/; do
	[ -f "$d/.env" ] && cp -p "$d/.env" "$saved/$(basename "$d")"
done

echo "--- setup: herdshop, with Herd's env"
cp -a "$DEMO_DIR" "$DIR"
rm -f "$DIR/.lerd.yaml" "$DIR/.lerd.local.yaml" "$DIR/.env.before_lerd"
[ -f "$DIR/.gitignore" ] && grep -v before_lerd "$DIR/.gitignore" >"$DIR/.gitignore.new" && mv "$DIR/.gitignore.new" "$DIR/.gitignore"
cat >"$ORIG" <<ENV
# moved from Herd
APP_NAME="Herd Shop"
APP_ENV=local
$(grep '^APP_KEY=' "$DEMO_DIR/.env")
APP_DEBUG=true
APP_URL=http://herdshop.test

DB_CONNECTION=mysql
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=herdshop
DB_USERNAME=root
DB_PASSWORD=

SESSION_DRIVER=file
CACHE_STORE=file
QUEUE_CONNECTION=sync
REDIS_HOST=127.0.0.1
REDIS_PASSWORD=null
REDIS_PORT=6379
MAIL_MAILER=smtp
MAIL_HOST=127.0.0.1
MAIL_PORT=2525
MAIL_USERNAME="\${APP_NAME}"
MAIL_FROM_ADDRESS="shop@example.com"

STRIPE_KEY=pk_test_123
SHOP_MOTTO='fast # and "cheap"'
ENV
cp "$ORIG" "$DIR/.env"

if [ "$(uname)" = Darwin ]; then
	sudo mkdir -p /etc/resolver
	echo 'nameserver 127.0.0.1' | sudo tee "$RESOLVER" >/dev/null
fi
out=$(reinstall </dev/null 2>&1) || _fail "setup: install"
if [ "$(uname)" = Darwin ]; then
	check_out "17.9 [partial] install warns the resolver belongs to another tool" 'belongs to another tool' echo "$out"
fi

check "setup: lerd link" bash -c "cd '$DIR' && lerd link"
put_on_lerd
check "17.1 the original is saved byte for byte" cmp "$ORIG" "$DIR/.env.before_lerd"
check "17.1 and gitignored" grep -qxF .env.before_lerd "$DIR/.gitignore"
check "17.2 the database points at lerd" on_lerd
check "17.2 the project's own keys are untouched" bash -c "grep -Fx 'STRIPE_KEY=pk_test_123' '$DIR/.env' && grep -F 'SHOP_MOTTO=' '$DIR/.env' | grep -Fxf <(grep SHOP_MOTTO '$ORIG')"
expect_200 17.3 "http://$(site_host "$DIR")"

check "17.4 env:restore runs" bash -c "cd '$DIR' && lerd env:restore"
check "17.4 env:restore puts every old value back" old_values_back
check "17.4 env:restore keeps the key added since" grep -qx 'ADDED_SINCE=kept' "$DIR/.env"
rm -f "$DIR/.env"
check "17.5 env:restore with no .env" bash -c "cd '$DIR' && lerd env:restore"
check "17.5 recreates it from the copy" cmp "$ORIG" "$DIR/.env"

put_on_lerd
before=$(cat "$DIR/.env")
out=$(uninstall_answering n 2>&1)
echo "$out"
check_out "17.6 uninstall names the project" "$DIR.*Put back the env values" echo "$out"
check_out "17.6 and prints where the copy stays" "still in .*herdshop" echo "$out"
check "17.6 answered no, .env is unchanged" test "$before" = "$(cat "$DIR/.env")"
check "17.6 and lerd is gone" bash -c '! test -e ~/.local/bin/lerd'

reinstall </dev/null >/dev/null 2>&1 || _fail "setup: second install"
check "setup: herdshop is linked after the reinstall" bash -c "cd '$DIR' && lerd link"
put_on_lerd
before=$(cat "$DIR/.env")
out=$(lerd uninstall --keep-data </dev/null 2>&1)
echo "$out"
check_not "17.7 --keep-data asks nothing" 'Put back the env values' echo "$out"
check_out "17.7 and prints where the copy stays" "still in .*herdshop" echo "$out"
check "17.7 .env is unchanged" test "$before" = "$(cat "$DIR/.env")"

reinstall </dev/null >/dev/null 2>&1 || _fail "setup: third install"
check "setup: herdshop is linked after the second reinstall" bash -c "cd '$DIR' && lerd link"
put_on_lerd
check "setup: herdshop is on lerd before the last uninstall" on_lerd
out=$(uninstall_answering y 2>&1)
echo "$out"
check_out "17.8 uninstall says it restored the project" "restored .*herdshop/\.env" echo "$out"
check "17.8 every old value is back" old_values_back
check "17.8 the key added since is kept" grep -qx 'ADDED_SINCE=kept' "$DIR/.env"
check_not "17.8 no copy is reported as left over" 'still in .*herdshop' echo "$out"
check "17.8 and lerd is gone" bash -c '! test -e ~/.local/bin/lerd'

if [ "$(uname)" = Darwin ]; then
	check "17.9 the resolver holds what it held before" bash -c "[ \"\$(cat $RESOLVER)\" = 'nameserver 127.0.0.1' ]"
else
	skip "17.9 macOS resolver given back" "macOS only"
fi

for f in "$saved"/*; do
	[ -f "$f" ] && [ "$(basename "$f")" != herdshop ] && cp -p "$f" "$PROJECTS/$(basename "$f")/.env"
done
reinstall </dev/null >/dev/null 2>&1 || _fail "setup: final install"
(cd "$DIR" && lerd unlink) </dev/null >/dev/null 2>&1
rm -rf "$HOME/Projects/herdshop"
