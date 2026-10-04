# Sourced by every phase script on the guest. Each check prints one line,
# PASS <name> or FAIL <name> (<why>); everything else a check prints is detail
# that vm.sh keeps in the log and out of the terminal. A check's name starts
# with the id of the plan checkbox it proves ("2.6 https answers 200"), which
# is how `vm.sh coverage` knows what is covered; `checkboxes.py list <phase>`
# prints the ids.
# shellcheck shell=bash

export PATH="$HOME/.local/bin:$PATH"
PASSES=0
FAILS=0
SKIPS=0
TODOS=0
PHASE=${PHASE:-$(basename "$0" .sh)}

_pass() { PASSES=$((PASSES + 1)); echo "PASS $1"; }
_fail() { FAILS=$((FAILS + 1)); echo "FAIL $1${2:+ ($2)}"; }

# check <name> <cmd...>: passes when the command exits 0.
check() {
	local name=$1 rc
	shift
	echo "--- $name: $*"
	"$@" </dev/null
	rc=$?
	if [ "$rc" = 0 ]; then _pass "$name"; else _fail "$name" "exit $rc"; fi
}

# check_out <name> <regex> <cmd...>: passes when the output matches the regex,
# whatever the exit status (a refusal is often the expected outcome).
check_out() {
	local name=$1 re=$2 out
	shift 2
	echo "--- $name: $*"
	out=$("$@" </dev/null 2>&1)
	echo "$out"
	# Matched line by line, then on the output joined into one line, so a
	# pattern spanning two lines of output still matches.
	if grep -Eiq -- "$re" <<<"$out" || grep -Eiq -- "$re" <<<"$(tr '\n' ' ' <<<"$out")"; then _pass "$name"; else _fail "$name" "no match for /$re/"; fi
}

# check_not <name> <regex> <cmd...>: passes when the output does not match.
check_not() {
	local name=$1 re=$2 out
	shift 2
	echo "--- $name: $*"
	out=$("$@" </dev/null 2>&1)
	echo "$out"
	if grep -Eiq -- "$re" <<<"$out"; then _fail "$name" "matched /$re/"; else _pass "$name"; fi
}

code() { curl -k -s -o /dev/null -w '%{http_code}' --max-time 30 "$1"; }

# expect_code [id] <url> <code...>: passes when the status is one of the codes;
# a leading checkbox id ("2.6") names the result after it.
expect_code() {
	local id="" url got
	if [[ $1 =~ ^[0-9]+\.[0-9]+( \[partial\])?$ ]]; then
		id="$1 "
		shift
	fi
	url=$1
	shift
	got=$(code "$url")
	echo "--- $url -> $got"
	for want in "$@"; do
		if [ "$got" = "$want" ]; then
			_pass "$id$url $got"
			return
		fi
	done
	_fail "$id$url" "got $got, want $*"
}

# expect_200 [id] <url>
expect_200() { expect_code "$@" 200; }

# skip <name> <reason>: only for a reason the release test plan names.
skip() { SKIPS=$((SKIPS + 1)); echo "SKIP $1 ($2)"; }

# todo <name> <why>: a checkbox with no script yet. Coverage counts it as not
# covered, so a gap is listed, never passed off as a skip.
todo() { TODOS=$((TODOS + 1)); echo "TODO $1 ($2)"; }

have() { command -v "$1" >/dev/null 2>&1; }

# wait_for <seconds> <cmd...>: retries the command once a second, bounded.
wait_for() {
	local n=$1
	shift
	for _ in $(seq "$n"); do
		"$@" >/dev/null 2>&1 && return 0
		sleep 1
	done
	return 1
}

# tld: the suffix sites are served under right now, test or localhost.
tld() {
	awk '/^dns:/{d=1;next} d&&/^[^ ]/{d=0} d&&/enabled:/{e=$2} d&&/tld:/{t=$2} END{if(e=="false")print "localhost"; else print (t?t:"test")}' \
		"${XDG_CONFIG_HOME:-$HOME/.config}/lerd/config.yaml" 2>/dev/null || echo test
}
host() { printf '%s.%s' "$1" "$(tld)"; }

# site_host <dir>: the domain lerd gave the project in dir (demo-2.test when a
# demo already existed), so no check assumes the folder name is the domain.
site_host() {
	local h
	h=$( (cd "$1" 2>/dev/null && lerd which 2>/dev/null) | awk '$1 == "Site" { sub(/,$/, "", $2); print $2; exit }')
	# lerd which does not answer inside a worktree; lerd sites lists it by path.
	[ -n "$h" ] || h=$(lerd sites 2>/dev/null | awk -v d="$1" '$NF == d { print ($1 == "↳" ? $3 : $2); exit }')
	printf '%s' "$h"
}

# site_name <dir>: the registered site name, which commands like lerd secure
# take; the domain minus its TLD.
site_name() { local h; h=$(site_host "$1"); printf '%s' "${h%.*}"; }

# db_container [dir]: the container holding the project's database, from its
# DB_HOST; a move (db:move, service migrate) can put it off lerd-mysql.
db_container() {
	local h
	h=$(grep -m1 '^DB_HOST=' "${1:-$DEMO_DIR}/.env" 2>/dev/null | cut -d= -f2)
	case $h in lerd-*) printf '%s' "$h" ;; *) printf 'lerd-mysql' ;; esac
}

# Shared state: phase 2 creates demo, phase 9 creates shop.
PROJECTS=$HOME/Projects
DEMO_DIR=$PROJECTS/demo
SHOP_DIR=$PROJECTS/shop

# pty_output <seconds> <cmd...>: what a command prints to a terminal, for
# wizards that only ask on one; stops early once PTY_UNTIL (a regex) shows up.
# The terminal gets a real size, or the prompt library draws nothing.
pty_output() {
	PTY_UNTIL=${PTY_UNTIL:-} PTY_ENTER_AT=${PTY_ENTER_AT:-} python3 - "$@" <<'PY'
import fcntl, os, pty, re, select, struct, sys, termios, time
secs = float(sys.argv[1]); pid, fd = pty.fork()
if pid == 0:
    fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    os.environ["TERM"] = "xterm-256color"
    os.execvp(sys.argv[2], sys.argv[2:])
until = os.environ.get("PTY_UNTIL")
# PTY_ENTER_AT: answer one question with its default (a single Enter) once
# this regex shows up, to reach the question after it.
enter_at, entered = os.environ.get("PTY_ENTER_AT"), False
out, end = b"", time.time() + secs
while time.time() < end:
    text = out.decode(errors="replace")
    if until and re.search(until, text, re.I):
        break
    if enter_at and not entered and re.search(enter_at, text, re.I):
        os.write(fd, b"\r")
        entered = True
    r, _, _ = select.select([fd], [], [], 0.5)
    if r:
        try: out += os.read(fd, 65536)
        except OSError: break
os.kill(pid, 9)
sys.stdout.write(out.decode(errors="replace"))
PY
}

# need_pyte: true once python3 can import pyte, installing the distro package
# when it is missing; tui_screen replays the TUI through it.
need_pyte() {
	python3 -c 'import pyte' 2>/dev/null && return 0
	if have apt-get; then sudo -n apt-get install -y -q python3-pyte >/dev/null 2>&1; fi
	if have dnf; then sudo -n dnf install -y -q python3-pyte >/dev/null 2>&1; fi
	if have pacman; then sudo -n pacman -S --noconfirm --needed python-pyte >/dev/null 2>&1; fi
	python3 -c 'import pyte' 2>/dev/null
}

# tui_screen <cols> <rows> <step...>: runs lerd tui in a pty of that size and
# prints the screen as a terminal would show it after the steps. The TUI only
# redraws what changed, so the raw stream is replayed through pyte rather than
# grepped. Steps: wait:<regex> (up to TUI_WAIT seconds, 30 by default, fails
# the run), keys:<text> with
# escapes (\x10 is ctrl+p, \t tab, \r enter, \x1b esc), sleep:<seconds>,
# until:<regex>:<keys> (sends the keys, up to 30 times, until the regex shows).
tui_screen() {
	python3 - "$@" <<'PY'
import codecs, fcntl, os, pty, re, select, struct, sys, termios, time
import pyte
cols, rows, steps = int(sys.argv[1]), int(sys.argv[2]), sys.argv[3:]
pid, fd = pty.fork()
if pid == 0:
    fcntl.ioctl(0, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
    # An unknown TERM keeps the renderer to cursor moves and plain text, which
    # pyte replays exactly; xterm's scroll and repeat shortcuts it gets wrong.
    os.environ["TERM"] = "vt220"
    os.execvp("lerd", ["lerd", "tui"])
screen = pyte.Screen(cols, rows)
stream = pyte.ByteStream(screen)
def pump(secs):
    end = time.time() + secs
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.1)
        if r:
            try: stream.feed(os.read(fd, 65536))
            except OSError: return
def text(): return "\n".join(line.rstrip() for line in screen.display)
ok = True
pump(1)
for step in steps:
    kind, _, arg = step.partition(":")
    if kind == "keys":
        for ch in codecs.decode(arg, "unicode_escape"):
            os.write(fd, ch.encode())
            pump(0.15)
    elif kind == "sleep":
        pump(float(arg))
    elif kind == "until":
        regex, _, keys = arg.rpartition(":")
        for _ in range(30):
            if re.search(regex, text()):
                break
            for ch in codecs.decode(keys, "unicode_escape"):
                os.write(fd, ch.encode())
            pump(0.4)
        if not re.search(regex, text()):
            print("TUI-TIMEOUT pressing %r for /%s/" % (keys, regex))
            ok = False
            break
    elif kind == "wait":
        end = time.time() + int(os.environ.get("TUI_WAIT", "30"))
        while not re.search(arg, text()) and time.time() < end:
            pump(0.3)
        if not re.search(arg, text()):
            print("TUI-TIMEOUT waiting for /%s/" % arg)
            ok = False
            break
pump(0.5)
print(text())
os.write(fd, b"\x03")
pump(0.5)
try: os.kill(pid, 9)
except OSError: pass
sys.exit(0 if ok else 1)
PY
}

# reclaim: frees images no container uses, so the PHP versions and services a
# phase tried out do not fill a small guest's disk before the phases after it.
reclaim() {
	podman image prune -af >/dev/null 2>&1
	echo "--- disk after reclaim: $(df -h / | awk 'NR==2 {print $4 " free"}')"
}

# hub_limited: true when Docker Hub refuses manifest requests (429) from this
# network, which is where lerd reads an image's size from.
hub_limited() {
	local t
	t=$(curl -s "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/alpine:pull" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' 2>/dev/null)
	[ "$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $t" https://registry-1.docker.io/v2/library/alpine/manifests/latest)" = 429 ]
}

# mcp_call <tool> <json-arguments>: one tools/call to lerd's MCP server over
# stdio, from the current directory; prints the response.
mcp_call() {
	printf '%s\n%s\n' \
		'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"lerd-vm","version":"1"}}}' \
		"{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" |
		timeout 900 lerd mcp 2>/dev/null | grep '"id":2'
}

_summary() { echo "SUMMARY $PHASE: $PASSES pass, $FAILS fail, $SKIPS skip, $TODOS todo"; }
trap _summary EXIT
