#!/usr/bin/env bash
# Lerd installer — https://lerd.sh
# Usage:
#   Install:   curl -fsSL https://lerd.sh/install.sh | bash
#      or:     wget -qO- https://lerd.sh/install.sh | bash
#   Update:    lerd-installer --update
#   Uninstall: lerd-installer --uninstall

set -euo pipefail

# ── Constants ────────────────────────────────────────────────────────────────
# REPO is the GitHub owner/name release assets are fetched from; override with
# LERD_REPO so a future org move needs no installer change.
REPO="${LERD_REPO:-lerd-env/lerd}"
BINARY="lerd"
# Command shown to install the optional Lerd desktop app (a dedicated window
# with native desktop notifications), distributed as a Flatpak. Overridable so
# the ref URL can move.
DESKTOP_INSTALL_CMD="${LERD_DESKTOP_INSTALL_CMD:-flatpak install --user https://lerd.sh/lerd.flatpakref}"
DESKTOP_APP_ID="sh.lerd.Desktop"
# On macOS the app ships as a notarized DMG from lerd-desktop's releases, signed
# by this team. Overridable so tests and mirrors can point elsewhere.
DESKTOP_DMG_BASE="${LERD_DESKTOP_DMG_BASE:-https://github.com/lerd-env/lerd-desktop/releases/latest/download}"
DESKTOP_TEAM_ID="ZY476R7D97"
# On Omarchy the Glance bar plugin shows lerd's state, so it replaces the tray.
OMARCHY_PLUGIN_REPO="https://github.com/lerd-env/lerd-omarchy-glance"
OMARCHY_PLUGIN_ID="sh.lerd.glance"
OMARCHY_PLUGINS_DIR="$HOME/.config/omarchy/plugins"
OMARCHY_SYSTEM_PATH="/usr/share/omarchy"
INSTALL_DIR="${LERD_INSTALL_DIR:-$HOME/.local/bin}"
LERD_CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/lerd"
# Set by --beta: install and update from the newest release including
# prereleases, rather than from the stable line alone.
BETA=0

# ── Colors ───────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
  RED='\033[0;31m'; YELLOW='\033[1;33m'; GREEN='\033[0;32m'
  CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
else
  RED=''; YELLOW=''; GREEN=''; CYAN=''; BOLD=''; RESET=''
fi

# ── Helpers ──────────────────────────────────────────────────────────────────
info()    { echo -e "  ${CYAN}-->${RESET} $*"; }
success() { echo -e "  ${GREEN}✓${RESET}  $*"; }
warn()    { echo -e "  ${YELLOW}!${RESET}  $*"; }
error()   { echo -e "  ${RED}✗${RESET}  $*" >&2; }
die()     { error "$*"; exit 1; }
header()  { echo -e "\n${BOLD}$*${RESET}"; }
# /dev/tty exists and is readable by its permission bits even when the process
# has no controlling terminal, so only opening it answers the question. Getting
# this wrong skips whatever the caller guarded, silently.
have_tty() { ( : >/dev/tty ) 2>/dev/null; }
# _ans is seeded because `set -u` is on and a read that never ran leaves it
# unset, which aborts the script instead of declining the question.
ask()     { local _ans=""; echo -en "  ${BOLD}?${RESET}  $* [y/N] "; read -r _ans 2>/dev/null </dev/tty || true; [[ "$_ans" =~ ^[Yy]$ ]]; }
ask_yes() { local _ans=""; echo -en "  ${BOLD}?${RESET}  $* [Y/n] "; read -r _ans 2>/dev/null </dev/tty || true; default_yes "$_ans"; }
default_yes() { [[ -z "$1" || "$1" =~ ^[Yy]([Ee][Ss])?$ ]]; }
star_note() {
  echo ""
  echo -e "  ${CYAN}★${RESET}  If lerd is useful to you, a GitHub star helps others find it:"
  echo -e "     https://github.com/${REPO}"
}

# ── Platform detection ───────────────────────────────────────────────────────
detect_os() {
  case "$(uname -s)" in
    Linux)  echo "linux" ;;
    Darwin) echo "darwin" ;;
    *) die "Unsupported OS: $(uname -s). Lerd supports Linux and macOS." ;;
  esac
}

detect_arch() {
  # macOS reports arm64; Linux reports aarch64: both map to the arm64 release.
  case "$(uname -m)" in
    x86_64)        echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) die "Unsupported architecture: $(uname -m)" ;;
  esac
}

detect_distro() {
  if [ -f /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    echo "${ID:-unknown}"
  else
    echo "unknown"
  fi
}

# detect_distro_like returns the ID_LIKE field from /etc/os-release, the space
# separated list of parent distros a derivative declares (e.g. bazzite -> fedora).
detect_distro_like() {
  if [ -f /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    echo "${ID_LIKE:-}"
  fi
}

# is_atomic reports whether this host booted from an ostree image (Fedora
# Silverblue, Bazzite, Kinoite, CoreOS). On these, packages are layered with
# rpm-ostree and a reboot, not installed into the running system.
is_atomic() {
  [ -f /run/ostree-booted ]
}

distro_family() {
  local distro; distro="$(detect_distro)"
  case "$distro" in
    arch|manjaro|endeavouros|garuda) echo "arch"; return ;;
    debian|ubuntu|pop|linuxmint|elementary|zorin) echo "debian"; return ;;
    fedora|rhel|centos|rocky|alma) echo "fedora"; return ;;
    opensuse*|sles) echo "suse"; return ;;
  esac
  # A derivative not listed above (bazzite, nobara, cachyos, ...) still declares
  # its parents in ID_LIKE, so fall back to that before giving up as unknown.
  local like; like=" $(detect_distro_like) "
  case "$like" in
    *" arch "*)                           echo "arch"; return ;;
    *" debian "*|*" ubuntu "*)            echo "debian"; return ;;
    *" fedora "*|*" rhel "*|*" centos "*) echo "fedora"; return ;;
    *" suse "*|*" opensuse "*)            echo "suse"; return ;;
  esac
  echo "unknown"
}

# ── Prerequisite checks ──────────────────────────────────────────────────────
MISSING_PKGS=()

# DNS mode chosen by the user before prerequisites are checked. "managed" runs
# lerd's DNS server and local CA so sites resolve at https://<name>.test with trusted
# certs; "localhost" skips all of that and serves http://<name>.localhost over
# plain HTTP. The choice is passed straight to `lerd install --dns` so the
# prompt isn't shown twice, and it lets us skip the HTTPS-only packages
# (certutil / nss-tools) when the user only wants .localhost.
DNS_MODE="managed"

# saved_dns_mode echoes the DNS mode already recorded in the config, or nothing
# when there is no config to read. Scoped to the dns block so an enabled: key
# belonging to some other section is never mistaken for this one.
saved_dns_mode() {
  local cfg="${LERD_CONFIG_DIR}/config.yaml"
  [ -f "$cfg" ] || return 0
  awk '
    /^[^[:space:]#]/ { in_dns = ($0 ~ /^dns:/) }
    in_dns && /^[[:space:]]+enabled:/ {
      print ($0 ~ /false/) ? "localhost" : "managed"; exit
    }
  ' "$cfg"
}

# should_pass_dns_mode reports whether `lerd install` should be told the mode.
# Only a first install names it: the binary settles the question once and
# honours the saved choice on every run after that, and --dns is the one input
# that overrides it. The arguments are the version already installed and the
# saved mode, either of which may be empty: an uninstall that kept its data
# leaves no binary but still has the choice on record.
should_pass_dns_mode() {
  [ -z "${1:-}" ] && [ -z "${2:-}" ]
}

ask_dns_mode() {
  local _ans=""
  header "DNS mode"
  echo "  lerd can manage local DNS so sites resolve at https://<name>.test with"
  echo "  trusted certificates, or stay out of the way and serve them at"
  echo "  http://<name>.localhost over plain HTTP. The .localhost mode needs no"
  echo "  DNS server, no local CA, and no extra packages."
  echo ""
  echo -en "  ${BOLD}?${RESET}  Let lerd manage DNS for .test sites with HTTPS? [Y/n] "
  read -r _ans 2>/dev/null </dev/tty || true
  if [[ "$_ans" =~ ^[Nn]$ ]]; then
    DNS_MODE="localhost"
    info "Using *.localhost over HTTP, no certificates or extra packages required"
  else
    DNS_MODE="managed"
  fi
}

check_cmd() {
  local cmd="$1" pkg="${2:-$1}" desc="${3:-}"
  if command -v "$cmd" &>/dev/null; then
    success "$cmd found ($(command -v "$cmd"))"
  else
    warn "$cmd not found${desc:+ — $desc}"
    MISSING_PKGS+=("$pkg")
  fi
}

check_systemd_user() {
  if systemctl --user status &>/dev/null 2>&1; then
    success "systemd user session active"
  else
    warn "systemd user session not active"
    warn "Run: loginctl enable-linger \$USER"
    warn "Then log out and back in"
    MISSING_PKGS+=("_systemd_linger")
  fi
}

check_dns_resolver() {
  if systemctl is-active --quiet NetworkManager 2>/dev/null; then
    success "NetworkManager running"
  elif systemctl is-active --quiet systemd-resolved 2>/dev/null; then
    success "systemd-resolved running"
  elif is_wsl; then
    # WSL resolves through systemd-resolved, never NetworkManager, and lerd install
    # installs or starts it at the point it can keep the distro's DNS working.
    info "systemd-resolved not running yet (lerd install sets it up on WSL)"
  else
    warn "No supported DNS resolver running (need NetworkManager or systemd-resolved)"
    MISSING_PKGS+=("networkmanager")
  fi
}

# dnsmasq lives in sbin on Debian-family systems, which a normal user's PATH
# often omits, so a plain PATH lookup would miss an installed binary.
DNSMASQ_PATHS="/usr/sbin/dnsmasq /sbin/dnsmasq /usr/local/sbin/dnsmasq"

host_has_dnsmasq() {
  command -v dnsmasq &>/dev/null && return 0
  local p
  for p in $DNSMASQ_PATHS; do
    [ -x "$p" ] && return 0
  done
  return 1
}

# Overridable so the tests can point the resolver check at a fixture.
RESOLV_CONF="${RESOLV_CONF:-/etc/resolv.conf}"

# systemd-resolved counts as the resolver only when resolv.conf actually routes
# through it, the same test internal/dns makes. A host where resolved is running
# but NetworkManager still writes resolv.conf takes the NM dnsmasq path instead.
resolved_is_resolver() {
  systemctl is-active --quiet systemd-resolved 2>/dev/null || return 1
  grep -qE '127\.0\.0\.53|systemd-resolved' "$RESOLV_CONF" 2>/dev/null
}

# NetworkManager without systemd-resolved resolves .test through NM's dnsmasq
# plugin, which needs the host dnsmasq binary that nothing pulls in on Arch.
# Queue it here so the consent prompt covers it instead of lerd stopping later.
check_nm_dnsmasq() {
  systemctl is-active --quiet NetworkManager 2>/dev/null || return 0
  resolved_is_resolver && return 0
  if host_has_dnsmasq; then
    success "dnsmasq found (NetworkManager's DNS plugin runs it)"
    return
  fi
  local pkg="dnsmasq"
  [ "$(distro_family)" = "debian" ] && pkg="dnsmasq-base"
  warn "dnsmasq missing (NetworkManager's DNS plugin needs it for .test resolution)"
  MISSING_PKGS+=("$pkg")
}

check_certutil() {
  if command -v certutil &>/dev/null; then
    success "certutil found (needed for the lerd CA to be trusted in browsers)"
    return
  fi
  local family; family="$(distro_family)"
  local pkg
  case "$family" in
    arch)   pkg="nss" ;;
    debian) pkg="libnss3-tools" ;;
    fedora) pkg="nss-tools" ;;
    suse)   pkg="mozilla-nss-tools" ;;
    *)      pkg="nss-tools" ;;
  esac
  if is_atomic; then
    # nss-tools can't be layered inline (rpm-ostree needs a reboot), so don't
    # queue it for the package installer, which would die or fail on ostree.
    # Guide instead, and leave localhost as the no-package alternative.
    warn "certutil not found — lerd can't trust HTTPS certs in Chrome/Firefox on this atomic image"
    info "For browser trust: rpm-ostree install $pkg, reboot, then run 'lerd dns:repair'"
    info "Or re-run and choose localhost DNS to serve plain http with no certificates"
    return
  fi
  warn "certutil not found — lerd won't be able to trust HTTPS certs in Chrome/Firefox"
  MISSING_PKGS+=("$pkg")
}

check_podman_rootless() {
  if ! command -v podman &>/dev/null; then
    return  # already flagged by check_cmd
  fi
  if podman info --format '{{.Host.Security.Rootless}}' 2>/dev/null | grep -q true; then
    success "Podman running rootless"
  else
    warn "Podman may not be configured for rootless — check 'podman info'"
  fi
}

check_prerequisites() {
  if [ "$(detect_os)" = "darwin" ]; then
    check_prerequisites_macos
  else
    check_prerequisites_linux
  fi
}

# macOS needs only the podman CLI on PATH; `lerd install` brings the Podman
# machine up itself, and the CA, DNS and launchd are all handled inside the binary.
# Missing packages are installed via Homebrew (no sudo, unlike the Linux path).
check_prerequisites_macos() {
  header "Checking prerequisites"

  check_cmd podman podman "container runtime"

  if [ ${#MISSING_PKGS[@]} -eq 0 ]; then
    success "All prerequisites met"
    return
  fi

  echo ""
  warn "Missing: ${MISSING_PKGS[*]}"
  if ask "Install missing packages now (via Homebrew)?"; then
    install_packages "${MISSING_PKGS[@]}"
  fi
}

check_prerequisites_linux() {
  header "Checking prerequisites"

  check_cmd podman podman "container runtime"
  check_dns_resolver
  check_systemd_user
  check_podman_rootless
  if [ "$DNS_MODE" = "managed" ]; then
    check_certutil
    check_nm_dnsmasq
  fi

  if [ ${#MISSING_PKGS[@]} -eq 0 ]; then
    success "All prerequisites met"
    return
  fi

  echo ""
  warn "Missing: ${MISSING_PKGS[*]}"

  local installable=()
  for p in "${MISSING_PKGS[@]}"; do
    [[ "$p" != _* ]] && installable+=("$p")
  done

  if [ ${#installable[@]} -gt 0 ] && ask "Install missing packages now?"; then
    install_packages "${installable[@]}"
  fi

  # Handle special cases
  for p in "${MISSING_PKGS[@]}"; do
    case "$p" in
      _systemd_linger)
        if ask "Enable systemd linger for $USER now?"; then
          loginctl enable-linger "$USER"
          success "Linger enabled — please log out and back in before running 'lerd install'"
        fi
        ;;
    esac
  done
}

install_packages() {
  local pkgs=("$@")

  header "Installing: ${pkgs[*]}"

  if [ "$(detect_os)" = "darwin" ]; then
    if ! command -v brew &>/dev/null; then
      die "Homebrew not found. Install it from https://brew.sh then re-run, or install manually: ${pkgs[*]}"
    fi
    brew install "${pkgs[@]}"
    success "Packages installed"
    return
  fi

  local family; family="$(distro_family)"

  if is_atomic; then
    warn "atomic image detected — packages are layered with rpm-ostree, not installed into the running system"
    info "Run:  rpm-ostree install ${pkgs[*]}  then reboot and re-run the installer"
    return
  fi

  case "$family" in
    arch)
      sudo pacman -S --needed --noconfirm "${pkgs[@]}"
      ;;
    debian)
      sudo apt-get update -q
      sudo apt-get install -y "${pkgs[@]}"
      ;;
    fedora)
      sudo dnf install -y "${pkgs[@]}"
      ;;
    suse)
      sudo zypper install -y "${pkgs[@]}"
      ;;
    *)
      die "Don't know how to install packages on this distro. Install manually: ${pkgs[*]}"
      ;;
  esac

  success "Packages installed"

  # Initialize podman storage for the current user after first install.
  # This runs any pending migrations and sets up ~/.local/share/containers.
  if command -v podman &>/dev/null; then
    podman system migrate &>/dev/null || true
  fi
}

# ── Download tool ────────────────────────────────────────────────────────────
# Prefer curl; fall back to wget. Errors out if neither is available.
_download_tool() {
  if command -v curl &>/dev/null; then
    echo "curl"
  elif command -v wget &>/dev/null; then
    echo "wget"
  else
    die "Neither curl nor wget found. Install one and retry."
  fi
}

# Retry transient failures with a short pause, bound connects, and abort a
# stalled transfer (curl: under 1 byte/s for 30s) so it fails into the retry
# path instead of hanging forever.
CURL_RETRY=(--retry 3 --retry-delay 2 --connect-timeout 15 --speed-limit 1 --speed-time 30)
WGET_RETRY=(--tries=3 --timeout=15)

# fetch <url> <dest>  — download URL to dest file
fetch() {
  local url="$1" dest="$2"
  case "$(_download_tool)" in
    curl) curl -fsSL "${CURL_RETRY[@]}" --progress-bar "$url" -o "$dest" ;;
    wget) wget -q --show-progress "${WGET_RETRY[@]}" "$url" -O "$dest" ;;
  esac
}

# fetch_stdout <url>  — download URL to stdout (for piping into grep/sed)
fetch_stdout() {
  local url="$1"
  case "$(_download_tool)" in
    curl) curl -fsSL "${CURL_RETRY[@]}" "$url" ;;
    wget) wget -qO- "${WGET_RETRY[@]}" "$url" ;;
  esac
}

# ── GitHub release helpers ───────────────────────────────────────────────────
latest_version() {
  # Use the HTML releases/latest redirect — no API key, not rate-limited.
  # GitHub redirects to the canonical release URL whose path contains the tag.
  local url="https://github.com/${REPO}/releases/latest"
  local location
  case "$(_download_tool)" in
    curl) location="$(curl -fsSLI "${CURL_RETRY[@]}" --stderr /dev/null \
            -H "User-Agent: lerd-installer" \
            "$url" | grep -i '^location:' | tail -1)" ;;
    wget) location="$(wget -qS --spider "${WGET_RETRY[@]}" \
            --header "User-Agent: lerd-installer" \
            "$url" 2>&1 | grep -i 'Location:'  | tail -1)" ;;
  esac

  # location header value looks like: .../releases/tag/v0.1.33
  echo "$location" | sed -E 's|.*/releases/tag/v?([^[:space:]]+).*|\1|' | tr -d '\r'
}

# The newest release including prereleases. GitHub's releases/latest redirect
# skips a prerelease by design, so the beta line is read off the atom feed,
# which is newest-first and needs no API token either.
latest_prerelease_version() {
  local tag
  tag="$(fetch_stdout "https://github.com/${REPO}/releases.atom" 2>/dev/null \
    | grep -oE 'releases/tag/v[^"<[:space:]]+' | head -1 || true)"
  echo "${tag#releases/tag/v}" | tr -d '\r'
}

# The release this run installs: the newest one under --beta, the newest stable
# one otherwise.
resolve_version() {
  if [ "$BETA" = "1" ]; then
    latest_prerelease_version
  else
    latest_version
  fi
}

# download_binary <version> <arch> <destdir>
# Downloads and extracts the release archive into <destdir>.
# The extracted binary will be at <destdir>/lerd.
# All output goes to stderr — nothing is printed to stdout.
download_binary() {
  local version="$1" arch="$2" destdir="$3"
  local os; os="$(detect_os)"
  local filename="lerd_${version}_${os}_${arch}.tar.gz"
  local url="https://github.com/${REPO}/releases/download/v${version}/${filename}"

  info "Downloading lerd v${version} (${arch}) via $(_download_tool) ..."
  if ! fetch "$url" "${destdir}/${filename}"; then
    die "Download failed (HTTP 404).\nNo release v${version} found at:\n  ${url}\n\nIf you built lerd locally, use:\n  bash install.sh --local ./build/lerd"
  fi

  if ! tar -xzf "${destdir}/${filename}" -C "$destdir" 2>&1; then
    die "Failed to extract archive: ${filename}"
  fi
}

# The version of the installed binary, prerelease and all, without the
# git-describe tail a local build carries. 1.34.0-beta.3 is a release and has to
# compare equal to the tag it came from.
installed_version() {
  local raw; raw="$(installed_version_raw)"
  [ -n "$raw" ] || { echo ""; return; }
  version_tag "$raw"
}

# Full version token including any git-describe suffix (e.g.
# 1.25.0-6-g7d030096-dirty). installed_version() collapses that to the bare
# 1.25.0, which is what version_is_dev needs the suffix from.
installed_version_raw() {
  if command -v lerd &>/dev/null; then
    lerd --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+[A-Za-z0-9.-]*' | head -1 || echo ""
  else
    echo ""
  fi
}

# The release tag a version token was built from: everything up to the
# git-describe tail. 1.34.0-beta.3-11-g632148be-dirty came from 1.34.0-beta.3.
version_tag() {
  echo "$1" | sed -E 's/(-[0-9]+-g[0-9a-f]+)?(-dirty)?$//'
}

# True when the token carries a git-describe tail, which means an
# ahead-of-release local build the installer must not silently overwrite with
# the release it was built on top of. A prerelease tag is not one of these.
version_is_dev() {
  [ "$1" != "$(version_tag "$1")" ]
}

# True for a tag carrying a semver prerelease, so a beta rather than a stable.
version_is_prerelease() {
  [[ "$(version_tag "$1")" =~ ^[0-9]+\.[0-9]+\.[0-9]+-[A-Za-z] ]]
}

# Guards an overwrite of a local development build. Prints a warning and asks
# before replacing it; keeps the build and exits 0 when declined or when no tty
# is available to confirm. $1 is the release version about to be installed.
guard_dev_build() {
  local target="$1" current_raw; current_raw="$(installed_version_raw)"
  version_is_dev "$current_raw" || return 0
  warn "A local development build (v${current_raw}) is installed."
  if have_tty && ask "Replace it with release v${target}?"; then
    return 0
  fi
  info "Keeping your local build. Reinstall a dev build with: install.sh --local <path>"
  exit 0
}

# Guards a move off the beta line. Someone running the plain installer on a
# prerelease is reinstalling, not asking to be taken back to stable, so the
# downgrade is confirmed first. $1 is the stable version about to be installed.
guard_prerelease() {
  local target="$1"
  [ "$BETA" = "1" ] && return 0
  local current; current="$(installed_version)"
  version_is_prerelease "$current" || return 0
  warn "A prerelease (v${current}) is installed."
  if have_tty && ask "Replace it with the stable release v${target}?"; then
    return 0
  fi
  info "Keeping your prerelease. Stay on the beta line with: install.sh --update --beta"
  exit 0
}

# ── Shell integration ────────────────────────────────────────────────────────
SHELL_MARKER="# Added by Lerd installer"

detect_shell_rc() {
  local shell; shell="$(basename "${SHELL:-bash}")"
  case "$shell" in
    fish) echo "$HOME/.config/fish/conf.d/lerd.fish" ;;
    zsh)  echo "$HOME/.zshrc" ;;
    *)
      # macOS Terminal launches bash as a login shell, which reads
      # .bash_profile, not .bashrc; Linux interactive bash reads .bashrc.
      if [ "$(detect_os)" = "darwin" ]; then
        echo "$HOME/.bash_profile"
      else
        echo "$HOME/.bashrc"
      fi
      ;;
  esac
}

add_to_path() {
  local shell; shell="$(basename "${SHELL:-bash}")"
  local rc; rc="$(detect_shell_rc)"

  # Check if already in current PATH
  if [[ ":$PATH:" == *":$INSTALL_DIR:"* ]]; then
    success "$INSTALL_DIR is already in PATH"
    return
  fi

  # Don't add if already present in rc file
  if grep -q "$SHELL_MARKER" "$rc" 2>/dev/null; then
    success "PATH already configured in $rc"
    return
  fi

  case "$shell" in
    fish)
      mkdir -p "$(dirname "$rc")"
      printf '\n%s\nfish_add_path %s\n' "$SHELL_MARKER" "$INSTALL_DIR" >> "$rc"
      ;;
    *)
      printf '\n%s\nexport PATH="%s:$PATH"\n' "$SHELL_MARKER" "$INSTALL_DIR" >> "$rc"
      ;;
  esac
  success "Added $INSTALL_DIR to PATH in $rc"
  warn "Reload your shell or run: source $rc"
}

# ── Install ──────────────────────────────────────────────────────────────────
cmd_install() {
  local local_binary="${1:-}"
  header "Installing Lerd"

  # Sampled before anything installs: empty means a genuinely fresh install,
  # the only time we offer the desktop app.
  local was_installed; was_installed="$(installed_version)"

  # Validate local binary path before running any checks so the error is clear.
  if [ -n "$local_binary" ]; then
    [ -f "$local_binary" ] || die "File not found: $local_binary"
  fi

  # Only a first install asks. A machine that already has lerd, or kept its
  # config through an uninstall, keeps the mode it settled on, read back here so
  # the prerequisite check knows whether this run needs certutil, and left to the
  # binary rather than forced with --dns.
  local saved_mode; saved_mode="$(saved_dns_mode)"
  if should_pass_dns_mode "$was_installed" "$saved_mode"; then
    ask_dns_mode
  else
    DNS_MODE="${saved_mode:-managed}"
  fi
  check_prerequisites

  if ! command -v podman &>/dev/null; then
    die "podman is required but not installed. Install it and re-run this script."
  fi

  mkdir -p "$INSTALL_DIR"

  if [ -n "$local_binary" ]; then
    # ── Local binary path supplied (e.g. ./build/lerd) ──
    [ -f "$local_binary" ] || die "File not found: $local_binary"
    install -m 755 "$local_binary" "${INSTALL_DIR}/${BINARY}"
    # lerd.exe is the Windows side of a WSL install; wsl:setup copies it out.
    local local_exe; local_exe="$(dirname "$local_binary")/lerd.exe"
    if is_wsl && [ -f "$local_exe" ]; then install -m 755 "$local_exe" "${INSTALL_DIR}/lerd.exe"; fi
    local version; version="$("${INSTALL_DIR}/${BINARY}" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "dev")"
    success "Installed lerd ${version} (local) → ${INSTALL_DIR}/${BINARY}"
  else
    # ── Download from GitHub releases ──
    local arch; arch="$(detect_arch)"
    local version; version="$(resolve_version)"
    if [ -z "$version" ]; then
      die "No releases found at https://github.com/${REPO}/releases\n\nIf you built lerd locally, install with:\n  bash install.sh --local ./build/lerd"
    fi

    local current; current="$(installed_version)"
    if [ -n "$current" ] && [ "$current" = "$version" ]; then
      success "Lerd v${version} is already installed and up to date"
      exit 0
    fi
    guard_dev_build "$version"
    guard_prerelease "$version"

    local tmpdir; tmpdir="$(mktemp -d)"
    download_binary "$version" "$arch" "$tmpdir"
    install -m 755 "${tmpdir}/lerd" "${INSTALL_DIR}/${BINARY}"
    [ -f "${tmpdir}/lerd-tray" ] && install -m 755 "${tmpdir}/lerd-tray" "${INSTALL_DIR}/lerd-tray"
    if is_wsl && [ -f "${tmpdir}/lerd.exe" ]; then install -m 755 "${tmpdir}/lerd.exe" "${INSTALL_DIR}/lerd.exe"; fi
    rm -rf "$tmpdir"
    success "Installed lerd v${version} → ${INSTALL_DIR}/${BINARY}"
  fi

  add_to_path

  # Offer the desktop app on a fresh install. Asked before `lerd install` so the
  # launcher it writes already knows the app is there: on Linux it names itself
  # to match, on macOS it steps aside. Not on WSL: the Flatpak cannot run there.
  if [ -z "$was_installed" ] && have_tty; then
    if [ "$(uname -s)" = "Darwin" ]; then
      offer_desktop_app_macos
    elif [ "$(uname -s)" = "Linux" ] && ! is_wsl; then
      offer_desktop_app
    fi
  fi

  echo ""
  info "Running 'lerd install' to complete setup ..."
  echo ""
  # When this script is piped through `curl|bash`, our own stdin is the pipe
  # and lerd's prompts would silently hit EOF. Hand it /dev/tty when one is
  # available so [Y/n] questions reach the user.
  # The +expansion keeps an empty array from tripping set -u on the bash 3.2
  # macOS still ships.
  local dns_args=()
  if should_pass_dns_mode "$was_installed" "$saved_mode"; then
    dns_args=(--dns "$DNS_MODE")
  fi
  if have_tty; then
    "${INSTALL_DIR}/${BINARY}" install ${dns_args[@]+"${dns_args[@]}"} </dev/tty
  else
    "${INSTALL_DIR}/${BINARY}" install ${dns_args[@]+"${dns_args[@]}"}
  fi

  # Only a fresh install, so an update never re-adds a plugin the user removed.
  if [ -z "$was_installed" ] && [ "$(uname -s)" = "Linux" ]; then
    setup_omarchy
  fi

  star_note
}

# omarchy_path_default points OMARCHY_PATH at the packaged install when the
# shell running us never set it. Only Omarchy's interactive bashrc exports it,
# and its plugin commands refuse to run without it.
omarchy_path_default() {
  [ -n "${OMARCHY_PATH:-}" ] && return 0
  [ -d "$OMARCHY_SYSTEM_PATH" ] && export OMARCHY_PATH="$OMARCHY_SYSTEM_PATH"
  return 0
}

# setup_omarchy swaps the tray for the Glance bar plugin on Omarchy, whose bar
# already has a place for lerd's state. Off Omarchy it does nothing.
setup_omarchy() {
  command -v omarchy-plugin-add &>/dev/null || return 0
  omarchy_path_default
  header "Omarchy"
  "${INSTALL_DIR}/${BINARY}" tray off >/dev/null 2>&1 || true
  success "System tray off, the Omarchy bar shows Lerd instead"
  # omarchy-plugin-add refuses an id already on disk, and a plugin the user
  # disabled stays disabled.
  if [ -d "$OMARCHY_PLUGINS_DIR/${OMARCHY_PLUGIN_ID}" ]; then
    success "Lerd Glance plugin already installed"
  elif omarchy-plugin-add "$OMARCHY_PLUGIN_REPO" --enable --yes; then
    success "Lerd Glance plugin added to the bar"
  else
    warn "Could not add the Lerd Glance plugin. Add it later with:"
    echo -e "     ${CYAN}omarchy-plugin-add ${OMARCHY_PLUGIN_REPO} --enable --yes${RESET}"
  fi
}

# Overridable so the tests can point the WSL check at a fixture.
OSRELEASE_FILE="${OSRELEASE_FILE:-/proc/sys/kernel/osrelease}"

# is_wsl reports a WSL distro: its kernel release names Microsoft.
is_wsl() {
  grep -qi microsoft "$OSRELEASE_FILE" 2>/dev/null
}

# offer_desktop_app installs the Lerd desktop app when flatpak is there and the
# user wants it, which also selects native notifications, and only selects them
# when the app is already installed. Otherwise it asks
# whether notifications go native or stay in the browser.
offer_desktop_app() {
  header "Desktop app"
  if command -v flatpak >/dev/null 2>&1 && flatpak info "$DESKTOP_APP_ID" >/dev/null 2>&1; then
    "${INSTALL_DIR}/${BINARY}" notify target native >/dev/null 2>&1 || true
    success "Lerd desktop app found, native desktop notifications enabled"
    return 0
  fi
  if command -v flatpak >/dev/null 2>&1; then
    info "Lerd has a desktop app: a dedicated window with native desktop notifications,"
    info "no browser tab needed."
    echo ""
    if ask_yes "Install the Lerd desktop app?"; then
      # Split on purpose, the command is overridable as one string. stdin is the
      # piped script under curl|bash, so flatpak must never read it.
      # shellcheck disable=SC2086
      if $DESKTOP_INSTALL_CMD -y </dev/null; then
        "${INSTALL_DIR}/${BINARY}" notify target native >/dev/null 2>&1 || true
        success "Lerd desktop app installed, native desktop notifications enabled"
        return 0
      fi
      warn "Could not install the desktop app."
    fi
    info "Install it anytime with:"
    echo -e "     ${CYAN}${DESKTOP_INSTALL_CMD}${RESET}"
    echo ""
  fi
  if ask_yes "Use native desktop notifications?"; then
    "${INSTALL_DIR}/${BINARY}" notify target native >/dev/null 2>&1 || true
    success "Native desktop notifications enabled"
  else
    "${INSTALL_DIR}/${BINARY}" notify target browser >/dev/null 2>&1 || true
    success "Using browser notifications"
  fi
}

# offer_desktop_app_macos installs the Lerd desktop app, which starts lerd,
# opens the dashboard and raises lerd's notifications natively. Without it
# macOS has no native notifier, so there is no second question to ask.
offer_desktop_app_macos() {
  header "Desktop app"
  if desktop_app_macos_path >/dev/null; then
    "${INSTALL_DIR}/${BINARY}" notify target native >/dev/null 2>&1 || true
    success "Lerd desktop app found, native desktop notifications enabled"
    return 0
  fi
  info "Lerd has a desktop app: a dedicated window with native desktop notifications,"
  info "no browser tab needed."
  echo ""
  if ask_yes "Install the Lerd desktop app?"; then
    if install_desktop_app_macos; then
      "${INSTALL_DIR}/${BINARY}" notify target native >/dev/null 2>&1 || true
      success "Lerd desktop app installed, native desktop notifications enabled"
      return 0
    fi
    warn "Could not install the desktop app."
  fi
  info "Download it anytime from:"
  echo -e "     ${CYAN}https://github.com/lerd-env/lerd-desktop/releases/latest${RESET}"
  echo ""
}

# bundle_id prints an app bundle's identifier, empty when there is no bundle.
bundle_id() {
  /usr/libexec/PlistBuddy -c "Print :CFBundleIdentifier" "$1/Contents/Info.plist" 2>/dev/null || true
}

# desktop_app_macos_path prints where the desktop app is installed, if it is.
desktop_app_macos_path() {
  local a
  for a in /Applications/Lerd.app "$HOME/Applications/Lerd.app"; do
    if [ "$(bundle_id "$a")" = "$DESKTOP_APP_ID" ]; then echo "$a"; return 0; fi
  done
  return 1
}

# desktop_app_slot is true when <dir>/Lerd.app can take the app: nothing is
# there, or lerd's own launcher is, which the app replaces.
desktop_app_slot() {
  local app="$1/Lerd.app"
  [ ! -e "$app" ] && return 0
  [ "$(bundle_id "$app")" = "sh.lerd.launcher" ]
}

# install_desktop_app_macos copies the app out of its DMG into /Applications,
# or ~/Applications when that is not writable, as lerd never asks for root.
install_desktop_app_macos() {
  local arch dest tmp
  case "$(detect_arch)" in arm64) arch="arm64" ;; *) arch="x64" ;; esac
  dest="/Applications"; [ -w "$dest" ] || dest="$HOME/Applications"
  desktop_app_slot "$dest" || { warn "${dest}/Lerd.app is another application"; return 1; }
  tmp="$(mktemp -d)"
  if ! fetch "${DESKTOP_DMG_BASE}/Lerd-${arch}.dmg" "${tmp}/Lerd.dmg" \
    || ! hdiutil attach -nobrowse -readonly -quiet -mountpoint "${tmp}/mnt" "${tmp}/Lerd.dmg"; then
    rm -rf "$tmp"; return 1
  fi
  # Gatekeeper never sees a file curl downloaded, so the signature is checked here.
  local ok=0
  if codesign --verify --deep --strict "${tmp}/mnt/Lerd.app" 2>/dev/null \
    && codesign -dv "${tmp}/mnt/Lerd.app" 2>&1 | grep -q "TeamIdentifier=${DESKTOP_TEAM_ID}"; then
    mkdir -p "$dest" && rm -rf "${dest}/Lerd.app" && ditto "${tmp}/mnt/Lerd.app" "${dest}/Lerd.app" && ok=1
  else
    warn "The downloaded app is not signed by Lerd, not installing it."
  fi
  hdiutil detach -quiet "${tmp}/mnt" || true
  rm -rf "$tmp"
  [ "$ok" = 1 ] || return 1
  # Registered now so lerd:// opens it, and `lerd install` sees it, before it first runs.
  /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "${dest}/Lerd.app" 2>/dev/null || true
}

# ── Update ───────────────────────────────────────────────────────────────────
cmd_update() {
  header "Updating Lerd"

  local arch; arch="$(detect_arch)"
  local latest; latest="$(resolve_version)"
  [ -n "$latest" ] || die "Could not fetch latest version."

  local current; current="$(installed_version)"

  if [ "$current" = "$latest" ]; then
    success "Already on latest: v${latest}"
    exit 0
  fi
  guard_dev_build "$latest"
  guard_prerelease "$latest"

  info "Updating v${current:-unknown} → v${latest}"
  local tmpdir; tmpdir="$(mktemp -d)"
  download_binary "$latest" "$arch" "$tmpdir"
  install -m 755 "${tmpdir}/lerd" "${INSTALL_DIR}/${BINARY}"
  [ -f "${tmpdir}/lerd-tray" ] && install -m 755 "${tmpdir}/lerd-tray" "${INSTALL_DIR}/lerd-tray"
  if is_wsl && [ -f "${tmpdir}/lerd.exe" ]; then install -m 755 "${tmpdir}/lerd.exe" "${INSTALL_DIR}/lerd.exe"; fi
  rm -rf "$tmpdir"
  success "Updated to lerd v${latest}"
  star_note
}

# ── Uninstall ────────────────────────────────────────────────────────────────

# lerd uninstall owns the whole teardown (DNS, units, the Omarchy plugin, PATH,
# data), so the installer hands over to it rather than keeping a second copy.
cmd_uninstall() {
  local bin="${INSTALL_DIR}/${BINARY}"
  [ -x "$bin" ] || bin="$(command -v lerd || true)"
  [ -n "$bin" ] || die "Lerd is not installed, nothing to uninstall."
  if have_tty; then
    exec "$bin" uninstall </dev/tty
  fi
  exec "$bin" uninstall
}

# ── Entry point ──────────────────────────────────────────────────────────────
main() {
  echo -e "${BOLD}"
  echo "  ██╗     ███████╗██████╗ ██████╗ "
  echo "  ██║     ██╔════╝██╔══██╗██╔══██╗"
  echo "  ██║     █████╗  ██████╔╝██║  ██║"
  echo "  ██║     ██╔══╝  ██╔══██╗██║  ██║"
  echo "  ███████╗███████╗██║  ██║██████╔╝"
  echo "  ╚══════╝╚══════╝╚═╝  ╚═╝╚═════╝ "
  echo -e "${RESET}"
  echo "  Lerd — Podman-powered local PHP dev environment for Linux and macOS"
  echo "  https://lerd.sh"
  echo ""

  # --beta is a modifier rather than a command, so it is taken out of the
  # argument list first and whatever is left goes through the same dispatch.
  local args=()
  local arg
  for arg in "$@"; do
    case "$arg" in
      --beta) BETA=1 ;;
      *) args+=("$arg") ;;
    esac
  done
  set -- "${args[@]:-}"

  case "${1:-install}" in
    --update|-u|update)     cmd_update ;;
    --uninstall|uninstall)  cmd_uninstall ;;
    --check|check)
      # Non-interactive: report the full requirement set (managed DNS), so the
      # check never blocks on a prompt. Picking .localhost at real install time
      # is what skips the HTTPS-only packages.
      MISSING_PKGS=()
      check_prerequisites
      ;;
    --local)
      [ -n "${2:-}" ] || die "--local requires a path argument, e.g: --local ./build/lerd"
      cmd_install "$2"
      ;;
    --help|-h)
      echo "Usage: $0 [--update | --uninstall | --check | --local <path>] [--beta]"
      echo ""
      echo "  (no args)       Install Lerd from latest GitHub release"
      echo "  --local <path>  Install from a locally built binary"
      echo "  --update        Update to the latest release"
      echo "  --uninstall     Run lerd uninstall"
      echo "  --check         Check prerequisites only"
      echo "  --beta          Install or update from the newest prerelease"
      ;;
    --install|install|"") cmd_install ;;
    *) die "Unknown option: $1. Run with --help for usage." ;;
  esac
}

# Only run main when executed directly or piped to bash, not when sourced.
# BASH_SOURCE may be an unset array when piped to bash (curl|bash / wget|bash),
# which triggers set -u on some bash versions even with the :- operator.
# Suspend nounset briefly to read it safely.
set +u
_lerd_src="${BASH_SOURCE[0]:-}"
set -u
if [[ -z "$_lerd_src" || "$_lerd_src" == "$0" ]]; then
  main "$@"
fi
unset _lerd_src
