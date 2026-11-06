#!/usr/bin/env bash
# The Windows installer lane: installs Lerd with Install-Lerd.exe on a Windows
# guest the way a person would, checks Windows and a site over HTTPS, then
# uninstalls and checks nothing is left. The guest runs OpenSSH with key auth
# for an admin user who is signed in at the console, since the wizard needs a
# desktop session.
#
#   run.sh <user@host> <Install-Lerd.exe> [local-dir] [keep|delete]
#
# local-dir is a folder on the guest holding install.sh, lerd and lerd.exe, to
# install an unreleased build (the installer's /local=); the last argument
# picks what the uninstall does with the distro, keep by default.
# KEEP_INSTALLED=1 stops after the site check, leaving Lerd installed for the
# Linux phases (wsl-ssh.sh, then vm.sh run against the distro).
set -uo pipefail
# report sets failed from the end of a pipe.
shopt -s lastpipe

G=${1:?user@host}
SETUP=${2:?path to Install-Lerd.exe}
LOCAL=${3:-}
DISTRO=${4:-keep}
HERE=$(cd "$(dirname "$0")" && pwd)
LANE='C:\lerd-vm'
SSH=(ssh -o BatchMode=yes -o ConnectTimeout=10 "$G")

win() { "${SSH[@]}" "$@" 2>/dev/null | tr -d '\0\r'; }
ps() { win "powershell -NoProfile -ExecutionPolicy Bypass $*"; }
# The installer keeps a distro from an earlier run, so ask rather than assume.
distro() { win 'type %LOCALAPPDATA%\lerd\bin\lerd-wsl.conf' | sed -n 's/^distro=//p'; }
wsl_user() { win "wsl -d $(distro) --cd ~ -- bash -lc \"$1\""; }

# task <name> <command>: an interactive scheduled task, so what it starts
# shows on the signed-in desktop with admin rights and no UAC prompt.
task() { win "schtasks /create /f /tn $1 /it /rl highest /sc once /st 00:00 /tr \"$2\"" >/dev/null; }

# report prints the check lines and remembers a FAIL, so the lane's exit status
# says whether the install held.
failed=0
report() {
	local out
	out=$(grep -E '^(PASS|FAIL|SUMMARY)')
	echo "$out"
	grep -q '^FAIL' <<<"$out" && failed=1
	return 0
}

# wait_result: the driver's last word, RESULT ..., within half an hour.
wait_result() {
	# A driver that died at launch logs its start line and nothing after; say so in
	# a minute and a half, not thirty.
	for _ in $(seq 9); do
		win "type $LANE\\drive.log" | grep -qv ' start ' && break
		sleep 10
	done
	win "type $LANE\\drive.log" | grep -qv ' start ' || { echo "FAIL the driver went quiet after launch: $(win "type $LANE\\drive.log" | tail -1)"; return 1; }
	for _ in $(seq 180); do
		win "type $LANE\\drive.log" | grep -q '^.* RESULT' && break
		sleep 10
	done
	win "type $LANE\\drive.log" | grep -E 'RESULT|retry'
	win "type $LANE\\drive.log" | grep -q 'RESULT \(finished\|uninstalled\)'
}

win "mkdir $LANE" >/dev/null
scp -q "$HERE/drive-installer.ps1" "$HERE/checks.ps1" "$SETUP" "$G:${LANE//\\//}/" || exit 2
exe="$LANE\\$(basename "$SETUP")"
# The lane drives a desktop; a display that sleeps would hide the wizard.
win 'powercfg /change monitor-timeout-ac 0' >/dev/null

echo "== install"
local_arg=${LOCAL:+ -Local $LOCAL}
task lerd-vm-install "powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File $LANE\\drive-installer.ps1 -Setup $exe -Log $LANE\\drive.log$local_arg"
win "del $LANE\\drive.log" >/dev/null
win 'schtasks /run /tn lerd-vm-install' >/dev/null
wait_result || exit 1
ps "-File $LANE\\checks.ps1" | report

echo "== site over HTTPS from Windows"
wsl_user "mkdir -p ~/Lerd/hello && printf '<?php echo \\\"hello from lerd\\\";' > ~/Lerd/hello/index.php && cd ~/Lerd/hello && lerd link >/dev/null && lerd secure hello >/dev/null" >/dev/null
got=$(ps "-Command \"(Invoke-WebRequest -UseBasicParsing -TimeoutSec 20 https://hello.test/).Content\"")
if [ "$got" = 'hello from lerd' ]; then echo 'PASS W.14 https://hello.test from Windows'; else echo "FAIL W.14 https://hello.test from Windows ($got)"; failed=1; fi

[ "${KEEP_INSTALLED:-}" = 1 ] && exit $failed

echo "== uninstall, $DISTRO the distro"
uninstaller=$(win "dir /b \"C:\\Program Files\\Lerd\\unins*.exe\"" | head -1)
flag=$([ "$DISTRO" = delete ] && echo ' -DeleteDistro')
task lerd-vm-uninstall "powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File $LANE\\drive-installer.ps1 -Uninstall C:\\Progra~1\\Lerd\\$uninstaller$flag -Log $LANE\\drive.log"
win "del $LANE\\drive.log" >/dev/null
win 'schtasks /run /tn lerd-vm-uninstall' >/dev/null
wait_result || exit 1
# The uninstaller deletes itself and its folder a moment after it exits.
for _ in $(seq 12); do
	ps '-Command "Test-Path $env:ProgramFiles\Lerd"' | grep -q False && break
	sleep 5
done
absent=$([ "$DISTRO" = delete ] && echo ' -DistroDeleted')
ps "-File $LANE\\checks.ps1 -Absent$absent" | report
exit $failed
