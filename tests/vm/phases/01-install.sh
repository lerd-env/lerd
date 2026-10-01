#!/usr/bin/env bash
# Phase 1, fresh install. Run it on a guest restored to its clean baseline
# (vm.sh reset); the fresh-install checkboxes fail on a guest that already has
# lerd, since they cannot be judged there. The published release goes in
# first through the public script (LERD_CHANNEL=beta follows the beta line),
# then the build pushed to ~/rc (vm.sh push) through the --local path.
source "$(dirname "$0")/../lib.sh"

# public_install: the one-line install a user copies from lerd.sh.
public_install() {
	if [ "${LERD_CHANNEL:-}" = beta ]; then
		wget -qO- https://lerd.sh/install.sh | bash -s -- --beta
	else
		wget -qO- https://lerd.sh/install.sh | bash
	fi
}
local_install() { bash "$HOME/rc/install.sh" --local "$HOME/rc/lerd"; }

fresh=0
have lerd || fresh=1
not_fresh() { _fail "$1" "lerd was already installed; vm.sh reset the guest for phase 1"; }

if [ "$fresh" = 1 ]; then
	check_not "1.1 ports 80, 443 and 5300 are free" ':(80|443|5300) ' ss -ltn
else
	not_fresh "1.1 ports 80, 443 and 5300 are free"
fi

# The fresh install is the build under test: the candidate pushed to ~/rc when
# there is one, otherwise the published release through the lerd.sh one-liner.
if [ -x "$HOME/rc/lerd" ]; then
	out=$(local_install </dev/null 2>&1)
	rc=$?
	want=$("$HOME/rc/lerd" --version | awk '{print $3}')
else
	out=$(public_install </dev/null 2>&1)
	rc=$?
	want=""
fi
echo "--- install (rc=$rc)"
echo "$out"
if [ -x "$HOME/rc/lerd" ]; then
	skip "1.2 the lerd.sh one-liner completes without a traceback" "the one-liner installs published releases; this run tests an unpublished build, rerun after publishing"
	if [ "$rc" = 0 ] && ! grep -Eq 'Traceback|panic:' <<<"$out"; then _pass "1.9 [partial] --local install completes"; else _fail "1.9 [partial] --local install completes" "rc=$rc"; fi
else
	if [ "$rc" = 0 ] && ! grep -Eq 'Traceback|panic:' <<<"$out"; then _pass "1.2 the lerd.sh one-liner completes without a traceback"; else _fail "1.2 the lerd.sh one-liner completes without a traceback" "rc=$rc"; fi
	skip "1.9 --local install completes" "no build pushed to ~/rc"
fi
# sudo caches the password, so the user is asked once as long as the first
# privileged step is the up-front system setup that says what it is for.
first_lock=$(grep -m1 '🔒' <<<"$out")
if [ "$fresh" = 0 ]; then
	not_fresh "1.3 sudo is asked once, up front, naming what it needs"
elif grep -q 'system setup' <<<"$first_lock"; then
	_pass "1.3 sudo is asked once, up front, naming what it needs"
else
	_fail "1.3 sudo is asked once, up front, naming what it needs" "first privileged step: ${first_lock:-none}"
fi
if [ "$fresh" = 0 ]; then
	not_fresh "1.4 the one-off bootstrap --system step is visible"
elif grep -q 'Applying system setup' <<<"$out"; then
	_pass "1.4 the one-off bootstrap --system step is visible"
else
	_fail "1.4 the one-off bootstrap --system step is visible" "no system setup step in the output"
fi
if [ "$fresh" = 0 ]; then
	not_fresh "1.14 [partial] an install that pulls names the images and their size first"
elif grep -Eq 'will download [0-9]+ images? \(~' <<<"$out"; then
	_pass "1.14 [partial] an install that pulls names the images and their size first"
else
	_fail "1.14 [partial] an install that pulls names the images and their size first" "no download disclosure"
fi

check_out "1.5 a new shell has lerd on PATH and --version prints the candidate" "^lerd version ${want:-[0-9]}" bash -lc 'lerd --version'
check_not "1.6 lerd status shows DNS, nginx and the watcher healthy, no update notice" '✗|update available|new version' lerd status
check_not "1.7 lerd doctor is clean" '✗' lerd doctor
if [ "$(tld)" = test ]; then
	check_not "1.8 lerd dns:check walks the chain and every layer is green" '✗' lerd dns:check
else
	check_out "1.8 lerd dns:check reports DNS managed externally" 'managed externally' lerd dns:check
fi

mode_before=$(tld)
out=$( { if [ -x "$HOME/rc/lerd" ]; then local_install; else public_install; fi; } </dev/null 2>&1)
echo "$out"
if ! grep -q 'Let lerd manage DNS' <<<"$out" && [ "$(tld)" = "$mode_before" ]; then
	_pass "1.10 re-running the installer asks no DNS question and keeps the mode"
else
	_fail "1.10 re-running the installer asks no DNS question and keeps the mode" "asked or mode now $(tld)"
fi

if [ "$fresh" = 1 ]; then
	check_not "1.11 [partial] a fresh install drives Node through mise, no missing fnm" 'fnm' bash -c 'lerd node:manager; lerd status'
else
	not_fresh "1.11 [partial] a fresh install drives Node through mise, no missing fnm"
fi

if [ -d /usr/share/omarchy ] || have omarchy-theme-set; then
	check_out "1.12 [partial] on Omarchy the tray is off and Lerd Glance is on the bar" 'sh\.lerd\.glance' bash -c 'systemctl --user is-enabled lerd-tray; grep -rl "sh.lerd.glance" ~/.config 2>/dev/null'
else
	skip "1.12 Omarchy tray off and Lerd Glance on the bar" "not an Omarchy guest"
fi

lerd stop </dev/null >/dev/null 2>&1
check_out "1.13 start --dry-run reports what it would pull and starts nothing" '(Nothing to download|would download).*nginx=(inactive|failed)' bash -c 'lerd start --dry-run 2>&1 | tr "\n" " "; echo nginx=$(systemctl --user is-active lerd-nginx)'
check "1.15 [partial] --no-pull and LERD_OFFLINE=1 still start a working stack" bash -c 'lerd start --no-pull && LERD_OFFLINE=1 lerd start'

if [ "${RUN_TIER3:-}" = 1 ]; then
	missing=""
	for v in 8.3 8.2 8.1 8.0; do lerd php:list 2>/dev/null | grep -q "$v" || { missing=$v; break; }; done
	gw=$(ip route show default | head -1)
	sudo ip route del default
	out=$(lerd fetch "$missing" </dev/null 2>&1)
	# shellcheck disable=SC2086
	sudo ip route add $gw
	echo "$out"
	if grep -Eq "php|$missing" <<<"$out" && ! lerd php:list 2>/dev/null | grep -q "$missing"; then
		_pass "1.16 a pull with the network down names the image and installs nothing"
	else
		_fail "1.16 a pull with the network down names the image and installs nothing" "see log"
	fi
	sudo sysctl -qw net.ipv6.conf.all.disable_ipv6=1
	lerd install --no-ipv6 </dev/null
	if [ -d "$DEMO_DIR" ]; then expect_200 1.17 "https://$(site_host "$DEMO_DIR")"; else check "1.17 with IPv6 off, lerd install --no-ipv6 brings the stack up" lerd status; fi
	sudo sysctl -qw net.ipv6.conf.all.disable_ipv6=0
	lerd install </dev/null >/dev/null 2>&1
else
	skip "1.16 a pull with the network down" "tier 3, set RUN_TIER3=1"
	skip "1.17 IPv6-off install" "tier 3, set RUN_TIER3=1"
fi

# The dashboard's Start is POST /api/lerd/start, streamed back one unit a line.
lerd stop </dev/null >/dev/null 2>&1
check_out "1.18 the dashboard's start streams back unit by unit" '"phase":"unit".*"phase":"done"' bash -c "curl -s -N -X POST -H 'X-Lerd-CSRF: 1' http://127.0.0.1:7073/api/lerd/start | tr '\n' ' '"
lerd start </dev/null >/dev/null 2>&1

if [ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]; then
	check_out "1.19 [partial] the Linux application entry cold-starts lerd" 'dashboard --splash' bash -c 'grep -h ^Exec= ~/.local/share/applications/*lerd*.desktop'
else
	skip "1.19 the Linux application entry cold-starts lerd" "no desktop session on this guest (only omarchy has one)"
fi
skip "1.20 the macOS app splash" "macOS only"
