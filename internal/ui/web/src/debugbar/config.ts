// What lerd-ui bakes into the bar's script (see internal/ui/debugbar.go).
import type { PaletteFile } from '$lib/palettes';

export interface BarConfig {
  style: 'dock' | 'compact';
  edge: 'bottom' | 'top';
  corner: 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left';
  theme: 'auto' | 'light' | 'dark';
  palette: string;
  base: string;
  site: string;
  path: string;
  // roots are the linked sites' folders; local says the site can only be
  // reached from this machine, so the bar may read code and open the editor.
  roots: string[];
  local: boolean;
  // themes are the user's and the desktop's theme files, so the bar draws in
  // the theme the dashboard is on, not only a built-in one.
  themes: PaletteFile[];
}

declare const __lerdBarConfig: BarConfig | undefined;

export const barConfig: BarConfig =
  typeof __lerdBarConfig === 'undefined'
    ? { style: 'dock', edge: 'bottom', corner: 'bottom-right', theme: 'auto', palette: '', base: '/_lerd/browser/bar/', site: '', path: '', roots: [], local: false, themes: [] }
    : __lerdBarConfig;
