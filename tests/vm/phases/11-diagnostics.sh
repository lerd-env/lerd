#!/usr/bin/env bash
# Phase 11, diagnostics and housekeeping. The reboot and the late-NIC rig are
# tier 3 and need a guest that can reboot unattended.
source "$(dirname "$0")/../lib.sh"
cd "$DEMO_DIR" || exit 1
api=http://127.0.0.1:7073/api

# site_url <dir>: the site's URL on the scheme it is actually served on.
site_url() {
	local n tls
	n=$(site_name "$1")
	tls=$(lerd sites 2>/dev/null | awk -v n="$n" '$1 == n { print $5; exit }')
	if [ "$tls" = Yes ]; then printf 'https://%s' "$(site_host "$1")"; else printf 'http://%s' "$(site_host "$1")"; fi
}
url=$(site_url "$DEMO_DIR")
name=$(site_name "$DEMO_DIR")
db=$(grep '^DB_DATABASE=' .env | cut -d= -f2)
mysql_root() { podman exec "$(db_container)" mysql -h127.0.0.1 -uroot -plerd "$@" 2>/dev/null; }
rc_mark='# lerd-vm-path-probe'
fakebin=/tmp/lerd-vm-fakebin

# The site checks below need demo on MySQL.
sed -i 's/^DB_CONNECTION=.*/DB_CONNECTION=mysql/' .env
lerd env </dev/null >/dev/null 2>&1

check_not "lerd doctor is clean" '✗' lerd doctor
check "11.1 lerd doctor --json is well formed with fix tiers" bash -c 'lerd doctor --json | python3 -c "import json,sys; d=json.load(sys.stdin); assert d[\"findings\"]; [f for f in d[\"findings\"] if \"fix\" in f and \"tier\" in f[\"fix\"]] or True"'

systemctl --user stop lerd-nginx
check_out "doctor names a stopped nginx and hints lerd start" 'lerd start' lerd doctor
check "it offers no auto fix for it" bash -c 'lerd doctor --json | python3 -c "
import json, sys
d = json.load(sys.stdin)
bad = [f for f in d[\"findings\"] if \"nginx\" in f[\"name\"].lower() and f[\"status\"] != \"ok\" and (f.get(\"fix\") or {}).get(\"tier\") == 1]
sys.exit(1 if bad else 0)"'
check "lerd start brings it back" lerd start
expect_200 11.2 "$url"

check "lerd doctor --fix --dry-run previews" lerd doctor --fix --dry-run
check "11.3 lerd doctor --fix --yes applies them" lerd doctor --fix --yes
check_not "lerd doctor reports the machine only" 'Project Config|Env File|Migrations' lerd doctor
check_out "11.4 a site's findings come from lerd site:doctor" 'Project Config' lerd site:doctor

mysql_root -e "DROP DATABASE \`$db\`"
check_out "a dropped database is a site:doctor finding" '✗ Database' lerd site:doctor
check "lerd site:doctor --fix creates it" lerd site:doctor --fix
expect_200 11.5 "$url"

check_out "doctor reports containers resolving an internet name" 'internet DNS from containers' lerd doctor
gw=$(ip route show default | head -1)
sudo ip route del default 2>/dev/null
# A cached answer would still resolve, so drop the caches with the route.
sudo resolvectl flush-caches 2>/dev/null
todo "11.6 the doctor says so honestly with the network down" "cutting DNS on a libvirt guest also cuts ssh; the guest's resolver is on the local link"
[ -n "$gw" ] && sudo ip route add $gw 2>/dev/null

mkdir -p "$fakebin"
printf '#!/bin/sh\necho fake\n' >"$fakebin/php"
printf '#!/bin/sh\necho v0.0.0\n' >"$fakebin/node"
chmod +x "$fakebin/php" "$fakebin/node"
printf 'export PATH=%s:$PATH %s\n' "$fakebin" "$rc_mark" >>"$HOME/.bashrc"
check_out "doctor names a host php that leads the shim" "$fakebin/php leads" bash -ic 'lerd doctor' 
check "lerd path:disable" lerd path:disable
check_not "11.7 and stays quiet once the machine chose its own" 'php on PATH' bash -ic 'lerd doctor'
check "lerd path:enable" lerd path:enable
# path:enable puts lerd's line last again; put the fake node ahead of it.
sed -i "\|$rc_mark|d" "$HOME/.bashrc"
printf 'export PATH=%s:$PATH %s\n' "$fakebin" "$rc_mark" >>"$HOME/.bashrc"
check_out "11.8 a node ahead of the shim gets the Node message" "$fakebin/node leads.*Node" bash -ic 'lerd doctor'
sed -i "\|$rc_mark|d" "$HOME/.bashrc"
rm -rf "$fakebin"

# RustFS answers on its own domain; a site pointed there is wired.
lerd service start rustfs </dev/null >/dev/null 2>&1
grep -q '^AWS_ENDPOINT=' .env || echo 'AWS_ENDPOINT=' >>.env
python3 - <<'EOF'
import yaml
cfg = yaml.safe_load(open(".lerd.yaml")) or {}
svcs = cfg.get("services") or []
if "rustfs" not in [s if isinstance(s, str) else s.get("name") for s in svcs]:
    svcs.append("rustfs")
cfg["services"] = svcs
yaml.safe_dump(cfg, open(".lerd.yaml", "w"), sort_keys=False)
EOF
lerd env </dev/null >/dev/null 2>&1
endpoint=$(grep '^AWS_ENDPOINT=' .env)
echo "$endpoint"
check_not "a site on a service's own domain is not reported unwired" 'rustfs.*(unwired|not wired)' lerd site:doctor
lerd site:doctor --fix </dev/null >/dev/null 2>&1
check_out "11.9 the fix leaves the shared hostname alone" "^${endpoint//./\\.}\$" grep '^AWS_ENDPOINT=' .env

mysql_root -e "DROP DATABASE \`$db\`"
fix_out=$(curl -s -X POST -H 'X-Lerd-CSRF: 1' "$api/sites/$(site_host "$DEMO_DIR")/doctor/fix/database_create/run")
echo "$fix_out"
check_not "a dashboard doctor fix finds lerd on the daemon's PATH" 'exit status 127|127' echo "$fix_out"
check_out "11.10 and actually lands" "$db" mysql_root -e 'SHOW DATABASES'
lerd artisan migrate --force </dev/null >/dev/null 2>&1

check "lerd site:doctor --json on demo" bash -c 'lerd site:doctor --json | python3 -m json.tool >/dev/null'
check "11.11 lerd site:doctor --json on the second site" bash -c "cd $SHOP_DIR && lerd site:doctor --json | python3 -m json.tool >/dev/null"

if [ "$(tld)" = test ]; then
	# Stopping lerd-dns breaks .test while DNS stays enabled in the config.
	systemctl --user stop lerd-dns
	check_out "a broken .test setup shows in dns:check" '✗' lerd dns:check
	check "lerd dns:repair" lerd dns:repair
	check_not "11.12 dns:repair fixes it" '✗' lerd dns:check
else
	skip "11.12 lerd dns:repair" "a .localhost install has no .test setup to break"
fi

check_out "lerd check validates .lerd.yaml" '✓ Project Config' lerd check
cp .lerd.yaml /tmp/lerd-vm-lerd.yaml
printf 'php_version: [broken\n' >>.lerd.yaml
check_out "11.13 lerd check rejects a broken one" '✗ Project Config' lerd check
cp /tmp/lerd-vm-lerd.yaml .lerd.yaml
rm -f /tmp/lerd-vm-lerd.yaml

in_use() { podman ps --format '{{.Image}}' | sort -u; }
before_use=$(in_use)
podman tag docker.io/library/busybox:latest localhost/lerd-vm-orphan:1 2>/dev/null
dry=$(lerd cleanup --dry-run 2>&1)
echo "$dry"
check_out "11.17 [partial] the preview says at least rather than about" 'At least|Nothing to reclaim' echo "$dry"
check_not "the safe (unattended) tier leaves a tagged image nothing holds" 'lerd-vm-orphan' lerd cleanup --safe --dry-run
check "lerd cleanup --yes" lerd cleanup --yes
check_out "11.15 the interactive tier reaped the image nothing holds" '^$' bash -c 'podman images --format "{{.Repository}}" | grep lerd-vm-orphan || true'
check "11.14 no in-use image was touched" bash -c "for i in $(echo $before_use); do podman image exists \$i || { echo missing \$i; exit 1; }; done"
expect_200 "$url"
check "the database survived cleanup" bash -c "podman exec \$(grep -m1 ^DB_HOST= $DEMO_DIR/.env | cut -d= -f2) mysql -h127.0.0.1 -uroot -plerd -e 'SHOW DATABASES' 2>/dev/null | grep -q '^$db\$'"
cc=$PROJECTS/ccapp
if [ -d "$cc" ]; then
	ccunit=$(systemctl --user list-units 'lerd-custom-*' --no-legend --plain | awk '/ccapp/ {print $1; exit}')
	ccimage=$(podman images --format '{{.Repository}}:{{.Tag}}' | grep -m1 ccapp)
	[ -n "$ccunit" ] && systemctl --user stop "$ccunit"
	lerd cleanup --yes </dev/null >/dev/null 2>&1
	check "11.16 a stopped site's installed image stays protected" podman image exists "$ccimage"
	[ -n "$ccunit" ] && systemctl --user start "$ccunit"
else
	check "11.16 needs the custom container site phase 8 creates" test -d "$cc"
fi

check "11.18 the resources disk figure and its breakdown, heaviest first" bash -c "curl -s $api/disk | python3 -c '
import json, sys
d = json.load(sys.stdin)
def lists(o):
    if isinstance(o, list) and o and all(isinstance(x, dict) for x in o):
        yield o
    if isinstance(o, dict):
        for v in o.values(): yield from lists(v)
sizes = []
for l in lists(d):
    key = next((k for k in (\"bytes\", \"size\", \"Bytes\") if k in l[0]), None)
    if key: sizes = [x[key] for x in l]; break
print(d if not sizes else sizes)
sys.exit(0 if sizes == sorted(sizes, reverse=True) and sizes else 1)'"

check_out "lerd cleanup auto status" 'on|off|enabled|disabled' lerd cleanup auto status
check "lerd cleanup auto off" lerd cleanup auto off
check "lerd cleanup auto on" lerd cleanup auto on
check_out "11.19 auto cleanup is on again" 'on|enabled' lerd cleanup auto status

demo_host=$(site_host "$DEMO_DIR")
check "bug-report anonymizes names by default" bash -c "lerd bug-report -o /tmp/lerd-vm-report.txt && ! grep -q '$demo_host' /tmp/lerd-vm-report.txt"
check "11.20 --show-real-names keeps them" bash -c "lerd bug-report --show-real-names -o /tmp/lerd-vm-report.txt && grep -q '$demo_host' /tmp/lerd-vm-report.txt"
rm -f /tmp/lerd-vm-report.txt

start=$(date +%s)
check "lerd framework update" lerd framework update
took=$(($(date +%s) - start))
check "lerd framework update finishes in a couple of seconds (took ${took}s)" test "$took" -le 5
check "11.21 [partial] --diff shows the changes before applying" lerd framework update --diff

bkp=$HOME/.local/share/lerd/sites.bkp
check "sites.bkp holds at most ten versions" bash -c "[ \$(ls $bkp | wc -l) -le 10 ]"
n_before=$(ls "$bkp" 2>/dev/null | wc -l)
lerd link </dev/null >/dev/null 2>&1
check "a save that changes nothing takes no slot" test "$(ls "$bkp" | wc -l)" = "$n_before"
check_out "lerd sites:restore lists them" '.' lerd sites:restore --list
latest=$(ls -t "$bkp" | head -1)
check "lerd sites:restore puts one back" lerd sites:restore --force "$latest"
lerd start </dev/null >/dev/null 2>&1
expect_200 11.22 "$url"
expect_code "$(site_url "$SHOP_DIR")" 200 404

check_out "11.23 [partial] lerd tools:update brings the tools to their pins" 'composer|mkcert|up to date' lerd tools:update

check "lerd env:check" lerd env:check
check "lerd env:override writes a key" bash -c 'lerd env:override LERD_VM_PROBE=1 && lerd env >/dev/null && grep -q "^LERD_VM_PROBE=1" .env'
# env:restore puts back a pre-lerd .env, so make one: the example file, which
# lerd has never written to, then lerd env over it to take the backup.
cp .env /tmp/lerd-vm-env
rm -f .env.before_lerd
cp .env.example .env
lerd env </dev/null >/dev/null 2>&1
check "11.24 lerd env:restore round trips" bash -c 'test -f .env.before_lerd && lerd env:restore </dev/null && cmp -s .env .env.example'
cp /tmp/lerd-vm-env .env
rm -f /tmp/lerd-vm-env

key=$HOME/.ssh/lerd_vm_probe
[ -f "$key" ] || ssh-keygen -q -t ed25519 -N '' -f "$key"
check_out "lerd auth ssh loads a key" 'lerd_vm_probe|ED25519' bash -c "lerd auth ssh $key </dev/null; lerd auth ssh --list"
todo "11.25 lerd composer reaching a private repo" "needs a private repository and its deploy key"
lerd auth ssh --remove </dev/null >/dev/null 2>&1

[ -d "$HOME/.config/composer" ] && mv "$HOME/.config/composer" "$HOME/.config/composer.lerd-vm"
mkdir -p "$HOME/.composer"
had_auth=0
[ -f "$HOME/.composer/auth.json" ] && had_auth=1 && cp "$HOME/.composer/auth.json" /tmp/lerd-vm-auth.json
echo '{"http-basic":{"repo.lerd-vm.invalid":{"username":"vmuser","password":"vmpass"}}}' >"$HOME/.composer/auth.json"
check_out "11.26 lerd composer reads a global auth.json in ~/.composer" 'repo\.lerd-vm\.invalid' lerd composer config --global --list
if [ "$had_auth" = 1 ]; then mv /tmp/lerd-vm-auth.json "$HOME/.composer/auth.json"; else rm -f "$HOME/.composer/auth.json"; fi
[ -d "$HOME/.config/composer.lerd-vm" ] && mv "$HOME/.config/composer.lerd-vm" "$HOME/.config/composer"

check "lerd autostart enable" lerd autostart enable
check_out "lerd autostart status" 'enabled|on' lerd autostart status
check "lerd autostart disable" lerd autostart disable
check_not "11.27 with autostart off no worker is armed for boot" '^enabled' bash -c "systemctl --user is-enabled \$(systemctl --user list-unit-files 'lerd-queue-*' 'lerd-schedule-*' --no-legend | awk '{print \$1}') 2>/dev/null"
lerd autostart enable </dev/null >/dev/null 2>&1

check "lerd stop then lerd start" bash -c 'lerd stop && lerd start'
expect_200 11.28 "$url"
expect_code "$(site_url "$SHOP_DIR")" 200 404

if [ "${AFTER_REBOOT:-}" = 1 ]; then
	expect_200 11.29 "$url"
elif [ "${RUN_TIER3:-}" = 1 ]; then
	skip "11.29 reboot with autostart" "reboot the guest, then run phase 11 again with AFTER_REBOOT=1"
else
	skip "11.29 reboot with autostart" "tier 3, set RUN_TIER3=1"
fi
if [ "${RUN_TIER3:-}" = 1 ] && grep -q '^ID=fedora' /etc/os-release; then
	todo "11.30 late-NIC network rig" "not scripted yet: the rig, the reboot and the fake docker0 link up"
elif grep -q '^ID=fedora' /etc/os-release; then
	skip "11.30 late-NIC network rig" "tier 3, set RUN_TIER3=1"
else
	skip "11.30 late-NIC network rig" "the plan runs it on the fedora guest only"
fi

check "lerd quit" lerd quit
check_not "11.31 quit stops lerd-dns, the UI, the watcher and the tray" '^active$' bash -c 'for u in lerd-dns lerd-ui lerd-watcher lerd-tray lerd-nginx; do systemctl --user is-active $u; done'
lerd start </dev/null >/dev/null 2>&1
