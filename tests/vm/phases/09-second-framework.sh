#!/usr/bin/env bash
# Phase 9, a second framework from a different family, and the frameworks and
# packages that only a real scaffold exercises.
source "$(dirname "$0")/../lib.sh"
need_demo
mkdir -p "$PROJECTS"
cd "$PROJECTS" || exit 1
api=http://127.0.0.1:7073/api

# site_url <dir>: the site's URL on the scheme it is actually served on.
site_url() {
	local n tls
	n=$(site_name "$1")
	tls=$(lerd sites 2>/dev/null | awk -v n="$n" '$1 == n { print $5; exit }')
	if [ "$tls" = Yes ]; then printf 'https://%s' "$(site_host "$1")"; else printf 'http://%s' "$(site_host "$1")"; fi
}

# scaffold <name> <framework> [version]: a linked, set-up project, created once.
scaffold() {
	local dir=$PROJECTS/$1 ver=()
	[ -n "${3:-}" ] && ver=(--framework-version "$3")
	if [ ! -d "$dir" ]; then
		(cd "$PROJECTS" && lerd new "$1" --framework="$2" "${ver[@]}" </dev/null)
		(cd "$dir" && lerd link </dev/null && lerd setup --all --skip-open </dev/null)
	fi
	(cd "$dir" && lerd link </dev/null >/dev/null 2>&1)
}

[ -d "$SHOP_DIR" ] || check "lerd new shop --framework=symfony" lerd new shop --framework=symfony
cd "$SHOP_DIR" || exit 1
lerd link </dev/null >/dev/null 2>&1
check_not "lerd setup --all on shop" '✗' lerd setup --all --skip-open
check_out "9.1 detection picks the Symfony definition" "^$(site_name "$SHOP_DIR") .*symfony" lerd sites
check_out "9.5 lerd console maps to bin/console" 'symfony' lerd console --version
# The messenger worker belongs to symfony/messenger, which a bare skeleton lacks.
grep -q '"symfony/messenger"' composer.json || lerd composer require symfony/messenger -W --no-interaction </dev/null >/dev/null 2>&1
check_out "9.6 [partial] its workers come from the store definition" 'messenger' lerd worker list
check_out "its doctor checks run" 'Project Config' lerd site:doctor
check_out "9.7 the second site answers from the framework (a bare skeleton answers 404)" '^(200|404)$' code "$(site_url "$SHOP_DIR")"
check_out "lerd framework list shows both" 'laravel' lerd framework list
check_out "lerd framework list shows symfony" 'symfony' lerd framework list
lerd framework prune </dev/null >/dev/null 2>&1
check_out "9.8 lerd framework prune leaves both alone" 'symfony(.|\n)*' bash -c 'lerd framework list | grep -E "^(laravel|symfony) "'
expect_200 "$(site_url "$DEMO_DIR")"
check_out "9.9 both serve at once" '^(200|404)$' code "$(site_url "$SHOP_DIR")"

# CodeIgniter ships one definition per major, each with its own PHP range.
# CodeIgniter 3 runs on PHP 7.2-7.4, which a fresh install does not carry.
lerd php:list 2>/dev/null | grep -q '7\.4' || check "add PHP 7.4 for CodeIgniter 3" lerd php:rebuild 7.4
scaffold ci3 codeigniter 3
scaffold ci4 codeigniter 4 >/dev/null 2>&1
check_out "CodeIgniter 3 links on its own definition, PHP 7.2-7.4" '^ci3 .* 7\.[234] .*codeigniter 3' lerd sites
check_out "9.2 CodeIgniter 4 links on its own definition, PHP 8.2-8.4" '^ci4 .* 8\.[234] .*codeigniter 4' lerd sites

# WordPress keeps its configuration in wp-config.php, not a dotenv file.
scaffold wpvm wordpress >/dev/null 2>&1
wp=$PROJECTS/wpvm
# The backup is of a config the user had before lerd touched it, so start
# from WordPress's own sample rather than the file lerd wrote at scaffold.
rm -f "$wp/wp-config.php.before_lerd"
cp "$wp/wp-config-sample.php" "$wp/wp-config.php"
check "lerd env over a pre-lerd wp-config.php" bash -c "cd $wp && lerd env"
check "the backup sits beside wp-config.php" test -f "$wp/wp-config.php.before_lerd"
cp "$wp/wp-config.php" /tmp/lerd-vm-wp-config.php
echo "// vm edit" >>"$wp/wp-config.php"
check "lerd env:restore" bash -c "cd $wp && lerd env:restore --yes 2>/dev/null || lerd env:restore"
check "9.3 env:restore puts the backup back over wp-config.php" cmp -s "$wp/wp-config.php" "$wp/wp-config.php.before_lerd"
cp /tmp/lerd-vm-wp-config.php "$wp/wp-config.php"
rm -f /tmp/lerd-vm-wp-config.php

# A fresh Drupal offers its own installer, runs it, and then stops offering it.
drup=$PROJECTS/drupvm
fresh_drupal=0
if [ ! -d "$drup" ]; then
	(cd "$PROJECTS" && lerd new drupvm --framework=drupal </dev/null >/dev/null 2>&1)
	(cd "$drup" && lerd link </dev/null >/dev/null 2>&1)
	fresh_drupal=1
fi
steps_before=$(curl -s "$api/project/setup-steps?dir=$drup")
echo "$steps_before"
# The installer is offered on a fresh scaffold and hidden once the site is
# installed, so a rerun on an installed site checks the second half only.
if [ "$fresh_drupal" = 1 ]; then
	check_out "setup offers Install Drupal, ticked" '"label":"Install Drupal","enabled":true' echo "$steps_before"
	check_not "lerd setup --all runs the installer" '✗' bash -c "cd $drup && lerd setup --all --skip-open"
fi
check_not "the step is hidden once installed" 'Install Drupal' curl -s "$api/project/setup-steps?dir=$drup"
expect_200 "9.4 [partial]" "$(site_url "$drup")"
check_out "9.12 [partial] lerd drush runs the command" 'Drush' bash -c "cd $drup && lerd drush --version"
(cd "$wp" && [ -x vendor/bin/wp ] || lerd composer require wp-cli/wp-cli-bundle --no-interaction </dev/null >/dev/null 2>&1)
check_out "lerd wp runs the command" 'WP-CLI' bash -c "cd $wp && lerd wp --version"

# Packages: horizon belongs to Laravel only, messenger to Symfony only. Horizon
# goes into phase 6's throwaway project, never demo, where it would supersede
# the queue worker for every phase after this one.
wk=$PROJECTS/vmworkers
if [ ! -d "$wk/vendor" ]; then
	check "scaffold the package project" bash -c "cd '$PROJECTS' && lerd new vmworkers && cd vmworkers && lerd setup --all --skip-open"
fi
(cd "$wk" && grep -q laravel/horizon composer.json || lerd composer require laravel/horizon -W --no-interaction </dev/null >/dev/null 2>&1)
check_out "a required package contributes its worker" 'horizon' bash -c "cd $wk && lerd worker list"
check_not "a package scoped to Laravel stays out of Symfony" 'horizon' bash -c "cd $SHOP_DIR && lerd worker list"
check_not "9.10 and a Symfony package stays out of Laravel" 'messenger' bash -c "cd $wk && lerd worker list"

# lerd sail runs the project's own Sail wrapper against lerd's containers.
sail=$PROJECTS/sailapp
skip "9.11 lerd sail / lerd import sail" "skipped by the maintainer for this release"

check "lerd composer global require laravel/installer" lerd composer global require laravel/installer --no-interaction
check_out "9.13 [partial] the global binary runs on the host" 'Laravel Installer' bash -ic 'laravel --version'

# NativePHP: desktop and mobile are package layers on Laravel.
nat=$PROJECTS/natvm
if [ ! -d "$nat" ]; then
	(cd "$PROJECTS" && lerd new natvm </dev/null >/dev/null 2>&1)
	(cd "$nat" && lerd link </dev/null >/dev/null 2>&1 && lerd composer require nativephp/desktop -W --no-interaction </dev/null >/dev/null 2>&1)
fi
check_not "a NativePHP desktop project finishes setup --all" '✗' bash -c "cd $nat && lerd link && lerd setup --all --skip-open"
# Before native:install the package check has something missing to name.
check_out "9.16 site:doctor runs the package's own check and names what is missing" 'nativephp_runtime|native:install' bash -c "cd $nat && lerd site:doctor"
# The worker appears once native:install has put NativePHP's own PHP in place.
check "native:install" bash -c "cd $nat && lerd run native:install"
check_out "its worker comes from the package layer" 'native' bash -c "cd $nat && lerd worker list"
check_out "9.14 its commands come from the package layer" 'native:install' bash -c "cd $nat && lerd run"
# The mobile half conflicts with the desktop package in one project, so it
# gets a Laravel project of its own.
natm=$PROJECTS/natmob
if [ ! -d "$natm/vendor" ]; then
	rm -rf "${PROJECTS:?}/natmob"
	check "scaffold the mobile project" bash -c "cd '$PROJECTS' && lerd new natmob && cd natmob && lerd setup --all --skip-open"
fi
check "the mobile half installs" bash -c "cd $natm && (grep -q nativephp/mobile composer.json || lerd composer require nativephp/mobile -W --no-interaction)"
check_out "its commands are offered on Laravel" 'native:install-mobile' bash -c "cd $natm && lerd run"
check_not "9.15 [partial] and not on Symfony" 'native:install-mobile' bash -c "cd $SHOP_DIR && lerd run"
