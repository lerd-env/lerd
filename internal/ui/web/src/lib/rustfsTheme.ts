import { embeddedSurfaces } from './embeddedTheme';

// The RustFS console keeps its own light and dark, chosen from a menu and
// remembered in its own storage, and paints from the custom properties its UI
// kit declares. Both halves are reachable now that the console is served on the
// dashboard's origin: lerd hands it the mode to boot in, moves the class for a
// switch made while it is open, and points its surfaces at the theme's.
const THEME_STYLE_ID = 'lerd-rustfs-theme';
const RUSTFS_THEME_KEY = 'active_theme';

// rememberRustfsTheme records the mode the console should boot in. Safe to call
// before the frame exists: same origin, so the storage is the dashboard's own.
export function rememberRustfsTheme(dark: boolean): void {
  try {
    localStorage.setItem(RUSTFS_THEME_KEY, dark ? 'dark' : 'light');
  } catch {
    // Storage disabled. The class below still themes a frame already open.
  }
}

// themeRustfsDocument switches the console's own mode and repaints its surfaces
// from the theme. The class is what its stylesheet keys the dark palette on, so
// it moves first; the variables then take the dashboard's colours in whichever
// palette is showing.
export function themeRustfsDocument(doc: Document, dark: boolean): void {
  const root = doc.documentElement;
  if (root) {
    root.classList.toggle('dark', dark);
    root.classList.toggle('light', !dark);
    root.style.colorScheme = dark ? 'dark' : 'light';
  }
  let style = doc.getElementById(THEME_STYLE_ID) as HTMLStyleElement | null;
  if (!style) {
    style = doc.createElement('style');
    style.id = THEME_STYLE_ID;
    doc.head?.appendChild(style);
  }
  const s = embeddedSurfaces(dark);
  style.textContent = `
:root, .light, .dark {
  --background: ${s.bg};
  --card: ${s.card};
  --popover: ${s.card};
  --sidebar: ${s.card};
  --secondary: ${s.card};
  --muted: ${s.card};
  --border: ${s.border};
  --input: ${s.border};
  --sidebar-border: ${s.border};
  --primary: ${s.accent};
  --sidebar-primary: ${s.accent};
  --ring: ${s.accent};
  --sidebar-ring: ${s.accent};
}
`;
}
