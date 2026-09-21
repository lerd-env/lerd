# Dashboard themes

The dashboard ships twelve themes and takes as many of your own as you care to
write. A theme is independent of light/dark: you pick the mode in the icon rail,
and the theme in **System → lerd → Theme**.

The theme is stored in `~/.config/lerd/config.yaml` under `ui.theme`, so every
device that opens the dashboard shows the same lerd, a phone on the LAN included.
Switching it on one device repaints the others straight away, over the socket
they already hold open, with no reload. The light/dark mode stays per browser,
since that follows the room you are sitting in rather than the install.

## Built-in themes

| Theme | Accent | Background (dark mode) |
|---|---|---|
| `lerd` | the bright brand red | `#0d0d0d` |
| `muted` | a desaturated brick | `#111113` |
| Ocean | a calm steel blue | `#0d1418` |
| Solarized Dark | Solarized blue | `#002b36` |
| Monokai | Monokai pink | `#272822` |
| Cobalt | Cobalt orange | `#193549` |
| Dracula | Dracula purple | `#282a36` |
| Nord | Nord frost | `#2e3440` |
| Gruvbox Dark | Gruvbox orange | `#282828` |
| Breeze | Plasma blue | `#141618` |
| Adwaita | Adwaita blue | `#1d1d20` |
| macOS | the system blue | `#1e1e1e` |

`lerd` is the default. `muted` is there for anyone who finds the default red too
sharp, especially on a bright screen.

The desktop themes are taken from what those desktops ship today, not from the
values most write-ups still quote: Breeze from Plasma 6.7's `BreezeDark.colors`,
Adwaita from libadwaita 1.9's named colours, macOS from Apple's documented system
blue and window background. All three have been darkened over the years.

Each theme carries a light tone and a dark tone for its accent, so switching
between light and dark keeps the colour readable on whichever surface the mode
paints. The editor and desktop schemes are dark schemes: their backgrounds apply
in dark mode, and in light mode you get their accent on the usual white.

## Writing your own

Drop a YAML file into `~/.config/lerd/themes/`. The file name is the theme's id,
so `lagoon.yaml` becomes the theme `lagoon`. Only `name` and `accent` are
required; every other tone is derived from the accent, and the surfaces fall
back to the built-in ones.

```yaml
name: Lagoon
accent: "#3b7ea1"
```

The full set of fields:

```yaml
name: Lagoon                # what the picker shows
accent: "#3b7ea1"           # buttons, active tabs, links, focus rings (light mode)
accent_hover: "#336b8a"     # the accent's hover tone (light mode)
accent_dark: "#7fb6d4"      # the accent in dark mode
accent_hover_dark: "#93c2dc"# its hover tone in dark mode
bg: "#0c1114"               # page background, dark mode
card: "#141b1f"             # card background, dark mode
border: "#222c32"           # card and divider borders, dark mode
muted: "#3c4a52"            # dim text and inactive marks, dark mode
```

Every value must be a plain hex colour, `#rgb` or `#rrggbb`. CSS colour names
(`rebeccapurple`) and functional notations (`rgb()`, `oklch()`) are refused: the
value is handed to the browser as a custom property, and only a literal colour
may come out of a file and decide how the page paints. The four surface fields
apply to dark mode only, where light mode draws on white and the standard greys.

A file named after a built-in replaces it rather than appearing twice, so you can
keep the name and change the colours.

Reload the dashboard and the theme appears in the picker. A file with a mistake
in it is listed under the picker with the reason, rather than quietly missing.

## Importing

**System → lerd → Import theme** takes a file or pasted YAML and writes it into
`~/.config/lerd/themes/` for you, then selects it. The trash icon beside a theme
asks before it deletes the file.

## Following the desktop theme

On Omarchy, KDE Plasma and GNOME, the desktop's own colours show up in the picker
as one more entry: `Omarchy (tokyo-night)` names the theme the desktop is on, and
on Plasma and GNOME the entry is the familiar `Breeze` or `Adwaita`, wearing
whatever the desktop is actually set to rather than a fixed copy of it. Pick it once
and lerd follows the desktop from then on: change the theme or the accent and
every open dashboard repaints, including the embedded service views. A theme you
chose deliberately is never overridden, and on a machine with no desktop to read
the entry is simply absent. There is one entry at most, and no trash icon beside
it: the desktop is the source, so the entry goes away when the desktop does.

**Omarchy** publishes everything in the active theme's `colors.toml`, which every
theme it ships carries. A dark desktop theme lends its accent and its surfaces; a
light one lends only its accent, because the surface fields are the dark ones and
a pale background would land behind type coloured to sit on a dark card. The
watch sits on `~/.local/state/omarchy/current`, which is where
`omarchy-theme-set` moves the new theme into place.

**Plasma** keeps the accent and a copy of the active colour scheme in
`~/.config/kdeglobals`, so the entry carries surfaces too: the view background,
the window background and its alternate become the page, the cards and their
borders. Whether the scheme counts as dark is read off the window background
rather than the scheme's name. A stock Plasma that never had an accent picked
lends the scheme's selection colour instead.

**GNOME** publishes the accent alone, one of the nine libadwaita colours, read
from `org.gnome.desktop.interface accent-color`. That is all it lends, since the
desktop's surfaces are not something you picked; the Adwaita entry keeps the
surfaces it always had and only its accent follows the desktop. GNOME 46 and
older have no accent setting, so there is nothing to follow there.

The entry has to be the desktop in front of you, not a file left behind by an
application. `kdeglobals` exists on any machine that has ever run a Qt app, and
the GNOME schemas ship with half the desktop packages out there, so a desktop
counts when `XDG_CURRENT_DESKTOP` says so, or when the accent is one you actually
recorded in that desktop's own settings. Picking up a live change needs the watch
that was set when `lerd-ui` started, so if you install a desktop or set an accent
for the first time, `lerd restart` once.

On Plasma and GNOME the desktop entry takes the place of the built-in that
imitates it, Breeze and Adwaita, rather than sitting beside it: the built-in is a
snapshot of one scheme, and the machine in front of you has the real one, on
whichever scheme it is currently wearing. It keeps that built-in's name, so the
picker still offers Breeze and Adwaita and picking one now follows the desktop. A
dashboard already set to either starts following it too, and any tone the desktop
does not publish comes from the built-in it replaced, which is where the GNOME
entry gets its surfaces.

A theme file named `omarchy.yaml`, `breeze.yaml` or `adwaita.yaml` is shadowed by
the desktop entry rather than replacing it: picking the entry named after a
desktop has to give you that desktop's colours.

## Installed as an app

Installed from the browser, the window and the launch splash are painted by the
browser rather than by the page. The title bar tint follows the theme as soon as
you switch, and the manifest carries the current tones so the app you install
matches what you were looking at. The splash is read once at install, so a theme
switch afterwards reaches it only when the browser next refreshes the manifest.

## Asking an assistant for one

There is no MCP tool for this, deliberately: a theme is a personal choice, and
nothing should be able to repaint your dashboard without you. The file is the
interface instead. An assistant with access to your files can write a theme into
`~/.config/lerd/themes/` the same way it writes any other file, when you ask it
to and not otherwise. Point it at the schema above and reload the dashboard.
