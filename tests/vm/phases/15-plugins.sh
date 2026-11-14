#!/usr/bin/env bash
# Phase 15, plugins. Stands on its own: it installs the build in ~/rc when lerd
# is gone (a full run reaches it after phase 13's uninstall) and links a plain
# PHP site of its own, so `vm.sh run <guest> 15` works on any guest.
source "$(dirname "$0")/../lib.sh"
need_lerd

SITE_DIR=$PROJECTS/vmplugsite
W=$HOME/lerd-vm/plugins-work
PDIR=$HOME/.local/share/lerd/plugins
DATA=$HOME/.local/share/lerd/plugin-data
UI=http://127.0.0.1:7073
CATPORT=18765

# A run left half way leaves plugins, a service and a catalog server behind.
cleanup() {
	for p in vmplug vmcopy vmbroken vmsvc vmcat vmnew vmsched; do lerd plugin remove "$p" </dev/null >/dev/null 2>&1; done
	lerd service remove vmplugcache </dev/null >/dev/null 2>&1
	pkill -f "http.server $CATPORT" 2>/dev/null
}
cleanup
rm -rf "$HOME/lerd-vm/plugins-work"
mkdir -p "$W" "$SITE_DIR"

# write_plugin <dir> <plugin.yaml body> [file body...]: a folder plugin; extra
# arguments come in pairs of a relative path and its body, made executable.
write_plugin() {
	local dir=$1
	mkdir -p "$dir/bin"
	printf '%s' "$2" >"$dir/plugin.yaml"
	shift 2
	while [ $# -ge 2 ]; do
		mkdir -p "$(dirname "$dir/$1")"
		printf '%s' "$2" >"$dir/$1"
		chmod +x "$dir/$1"
		shift 2
	done
}
mcp_tools() {
	printf '%s\n%s\n' \
		'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"lerd-vm","version":"1"}}}' \
		'{"jsonrpc":"2.0","id":2,"method":"tools/list"}' | timeout 60 lerd mcp 2>/dev/null | grep '"id":2'
}
export -f mcp_tools
# manager <python expression on p>: prints the vmplug row of the plugin
# manager's answer when the expression holds for it.
manager() {
	curl -fsS "$UI/api/plugin-manager" | python3 -c "import json,sys
p = next((x for x in json.load(sys.stdin) if x['name'] == 'vmplug'), {})
print(json.dumps(p)) if ($1) else sys.exit(1)"
}

check "15.1 lerd plugin new writes a plugin" lerd plugin new vmplug
check_out "15.1 lerd plugin list shows it loaded" 'vmplug +0\.1\.0' lerd plugin list

# The site is linked after the plugin exists, so its site.linked hook fires.
printf '<!doctype html><html><head><title>vmplug</title></head><body>plugins</body></html>\n' >"$SITE_DIR/index.php"
check "15.7 [partial] the plugin's site gets linked" bash -c "cd '$SITE_DIR' && lerd link"
host=$(site_host "$SITE_DIR")
name=$(site_name "$SITE_DIR")
check_out "15.7 the site.linked hook wrote to the plugin's data folder" "site\.linked .*\"site\":\"$name\"" cat "$DATA/vmplug/events.log"
check_out "15.7 the hook's run is logged as ok" 'site\.linked bin/on-event: ok' cat "$DATA/vmplug/hooks.log"

check_out "15.2 the CLI command gets the site inside the project" "Hello from vmplug, $name! \(via cli\)" \
	bash -c "cd '$SITE_DIR' && lerd vmplug:greet"
check_out "15.3 the API endpoint answers with the site" "Hello from vmplug, $name! \(via api\)" \
	curl -fsS "$UI/api/plugins/vmplug/greet?site=$name"
check_out "15.4 the MCP tool is listed" '"name":"vmplug_greet"' mcp_tools
check_out "15.4 the MCP tool answers" "Hello from vmplug, $name! \(via mcp\)" mcp_call vmplug_greet "{\"site\":\"$name\"}"
check_out "15.5 [partial] the plugin MCP tool returns the guide" 'Building a lerd plugin' mcp_call plugin '{"action":"guide"}'

check "15.6 [partial] lerd mcp:inject runs in the project" lerd mcp:inject --path "$SITE_DIR"
check "15.6 [partial] plugins.md sits beside lerd's skill" test -s "$SITE_DIR/.claude/skills/lerd/plugins.md"
check "15.6 [partial] the plugin's own skill is written" test -s "$SITE_DIR/.claude/skills/lerd-vmplug/SKILL.md"

(cd "$SITE_DIR" && lerd plugin page vmplug on </dev/null >/dev/null 2>&1)
check_out "15.8 the page script is in the site's HTML" '_lerd/plugins/vmplug/page\.js' curl -s "http://$host/"
expect_200 "15.8" "http://$host/_lerd/plugins/vmplug/page.js"
check_out "15.9 a page's call reaches the endpoint for the site" "Hello from vmplug, $name! \(via api\)" \
	curl -s -H 'Sec-Fetch-Site: same-origin' "http://$host/_lerd/plugins/vmplug/api/greet"
check_out "15.9 the same call from another site's page is refused" '^403$' \
	curl -s -o /dev/null -w '%{http_code}' -H 'Sec-Fetch-Site: cross-site' "http://$host/_lerd/plugins/vmplug/api/greet"
(cd "$SITE_DIR" && lerd plugin page vmplug off </dev/null >/dev/null 2>&1)
check_not "15.8 off takes the script out again" '_lerd/plugins/vmplug/page\.js' curl -s "http://$host/"

check "15.10 [partial] lerd plugin card switches the card on" bash -c "cd '$SITE_DIR' && lerd plugin card vmplug on"
check_out "15.10 the dashboard's API reports the card on" '"name":"vmplug"[^}]*"kind":"overview_card","on":true' \
	curl -fsS "$UI/api/page-plugins?site=$name"
expect_200 "15.10" "$UI/plugins/vmplug/ui/card.html"

check "15.11 the plugin manager lists what it adds and its icon" \
	manager "'overview_card' in p.get('adds', []) and p.get('icon') == '/plugins/vmplug/ui/icon.svg'"

cp -a "$PDIR/vmplug" "$W/vmcopy"
sed -i 's/^name: vmplug$/name: vmcopy/' "$W/vmcopy/plugin.yaml"
check "15.12 installing from a folder copies the plugin in" lerd plugin install "$W/vmcopy"
check "15.12 [partial] the copy is installed apart from its folder" test -f "$PDIR/vmcopy/plugin.yaml"
check_out "15.12 a second install of the same name is refused" 'already installed' lerd plugin install "$W/vmcopy"
lerd plugin remove vmcopy </dev/null >/dev/null 2>&1

other=macos
[ "$(uname -s)" = Darwin ] && other=linux
write_plugin "$W/vmphp" $'api_version: 1\nname: vmphp\ncommands:\n  - {name: x, run: bin/x}\n' bin/x $'#!/usr/bin/env php\n<?php echo 1;\n'
write_plugin "$W/vmplat" "api_version: 1"$'\n'"name: vmplat"$'\n'"platforms: [$other]"$'\n'
write_plugin "$W/vmnew" $'api_version: 1\nname: vmnew\nlerd: ">=99.0"\n'
write_plugin "$W/vmreq" $'api_version: 1\nname: vmreq\nrequires: [vm-missing-tool]\n'
write_plugin "$W/vmclash" $'api_version: 1\nname: vmclash\n'
mkdir -p "$W/vmclash/services"
printf 'name: redis\nimage: docker.io/library/redis:7.4\n' >"$W/vmclash/services/redis.yaml"
check_out "15.13 a PHP script is refused by name" 'php' lerd plugin install "$W/vmphp"
check_out "15.13 a platform this is not is refused by name" "runs only on $other" lerd plugin install "$W/vmplat"
# A dev build loads every plugin on purpose, so only a release build can refuse one.
# Dev is what plugins.Compatible calls it: past a tag, dirty, or no version number.
ver=$(lerd --version | awk '{print $3}')
if ! echo "$ver" | grep -Eq '^v?[0-9]+\.[0-9]+' || echo "$ver" | grep -Eq -- '-[0-9]+-g[0-9a-f]+(-dirty)?$|-dirty$'; then
	check "15.13 a dev build installs a plugin bound to a newer lerd" lerd plugin install "$W/vmnew"
	lerd plugin remove vmnew </dev/null >/dev/null 2>&1
else
	check_out "15.13 a newer lerd bound is refused by name" '99\.0' lerd plugin install "$W/vmnew"
fi
check_out "15.13 a missing requirement is refused by name" 'vm-missing-tool' lerd plugin install "$W/vmreq"
check_out "15.13 a service name lerd has is refused by name" 'already has: .*redis' lerd plugin install "$W/vmclash"
check_not "15.13 none of them was installed" 'vmphp|vmplat|vmnew|vmreq|vmclash' lerd plugin list

mkdir -p "$PDIR/vmbroken"
printf 'api_version: 1\nname: vmbroken\nhooks:\n  site.exploded: bin/x\n' >"$PDIR/vmbroken/plugin.yaml"
check_out "15.14 a broken manifest is listed with its reason" 'vmbroken.*site\.exploded' lerd plugin list
check_out "15.14 the other plugins keep working" "Hello from vmplug, $name!" bash -c "cd '$SITE_DIR' && lerd vmplug:greet"
check_out "15.5 the plugin MCP tool's list names the broken plugin's reason" 'site\.exploded' mcp_call plugin '{"action":"list"}'
lerd plugin remove vmbroken </dev/null >/dev/null 2>&1

# greet without a site exits 2, which the endpoint answers with a 500.
check_out "15.15 [partial] a failing endpoint answers 500" '^500$' \
	curl -s -o /dev/null -w '%{http_code}' "$UI/api/plugins/vmplug/greet"
check "15.15 it is recorded as the plugin's last failure" manager "p.get('failure', {}).get('what') == 'greet'"

# A service the plugin ships, on a redis image the guest already has when it can.
img=$(podman images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | grep -m1 '^docker.io/library/redis:')
if [ -z "$img" ] && hub_limited; then
	skip "15.16 a plugin's service" "Docker Hub refuses this network's pulls and no redis image is cached"
else
	img=${img:-docker.io/library/redis:7.4-alpine}
	write_plugin "$W/vmsvc" $'api_version: 1\nname: vmsvc\n'
	mkdir -p "$W/vmsvc/services"
	printf 'name: vmplugcache\nimage: %s\nports:\n  - "16399:6379"\ndata_dir: /data\ndescription: A cache the vmsvc plugin ships\n' "$img" >"$W/vmsvc/services/vmplugcache.yaml"
	check "15.16 [partial] the plugin with a service installs" lerd plugin install "$W/vmsvc"
	check_out "15.16 its service shows up in lerd service preset" 'vmplugcache' lerd service preset
	check "15.16 [partial] the service installs" lerd service preset vmplugcache
	check "15.16 [partial] the service starts" lerd service start vmplugcache
	check_out "15.16 the service is marked as the plugin's" 'vmplugcache.*\[plugin vmsvc\]' lerd service list
	check "15.16 the service answers" wait_for 60 bash -c 'podman exec lerd-vmplugcache redis-cli ping | grep -q PONG'
	check "15.16 [partial] the plugin is removed" lerd plugin remove vmsvc
	check_not "15.16 removing the plugin removed its service" 'vmplugcache' lerd service list
	check_not "15.16 and its unit is stopped" '^active' systemctl --user is-active lerd-vmplugcache
	check "15.16 its data is kept" test -d "$HOME/.local/share/lerd/data/vmplugcache"
	lerd service remove vmplugcache --purge </dev/null >/dev/null 2>&1
fi

# A local catalog: a git repository of the plugin and an index naming one commit.
cp -a "$PDIR/vmplug" "$W/catrepo"
sed -i 's/^name: vmplug$/name: vmcat/' "$W/catrepo/plugin.yaml"
git -C "$W/catrepo" init -q && git -C "$W/catrepo" add -A && git -C "$W/catrepo" -c user.email=t@t -c user.name=t commit -qm one
first=$(git -C "$W/catrepo" rev-parse HEAD)
echo later >"$W/catrepo/LATER" && git -C "$W/catrepo" add -A && git -C "$W/catrepo" -c user.email=t@t -c user.name=t commit -qm two
second=$(git -C "$W/catrepo" rev-parse HEAD)
mkdir -p "$W/catindex"
index() { printf '{"plugins":[{"name":"vmcat","description":"Lane catalog plugin","repo":"file://%s","commit":"%s"}]}\n' "$W/catrepo" "$1" >"$W/catindex/index.json"; }
index "$first"
(cd "$W/catindex" && setsid python3 -m http.server "$CATPORT" --bind 127.0.0.1 </dev/null >/dev/null 2>&1 &)
wait_for 10 curl -fsS "http://127.0.0.1:$CATPORT/index.json" >/dev/null
export LERD_PLUGIN_CATALOG=http://127.0.0.1:$CATPORT/index.json
# A run before this one cached its own copy of this index for a day; age it.
touch -d '2 days ago' "$HOME"/.local/share/lerd/plugin-catalog-*.json 2>/dev/null
check_out "15.17 lerd plugin search lists the local index's plugin" 'vmcat' lerd plugin search
check "15.17 [partial] lerd plugin install puts it in" lerd plugin install vmcat
check "15.17 the approved commit and nothing newer" bash -c "test -f '$PDIR/vmcat/plugin.yaml' && test ! -e '$PDIR/vmcat/LATER'"
index "$second"
# The index is cached for a day; age the copy so the next read fetches it.
touch -d '2 days ago' "$HOME"/.local/share/lerd/plugin-catalog-*.json
check "15.17 [partial] lerd plugin update moves to the newly approved commit" lerd plugin update vmcat
check "15.17 the update brought the new commit's files" test -f "$PDIR/vmcat/LATER"
lerd plugin remove vmcat </dev/null >/dev/null 2>&1
unset LERD_PLUGIN_CATALOG
pkill -f "http.server $CATPORT" 2>/dev/null

# Schedules: nothing runs until the user allows it, then lerd-ui runs it about
# once a minute, and off stops it again. Lerd checks once a minute, hence the waits.
write_plugin "$W/vmsched" $'api_version: 1\nname: vmsched\nschedules:\n  - {name: tick, every: 1m, run: bin/tick}\n' \
	bin/tick $'#!/bin/sh\necho "$LERD_SCHEDULE $LERD_SURFACE" >>"$LERD_PLUGIN_DATA/ticks"\necho ticked\n'
ticks=$DATA/vmsched/ticks
rm -f "$ticks" "$DATA/vmsched/schedule.log"
check "15.20 [partial] a plugin with a schedule installs" lerd plugin install "$W/vmsched"
check_out "15.20 its schedule waits for an answer" 'Waiting for your answer' lerd plugin schedule vmsched
sleep 75
check "15.20 nothing runs before it is allowed" test ! -e "$ticks"
check "15.20 [partial] lerd plugin schedule on allows it" lerd plugin schedule vmsched on
check "15.20 it runs with its name and the schedule surface" wait_for 150 grep -qx 'tick schedule' "$ticks"
check "15.20 its output lands in schedule.log" wait_for 10 grep -q ticked "$DATA/vmsched/schedule.log"
check "15.20 it runs again once its interval has passed" wait_for 150 bash -c "[ \$(wc -l <'$ticks') -ge 2 ]"
check_out "15.20 lerd plugin schedule reports it allowed" 'Allowed' lerd plugin schedule vmsched
check "15.20 [partial] off stops it" lerd plugin schedule vmsched off
before=$(wc -l <"$ticks")
sleep 130
check "15.20 nothing runs after off" test "$(wc -l <"$ticks")" -le "$((before + 1))"
lerd plugin remove vmsched </dev/null >/dev/null 2>&1
check "15.20 [partial] it installs again" lerd plugin install "$W/vmsched"
check_out "15.20 removing the plugin forgot the answer" 'Waiting for your answer' lerd plugin schedule vmsched
lerd plugin remove vmsched </dev/null >/dev/null 2>&1

check "15.18 [partial] lerd plugin remove takes the plugin away" lerd plugin remove vmplug
check "15.18 its data folder is kept" test -s "$DATA/vmplug/events.log"
check "15.6 [partial] lerd mcp:eject runs once the plugin is gone" lerd mcp:eject --path "$SITE_DIR"
check "15.6 the removed plugin's skill is taken away" test ! -e "$SITE_DIR/.claude/skills/lerd-vmplug/SKILL.md"

check "15.19 lerd plugin search against plugins.lerd.sh answers" lerd plugin search
expect_200 "15.19" "http://$host/"
cleanup
