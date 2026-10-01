#!/usr/bin/env bash
# Phase 3, the .localhost path and the DNS round trip. Leaves the guest in the
# mode it started in.
source "$(dirname "$0")/../lib.sh"
cd "$DEMO_DIR" || exit 1
start_mode=$(tld)
name=$(site_name "$DEMO_DIR")


# A site deliberately kept on plain HTTP, to prove it stays that way.
mkdir -p "$PROJECTS/plainsite/public" && echo '<?php echo "plain";' >"$PROJECTS/plainsite/public/index.php"
(cd "$PROJECTS/plainsite" && lerd link </dev/null >/dev/null 2>&1 && lerd unsecure "$(site_name "$PROJECTS/plainsite")" </dev/null >/dev/null 2>&1)
[ "$start_mode" = test ] && lerd secure "$name" </dev/null >/dev/null 2>&1

if [ "$start_mode" = test ]; then
	check "lerd dns:disable" lerd dns:disable
fi
check_out "3.1 lerd-dns is torn down and sites move to .localhost" 'lerd-dns=(inactive|failed|unknown).*\.localhost' bash -c "echo lerd-dns=\$(systemctl --user is-active lerd-dns) \$(lerd which | grep Site)"
check_out "3.2 lerd sites now shows the .localhost domain" "$name +[^ ]*\\.localhost" lerd sites
expect_200 3.3 "http://$(site_host "$DEMO_DIR")"
check_out "3.4 lerd secure refuses with a message about managed DNS" 'HTTPS requires lerd-managed DNS' lerd secure "$name"
tmp=$(mktemp -d "$HOME/lerd-vm-init.XXXX")
cp -r "$DEMO_DIR/composer.json" "$DEMO_DIR/artisan" "$tmp/" 2>/dev/null
init_out=$(cd "$tmp" && pty_output 8 lerd init --fresh)
echo "$init_out"
check "3.5 lerd init asks its questions but not the HTTPS one" bash -c 'grep -qiE "php|worker|service|framework" <<<"$1" && ! grep -qiE "https" <<<"$1"' _ "$init_out"
rm -rf "$tmp"
check_out "3.6 dns:check prints DNS managed externally" 'DNS managed externally' lerd dns:check
check_out "3.7 [partial] the dashboard's DNS status reads disabled" '"enabled": ?false' bash -c "curl -s http://127.0.0.1:7073/api/status | python3 -c 'import json,sys; print(json.dumps(json.load(sys.stdin)[\"dns\"]))'"
skip "3.8 the per-site HTTPS toggle is a muted lock with an explanation" "dashboard styling, phase 10's browser pass"

check "lerd dns:enable" lerd dns:enable
check_out "3.9 lerd-dns is back up and sites move to .test" 'lerd-dns=active.*\.test' bash -c "echo lerd-dns=\$(systemctl --user is-active lerd-dns) \$(lerd which | grep Site)"
check_out "3.10 [partial] a site that was HTTPS comes back HTTPS with .env synced" "^APP_URL=https://$(site_host "$DEMO_DIR")" grep ^APP_URL= .env
check_out "3.11 a site left on plain HTTP stays HTTP" "^$(site_name "$PROJECTS/plainsite") .* No " lerd sites
expect_200 3.12 "https://$(site_host "$DEMO_DIR")"

if [ "$start_mode" = localhost ]; then
	check "back to .localhost, as this guest started" lerd dns:disable
	expect_200 "http://$(site_host "$DEMO_DIR")"
fi
