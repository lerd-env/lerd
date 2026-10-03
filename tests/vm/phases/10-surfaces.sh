#!/usr/bin/env bash
# Phase 10, surfaces: the TUI part, driven through a pty and read back as the
# screen a terminal would show. The dashboard, tray and other surfaces are
# signed off by hand.
source "$(dirname "$0")/../lib.sh"
cd "$DEMO_DIR" || exit 1
host=$(site_host "$DEMO_DIR")

if ! need_pyte; then
	todo "10.36 the TUI items" "python3 pyte is not installable here"
	exit 0
fi

# palette <query>: ctrl+p with a query typed, as the screen shows it.
palette() { tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:$1" "sleep:1"; }
export -f tui_screen palette

check_out "10.36 [partial] the sidebar lists the pages" 'Dashboard.*Databases.*PHP & Node.*Settings' \
	bash -c 'tui_screen 140 45 "wait:SERVICES" | tr "\n" " "'
check_out "10.36 [partial] sites and services carry running/total counts" 'SITES +[0-9]+/[0-9]+.*SERVICES +[0-9]+/[0-9]+' \
	bash -c 'tui_screen 140 45 "wait:SERVICES" | tr "\n" " "'
check_out "10.36 [partial] dns, nginx and the watcher sit at the foot with their state" 'dns +resolving.*nginx +running.*watcher +running' \
	bash -c 'tui_screen 140 45 "wait:watcher" | tr "\n" " "'
todo "10.37 a folded workspace shows a crashed worker" "needs a workspace and a worker made to crash"

systemctl --user stop lerd-watcher
check_out "10.38 a stopped watcher is listed under NEEDS ATTENTION" 'The watcher is stopped' \
	tui_screen 140 45 "wait:NEEDS ATTENTION"
tui_screen 140 45 "wait:The watcher is stopped" "keys:\\t" "keys:r" "wait:Everything is running" >/dev/null
check "10.38 r starts lerd and the watcher runs again" wait_for 30 systemctl --user is-active --quiet lerd-watcher
check_out "10.38 the dashboard returns to everything running" 'Everything is running' \
	tui_screen 140 45 "wait:Everything is running"
todo "10.39 a crashed worker card, r restarts it, H heals" "needs a worker made to crash"

site=$(tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:$host" "sleep:1" "keys:\\r" "wait:Doctor")
echo "$site"
check_out "10.40 [partial] the site header carries its URL and PHP version" "https?://$host.*php [0-9]" bash -c "tr '\n' ' ' <<<\"\$1\"" _ "$site"
check_out "10.40 [partial] the site has its five tabs" 'Overview +Logs +Env +Debug +Doctor' echo "$site"
check_out "10.40 [partial] 3 switches to the Env tab" 'APP_URL' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:$host" "sleep:1" "keys:\\r" "wait:Doctor" "keys:3" "wait:APP_URL"
todo "10.41 the Overview's reversible controls" "each control needs its own site state; drive by hand"

db=$(grep -m1 '^DB_DATABASE=' .env | cut -d= -f2)
dbs=$(tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:go to databases" "sleep:1" "keys:\\r" "wait:$db")
echo "$dbs"
check_out "10.42 [partial] Databases lists the site's database" "$db" echo "$dbs"
check_not "10.42 [partial] its _testing twin is folded into the same row" "^ *[^ ].*${db}_testing *$" echo "$dbs"

check_out "10.43 [partial] words match in any order" "Show Logs.*$host" bash -c "palette 'logs $host' | tr '\n' ' '"
check_out "10.43 [partial] the palette hands over to the lerd command prompt" 'Run lerd command' \
	tui_screen 140 45 "wait:Dashboard" "keys:\\x10" "wait:Go to or do" "keys:run a lerd command" "sleep:1" "keys:\\r" "wait:Run lerd command"
# Control: the same read finds a verb the palette does offer.
check_out "10.44 [partial] the palette read finds an action it offers" '\bRestart\b' bash -c 'palette restart | grep -v "›"'
for verb in remove drop unlink restore uninstall purge reset; do
	# The input line echoes the query, so only the entries below it are read.
	check_not "10.44 [partial] ctrl+p offers nothing for $verb" '\b(Remove|Drop|Unlink|Restore|Uninstall|Purge|Reset)\b' bash -c 'palette "$1" | grep -v "›"' _ "$verb"
done

check_not "10.45 below 96 columns the sidebar folds away" 'PHP & Node' tui_screen 90 40 "wait:Everything is running|NEEDS ATTENTION"
check_out "10.45 \\ opens the sidebar over the main area" 'PHP & Node' tui_screen 90 40 "wait:Everything is running|NEEDS ATTENTION" "keys:\\\\" "wait:PHP & Node"
check_out "10.45 below 60x12 only terminal too small is drawn" 'terminal too small' tui_screen 50 10 "sleep:2"
todo "10.46 colours follow the terminal's palette" "needs a real terminal with a light and a dark profile"
todo "10.47 dialogs over the dimmed screen, debug lenses, SPX hot function" "visual; drive by hand"
todo "10.48 shell drop-in and a service's web dashboard" "opens a shell and a browser; drive by hand"
