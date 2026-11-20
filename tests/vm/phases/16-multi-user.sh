#!/usr/bin/env bash
# Phase 16, two users on one machine. Stands on its own: it installs the build
# in ~/rc when lerd is gone, makes a second user with its own install, and
# drives logind's seat0 by giving each user a console login and switching VTs.
source "$(dirname "$0")/../lib.sh"
need_lerd

U2=lerdvm2
# Past logind's NAutoVTs (6): switching to a lower VT with no getty starts one,
# which takes the tty and ends the console login there.
SELF_VT=8
OTHER_VT=9
SELF_DIR=$PROJECTS/vmmuself

if ! loginctl show-seat seat0 >/dev/null 2>&1 || ! have chvt; then
	for i in 1 2 3 4 5 6; do skip "16.$i" "no seat0 to switch on this guest"; done
	exit 0
fi

# as_u2 <cmd...>: runs a command as the second user inside its own session bus,
# the way the deb's postinst runs lerd install for the user who ran apt.
as_u2() {
	sudo -u "$U2" env HOME="/home/$U2" USER="$U2" LOGNAME="$U2" \
		XDG_RUNTIME_DIR="/run/user/$UID2" DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$UID2/bus" \
		PATH="/home/$U2/.local/bin:/usr/local/bin:/usr/bin:/bin" \
		bash -c 'cd ~ && "$@"' _ "$@"
}

# seat_login <user> <vt> <unit>: a console login on the VT, so logind counts the
# user as on screen whenever that VT is the active one.
seat_login() {
	sudo systemctl stop "$3" 2>/dev/null
	sudo systemctl reset-failed "$3" 2>/dev/null
	sudo systemd-run --unit="$3" -p PAMName=login -p User="$1" -p TTYPath="/dev/tty$2" \
		-p StandardInput=tty -p StandardOutput=tty \
		-E XDG_SESSION_TYPE=tty -E XDG_SEAT=seat0 -E "XDG_VTNR=$2" /bin/sleep infinity >/dev/null
	wait_for 15 bash -c "loginctl list-sessions --no-legend | grep -Eq ' $1 +seat0 .*tty$2'"
}

serves() { curl -s --max-time 5 "http://$1/" | grep -q "$2"; }

# owns_ports <uid>: every port lerd needs is held by that user's lerd.
owns_ports() {
	local out
	out=$(ss -tlne)
	for p in 80 443 5300 7073; do
		grep -Eq ":$p .*uid:$1 .*lerd-" <<<"$out" || return 1
	done
}

# seat_state: who is on screen and how both stacks stand, printed when a switch
# did not hand over, so the log says which side did not move.
seat_state() {
	echo "--- seat0: $(loginctl show-seat seat0 -p ActiveSession --value), VT $(sudo fgconsole)"
	echo "--- us: $(systemctl --user is-active lerd-nginx lerd-ui lerd-dns lerd-watcher | paste -sd' ')"
	echo "--- $U2: $(as_u2 systemctl --user is-active lerd-nginx lerd-ui lerd-dns lerd-watcher | paste -sd' ')"
	ss -tlne | grep -E ':(80|443|5300|7073) ' | sed 's/^/    /'
	as_u2 journalctl --user -u lerd-watcher --since -3min --no-pager -o cat 2>&1 | tail -15 | sed 's/^/    /'
}

sites_api() { curl -s --max-time 10 http://127.0.0.1:7073/api/sites; }

ORIG_VT=$(sudo fgconsole 2>/dev/null || echo 1)
cleanup() {
	lerd multi-user disable </dev/null >/dev/null 2>&1
	# sudo: the second user's home is not readable to us.
	if id "$U2" >/dev/null 2>&1 && sudo test -x "/home/$U2/.local/bin/lerd"; then
		as_u2 lerd multi-user disable </dev/null >/dev/null 2>&1
		as_u2 lerd quit </dev/null >/dev/null 2>&1
	fi
	sudo systemctl stop lerdvm-seat-self lerdvm-seat-other 2>/dev/null
	sudo chvt "$ORIG_VT" 2>/dev/null
}
cleanup

echo "--- setup: our site, and a second user with sudo and linger"
lerd start </dev/null >/dev/null 2>&1
mkdir -p "$SELF_DIR"
printf '<?php echo "vmmu-self";\n' >"$SELF_DIR/index.php"
(cd "$SELF_DIR" && lerd link </dev/null >/dev/null 2>&1)
SELF_HOST=$(site_host "$SELF_DIR")
id "$U2" >/dev/null 2>&1 || sudo useradd -m -s /bin/bash "$U2"
echo "$U2 ALL=(ALL) NOPASSWD:ALL" | sudo tee "/etc/sudoers.d/99-$U2" >/dev/null
sudo chmod 440 "/etc/sudoers.d/99-$U2"
UID2=$(id -u "$U2")
sudo loginctl enable-linger "$U2"
wait_for 30 sudo test -S "/run/user/$UID2/bus" || _fail "setup: $U2 has no user manager"
sudo rm -rf "/home/$U2/rc"
sudo cp -r "$HOME/rc" "/home/$U2/rc"
sudo chown -R "$U2:$U2" "/home/$U2/rc"
OTHER_HOST=$(host vmmuother)

wait_for 120 serves "$SELF_HOST" vmmu-self || _fail "setup: our site does not answer"
check_out "16.1 the second user's install names us and lerd quit" "the lerd of user $USER is running.*lerd quit" \
	as_u2 bash rc/install.sh --local rc/lerd
check_out "16.2 the second user's start names us the same way" "the lerd of user $USER is running.*lerd quit" \
	as_u2 lerd start

lerd quit </dev/null >/dev/null 2>&1
as_u2 lerd start </dev/null >/dev/null 2>&1
as_u2 bash -c 'mkdir -p ~/Projects/vmmuother && printf "<?php echo \"vmmu-other\";\n" >~/Projects/vmmuother/index.php && cd ~/Projects/vmmuother && lerd link' </dev/null >/dev/null 2>&1
wait_for 60 serves "$OTHER_HOST" vmmu-other
check_out "16.3 after our quit its start serves its site" 'vmmu-other' curl -s --max-time 10 "http://$OTHER_HOST/"
check_out "16.3 its dashboard lists its site" "/home/$U2/Projects/vmmuother" sites_api
check_not "16.3 its dashboard lists none of ours" "\"path\":\"$HOME/" sites_api
as_u2 lerd quit </dev/null >/dev/null 2>&1
lerd start </dev/null >/dev/null 2>&1
wait_for 120 serves "$SELF_HOST" vmmu-self

check "16.4 [partial] multi-user mode turns on for us" lerd multi-user enable
check "16.4 [partial] multi-user mode turns on for the second user" as_u2 lerd multi-user enable
# As if the second user logged in while our lerd runs: its stack starts and is
# blocked on our ports. A quit stack would count as stopped by hand instead.
as_u2 lerd start </dev/null >/dev/null 2>&1
seat_login "$USER" "$SELF_VT" lerdvm-seat-self || _fail "setup: no console login for $USER"
seat_login "$U2" "$OTHER_VT" lerdvm-seat-other || _fail "setup: no console login for $U2"
sudo chvt "$SELF_VT"
sleep 10 # both watchers see us on screen before the switch

sudo chvt "$OTHER_VT"
wait_for 90 serves "$OTHER_HOST" vmmu-other || seat_state
check "16.4 the second user's site answers after the switch" serves "$OTHER_HOST" vmmu-other
wait_for 30 owns_ports "$UID2"
check "16.4 the second user's lerd holds 80, 443, 5300 and 7073" owns_ports "$UID2"

sudo chvt "$SELF_VT"
wait_for 120 serves "$SELF_HOST" vmmu-self
expect_200 "16.5" "http://$SELF_HOST/"
wait_for 30 owns_ports "$(id -u)"
check "16.5 our lerd holds the ports again" owns_ports "$(id -u)"
check_out "16.5 our dashboard lists our site again" "\"path\":\"$SELF_DIR\"" sites_api

lerd stop </dev/null >/dev/null 2>&1
# Two rounds, since a stop marker lost on the first would show on the second.
for round in 1 2; do
	sudo chvt "$OTHER_VT"
	wait_for 90 serves "$OTHER_HOST" vmmu-other || seat_state
	sudo chvt "$SELF_VT"
	sleep 30 # a resume that wrongly started anything would have done so by now
	check_out "16.6 nothing of ours comes back (round $round)" '^inactive inactive inactive$' \
		bash -c 'systemctl --user is-active lerd-nginx lerd-ui lerd-dns | paste -sd " "'
done

cleanup
lerd start </dev/null >/dev/null 2>&1
wait_for 120 serves "$SELF_HOST" vmmu-self
