#!/usr/bin/env bash
# Drives lerd.exe and its shim twins from Windows with cli.ps1, in the signed-in
# desktop session: an SSH session cannot always reach \\wsl.localhost, which is
# where a user's PowerShell and cmd stand. Needs Lerd installed and the Linux
# phases' demo site linked.
#
#   run-cli.sh <user@host>
set -uo pipefail

G=${1:?user@host}
HERE=$(cd "$(dirname "$0")" && pwd)
LANE='C:\lerd-vm'
win() { ssh -o BatchMode=yes -o ConnectTimeout=10 "$G" "$@" 2>/dev/null | tr -d '\0\r'; }

win "mkdir $LANE" >/dev/null
scp -q "$HERE/cli.ps1" "$G:${LANE//\\//}/" || exit 2
win "del $LANE\\cli.log" >/dev/null
win "schtasks /create /f /tn lerd-vm-cli /it /rl highest /sc once /st 00:00 /tr \"powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -Command & $LANE\\cli.ps1 | Out-File -Encoding ascii $LANE\\cli.log\"" >/dev/null
win 'schtasks /run /tn lerd-vm-cli' >/dev/null
# The grid takes over 20 minutes on a 10-core guest; allow an hour.
for _ in $(seq 360); do
	win "type $LANE\\cli.log" | grep -q '^SUMMARY' && break
	sleep 10
done
out=$(win "type $LANE\\cli.log" | grep -E '^(PASS|FAIL|SUMMARY)')
echo "$out"
grep -q '^SUMMARY' <<<"$out" && ! grep -q '^FAIL' <<<"$out"
