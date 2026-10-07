#!/usr/bin/env bash
# Phase 7, git worktrees, through the wrapper, bare git and MCP.
source "$(dirname "$0")/../lib.sh"
need_demo
cd "$DEMO_DIR" || exit 1
name=$(site_name "$DEMO_DIR")
scheme=https
[ "$(tld)" = localhost ] && scheme=http
parent_url="$scheme://$(site_host "$DEMO_DIR")"
tag=$(date +%s | tail -c 5)
x=vmx$tag y=vmy$tag i=vmi$tag
wt() { printf '%s/%s-%s' "$DEMO_DIR" "$(basename "$DEMO_DIR")" "$1"; }
wt_url() { printf '%s://%s' "$scheme" "$(site_host "$(wt "$1")")"; }
db=$(grep '^DB_DATABASE=' .env | cut -d= -f2)
# The site's own database container: phase 5 may have moved it off lerd-mysql.
dbc=$(grep '^DB_HOST=' .env | cut -d= -f2)
case $dbc in lerd-*) ;; *) dbc=lerd-mysql ;; esac
mysql_q() { podman exec "$dbc" mysql -h127.0.0.1 -uroot -plerd -N -e "$1" 2>/dev/null; }
git rev-parse --git-dir >/dev/null 2>&1 || { git init -q && git add -A && git -c user.email=t@t -c user.name=t commit -qm init; }

# The questions come after the dependency install, so wait for them.
prompts=$(PTY_ENTER_AT='frontend assets' PTY_UNTIL='database|isolat' pty_output 300 lerd worktree add -b "vmp$tag")
echo "$prompts"
check "7.1 the wrapper prompts for DB isolation and the frontend build" bash -c 'grep -qiE "database|isolat" <<<"$1" && grep -qiE "build|asset|vite" <<<"$1"' _ "$prompts"
lerd worktree remove --force "$(wt "vmp$tag")" </dev/null >/dev/null 2>&1
git branch -q -D "vmp$tag" 2>/dev/null

check "lerd worktree add -b $x" lerd worktree add -b "$x"
check "7.2 the checkout lands inside the project and the parent stays clean" bash -c "test -d '$(wt "$x")' && ! git status --porcelain | grep -q '$x'"
check "7.9 lerd worktree wait returns once settled" lerd worktree wait "$(wt "$x")" --timeout 10m
check "7.3 dependencies install, env is seeded, a vhost appears" bash -c "test -d '$(wt "$x")/vendor' && test -f '$(wt "$x")/.env' && ls ~/.local/share/lerd/nginx/conf.d | grep -q '$x'"
check_out "7.4 lerd sites lists the worktree site" "$x" lerd sites
expect_200 7.5 "$(wt_url "$x")"
check_out "7.7 with a shared DB the worktree uses the parent's schema" "^DB_DATABASE=$db\$" grep ^DB_DATABASE= "$(wt "$x")/.env"
(cd "$(wt "$x")" && lerd worker start vite </dev/null >/dev/null 2>&1)
check_out "7.8 [partial] the worktree's asset worker is its own unit on its own port" "lerd-vite-$name-.*$x" bash -c "systemctl --user list-units 'lerd-vite-*' --no-legend"

mkdir -p "$(wt "$x")/emptypkg"
check "7.10 [partial] a worktree whose package.json declares nothing settles" bash -c "cd '$(wt "$x")' && lerd worktree wait '$(wt "$x")' --timeout 3m"

git worktree add -q "$(wt "$y")" -b "$y"
check "7.11 lerd worktree setup finishes a plain git worktree" bash -c "cd '$(wt "$y")' && lerd worktree setup --db share"
expect_200 "7.12 [partial]" "$(wt_url "$y")"

check "stop and start" bash -c 'lerd stop && lerd start'
expect_200 "$(wt_url "$x")"
expect_200 "7.13 [partial]" "$(wt_url "$y")"

check "lerd worktree remove the bare one" lerd worktree remove "$(wt "$y")"
check_out "7.14 [partial] units stop before git and nothing restart-loops" '^0$' bash -c "journalctl --user --since '-3min' -u 'lerd-*' | grep -c 'Scheduled restart' || true"

check "an isolated worktree: add" lerd worktree add -b "$i"
lerd worktree wait "$(wt "$i")" --timeout 10m </dev/null >/dev/null 2>&1
check "lerd db:isolate --source main" bash -c "cd '$(wt "$i")' && lerd db:isolate --source main"
idb=$(grep '^DB_DATABASE=' "$(wt "$i")/.env" | cut -d= -f2)
check "the isolated schema exists" bash -c "podman exec '$dbc' mysql -h127.0.0.1 -uroot -plerd -N -e 'SHOW DATABASES' 2>/dev/null | grep -qx '$idb'"
check "the worktree env points at it" bash -c "[ '$idb' != '$db' ]"
check "db_isolated is in the worktree's .lerd.local.yaml" grep -q 'db_isolated: true' "$(wt "$i")/.lerd.local.yaml"
check_not "7.6 the committed .lerd.yaml is untouched" 'lerd.yaml' git -C "$(wt "$i")" status --porcelain
check_out "7.17 db:isolate --source main clones into <parent_db>_<branch>" "^${db}_" echo "$idb"
check "7.18 a bare db:isolate starts the schema empty" bash -c "cd '$(wt "$i")' && lerd db:share && lerd db:isolate && [ \"\$(podman exec '$dbc' mysql -h127.0.0.1 -uroot -plerd -N -e 'SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=\"$idb\"' 2>/dev/null)\" = 0 ]"
check "7.20 lerd db:share puts the worktree back on the parent's schema" bash -c "cd '$(wt "$i")' && lerd db:share && grep -q '^DB_DATABASE=$db\$' .env"
expect_200 "$(wt_url "$i")"
check "isolate again from main" bash -c "cd '$(wt "$i")' && lerd db:isolate --source main"
check "7.19 db:isolate --source <branch> clones from another isolated worktree" bash -c "cd '$(wt "$x")' && lerd db:isolate --source '$i' && grep -q '^DB_DATABASE=${db}_' .env"
(cd "$(wt "$x")" && lerd db:share </dev/null >/dev/null 2>&1)
orig_port=$(podman port "$dbc" 3306/tcp 2>/dev/null | head -1 | awk -F: '{print $NF}')
check "lerd service port ${dbc#lerd-} 3317" lerd service port "${dbc#lerd-}" 3317
# On the container runtime the env names lerd-mysql:3306 inside the network,
# so moving the published port changes nothing; parent and worktree must agree.
check "7.21 [partial] after a port move the worktree's DB settings still match the parent's" bash -c "[ \"\$(grep -E '^DB_(HOST|PORT)=' '$(wt "$x")/.env')\" = \"\$(grep -E '^DB_(HOST|PORT)=' .env)\" ]"
expect_200 "$(wt_url "$x")"
lerd service port "${dbc#lerd-}" "${orig_port:-3306}" </dev/null >/dev/null 2>&1
expect_200 "$(wt_url "$x")"

check "remove the isolated one keeping its database" lerd worktree remove "$(wt "$i")"
check "7.15 [partial] the preserved schema is still there to reuse" bash -c "podman exec '$dbc' mysql -h127.0.0.1 -uroot -plerd -N -e 'SHOW DATABASES' 2>/dev/null | grep -qx '$idb'"
lerd worktree add "$(wt "$i")" "$i" </dev/null >/dev/null 2>&1
lerd worktree wait "$(wt "$i")" --timeout 10m </dev/null >/dev/null 2>&1
check_out "7.16 removing with drop-database on drops the schema" 'isolated_db_dropped\\?": ?true' mcp_call worktree "{\"action\":\"remove\",\"site\":\"$name\",\"branch\":\"$i\",\"keep_db\":false,\"force\":true}"
check_not "the schema is gone" "^$idb\$" bash -c "podman exec '$dbc' mysql -h127.0.0.1 -uroot -plerd -N -e 'SHOW DATABASES' 2>/dev/null"

echo 'vm-include' >vm-include.txt
grep -q '^vm-include.txt$' .gitignore 2>/dev/null || echo 'vm-include.txt' >>.gitignore
echo 'tools: {}' >mise.toml
grep -q '^mise.toml$' .gitignore || echo 'mise.toml' >>.gitignore
cp .lerd.yaml /tmp/lerd-vm-lerd.yaml
printf 'worktree_include:\n  - vm-include.txt\n  - mise.toml\n' >>.lerd.yaml
check "a worktree with worktree_include" lerd worktree add -b "vmw$tag"
lerd worktree wait "$(wt "vmw$tag")" --timeout 10m </dev/null >/dev/null 2>&1
check "7.22 paths under worktree_include are copied in" test -f "$(wt "vmw$tag")/vm-include.txt"
check "7.23 [partial] a gitignored mise.toml reaches the new worktree" test -f "$(wt "vmw$tag")/mise.toml"
lerd worktree remove --force "$(wt "vmw$tag")" </dev/null >/dev/null 2>&1
cp /tmp/lerd-vm-lerd.yaml .lerd.yaml
rm -f /tmp/lerd-vm-lerd.yaml vm-include.txt mise.toml

todo "7.24 a SQLite project's worktree gets its own database file" "needs the SQLite project fixture"
check_out "7.25 MCP worktree add returns ready with setup done" 'ready\\?": ?true' mcp_call worktree "{\"action\":\"add\",\"site\":\"$name\",\"branch\":\"vmm$tag\",\"base\":\"HEAD\"}"
expect_200 "$(wt_url "vmm$tag")"
todo "7.26 worktree.migrations picks share / clone / empty with its reason" "needs branches that add or lack migrations"

for b in "$x" "vmm$tag" "vmw$tag"; do lerd worktree remove --force "$(wt "$b")" </dev/null >/dev/null 2>&1; done
git branch -q -D "$x" "$y" "$i" "vmm$tag" "vmw$tag" 2>/dev/null
expect_200 7.27 "$parent_url"
