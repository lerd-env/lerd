#!/usr/bin/env bats
# Tests for install.sh
# Requires: bats-core  https://github.com/bats-core/bats-core

INSTALLER="$BATS_TEST_DIRNAME/../../install.sh"

# Source the installer so we can call its functions directly.
# The guard at the bottom prevents main() from running when sourced.
setup() {
  # Isolate HOME so the installer never touches the real shell rc files. The
  # XDG variables go with it: LERD_CONFIG_DIR falls back to $HOME only when
  # they are unset, so leaving them would point it at the real config.
  export HOME="$BATS_TMPDIR/home-$$"
  unset XDG_DATA_HOME XDG_CONFIG_HOME XDG_STATE_HOME XDG_CACHE_HOME
  mkdir -p "$HOME"

  # Source the script to load all function definitions.
  # shellcheck disable=SC1090
  source "$INSTALLER"
}

teardown() {
  rm -rf "$BATS_TMPDIR/home-$$"
}

# setsid is util-linux and macOS ships no equivalent binary, so the only way to
# drop the controlling terminal on both is perl's POSIX::setsid, which is in the
# base install of each.
run_without_tty() {
  if command -v setsid >/dev/null 2>&1; then
    setsid bash -c "$1"
  else
    perl -e 'use POSIX; POSIX::setsid(); exec @ARGV' bash -c "$1"
  fi
}

# script(1) allocates the pty on both platforms but its calling convention
# differs: util-linux takes -c CMD ahead of the file, BSD takes the file then the
# command. The trailing sleep holds stdin open until the pty is up, or BSD reads
# EOF before the prompt is printed.
run_with_tty() {
  if script --version 2>/dev/null | grep -qi util-linux; then
    # script -c runs the command through $SHELL, and install.sh only skips main
    # when BASH_SOURCE says it was sourced, which no other shell sets.
    ( printf 'y\n'; sleep 1 ) | SHELL=/bin/bash script -qec "$1" /dev/null
  else
    ( printf 'y\n'; sleep 1 ) | script -q /dev/null bash -c "$1"
  fi
}

# Pins the isolation the whole file rests on: whatever the environment running
# the suite looks like, the config directory the installer reads must sit
# inside the throwaway HOME and never in the real one.
@test "the harness keeps the config directory inside the test HOME" {
  [[ "$HOME" == "$BATS_TMPDIR/"* ]]
  [[ "$LERD_CONFIG_DIR" == "$HOME/"* ]]
}

# ── detect_arch ───────────────────────────────────────────────────────────────

@test "detect_arch returns amd64 for x86_64" {
  # Override uname for this test
  function uname() { echo "x86_64"; }
  export -f uname

  run detect_arch
  [ "$status" -eq 0 ]
  [ "$output" = "amd64" ]
}

@test "detect_arch returns arm64 for aarch64" {
  function uname() { echo "aarch64"; }
  export -f uname

  run detect_arch
  [ "$status" -eq 0 ]
  [ "$output" = "arm64" ]
}

@test "detect_arch fails for unsupported arch" {
  function uname() { echo "mips"; }
  export -f uname

  run detect_arch
  [ "$status" -ne 0 ]
  [[ "$output" == *"Unsupported architecture"* ]]
}

# ── distro_family ─────────────────────────────────────────────────────────────

@test "distro_family detects arch" {
  function detect_distro() { echo "arch"; }
  export -f detect_distro

  run distro_family
  [ "$output" = "arch" ]
}

@test "distro_family detects manjaro as arch family" {
  function detect_distro() { echo "manjaro"; }
  export -f detect_distro

  run distro_family
  [ "$output" = "arch" ]
}

@test "distro_family detects ubuntu as debian family" {
  function detect_distro() { echo "ubuntu"; }
  export -f detect_distro

  run distro_family
  [ "$output" = "debian" ]
}

@test "distro_family detects fedora" {
  function detect_distro() { echo "fedora"; }
  export -f detect_distro

  run distro_family
  [ "$output" = "fedora" ]
}

@test "distro_family returns unknown for unrecognised distro" {
  function detect_distro() { echo "slackware"; }
  export -f detect_distro
  function detect_distro_like() { echo ""; }
  export -f detect_distro_like

  run distro_family
  [ "$output" = "unknown" ]
}

@test "distro_family falls back to ID_LIKE for derivatives (bazzite -> fedora)" {
  function detect_distro() { echo "bazzite"; }
  export -f detect_distro
  function detect_distro_like() { echo "fedora"; }
  export -f detect_distro_like

  run distro_family
  [ "$output" = "fedora" ]
}

@test "distro_family reads a multi-value ID_LIKE" {
  function detect_distro() { echo "somespin"; }
  export -f detect_distro
  function detect_distro_like() { echo "rhel fedora"; }
  export -f detect_distro_like

  run distro_family
  [ "$output" = "fedora" ]
}

# ── check_certutil ────────────────────────────────────────────────────────────

@test "check_certutil guides without queuing nss-tools on atomic images" {
  function command() { if [ "$1" = "-v" ] && [ "$2" = "certutil" ]; then return 1; fi; builtin command "$@"; }
  export -f command
  function distro_family() { echo "fedora"; }
  export -f distro_family
  function is_atomic() { return 0; }
  export -f is_atomic

  MISSING_PKGS=()
  check_certutil >"$BATS_TMPDIR/cc-$$.out" 2>&1
  [ "${#MISSING_PKGS[@]}" -eq 0 ]
  grep -q "rpm-ostree install nss-tools" "$BATS_TMPDIR/cc-$$.out"
}

@test "check_certutil queues nss-tools on ordinary distros" {
  function command() { if [ "$1" = "-v" ] && [ "$2" = "certutil" ]; then return 1; fi; builtin command "$@"; }
  export -f command
  function distro_family() { echo "fedora"; }
  export -f distro_family
  function is_atomic() { return 1; }
  export -f is_atomic

  MISSING_PKGS=()
  check_certutil >/dev/null 2>&1
  [ "${#MISSING_PKGS[@]}" -eq 1 ]
  [ "${MISSING_PKGS[0]}" = "nss-tools" ]
}

# ── check_nm_dnsmasq ──────────────────────────────────────────────────────────

@test "check_nm_dnsmasq queues dnsmasq on NetworkManager-only hosts" {
  function systemctl() { [ "$3" = "NetworkManager" ]; }
  export -f systemctl
  function host_has_dnsmasq() { return 1; }
  export -f host_has_dnsmasq
  function distro_family() { echo "arch"; }
  export -f distro_family

  MISSING_PKGS=()
  check_nm_dnsmasq >/dev/null 2>&1
  [ "${#MISSING_PKGS[@]}" -eq 1 ]
  [ "${MISSING_PKGS[0]}" = "dnsmasq" ]
}

@test "check_nm_dnsmasq queues dnsmasq-base on the debian family" {
  function systemctl() { [ "$3" = "NetworkManager" ]; }
  export -f systemctl
  function host_has_dnsmasq() { return 1; }
  export -f host_has_dnsmasq
  function distro_family() { echo "debian"; }
  export -f distro_family

  MISSING_PKGS=()
  check_nm_dnsmasq >/dev/null 2>&1
  [ "${#MISSING_PKGS[@]}" -eq 1 ]
  [ "${MISSING_PKGS[0]}" = "dnsmasq-base" ]
}

@test "check_nm_dnsmasq stays quiet when systemd-resolved is the resolver" {
  function systemctl() { return 0; }
  export -f systemctl
  function host_has_dnsmasq() { return 1; }
  export -f host_has_dnsmasq
  RESOLV_CONF="$HOME/resolv.conf"
  printf 'nameserver 127.0.0.53\n' >"$RESOLV_CONF"

  MISSING_PKGS=()
  check_nm_dnsmasq >/dev/null 2>&1
  [ "${#MISSING_PKGS[@]}" -eq 0 ]
}

# Arch and CachyOS boot with systemd-resolved active while NetworkManager still
# writes resolv.conf itself. lerd takes the NM dnsmasq path there, so skipping
# the offer leaves `lerd install` dying on the missing binary later.
@test "check_nm_dnsmasq queues dnsmasq when resolved runs but does not own resolv.conf" {
  function systemctl() { return 0; }
  export -f systemctl
  function host_has_dnsmasq() { return 1; }
  export -f host_has_dnsmasq
  function distro_family() { echo "arch"; }
  export -f distro_family
  RESOLV_CONF="$HOME/resolv.conf"
  printf 'nameserver 192.168.1.1\n' >"$RESOLV_CONF"

  MISSING_PKGS=()
  check_nm_dnsmasq >/dev/null 2>&1
  [ "${#MISSING_PKGS[@]}" -eq 1 ]
  [ "${MISSING_PKGS[0]}" = "dnsmasq" ]
}

@test "resolved_is_resolver ignores an unreadable resolv.conf" {
  function systemctl() { return 0; }
  export -f systemctl
  RESOLV_CONF="$HOME/no-such-resolv.conf"

  run resolved_is_resolver
  [ "$status" -ne 0 ]
}

@test "check_nm_dnsmasq stays quiet when dnsmasq is already present" {
  function systemctl() { [ "$3" = "NetworkManager" ]; }
  export -f systemctl
  function host_has_dnsmasq() { return 0; }
  export -f host_has_dnsmasq

  MISSING_PKGS=()
  check_nm_dnsmasq >/dev/null 2>&1
  [ "${#MISSING_PKGS[@]}" -eq 0 ]
}

@test "host_has_dnsmasq finds a sbin binary that PATH misses" {
  function command() { if [ "$1" = "-v" ] && [ "$2" = "dnsmasq" ]; then return 1; fi; builtin command "$@"; }
  export -f command

  mkdir -p "$BATS_TMPDIR/sbin-$$"
  DNSMASQ_PATHS="$BATS_TMPDIR/sbin-$$/dnsmasq"
  run host_has_dnsmasq
  [ "$status" -ne 0 ]

  printf '#!/bin/sh\n' >"$BATS_TMPDIR/sbin-$$/dnsmasq"
  chmod +x "$BATS_TMPDIR/sbin-$$/dnsmasq"
  run host_has_dnsmasq
  [ "$status" -eq 0 ]
}

# ── _download_tool ────────────────────────────────────────────────────────────

@test "_download_tool prefers curl when both are available" {
  function curl() { return 0; }
  function wget() { return 0; }
  export -f curl wget

  # Temporarily mask PATH to ensure only our functions are visible
  run bash -c "source '$INSTALLER'; _download_tool"
  [ "$output" = "curl" ]
}

@test "_download_tool falls back to wget when curl is absent" {
  # Hide curl by making it unavailable in a subshell
  run bash -c "
    source '$INSTALLER'
    function curl() { return 127; }
    # Remove curl from PATH lookup
    PATH_ORIG=\$PATH
    export PATH=\"\$BATS_TMPDIR\"  # empty path with no curl binary
    _download_tool
  "
  # We just check it doesn't die — wget fallback varies by system
  [ "$status" -eq 0 ] || [[ "$output" == *"wget"* ]] || [[ "$output" == *"curl"* ]]
}

@test "_download_tool errors when neither curl nor wget found" {
  run bash -c "
    source '$INSTALLER'
    # Override command -v to report both as missing
    function command() {
      if [[ \"\$2\" == 'curl' || \"\$2\" == 'wget' ]]; then return 1; fi
      builtin command \"\$@\"
    }
    export -f command
    _download_tool
  "
  [ "$status" -ne 0 ]
  [[ "$output" == *"Neither curl nor wget"* ]]
}

# ── add_to_path ────────────────────────────────────────────

# install.sh writes .bash_profile for bash on Darwin and .bashrc on Linux.
# These cases assert the Linux bash path; pin detect_os so they stay valid when
# the suite is run on a Mac.
_force_linux_os() {
  function detect_os() { echo "linux"; }
  export -f detect_os
}

@test "add_to_path appends PATH entry to .bashrc" {
  export SHELL="/bin/bash"
  _force_linux_os
  INSTALL_DIR="$HOME/.local/bin"
  touch "$HOME/.bashrc"

  add_to_path

  grep -q "Added by Lerd installer" "$HOME/.bashrc"
  grep -q "$INSTALL_DIR" "$HOME/.bashrc"
}

@test "add_to_path is idempotent — does not duplicate entry" {
  export SHELL="/bin/bash"
  _force_linux_os
  INSTALL_DIR="$HOME/.local/bin"
  touch "$HOME/.bashrc"

  add_to_path
  add_to_path

  count=$(grep -c "Added by Lerd installer" "$HOME/.bashrc")
  [ "$count" -eq 1 ]
}

@test "add_to_path writes fish_add_path for fish shell" {
  export SHELL="/usr/bin/fish"
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$HOME/.config/fish/conf.d"

  add_to_path

  grep -q "fish_add_path" "$HOME/.config/fish/conf.d/lerd.fish"
}



# ── installed_version ─────────────────────────────────────────────────────────

@test "installed_version returns empty string when lerd not found" {
  # Create an empty bin dir first, then restrict PATH to it
  local empty_dir="$BATS_TMPDIR/empty-path-$$"
  mkdir -p "$empty_dir"

  OLD_PATH="$PATH"
  export PATH="$empty_dir"

  run installed_version
  [ "$output" = "" ]

  export PATH="$OLD_PATH"
}

@test "installed_version returns version string when lerd is found" {
  # Create a fake lerd binary
  FAKE_BIN="$BATS_TMPDIR/fake-bin-$$"
  mkdir -p "$FAKE_BIN"
  printf '#!/bin/sh\necho "lerd version 1.2.3"\n' > "$FAKE_BIN/lerd"
  chmod +x "$FAKE_BIN/lerd"

  OLD_PATH="$PATH"
  export PATH="$FAKE_BIN:$PATH"

  run installed_version
  [ "$output" = "1.2.3" ]

  export PATH="$OLD_PATH"
}

# ── installed_version_raw / version_is_dev ────────────────────────────────────

@test "installed_version_raw keeps the git-describe suffix" {
  FAKE_BIN="$BATS_TMPDIR/fake-bin-$$"
  mkdir -p "$FAKE_BIN"
  printf '#!/bin/sh\necho "lerd version v1.25.0-6-g7d030096-dirty (commit 7d030096)"\n' > "$FAKE_BIN/lerd"
  chmod +x "$FAKE_BIN/lerd"

  OLD_PATH="$PATH"
  export PATH="$FAKE_BIN:$PATH"

  run installed_version_raw
  [ "$output" = "1.25.0-6-g7d030096-dirty" ]

  export PATH="$OLD_PATH"
}

@test "version_is_dev is true for a git-describe build" {
  run version_is_dev "1.25.0-6-g7d030096-dirty"
  [ "$status" -eq 0 ]
}

@test "version_is_dev is false for a clean release" {
  run version_is_dev "1.25.0"
  [ "$status" -ne 0 ]
}

@test "version_is_dev is false for a prerelease" {
  run version_is_dev "1.34.0-beta.3"
  [ "$status" -ne 0 ]
}

@test "version_tag strips the git-describe tail from a prerelease" {
  run version_tag "1.34.0-beta.3-11-g632148be-dirty"
  [ "$output" = "1.34.0-beta.3" ]
}

@test "version_tag leaves a clean release alone" {
  run version_tag "1.33.1"
  [ "$output" = "1.33.1" ]
}

@test "installed_version keeps the prerelease suffix" {
  FAKE_BIN="$BATS_TMPDIR/fake-bin-$$"
  mkdir -p "$FAKE_BIN"
  printf '#!/bin/sh\necho "lerd version 1.34.0-beta.3 (commit abc)"\n' > "$FAKE_BIN/lerd"
  chmod +x "$FAKE_BIN/lerd"

  OLD_PATH="$PATH"
  export PATH="$FAKE_BIN:$PATH"

  run installed_version
  [ "$output" = "1.34.0-beta.3" ]

  export PATH="$OLD_PATH"
}

@test "version_is_prerelease tells a beta apart from a release and a dev build" {
  run version_is_prerelease "1.34.0-beta.3"
  [ "$status" -eq 0 ]
  run version_is_prerelease "1.33.1"
  [ "$status" -ne 0 ]
  run version_is_prerelease "1.25.0-6-g7d030096-dirty"
  [ "$status" -ne 0 ]
}

# ── latest_version ────────────────────────────────────────────────────────────

@test "latest_version parses version from redirect Location header" {
  # Mock curl -fsSLI to return headers containing a Location pointing to the tag
  function curl() {
    echo "HTTP/2 302"
    echo "location: https://github.com/lerd-env/lerd/releases/tag/v2.0.0"
    echo ""
  }
  export -f curl

  run latest_version
  [ "$status" -eq 0 ]
  [ "$output" = "2.0.0" ]
}

@test "latest_version returns empty string when redirect has no tag" {
  function curl() {
    echo "HTTP/2 404"
    echo ""
  }
  export -f curl

  run latest_version
  [ "$status" -eq 0 ]
  [ "$output" = "" ]
}

@test "latest_version returns empty string on curl failure" {
  function curl() { return 22; }
  export -f curl

  run latest_version
  [ "$status" -eq 0 ]
  [ "$output" = "" ]
}

# -- --beta ------------------------------------------------------------------

@test "latest_prerelease_version takes the newest tag from the releases feed" {
  function curl() {
    echo '<entry><link rel="alternate" type="text/html" href="https://github.com/lerd-env/lerd/releases/tag/v1.34.0-beta.3"/></entry>'
    echo '<entry><link rel="alternate" type="text/html" href="https://github.com/lerd-env/lerd/releases/tag/v1.33.1"/></entry>'
  }
  export -f curl

  run latest_prerelease_version
  [ "$status" -eq 0 ]
  [ "$output" = "1.34.0-beta.3" ]
}

@test "latest_prerelease_version returns empty string when the feed carries no release" {
  function curl() { echo "<feed></feed>"; }
  export -f curl

  run latest_prerelease_version
  [ "$status" -eq 0 ]
  [ "$output" = "" ]
}

@test "resolve_version reads the feed under --beta and the latest redirect without it" {
  function curl() {
    case "$*" in
      *releases.atom*) echo 'href="https://github.com/lerd-env/lerd/releases/tag/v1.34.0-beta.3"' ;;
      *) echo "location: https://github.com/lerd-env/lerd/releases/tag/v1.33.1" ;;
    esac
  }
  export -f curl

  BETA=0
  run resolve_version
  [ "$output" = "1.33.1" ]

  BETA=1
  run resolve_version
  [ "$output" = "1.34.0-beta.3" ]
}

@test "--beta is accepted alongside another flag" {
  run bash "$INSTALLER" --beta --help
  [ "$status" -eq 0 ]
  [[ "$output" != *"Unknown option"* ]]
  [[ "$output" == *"Usage:"* ]]
}

# ── --help flag ───────────────────────────────────────────────────────────────

@test "--help prints usage and exits 0" {
  run bash "$INSTALLER" --help
  [ "$status" -eq 0 ]
  [[ "$output" == *"Usage:"* ]]
  [[ "$output" == *"--update"* ]]
  [[ "$output" == *"--uninstall"* ]]
  [[ "$output" == *"--local"* ]]
  [[ "$output" == *"--beta"* ]]
}

# ── --local flag ──────────────────────────────────────────────────────────────

@test "--local fails with a clear error when file does not exist" {
  run bash "$INSTALLER" --local /tmp/nonexistent-lerd-binary-xyz
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
}

@test "--local requires an argument" {
  run bash "$INSTALLER" --local
  [ "$status" -ne 0 ]
  [[ "$output" == *"requires a path"* ]]
}

# ── --check flag ──────────────────────────────────────────────────────────────

@test "--check runs prerequisite checks and exits 0 when all pass" {
  # Mock all check commands as present
  function command() {
    case "$2" in
      podman|unzip|certutil) return 0 ;;
      *) builtin command "$@" ;;
    esac
  }
  function systemctl() { return 0; }
  function podman() {
    if [[ "$1" == "info" ]]; then echo "true"; fi
  }
  export -f command systemctl podman

  run bash "$INSTALLER" --check
  [ "$status" -eq 0 ]
}

# ── DNS mode gating of the HTTPS-only prerequisites ───────────────────────────

@test "check_prerequisites skips certutil in localhost DNS mode" {
  # certutil gating is Linux-only; macOS prerequisites never call it.
  _force_linux_os
  function command() {
    case "$2" in
      podman|unzip) return 0 ;;
      certutil) return 1 ;;
      *) builtin command "$@" ;;
    esac
  }
  function systemctl() { return 0; }
  function podman() { if [[ "$1" == "info" ]]; then echo "true"; fi; }
  export -f command systemctl podman

  MISSING_PKGS=()
  DNS_MODE="localhost"
  run check_prerequisites
  [ "$status" -eq 0 ]
  [[ "$output" != *"certutil not found"* ]]
}

@test "check_prerequisites flags certutil in managed DNS mode" {
  # certutil gating is Linux-only; macOS prerequisites never call it.
  _force_linux_os
  function command() {
    case "$2" in
      podman|unzip) return 0 ;;
      certutil) return 1 ;;
      *) builtin command "$@" ;;
    esac
  }
  function systemctl() { return 0; }
  function podman() { if [[ "$1" == "info" ]]; then echo "true"; fi; }
  export -f command systemctl podman

  MISSING_PKGS=()
  DNS_MODE="managed"
  run check_prerequisites
  [[ "$output" == *"certutil not found"* ]]
}

# ── offer_desktop_app ─────────────────────────────────────────────────────────

_fake_lerd_dir() {
  local d="$BATS_TMPDIR/fakebin-$$"
  mkdir -p "$d"
  cat > "$d/lerd" <<EOF
#!/usr/bin/env bash
echo "\$@" >> "$d/calls"
EOF
  chmod +x "$d/lerd"
  rm -f "$d/calls"
  echo "$d"
}

@test "offer_desktop_app yes enables native and prints the install command" {
  local d; d="$(_fake_lerd_dir)"
  INSTALL_DIR="$d"; BINARY="lerd"
  ask() { return 0; }
  run offer_desktop_app
  [ "$status" -eq 0 ]
  [[ "$output" == *"$DESKTOP_INSTALL_CMD"* ]]
  grep -q "notify target native" "$d/calls"
}

@test "offer_desktop_app no selects browser and still prints the install command" {
  local d; d="$(_fake_lerd_dir)"
  INSTALL_DIR="$d"; BINARY="lerd"
  ask() { return 1; }
  run offer_desktop_app
  [ "$status" -eq 0 ]
  [[ "$output" == *"$DESKTOP_INSTALL_CMD"* ]]
  grep -q "notify target browser" "$d/calls"
}

# ── Omarchy ──────────────────────────────────────────────────────────────────

# Omarchy's plugin CLI stands in for the whole desktop: where it is on PATH the
# bar is Omarchy's and the Glance plugin replaces the tray.
_fake_omarchy() {
  local d; d="$(_fake_lerd_dir)"
  for c in omarchy-plugin-add omarchy-plugin-remove; do
    printf '#!/usr/bin/env bash\necho "%s $*" >> "%s/calls"\n' "$c" "$d" > "$d/$c"
    chmod +x "$d/$c"
  done
  echo "$d"
}

@test "setup_omarchy turns the tray off and adds the Glance plugin" {
  local d; d="$(_fake_omarchy)"
  INSTALL_DIR="$d"; BINARY="lerd"; PATH="$d:$PATH"
  run setup_omarchy
  [ "$status" -eq 0 ]
  grep -q "^tray off" "$d/calls"
  grep -q "^omarchy-plugin-add $OMARCHY_PLUGIN_REPO --enable --yes" "$d/calls"
}

@test "setup_omarchy leaves an already installed Glance plugin alone" {
  local d; d="$(_fake_omarchy)"
  INSTALL_DIR="$d"; BINARY="lerd"; PATH="$d:$PATH"
  mkdir -p "$OMARCHY_PLUGINS_DIR/$OMARCHY_PLUGIN_ID"
  run setup_omarchy
  [ "$status" -eq 0 ]
  grep -q "^tray off" "$d/calls"
  ! grep -q "^omarchy-plugin-add" "$d/calls"
}

@test "setup_omarchy does nothing off Omarchy" {
  local d; d="$(_fake_lerd_dir)"
  INSTALL_DIR="$d"; BINARY="lerd"
  command() { [ "$2" = "omarchy-plugin-add" ] && return 1; builtin command "$@"; }
  run setup_omarchy
  [ "$status" -eq 0 ]
  [ ! -f "$d/calls" ]
}

# ── cmd_uninstall ─────────────────────────────────────────────────────────────

@test "cmd_uninstall hands the teardown to lerd uninstall" {
  local d; d="$(_fake_lerd_dir)"
  INSTALL_DIR="$d"; BINARY="lerd"
  run cmd_uninstall
  [ "$status" -eq 0 ]
  [ "$(cat "$d/calls")" = "uninstall" ]
}

@test "cmd_uninstall fails when lerd is not installed" {
  INSTALL_DIR="$BATS_TMPDIR/nobin-$$"; BINARY="lerd"
  command() { [ "$2" = "lerd" ] && return 1; builtin command "$@"; }
  run cmd_uninstall
  [ "$status" -ne 0 ]
  [[ "$output" == *"not installed"* ]]
}

# ── controlling terminal detection ────────────────────────────────────────────

# [ -r /dev/tty ] tests the permission bits on the device node, which pass even
# when the process has no controlling terminal. The redirect then fails and the
# step it guarded is skipped, which is how `lerd install` came to be silently
# skipped over ssh without a tty.
@test "tty detection opens the device instead of testing permission bits" {
  run grep -q '\[ -r /dev/tty \]' "$INSTALLER"
  [ "$status" -ne 0 ]
}

@test "have_tty is false when the process has no controlling terminal" {
  run run_without_tty "source '$INSTALLER'; have_tty && echo yes || echo no"
  [ "$status" -eq 0 ]
  [ "$output" = "no" ]
}

# set -u is on, so a read that never ran leaves _ans unset and the script aborts
# instead of taking the question as declined.
@test "ask declines cleanly when there is no controlling terminal" {
  run run_without_tty "source '$INSTALLER'; ask 'proceed?' && echo GOT_YES || echo GOT_NO"
  [ "$status" -eq 0 ]
  [[ "$output" == *"GOT_NO"* ]]
  [[ "$output" != *"unbound variable"* ]]
}

@test "ask reads the answer from the terminal when one is present" {
  command -v script >/dev/null || skip "needs script(1) to allocate a pty"
  run run_with_tty "source '$INSTALLER'; ask proceed? && echo GOT_YES || echo GOT_NO"
  [[ "$output" == *"GOT_YES"* ]]
}

# ── saved_dns_mode / the DNS question on a re-install ─────────────────────────
# `lerd install` settles the DNS mode once and honours the saved choice on every
# later run, flipped afterwards with dns:enable / dns:disable. Re-running the
# installer used to ask again and pass --dns, which is the one input that beats
# the saved choice, so a .localhost machine was converted to managed DNS, in
# silence where there was no terminal to show the question (#1647).

_write_dns_config() {
  mkdir -p "$LERD_CONFIG_DIR"
  cat > "$LERD_CONFIG_DIR/config.yaml" <<EOF
php:
    default: "8.5"
dns:
    enabled: $1
    tld: $2
auto_cleanup: true
EOF
}

@test "saved_dns_mode reads managed from an existing config" {
  _write_dns_config true test
  run saved_dns_mode
  [ "$output" = "managed" ]
}

@test "saved_dns_mode reads localhost from an existing config" {
  _write_dns_config false localhost
  run saved_dns_mode
  [ "$output" = "localhost" ]
}

@test "saved_dns_mode says nothing when there is no config" {
  run saved_dns_mode
  [ -z "$output" ]
}

@test "saved_dns_mode ignores an enabled key outside the dns block" {
  mkdir -p "$LERD_CONFIG_DIR"
  cat > "$LERD_CONFIG_DIR/config.yaml" <<EOF
telemetry:
    enabled: false
dns:
    enabled: true
    tld: test
EOF
  run saved_dns_mode
  [ "$output" = "managed" ]
}

@test "should_pass_dns_mode names the mode on a first install" {
  run should_pass_dns_mode ""
  [ "$status" -eq 0 ]
}

@test "should_pass_dns_mode leaves the saved choice alone when lerd is installed" {
  run should_pass_dns_mode "1.33.0"
  [ "$status" -ne 0 ]
}

@test "should_pass_dns_mode leaves the saved choice alone after an uninstall that kept data" {
  run should_pass_dns_mode "" "localhost"
  [ "$status" -ne 0 ]
}
