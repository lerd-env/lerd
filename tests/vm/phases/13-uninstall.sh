#!/usr/bin/env bash
# Phase 13, uninstall keeping data, then reinstall on top and serve again.
# Destructive; runs last.
source "$(dirname "$0")/../lib.sh"
need_demo
mode_before=$(tld)
scheme=https
[ "$mode_before" = localhost ] && scheme=http
db=$(grep '^DB_DATABASE=' "$DEMO_DIR/.env" 2>/dev/null | cut -d= -f2)

reinstall() {
	if [ -x "$HOME/rc/lerd" ]; then
		bash "$HOME/rc/install.sh" --local "$HOME/rc/lerd"
	elif [ "${LERD_CHANNEL:-}" = beta ]; then
		curl -fsSL https://lerd.sh/install.sh | bash -s -- --beta
	else
		curl -fsSL https://lerd.sh/install.sh | bash
	fi
}

check_out "without a terminal or a flag, uninstall refuses and exits non-zero" 'aborted.*exit=[1-9]' bash -c 'lerd uninstall 2>&1 | tr "\n" " "; echo "exit=${PIPESTATUS[0]}"'
out=$(lerd uninstall --keep-data </dev/null 2>&1)
echo "$out"
check_out "13.1 uninstall stops every container and unit" '^0 0$' bash -c "echo \$(systemctl --user list-units 'lerd-*' --no-legend | wc -l) \$(podman ps -a --format '{{.Names}}' | grep -c '^lerd-')"
check "13.2 [partial] the sudoers rule and the mkcert CA are removed" bash -c '! sudo test -e /etc/sudoers.d/lerd && ! ls /usr/local/share/ca-certificates/ /etc/pki/ca-trust/source/anchors/ 2>/dev/null | grep -qi mkcert'
check "13.3 the sysctl drop-in is gone" bash -c '! test -e /etc/sysctl.d/99-lerd-ports.conf'
if [ -d /usr/share/omarchy ]; then
	check_not "13.4 lerd uninstall takes Lerd Glance off the bar" 'sh\.lerd\.glance' bash -c 'grep -rl "sh.lerd.glance" ~/.config 2>/dev/null'
else
	skip "13.4 Omarchy: Glance taken off the bar" "not an Omarchy guest"
fi
if systemctl is-active --quiet systemd-resolved; then
	skip "13.5 no DNS step drawn failed without systemd-resolved" "this guest runs systemd-resolved"
else
	check_not "13.5 no DNS step drawn failed without systemd-resolved" '✗.*DNS' echo "$out"
fi
if rpm -q lerd >/dev/null 2>&1 || dpkg -s lerd >/dev/null 2>&1; then
	check_out "13.6 a packaged binary stays and the removal command is printed" 'dnf remove|apt remove|brew uninstall' echo "$out"
else
	check "13.6 ~/.local/bin/lerd is gone on a script install" bash -c '! test -e ~/.local/bin/lerd'
fi
check_not "13.7 no lerd units, containers or networks remain" 'lerd' bash -c "systemctl --user list-units 'lerd-*' --no-legend; podman ps -a --format '{{.Names}}'; podman network ls --format '{{.Name}}'"
# The host's own lerd answers .test through libvirt's forwarder, so judge the
# guest's resolver setup rather than whether a .test name resolves.
check "13.8 [partial] the guest's .test resolver setup is gone and general DNS works" bash -c '! resolvectl domain 2>/dev/null | grep -q "~test" && ! test -e /etc/NetworkManager/dispatcher.d/99-lerd-dns && getent hosts lerd.sh'
check "13.9 project directories and .env files are untouched" test -f "$DEMO_DIR/.env"
check "13.10 keep my data keeps the databases, and the run says so" bash -c "test -d '$HOME/.local/share/lerd/data' && grep -qi 'kept' <<<\"\$1\"" _ "$out"
check_not "13.11 [partial] no unit is left failed behind the uninstall" 'lerd-' bash -c 'systemctl --user list-units --state=failed --no-legend'

check "reinstall on top" reinstall
check_out "the DNS mode is kept" "^$mode_before\$" tld
check "re-link the project" bash -c "cd '$DEMO_DIR' && lerd link"
expect_200 "$scheme://$(site_host "$DEMO_DIR")"
check_out "13.13 reinstalled: the re-linked project serves with its data intact" '[1-9]' bash -c "cd '$DEMO_DIR' && lerd artisan tinker --execute='echo DB::table(\"migrations\")->count();'"
echo "database: $db"


# --force wipes the data, so it runs last, after the keep-data checks above.
if [ "${FORCE:-}" = 1 ]; then
	check "13.12 lerd uninstall --force skips prompts" lerd uninstall --force
	check "reinstall after --force" reinstall
else
	skip "13.12 lerd uninstall --force on a second guest" "run phase 13 with FORCE=1 on the second guest"
fi
