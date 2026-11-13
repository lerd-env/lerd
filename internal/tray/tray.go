//go:build !nogui

package tray

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/geodro/lerd/internal/config"
	lerdSystemd "github.com/geodro/lerd/internal/systemd"
	"github.com/geodro/lerd/internal/traymenu"
	lerdUpdate "github.com/geodro/lerd/internal/update"
	"github.com/geodro/lerd/internal/version"
	"github.com/getlantern/systray"
)

// apiBase is the loopback URL used for polling the lerd HTTP API. It
// deliberately stays on 127.0.0.1 because the tray should work even
// before nginx / DNS are up.
var apiBase = "http://127.0.0.1:7073"

// The snapshot and its reader are shared with the menu lerd-ui serves on WSL.
type (
	Snapshot    = traymenu.Snapshot
	phpInfo     = traymenu.PHPInfo
	serviceInfo = traymenu.ServiceInfo
)

const daemonEnv = "LERD_TRAY_DAEMON"

// lerdBin resolves the absolute path to the `lerd` binary. The tray often
// runs under launchd, whose environment has no PATH covering Homebrew or
// ~/.local/bin, so a bare `exec.Command("lerd", …)` silently fails.
// Resolved on every call so reinstalls don't strand on a cached path.
func lerdBin() string {
	if p, err := lerdBinLookPath("lerd"); err == nil {
		return p
	}
	for _, c := range lerdBinCandidates() {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	lerdBinFallbackWarn.Do(func() {
		fmt.Fprintln(lerdBinWarnW, "lerd-tray: could not locate the lerd binary on PATH or in known install dirs; menu actions will fail with 'executable file not found'")
	})
	return "lerd"
}

func defaultLerdBinCandidates() []string {
	cands := []string{"/opt/homebrew/bin/lerd", "/usr/local/bin/lerd"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cands = append(cands, filepath.Join(home, ".local", "bin", "lerd"))
	}
	return cands
}

// Seams for tests so the fallback path can be exercised without depending
// on what happens to be installed on the host running the tests.
var (
	lerdBinCandidates             = defaultLerdBinCandidates
	lerdBinLookPath               = exec.LookPath
	lerdBinWarnW        io.Writer = os.Stderr
	lerdBinFallbackWarn sync.Once
)

// lerdCmd is a thin wrapper around exec.Command that uses the resolved
// `lerd` binary path so handlers work under launchd's empty PATH.
func lerdCmd(args ...string) *exec.Cmd {
	return exec.Command(lerdBin(), args...)
}

// Run starts the system tray applet.
// Unless already running as a daemon (or under launchd), it re-execs itself
// detached from the terminal and returns immediately so the shell prompt is
// restored. When launched by launchd, XPC_SERVICE_NAME is set and we run
// foreground directly — launchd owns the process lifecycle.
func Run(mono bool) error {
	if os.Getenv(daemonEnv) == "" && os.Getenv("XPC_SERVICE_NAME") == "" {
		return detach(mono)
	}
	if !acquireLock() {
		// Another instance is already running — exit silently.
		return nil
	}
	systray.Run(func() { onReady(mono) }, nil)
	return nil
}

// lockFile holds the instance lock for the life of the process. It has to be
// package-level: an unreferenced *os.File is closed by its GC finalizer, which
// silently drops the flock and lets the next start run a second tray.
var lockFile *os.File

// acquireLock tries to acquire an exclusive flock on a per-user lock file.
// It returns true if the lock was acquired (safe to start), false if another
// instance already holds it.
func acquireLock() bool {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, "lerd-tray.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		// Can't create the lock file — allow startup rather than blocking.
		return true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false
	}
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	lockFile = f
	return true
}

// detach re-execs the current binary with the same tray arguments in a new
// session, detached from the controlling terminal.
func detach(mono bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// lerd-tray is a standalone binary — no "tray" subcommand needed.
	var args []string
	if filepath.Base(exe) != "lerd-tray" {
		args = append(args, "tray")
	}
	if mono {
		args = append(args, "--mono")
	} else {
		args = append(args, "--mono=false")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), daemonEnv+"=1")
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func onReady(mono bool) {
	// In mono mode we register a template icon so the OS panel recolors
	// it to match the theme. Both modes start on the stopped icon and the
	// applyLoop swaps it on every state change.
	if mono {
		systray.SetTemplateIcon(iconMonoOffPNG, iconMonoOffPNG)
	} else {
		systray.SetIcon(iconOffWhitePNG)
	}
	// SetTitle would show text next to the icon in the macOS menu bar — skip it.
	systray.SetTooltip("Lerd — local dev environment")

	menu := buildMenu(mono)

	ctx, cancel := context.WithCancel(context.Background())

	updateCh := make(chan *Snapshot, 1)
	// refresh schedules an immediate fetch so menu labels redraw right
	// after a click instead of waiting for the next 30s poll tick.
	refresh := func() {
		go func() {
			snap := fetchSnapshot()
			select {
			case updateCh <- snap:
			default:
			}
		}()
	}

	// In color mode the running icon is white, which vanishes on a light
	// panel. Watch the desktop light/dark preference and swap to a dark icon
	// when the panel is light, reacting live when the user toggles their theme.
	var icons *iconState
	if !mono {
		icons = newIconState()
		go watchAppearance(ctx, icons.setLight)
	}

	go runPoller(ctx, updateCh)
	go applyLoop(menu, updateCh, mono, icons)
	go handleDash(menu.mDash, refresh)
	go handleToggle(menu, icons, refresh)
	menu.svcs.watch(refresh)
	menu.php.watch(refresh)
	go handleAutostart(menu.mAutostart, refresh)
	if menu.mLAN != nil {
		go handleLAN(menu.mLAN, refresh)
	}
	go handleLANServices(menu.mLANServices, refresh)
	go handleDumps(menu.mDumps, refresh)
	go handleNotifications(menu.mNotifications, refresh)
	go handleTrayOff(menu.mTrayOff)
	if menu.mIconStyle != nil {
		go handleIconStyle(menu.mIconStyle, refresh)
	}
	go handleUpdate(menu.mUpdate)
	go handleQuit(menu.mQuit, cancel)
}

func runPoller(ctx context.Context, updateCh chan<- *Snapshot) {
	// Poll immediately, then every 5 s.
	send := func() {
		snap := fetchSnapshot()
		select {
		case updateCh <- snap:
		default:
			// drop if channel full (previous update not consumed yet)
		}
	}
	send()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

func fetchSnapshot() *Snapshot {
	// The services snapshot is rebuilt from a subprocess per installed service,
	// and the tray is the only poller for minutes at a time, so its request is
	// routinely the one that pays for a cold cache. Four seconds lost that race
	// on an ordinary install; the poll runs every 30s, so waiting is free.
	client := &http.Client{Timeout: 20 * time.Second}

	get := func(path string) (io.ReadCloser, error) {
		r, err := client.Get(apiBase + path)
		if err != nil {
			return nil, err
		}
		return r.Body, nil
	}
	return traymenu.ReadSnapshot(get, traymenu.LocalSettings)
}

// iconKind identifies which color-mode icon is currently shown so we don't
// thrash the systray with identical SetIcon calls.
type iconKind int

const (
	iconKindStoppedDark  iconKind = iota // light slashed containers, dark panel
	iconKindStoppedLight                 // dark slashed containers, light panel
	iconKindStoppedHiC                   // red slashed containers, any panel (high-contrast opt-in)
	iconKindRunningDark                  // light containers, dark panel
	iconKindRunningLight                 // dark containers, light panel
	iconKindRunningHiC                   // green containers, any panel (high-contrast opt-in)
)

// pickColorIcon matches the panel, light on dark and dark on light, slashed while
// stopped. High contrast is green running and red stopped, readable on any panel,
// for mixed themes like KDE Breeze Twilight where the panel-colour guess fails.
func pickColorIcon(running, light, highContrast bool) (iconKind, []byte) {
	switch {
	case !running && highContrast:
		return iconKindStoppedHiC, iconOffRedPNG
	case !running && light:
		return iconKindStoppedLight, iconOffDarkPNG
	case !running:
		return iconKindStoppedDark, iconOffWhitePNG
	case highContrast:
		return iconKindRunningHiC, iconGreenPNG
	case light:
		return iconKindRunningLight, iconDarkPNG
	default:
		return iconKindRunningDark, iconWhitePNG
	}
}

// iconState owns the color-mode tray icon and recomputes it whenever the lerd
// running state or the desktop light/dark preference changes. It serializes the
// two input goroutines (poller and appearance watcher) and skips redundant
// SetIcon calls.
type iconState struct {
	mu           sync.Mutex
	running      bool
	light        bool
	highContrast bool
	// pulsing blinks the icon while a start or stop runs; lit is which half of
	// the blink is showing.
	pulsing bool
	lit     bool
	last    iconKind
	// apply performs the actual icon swap; overridable in tests so the logic
	// can be exercised without a live systray.
	apply func([]byte)
}

func newIconState() *iconState {
	// last defaults to iconKindStoppedDark, matching the icon onReady sets.
	return &iconState{last: iconKindStoppedDark, apply: systray.SetIcon}
}

func (s *iconState) setRunning(running bool) {
	s.mu.Lock()
	s.running = running
	s.mu.Unlock()
	s.refresh()
}

func (s *iconState) setLight(light bool) {
	s.mu.Lock()
	s.light = light
	s.mu.Unlock()
	s.refresh()
}

func (s *iconState) setHighContrast(highContrast bool) {
	s.mu.Lock()
	s.highContrast = highContrast
	s.mu.Unlock()
	s.refresh()
}

func (s *iconState) setPulsing(pulsing bool) {
	s.mu.Lock()
	s.pulsing, s.lit = pulsing, false
	s.mu.Unlock()
	s.refresh()
}

func (s *iconState) pulseStep() {
	s.mu.Lock()
	s.lit = !s.lit
	s.mu.Unlock()
	s.refresh()
}

// pulse blinks the icon until the returned stop is called.
func (s *iconState) pulse() (stop func()) {
	s.setPulsing(true)
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				s.pulseStep()
			}
		}
	}()
	return func() {
		close(done)
		s.setPulsing(false)
	}
}

func (s *iconState) refresh() {
	// Hold the lock across apply so the decision and the swap are atomic:
	// otherwise two concurrent updates (poller + theme watcher) can run their
	// SetIcon calls out of order and leave the displayed icon disagreeing with
	// s.last. SetIcon never re-enters iconState, so there's no deadlock.
	s.mu.Lock()
	defer s.mu.Unlock()
	running := s.running
	if s.pulsing {
		running = s.lit
	}
	kind, icon := pickColorIcon(running, s.light, s.highContrast)
	if kind == s.last {
		return
	}
	s.last = kind
	s.apply(icon)
}

// pickMonoIcon chooses the template icon; the OS recolors it, so the slash is
// the only status cue mono mode has.
func pickMonoIcon(running bool) []byte {
	if running {
		return iconMonoPNG
	}
	return iconMonoOffPNG
}

func applyLoop(menu *menuState, updateCh <-chan *Snapshot, mono bool, icons *iconState) {
	monoRunning := false // matches the stopped template icon onReady sets
	for snap := range updateCh {
		menu.apply(snap)
		running := snap != nil && snap.Running
		if mono {
			if running != monoRunning {
				monoRunning = running
				icon := pickMonoIcon(running)
				systray.SetTemplateIcon(icon, icon)
			}
			continue
		}
		if icons != nil {
			applyIcon(icons, snap)
		}
	}
}

// applyIcon sets the colour icon from a poll. High contrast is read from the
// saved config, not the snapshot, which comes back empty while lerd-ui is down.
func applyIcon(icons *iconState, snap *Snapshot) {
	highContrast := false
	if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
		highContrast = cfg.IsHighContrastTrayIcon()
	}
	icons.setHighContrast(highContrast)
	icons.setRunning(snap != nil && snap.Running)
}

func handleDash(item *systray.MenuItem, refresh func()) {
	for range item.ClickedCh {
		// `lerd dashboard` owns the whole choice: the desktop app, a Chromium
		// app window or a browser tab, starting the stack first when it is down.
		go runAndRefresh(lerdCmd("dashboard"), refresh)
	}
}

// handleToggle starts or stops lerd. Either can take a while, so the item says
// what is happening and, in colour mode, the icon blinks until it is done.
func handleToggle(menu *menuState, icons *iconState, refresh func()) {
	for range menu.mToggle.ClickedCh {
		arg, ok := menu.claim()
		if !ok {
			continue
		}
		go func() {
			stop := func() {}
			if icons != nil {
				stop = icons.pulse()
			}
			err := runLifecycle(arg)
			// On success show the outcome until the refresh confirms it, so the
			// item does not flash back to the old state in between.
			if err == nil && icons != nil {
				icons.setRunning(arg == "start")
			}
			stop()
			menu.finish(err == nil)
			refresh()
		}()
	}
}

// runLifecycle starts or stops lerd through lerd-ui, or the CLI when lerd-ui
// is not answering.
func runLifecycle(arg string) error {
	if reached, err := lifecycleViaUI(arg); reached {
		return err
	}
	return lerdCmd(arg).Run()
}

func handleAutostart(item *systray.MenuItem, refresh func()) {
	for range item.ClickedCh {
		arg := "enable"
		if lerdSystemd.IsAutostartEnabled() {
			arg = "disable"
		}
		runAndRefresh(lerdCmd("autostart", arg), refresh)
	}
}

// handleLAN toggles `lerd lan expose` / `lerd lan unexpose` on click. We
// read the current state from config rather than relying on the snapshot
// so the click is robust to a stale in-memory copy.
func handleLAN(item *systray.MenuItem, refresh func()) {
	for range item.ClickedCh {
		exposed := false
		if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
			exposed = cfg.LAN.Exposed
		}
		arg := "expose"
		if exposed {
			arg = "unexpose"
		}
		runAndRefresh(lerdCmd("lan", arg), refresh)
	}
}

func handleLANServices(item *systray.MenuItem, refresh func()) {
	for range item.ClickedCh {
		enabled := false
		if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
			enabled = cfg.LAN.ServicesExposed
		}
		runAndRefresh(lerdCmd("lan", "services", offOn(enabled)), refresh)
	}
}

func handleDumps(item *systray.MenuItem, refresh func()) {
	for range item.ClickedCh {
		enabled := false
		if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
			enabled = cfg.IsDumpsEnabled()
		}
		runAndRefresh(lerdCmd("dump", offOn(enabled)), refresh)
	}
}

func handleNotifications(item *systray.MenuItem, refresh func()) {
	for range item.ClickedCh {
		enabled := true
		if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
			enabled = cfg.IsNotificationsEnabled()
		}
		runAndRefresh(lerdCmd("notify", offOn(enabled)), refresh)
	}
}

// handleTrayOff turns the tray off for good. The command kills this very
// process, so there is nothing to refresh afterwards.
func handleTrayOff(item *systray.MenuItem) {
	for range item.ClickedCh {
		_ = lerdCmd("tray", "off").Run()
	}
}

func handleIconStyle(item *systray.MenuItem, refresh func()) {
	for range item.ClickedCh {
		highContrast := false
		if cfg, err := config.LoadGlobal(); err == nil && cfg != nil {
			highContrast = cfg.IsHighContrastTrayIcon()
		}
		style := "high-contrast"
		if highContrast {
			style = "default"
		}
		runAndRefresh(lerdCmd("tray", "icon", style), refresh)
	}
}

// runAndRefresh waits for cmd to finish then calls refresh, so the menu
// redraws against the post-command state instead of the next poll tick.
// Errors are swallowed; the user sees the resulting state, not the trace.
func runAndRefresh(cmd *exec.Cmd, refresh func()) {
	_ = cmd.Run()
	refresh()
}

// offOn returns the next-state CLI subcommand for a current boolean: true →
// "off", false → "on". Keeps the two new toggle handlers DRY.
func offOn(currentlyEnabled bool) string {
	if currentlyEnabled {
		return "off"
	}
	return "on"
}

func handleUpdate(item *systray.MenuItem) {
	for range item.ClickedCh {
		info, _ := lerdUpdate.CachedUpdateCheck(version.Version)
		if info != nil {
			openUpdateTerminal(lerdUpdate.StripV(info.LatestVersion))
		} else {
			item.SetTitle("✔ Up to date")
			go func() {
				time.Sleep(3 * time.Second)
				item.SetTitle("Check for update...")
			}()
		}
	}
}

func openUpdateTerminal(latestVer string) {
	// lerd-ui first: lerd update restarts the tray, and a terminal started here
	// sits in the tray's unit, which systemd kills whole, update included.
	uiErr := uiUpdateTerminal()
	if uiErr == nil {
		return
	}
	script := fmt.Sprintf(
		`echo "Lerd update available: v%s"; `+
			`read -rp "Update now? [y/N] " ans; `+
			`[[ "$ans" =~ ^[Yy]$ ]] && lerd update; `+
			`echo; read -rp "Press Enter to close..."`,
		latestVer,
	)
	terminals := [][]string{
		{"konsole", "-e", "bash", "-c", script},
		{"ptyxis", "--", "bash", "-c", script},
		{"gnome-terminal", "--", "bash", "-c", script},
		{"xfce4-terminal", "-e", "bash -c '" + script + "'"},
		{"xterm", "-e", "bash", "-c", script},
	}
	for _, t := range terminals {
		if _, err := lookPathFn(t[0]); err == nil {
			_ = startCmdFn(t[0], t[1:]...)
			return
		}
	}
	fmt.Fprintf(lerdBinWarnW, "lerd-tray: could not open a terminal to run lerd update: %v\n", uiErr)
}

// lookPathFn, startCmdFn and uiUpdateTerminal are seams for tests.
var (
	lookPathFn       = exec.LookPath
	startCmdFn       = startOwnScope
	uiUpdateTerminal = requestUpdateTerminal
)

// startOwnScope starts a command outside the tray's unit, which lerd update
// restarts, taking everything inside it down.
func startOwnScope(name string, args ...string) error {
	bin, rest := lerdSystemd.OwnScope(name, args)
	return exec.Command(bin, rest...).Start()
}

// requestUpdateTerminal asks lerd-ui to open the user's terminal on lerd update.
// lifecycleViaUI has lerd-ui start or stop lerd; reached is false only when it
// is not answering: as the tray's child, a start died when it relaunched the tray.
func lifecycleViaUI(arg string) (reached bool, err error) {
	req, err := http.NewRequest(http.MethodPost, apiBase+"/api/lerd/"+arg, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("X-Lerd-CSRF", "1")
	// No timeout: a cold start streams progress for minutes.
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return true, fmt.Errorf("lerd-ui answered %s", res.Status)
	}
	return true, lifecycleFailure(res.Body)
}

// lifecycleFailure reads a start's NDJSON progress or a stop's JSON answer and
// returns the failure either one reports.
func lifecycleFailure(body io.Reader) error {
	dec := json.NewDecoder(body)
	for {
		var line struct {
			Phase string `json:"phase"`
			OK    *bool  `json:"ok"`
			Error string `json:"error"`
		}
		if err := dec.Decode(&line); err != nil {
			return nil
		}
		if line.Phase == "failed" || (line.OK != nil && !*line.OK) {
			return errors.New(line.Error)
		}
	}
}

func requestUpdateTerminal() error {
	req, err := http.NewRequest(http.MethodPost, apiBase+"/api/lerd/update-terminal", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Lerd-CSRF", "1")
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return fmt.Errorf("lerd-ui answered %s", res.Status)
	}
	if !body.OK {
		return errors.New(body.Error)
	}
	return nil
}

func handleQuit(item *systray.MenuItem, cancel context.CancelFunc) {
	<-item.ClickedCh
	cancel()
	_ = lerdCmd("quit").Run()
	systray.Quit()
}
