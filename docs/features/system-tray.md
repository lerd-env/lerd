# System Tray

`lerd tray` launches a system tray applet that gives you at-a-glance status and one-click control without opening a browser.

```bash
lerd tray              # launch (detaches from terminal automatically)
lerd tray --mono=false # use the red colour icon instead of monochrome white
```

The tray detaches from the terminal immediately, your shell prompt returns straight away.

---

## Menu layout

```
🟢 Running          ← overall status (disabled, informational)
  🟢 nginx
  🟢 dns
  🟢 4 workers       ← per-site workers, hidden when there is nothing to report
─────────────────
Open Dashboard       ← opens http://lerd.localhost
Stop Lerd            ← toggles between Start / Stop Lerd
─────────────────
Services (2/3)  ▸    ← submenu, the count is running out of installed
PHP 8.5         ▸    ← submenu, the label is the current default
─────────────────
Settings        ▸    ← submenu holding the global toggles
Check for update...  ← becomes "⬆ Update to vX.Y.Z" once one is cached
Quit Lerd            ← stops the environment and exits the tray
```

The submenus hold the entries you click:

```
Services (2/3) ▸   🟢 mysql        ← click to stop
                   🔴 redis        ← click to start
                   🟡 meilisearch  ← paused, click to start

PHP 8.5        ▸   ✔ 8.5           ← current default
                   8.4             ← click to switch

Settings       ▸   Autostart at login: ✔ On   ← enables/disables every lerd unit
                   Expose to LAN: Off         ← Linux only
                   Debug bridge: Off          ← `lerd dump on/off`
                   Notifications: ✔ On        ← `lerd notify on/off`
                   High-contrast icon: Off    ← `lerd tray icon default/high-contrast`
```

The menu refreshes every 30 seconds, and again right after any click so it redraws against the result instead of waiting for the next poll. Clicking a service toggles it on/off. Clicking a PHP version sets it as the global default. "Quit Lerd" stops the entire environment before closing.

The **Debug bridge** item shells out to `lerd dump on` / `lerd dump off`, see [Dumps](dumps.md). The **Notifications** item shells out to `lerd notify on` / `lerd notify off`, see [Notifications](notifications.md). Both are global toggles, persisted to `config.yaml`.

The **Services** submenu shows only core services (MySQL, Redis, PostgreSQL, etc.), and its parent row hides itself when none are installed. If you ever install more than the menu has room for, the last row reports how many are not shown rather than dropping them quietly.

The **workers line** summarises the per-site workers (queue, schedule, Horizon, Reverb, Stripe, and any framework-declared worker) without listing them, since their number grows with your sites. It shows the running count in green, or a red warning naming the site when one has actually broken. A worker stopped because its site is paused or idle-suspended is not a fault, so it never raises the warning, and the line hides itself entirely when nothing is running and nothing is wrong. Workers are still started and stopped from the web UI or their CLI commands.

The **update item** shows "Check for update..." when no update information is cached, and "⬆ Update to vX.Y.Z" once the background checker finds a newer release. Clicking it opens a terminal to run `lerd update`.

---

## Icon appearance

In the default colour mode the icon doubles as a status light: a red **L** when lerd is stopped and a white **L** when it is running. The white running icon would disappear on a light panel, so the tray reads your desktop's light/dark preference and switches the running icon to a dark **L** whenever the panel is light. It reacts live: toggle your system theme and the icon recolours without restarting the tray.

The preference comes from the cross-desktop XDG desktop portal (`org.freedesktop.appearance` `color-scheme`), so it works the same on GNOME, KDE Plasma, and anything else that implements the portal. On a setup without a portal the tray keeps the white running icon, which is the right default for the common dark panel.

The red stopped icon is left as-is since it reads on both light and dark panels.

### High-contrast icon

The light/dark switch relies on the portal reporting a preference that matches the panel. On mixed themes that pairing breaks: KDE's Breeze Twilight uses light application colours with a dark Plasma panel, so the portal reports "light", the tray hands over the dark running icon, and it vanishes against the dark panel. There is no portable way to read the panel's real colour, so no amount of detection fixes this case.

For that, turn on the high-contrast icon. It replaces the theme-adaptive running icon with a single green **L** that reads on any panel, the same way the red stopped icon already does, and stops depending on the panel colour entirely.

```bash
lerd tray icon high-contrast   # always-visible green running icon
lerd tray icon default         # back to the theme-adaptive white/dark icon
lerd tray icon                 # print the current style
```

You can also toggle it from the **High-contrast icon** item in the tray menu. It is off by default and persisted to `config.yaml`.

If you would rather the OS own the colouring entirely, use `lerd tray --mono`, which registers a monochrome template icon that GNOME and KDE recolour to match the panel themselves (this mode does not change with lerd state).

---

## Autostart

The tray follows the global `lerd autostart` toggle: when autostart is on (the default), `lerd install` writes and enables `lerd-tray.service` so the tray comes up on every graphical login. Run `lerd autostart disable` to turn off autostart for the entire environment, including the tray.

The tray is also started automatically by `lerd start` if it isn't already running.

The unit is wired to `graphical-session.target`, which is reached automatically by GNOME, KDE Plasma, and any Wayland compositor launched through `uwsm` (including Omarchy's Hyprland setup). On bare Hyprland / Sway / i3 launched without `uwsm`, `graphical-session.target` is never started, so the tray will not autostart. Either run the compositor under `uwsm` or replace `WantedBy=graphical-session.target` with `WantedBy=default.target` in `~/.config/systemd/user/lerd-tray.service`.

---

## Desktop environment compatibility

The tray uses the **StatusNotifierItem (SNI) / AppIndicator** protocol (DBus-based).

| Environment | Status |
|---|---|
| KDE Plasma | Works out of the box |
| GNOME | Requires the [AppIndicator and KStatusNotifierItem Support](https://extensions.gnome.org/extension/615/appindicator-support/) extension |
| Sway / Hyprland with waybar | Works with `"tray"` module in waybar config |
| i3 with i3bar | Requires [snixembed](https://git.sr.ht/~yerlan/snixembed) to bridge SNI to XEmbed |
| XFCE / LXQt | Works out of the box |

---

## Build requirements

The tray uses CGO and requires `libayatana-appindicator` at build time:

::: code-group

```bash [Arch / CachyOS / omarchy]
sudo pacman -S libayatana-appindicator
```

```bash [Debian / Ubuntu]
sudo apt install libayatana-appindicator3-dev
```

```bash [Fedora]
sudo dnf install libayatana-appindicator-gtk3
```

:::

For headless / CI builds without the tray:

```bash
make build-nogui   # produces ./build/lerd-nogui (lerd tray returns an error)
```
