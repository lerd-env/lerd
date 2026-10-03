#!/usr/bin/env bash
# Drives the release test plan's phases on test machines.
#
#   vm.sh list                       guests from guests.local
#   vm.sh up <guest>                 start a libvirt guest, wait for ssh, print its address
#   vm.sh down <guest>               sync and stop it (destroy when ACPI is ignored)
#   vm.sh reset <guest> [snapshot]   revert to the phase 0 baseline (default clean-no-lerd)
#   vm.sh push <guest> [repo-dir]    copy build/lerd, build/lerd-tray and install.sh to ~/rc
#   vm.sh run <guest> <phase|all>    run phases/NN-*.sh there; PASS/FAIL lines here, the rest in logs/
#   vm.sh ssh <guest> <cmd...>       run one command there
#   vm.sh status <guest>             where a run is now (phase, checks done, failures) and the finished phases
#   vm.sh coverage <guest>           plan checkboxes that guest's logs cover, and the ones they don't
#
# Each finished phase raises a short-lived desktop notification (notify-send)
# with its summary, and the run ends with one that stays when anything failed;
# LERD_VM_NOTIFY=0 turns them off.
#
# Knobs forwarded to the phases: LERD_CHANNEL=beta (install and update from the
# beta line), RUN_TIER3=1 (once-per-release items), FROM_VERSION=1.35.0 (phase
# 12 installs that release first), AFTER_REBOOT=1 (phase 11's second half),
# GITHUB_TOKEN (lerd update's release lookups; every guest shares the host's
# address, and GitHub allows 60 unauthenticated calls an hour).
#
# <guest> is a name from tests/vm/guests.local or any ssh target (user@host), so
# a contributor without these VMs can point it at their own machine. Only up,
# down and reset need libvirt.
set -uo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
GUESTS=$HERE/guests.local
LOGS=$HERE/logs
VIRSH=(virsh -c "${LERD_VM_LIBVIRT:-qemu:///system}")
SSH_OPTS=(-o ConnectTimeout=5 -o BatchMode=yes -o StrictHostKeyChecking=accept-new)

die() {
	echo "vm.sh: $*" >&2
	exit 2
}

# guest_field <guest> <n>: column n of the guest's line in guests.local.
guest_field() {
	[ -f "$GUESTS" ] || return 1
	awk -v g="$1" -v n="$2" '$1 == g { print $n; exit }' "$GUESTS"
}

guest_flag() { guest_field "$1" 4 | tr ',' '\n' | grep -qx "$2"; }

domain() {
	local d
	d=$(guest_field "$1" 3)
	[ -n "$d" ] && [ "$d" != "-" ] || die "$1 has no libvirt domain in guests.local"
	command -v virsh >/dev/null || die "virsh is not installed; up/down/reset need libvirt"
	echo "$d"
}

# target <guest>: the ssh target, resolving user@dhcp through libvirt's lease.
target() {
	local t
	t=$(guest_field "$1" 2)
	if [ -z "$t" ]; then
		[[ $1 == *@* ]] || die "unknown guest $1 (add it to guests.local or pass user@host)"
		echo "$1"
		return
	fi
	if [[ $t == *@dhcp ]]; then
		local ip
		ip=$("${VIRSH[@]}" domifaddr "$(domain "$1")" 2>/dev/null | awk '/ipv4/ { split($4, a, "/"); print a[1]; exit }')
		[ -n "$ip" ] || die "$1 has no DHCP lease yet"
		t=${t%@dhcp}@$ip
	fi
	echo "$t"
}

refuse_noreboot() {
	if guest_flag "$1" noreboot; then
		die "$1 is marked noreboot (it cannot boot unattended); refusing to $2 it"
	fi
}

cmd_list() {
	[ -f "$GUESTS" ] || die "no guests.local; copy guests.example to guests.local"
	grep -v '^#' "$GUESTS" | awk 'NF { printf "%-18s %-28s %-18s %s\n", $1, $2, $3, $4 }'
}

cmd_up() {
	local d
	d=$(domain "$1")
	"${VIRSH[@]}" domstate "$d" | grep -q running || "${VIRSH[@]}" start "$d" >/dev/null || die "could not start $d"
	for _ in $(seq 180); do
		if t=$(target "$1" 2>/dev/null) && ssh "${SSH_OPTS[@]}" "$t" true 2>/dev/null; then
			echo "$t"
			return 0
		fi
		sleep 1
	done
	die "$1 did not answer ssh within 3 minutes"
}

cmd_down() {
	local d t
	refuse_noreboot "$1" stop
	d=$(domain "$1")
	"${VIRSH[@]}" domstate "$d" | grep -q running || return 0
	t=$(target "$1" 2>/dev/null) && ssh "${SSH_OPTS[@]}" "$t" sync 2>/dev/null
	# Some guests ignore ACPI shutdown; the sync above made destroy safe.
	"${VIRSH[@]}" shutdown "$d" >/dev/null 2>&1
	for _ in $(seq 30); do
		"${VIRSH[@]}" domstate "$d" | grep -q 'shut off' && return 0
		sleep 1
	done
	"${VIRSH[@]}" destroy "$d" >/dev/null
}

cmd_reset() {
	local d snap=${2:-$(guest_field "$1" 5)}
	refuse_noreboot "$1" reset
	d=$(domain "$1")
	"${VIRSH[@]}" snapshot-revert "$d" "${snap:-clean-no-lerd}" --running || die "revert of $d to ${snap:-clean-no-lerd} failed"
	cmd_up "$1"
}

cmd_push() {
	local t repo=${2:-$(cd "$HERE/../.." && pwd)}
	t=$(target "$1")
	[ -x "$repo/build/lerd" ] || die "$repo/build/lerd missing; run make build first"
	ssh "${SSH_OPTS[@]}" "$t" 'mkdir -p ~/rc'
	scp -q "$repo/build/lerd" "$repo/install.sh" "$t:rc/"
	if [ -x "$repo/build/lerd-tray" ]; then scp -q "$repo/build/lerd-tray" "$t:rc/"; fi
	echo "pushed $("$repo/build/lerd" --version | head -1) to $t:~/rc"
}

# passthrough: the knobs phase scripts read, forwarded from this shell.
passthrough() {
	local v
	for v in LERD_CHANNEL RUN_TIER3 FROM_VERSION AFTER_REBOOT GITHUB_TOKEN FORCE; do
		[ -n "${!v:-}" ] && printf '%s=%q ' "$v" "${!v}"
	done
}

cmd_run() {
	local guest=$1 which=$2 t rc=0 phases=()
	t=$(target "$guest")
	if [ "$which" = all ]; then
		mapfile -t phases < <(ls "$HERE"/phases/[0-9][0-9]-*.sh)
	else
		mapfile -t phases < <(ls "$HERE"/phases/"$(printf '%02d' "$((10#$which))")"-*.sh 2>/dev/null)
	fi
	[ ${#phases[@]} -gt 0 ] || die "no phase script for $which"
	mkdir -p "$LOGS"
	ssh "${SSH_OPTS[@]}" "$t" 'mkdir -p ~/lerd-vm/phases'
	scp -q "$HERE/lib.sh" "$t:lerd-vm/"
	scp -q "${phases[@]}" "$t:lerd-vm/phases/"
	local n=0 total=${#phases[@]}
	for p in "${phases[@]}"; do
		local name log watcher
		name=$(basename "$p" .sh)
		n=$((n + 1))
		log=$LOGS/${guest//[@\/]/_}-$name.log
		: >"$log"
		echo "$name $n $total $(expected_checks "$p") $log" >"$LOGS/${guest//[@\/]/_}.current"
		echo "== $name on $guest (log: ${log#"$HERE"/})"
		notify brief "$guest: phase $n of $total" "$name started"
		progress_watcher "$guest" &
		watcher=$!
		ssh "${SSH_OPTS[@]}" "$t" "cd ~/lerd-vm && PHASE=$name $(passthrough) bash phases/$name.sh" >"$log" 2>&1
		kill "$watcher" 2>/dev/null
		grep -E '^(PASS|FAIL|SKIP|TODO|SUMMARY) ' "$log"
		local summary
		summary=$(grep '^SUMMARY ' "$log" || echo "SUMMARY $name: stopped before its summary, see the log")
		echo "$(date +%T) $summary" >>"$LOGS/${guest//[@\/]/_}.status"
		if grep -q '^FAIL ' "$log" || ! grep -q '^SUMMARY ' "$log"; then
			rc=1
		fi
		notify brief "$guest $name" "$summary"
	done
	rm -f "$LOGS/${guest//[@\/]/_}.current"
	# One notification that stays: the run as a whole.
	if [ $rc = 0 ]; then notify normal "$guest: passed" "$(basename "${phases[-1]}" .sh) done"; else notify critical "$guest: finished with failures" "vm.sh status $guest"; fi
	return $rc
}

# notify <urgency> <title> <body>: a desktop notification when the host has
# one; LERD_VM_NOTIFY=0 turns them off.
notify() {
	[ "${LERD_VM_NOTIFY:-1}" = 0 ] && return
	# "brief" pops up like a normal one but closes itself; KDE hides low urgency.
	local urgency=$1 expire=()
	if [ "$urgency" = brief ]; then
		urgency=normal
		expire=(-t 8000)
	fi
	command -v notify-send >/dev/null && notify-send -u "$urgency" "${expire[@]}" -a lerd-vm "lerd vm: $2" "$3" 2>/dev/null
	return 0
}

# cmd_coverage <guest>: which plan checkboxes that guest's logs cover.
cmd_coverage() {
	local g=${1//[@\/]/_}
	compgen -G "$LOGS/$g-*.log" >/dev/null || die "no logs for $1"
	python3 "$HERE/checkboxes.py" coverage "$LOGS/$g"-*.log
}

# expected_checks <script>: roughly how many results a phase prints, from the
# calls in it (a loop can print more, so it is an estimate).
expected_checks() { grep -cE '^[[:space:]]*(check|check_out|check_not|expect_200|expect_code|skip|todo|_pass|_fail|not_fresh)[[:space:]]' "$1"; }

# progress <guest>: one line on the phase running now, from its live log.
progress() {
	local cur=$LOGS/${1//[@\/]/_}.current name n total want log done fails last
	[ -f "$cur" ] || return 1
	read -r name n total want log <"$cur"
	done=$(grep -cE '^(PASS|FAIL|SKIP|TODO) ' "$log")
	fails=$(grep -c '^FAIL ' "$log")
	last=$(grep -E '^(PASS|FAIL|SKIP|TODO) ' "$log" | tail -1 | cut -c1-90)
	echo "phase $n of $total, $name: $done of ~$want checks, $fails failed; last: ${last:-starting}"
}

# progress_watcher <guest>: a short notification every two minutes while a
# phase runs, so a long phase still shows it is moving.
progress_watcher() {
	while sleep 120; do
		notify brief "$1 progress" "$(progress "$1")"
	done
}

cmd_status() {
	local f=$LOGS/${1//[@\/]/_}.status
	progress "$1" && echo
	[ -f "$f" ] || die "no runs recorded for $1"
	echo "finished phases:"
	tail -n 20 "$f"
}

[ $# -ge 1 ] || die "usage: vm.sh list|up|down|reset|push|run|status|coverage|ssh <guest> ..."
sub=$1
shift
case $sub in
list) cmd_list ;;
up) cmd_up "${1:?guest}" ;;
down) cmd_down "${1:?guest}" ;;
reset) cmd_reset "${1:?guest}" "${2:-}" ;;
push) cmd_push "${1:?guest}" "${2:-}" ;;
run) cmd_run "${1:?guest}" "${2:?phase or all}" ;;
status) cmd_status "${1:?guest}" ;;
coverage) cmd_coverage "${1:?guest}" ;;
ssh)
	t=$(target "${1:?guest}")
	shift
	ssh "${SSH_OPTS[@]}" "$t" "$@"
	;;
*) die "unknown command $sub" ;;
esac
