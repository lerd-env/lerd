#!/usr/bin/env bash
# Opens a Windows guest's WSL distro to the Linux release phases: an sshd in
# the distro on port 2222, reachable through mirrored networking, the host's
# key, and passwordless sudo for its user, which the phases expect the way a
# Linux test guest has it. Run after run.sh installed Lerd with KEEP_INSTALLED=1.
#
#   wsl-ssh.sh <user@windows-host> [public-key]
set -uo pipefail

G=${1:?user@windows-host}
KEY=${2:-$(ls ~/.ssh/id_ed25519.pub ~/.ssh/id_rsa.pub 2>/dev/null | head -1)}
[ -f "$KEY" ] || { echo "no public key; pass one" >&2; exit 2; }
SSH=(ssh -o BatchMode=yes -o ConnectTimeout=10 "$G")
DISTRO=Ubuntu-26.04
PORT=2222

# as_root <script>: a script run as root in the distro, fed on stdin so no
# quoting crosses cmd.exe.
as_root() { "${SSH[@]}" "wsl -d $DISTRO -u root -- bash -s" <<<"$1" 2>/dev/null | tr -d '\0\r'; }

user=$(as_root 'id -nu 1000')
[ -n "$user" ] || { echo "no Linux user in $DISTRO; install Lerd first" >&2; exit 1; }
as_root "set -e
export DEBIAN_FRONTEND=noninteractive
# The port is set before the package installs: its setup starts sshd, and
# under mirrored networking port 22 already belongs to Windows' own sshd.
mkdir -p /etc/ssh/sshd_config.d
printf 'Port $PORT\nPasswordAuthentication no\n' > /etc/ssh/sshd_config.d/lerd-vm.conf
dpkg --configure -a >/dev/null 2>&1 || true
apt-get install -y -q openssh-server >/dev/null
install -d -m 700 -o $user -g $user /home/$user/.ssh
printf '%s\n' '$(cat "$KEY")' > /home/$user/.ssh/authorized_keys
chown $user:$user /home/$user/.ssh/authorized_keys && chmod 600 /home/$user/.ssh/authorized_keys
printf '$user ALL=(ALL) NOPASSWD:ALL\n' > /etc/sudoers.d/lerd-vm && chmod 440 /etc/sudoers.d/lerd-vm
# Ubuntu starts sshd from a socket whose port comes from the config.
systemctl daemon-reload
systemctl restart ssh.socket 2>/dev/null || systemctl restart ssh
systemctl enable ssh.socket >/dev/null 2>&1 || systemctl enable ssh >/dev/null 2>&1
echo ready"

# Mirrored networking puts the distro on the Windows address, behind both the
# Windows firewall and Hyper-V's own one for WSL.
"${SSH[@]}" "powershell -NoProfile -Command \"if (-not (Get-NetFirewallRule -Name lerd-vm-wsl-ssh -ErrorAction SilentlyContinue)) { New-NetFirewallRule -Name lerd-vm-wsl-ssh -DisplayName 'lerd vm: WSL ssh' -Direction Inbound -Protocol TCP -LocalPort $PORT -Action Allow | Out-Null }; if (-not (Get-NetFirewallHyperVRule -Name lerd-vm-wsl-ssh -ErrorAction SilentlyContinue)) { New-NetFirewallHyperVRule -Name lerd-vm-wsl-ssh -DisplayName 'lerd vm: WSL ssh' -Direction Inbound -VMCreatorId '{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}' -Protocol TCP -LocalPorts $PORT -Action Allow | Out-Null }\"" >/dev/null 2>&1

host=${G#*@}
# Every run builds a new distro with a new host key, so the last one's is dropped.
ssh-keygen -R "[$host]:$PORT" >/dev/null 2>&1
if ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new -p $PORT "$user@$host" true 2>/dev/null; then
	echo "ssh -p $PORT $user@$host reaches $DISTRO"
else
	echo "ssh -p $PORT $user@$host does not answer" >&2
	exit 1
fi
