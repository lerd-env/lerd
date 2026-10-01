#!/usr/bin/env bash
# Phase 12, upgrade and rollback. FROM_VERSION=1.35.0 installs that release
# first, through its own installer's --local path, so the guest starts where a
# real user would. The target is the release lerd update finds; LERD_CHANNEL=beta
# follows the beta line. GITHUB_TOKEN avoids the API's 60-an-hour limit.
source "$(dirname "$0")/../lib.sh"

# Installing an older release rebuilds its own images; free the throwaway
# projects and unused images of the phases before so a small disk has room.
for d in vmworkers natvm natmob vmhz drupvm wpvm ci3 ci4 shop sailapp ccapp plainsite "space dir"; do
	[ -d "$PROJECTS/$d" ] && (cd "$PROJECTS/$d" && lerd unlink </dev/null >/dev/null 2>&1) && rm -rf "${PROJECTS:?}/$d"
done
reclaim

if [ -n "${FROM_VERSION:-}" ]; then
	arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
	mkdir -p "$HOME/from" && cd "$HOME/from" || exit 1
	curl -fsSLO "https://github.com/lerd-env/lerd/releases/download/v$FROM_VERSION/lerd_${FROM_VERSION}_linux_$arch.tar.gz"
	tar xzf "lerd_${FROM_VERSION}_linux_$arch.tar.gz"
	curl -fsSL "https://raw.githubusercontent.com/lerd-env/lerd/v$FROM_VERSION/install.sh" -o install.sh
	check "install $FROM_VERSION" bash install.sh --local "$HOME/from/lerd"
	cd "$HOME" || exit 1
fi
# Hand phase 13 the build under test, not the release this phase moved to.
reinstall_candidate() {
	[ -x "$HOME/rc/lerd" ] || return 0
	check "reinstall the build under test for the phases after" bash "$HOME/rc/install.sh" --local "$HOME/rc/lerd"
}
before=$(lerd --version | awk '{print $3}')
site=$([ -d "$DEMO_DIR" ] && site_name "$DEMO_DIR")
[ -n "$site" ] || site=$(lerd sites 2>/dev/null | awk 'NR>1 && $1 !~ /^[-─]/ {print $1; exit}')
[ -n "$site" ] || { echo "FAIL 12.1 no linked site to carry through the upgrade"; exit 1; }
dir=$(lerd sites | awk -v s="$site" '$1 == s {print $NF}')
site_url="https://$(host "$site")"
[ "$(tld)" = localhost ] && site_url="http://$(host "$site")"
flag=""
[ "${LERD_CHANNEL:-}" = beta ] && flag=--beta
manager_before=$(grep -A3 '^node:' ~/.config/lerd/config.yaml 2>/dev/null | awk '/manager:/ {print $2}')
sites_before=$(lerd sites | awk 'NR>1 {print $1}' | sort | tr '\n' ' ')
services_before=$(lerd service list 2>/dev/null | awk 'NR>1 {print $1}' | sort | tr '\n' ' ')

echo "--- upgrading from $before, site $site"
check_out "12.1 --version reports N-1 and the site answers" '^200$' code "$site_url"
# status announces the stable channel's latest release; on the beta line
# there is no newer stable to announce until 1.36.0 is published.
if [ "${LERD_CHANNEL:-}" = beta ]; then
	skip "12.2 lerd status shows the update notice" "the notice follows the stable channel, and $before is the newest stable release; judge it against the published 1.36.0"
else
	check_out "12.2 lerd status shows the update notice" 'update|new version|available' lerd status
fi
check_out "12.3 [partial] lerd whatsnew lists the changes" '.' lerd whatsnew
upgrade_out=$(yes | lerd update $flag 2>&1)
echo "$upgrade_out"
after=$(lerd --version | awk '{print $3}')
check "12.4 lerd update upgrades without a reinstall" test "$after" != "$before"
# Every check below judges an upgrade; with none there is nothing to judge.
if [ "$after" = "$before" ]; then
	for i in 12.5 12.6 12.7 12.8 12.9 12.10 12.11 12.12; do _fail "$i" "no upgrade happened (still $before); LERD_CHANNEL=beta upgrades to a pre-release"; done
	reinstall_candidate
	exit 1
fi
check_out "12.5 [partial] config, sites and services survive" "^$sites_before\$" bash -c "lerd sites | awk 'NR>1 {print \$1}' | sort | tr '\n' ' '"
expect_200 12.6 "$site_url"
check_not "12.7 [partial] migrations run or are announced, nothing left failed" 'migration.*fail|✗' echo "$upgrade_out"
check_out "12.8 [partial] the update reports what changed without being asked" "what'?s new|changes|changelog|$after" echo "$upgrade_out"
check_not "12.9 no step drawn failed for a service that was not running" '✗' echo "$upgrade_out"
check_not "12.10 lerd doctor is clean after the upgrade" '✗' lerd doctor
check "lerd update --rollback" bash -c 'yes | lerd update --rollback'
check_out "12.11 rollback reverts to N-1" "$before" lerd --version
expect_200 "$site_url"
check "lerd update again" bash -c "yes | lerd update $flag"
check_out "12.12 update again returns to the new version" "$after" lerd --version
expect_200 "$site_url"
if [ "$flag" = --beta ]; then
	check_out "12.13 update --beta picks the pre-release" 'beta|rc' lerd --version
else
	skip "12.13 update --beta picks the pre-release" "run with LERD_CHANNEL=beta"
fi
todo "12.14 an install on a beta is offered the next beta" "needs two published betas after the installed one"
check_out "12.15 [partial] update:beta on/off set update.beta and the bare command reports it" 'beta' bash -c 'lerd update:beta on && lerd update:beta && lerd update:beta off && lerd update:beta'
if [ -z "$manager_before" ]; then
	check_out "12.16 an N-1 install with no node.manager stays on fnm" 'fnm' lerd node:manager
else
	skip "12.16 an N-1 install with no node.manager stays on fnm" "the N-1 install already had node.manager=$manager_before"
fi
todo "12.17 a service's newly gained dashboard shows by the next watcher sweep" "needs a preset that gained a dashboard since N-1"
check_out "12.18 [partial] a beta build follows the beta line whatever the flag says" 'beta' bash -c 'lerd update:beta off >/dev/null 2>&1; lerd update:beta'
todo "12.19 one update writes each skill file once and bounces each daemon once" "needs a count of skill writes and daemon restarts from the update log"
if have dnf && rpm -q lerd >/dev/null 2>&1; then
	check_out "12.20 a packaged install defers to dnf" 'dnf' lerd update
	check "12.21 sudo dnf upgrade on a COPR guest" sudo dnf -y upgrade lerd
	expect_200 "$site_url"
elif have apt && dpkg -s lerd >/dev/null 2>&1; then
	check_out "12.20 a packaged install defers to apt" 'apt' lerd update
	skip "12.21 dnf upgrade on a COPR guest" "apt guest"
else
	skip "12.20 a packaged install defers to its package manager" "no package-manager install on this guest"
	skip "12.21 dnf upgrade on a COPR guest" "no COPR install on this guest"
fi
if have brew && brew list lerd >/dev/null 2>&1; then
	check "12.22 brew upgrade lerd" brew upgrade lerd
	expect_200 "$site_url"
else
	skip "12.22 brew upgrade lerd" "no Homebrew install on this guest"
fi
echo "sites and services before: $sites_before / $services_before (dir $dir)"

reinstall_candidate
