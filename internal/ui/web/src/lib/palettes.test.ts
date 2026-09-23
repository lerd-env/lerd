import { describe, expect, it } from 'vitest';
import { luminance, parseHex } from './brandTint';
import {
  BUILTIN_PALETTES,
  DEFAULT_PALETTE_ID,
  asDesktopStandIn,
  chromeBorder,
  onAccent,
  paletteById,
  paletteVars,
  resolvePalette
} from './palettes';

describe('resolvePalette', () => {
  it('derives every missing tone from the accent', () => {
    const p = resolvePalette({ id: 'ocean', name: 'Ocean', accent: '#3B7EA1' })!;
    expect(p.accent).toBe('#3b7ea1');
    expect(p.accentHover).not.toBe(p.accent);
    expect(p.accentDark).toBeTruthy();
    expect(p.accentHoverDark).not.toBe(p.accentDark);
    expect(p.card).toBe(BUILTIN_PALETTES[0].card);
    expect(p.source).toBe('user');
  });

  it('marks a theme the desktop supplied so nothing offers to remove it', () => {
    const p = resolvePalette({ id: 'omarchy', name: 'Omarchy (nord)', accent: '#81a1c1', source: 'desktop' })!;
    expect(p.source).toBe('desktop');
  });

  it('lifts a near-black accent so it reads on the dark card', () => {
    const p = resolvePalette({ id: 'ink', name: 'Ink', accent: '#050505' })!;
    expect(p.accentDark).not.toBe(p.accent);
  });

  it('keeps tones the file declared', () => {
    const p = resolvePalette({
      id: 'ocean',
      name: 'Ocean',
      accent: '#3b7ea1',
      accent_dark: '#7fb6d4',
      card: '#141b1f'
    })!;
    expect(p.accentDark).toBe('#7fb6d4');
    expect(p.card).toBe('#141b1f');
  });

  it('rejects anything that is not a plain hex colour', () => {
    expect(resolvePalette({ id: 'x', name: 'X', accent: 'url(evil)' })).toBeNull();
    expect(resolvePalette({ id: 'x', name: 'X', accent: 'rebeccapurple' })).toBeNull();
    expect(resolvePalette({ id: '', name: 'X', accent: '#112233' })).toBeNull();
  });
});

describe('the built-in themes', () => {
  it('are all usable definitions with distinct ids', () => {
    const ids = BUILTIN_PALETTES.map((p) => p.id);
    expect(new Set(ids).size).toBe(ids.length);
    for (const p of BUILTIN_PALETTES) {
      expect(p.source).toBe('builtin');
      for (const tone of [p.accent, p.accentHover, p.accentDark, p.accentHoverDark, p.bg, p.card, p.border, p.muted]) {
        expect(tone).toMatch(/^#[0-9a-f]{6}$/);
      }
    }
  });

  // The accent is link and label text, so the light tone has to read on the
  // white card and the dark tone on the theme's own background. A classic
  // scheme's colour is chosen for a dark editor and rarely passes on white as
  // given, which is why each theme carries both tones rather than one.
  //
  // Every theme but the default clears WCAG AA. The default is the bright red
  // that prompted all of this and sits under it; it keeps its colour because
  // changing it would repaint the dashboard for everyone who never complained.
  it('keep both accent tones readable on the surface they land on', () => {
    for (const p of BUILTIN_PALETTES) {
      expect(contrast(p.accent, '#ffffff')).toBeGreaterThanOrEqual(3);
      expect(contrast(p.accentDark, p.bg)).toBeGreaterThanOrEqual(3);
      if (p.id !== DEFAULT_PALETTE_ID) {
        expect(contrast(p.accent, '#ffffff')).toBeGreaterThanOrEqual(4.5);
      }
    }
  });
});

describe('paletteById', () => {
  it('falls back to the default when the theme is gone', () => {
    expect(paletteById(BUILTIN_PALETTES, 'deleted').id).toBe(DEFAULT_PALETTE_ID);
  });

  it('finds a theme it has', () => {
    expect(paletteById(BUILTIN_PALETTES, 'muted').id).toBe('muted');
  });
});

describe('paletteVars', () => {
  it('picks the tone for the mode in effect', () => {
    const p = resolvePalette({
      id: 'ocean',
      name: 'Ocean',
      accent: '#3b7ea1',
      accent_dark: '#7fb6d4'
    })!;
    expect(paletteVars(p, false)['--lerd-accent']).toBe('#3b7ea1');
    expect(paletteVars(p, true)['--lerd-accent']).toBe('#7fb6d4');
  });
});


function contrast(a: string, b: string): number {
  const la = luminance(parseHex(a)!);
  const lb = luminance(parseHex(b)!);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

describe('asDesktopStandIn', () => {
  const builtin = (id: string) => BUILTIN_PALETTES.find((p) => p.id === id)!;

  it('gives a Plasma entry the id of the built-in it stands in for', () => {
    const f = asDesktopStandIn({
      id: 'plasma',
      name: 'Plasma (Breeze Dark)',
      accent: '#3dd425',
      bg: '#141618',
      card: '#202326',
      border: '#292c30',
      source: 'desktop'
    });
    expect(f.id).toBe('breeze');
    // The palette people know by name, wearing what the desktop is actually on.
    expect(f.name).toBe('Breeze');
    expect(f.card).toBe('#202326');
    // Plasma publishes no tone for dim text, and Breeze's own reads better
    // against its surfaces than the default grey would.
    expect(f.muted).toBe(builtin('breeze').muted);
  });

  it('lends the Adwaita surfaces to a GNOME entry that carries only an accent', () => {
    const f = asDesktopStandIn({
      id: 'gnome',
      name: 'GNOME (purple)',
      accent: '#9141ac',
      source: 'desktop'
    });
    expect(f.id).toBe('adwaita');
    expect(f.name).toBe('Adwaita');
    expect(f.accent).toBe('#9141ac');
    expect(f.bg).toBe(builtin('adwaita').bg);
    expect(f.card).toBe(builtin('adwaita').card);
  });

  it('keeps the macOS surfaces under the accent the Mac is set to', () => {
    const f = asDesktopStandIn({
      id: 'macos',
      name: 'macOS (green)',
      accent: '#62ba46',
      source: 'desktop'
    });
    expect(f.id).toBe('macos');
    expect(f.name).toBe('macOS');
    expect(f.accent).toBe('#62ba46');
    expect(f.card).toBe(builtin('macos').card);
    expect(f.chrome_light).toBe(builtin('macos').chromeLight);
  });

  it('leaves a desktop with no built-in of its own alone', () => {
    const f = asDesktopStandIn({
      id: 'omarchy',
      name: 'Omarchy (nord)',
      accent: '#81a1c1',
      source: 'desktop'
    });
    expect(f.id).toBe('omarchy');
    expect(f.bg).toBeUndefined();
  });

  it('leaves a file alone, whatever it is called', () => {
    const f = asDesktopStandIn({ id: 'plasma', name: 'My Plasma', accent: '#112233' });
    expect(f.id).toBe('plasma');
  });
});

describe('onAccent', () => {
  it('writes white on a dark accent', () => {
    expect(onAccent(BUILTIN_PALETTES[0].accent)).toBe('#ffffff');
    expect(onAccent('#1c71d8')).toBe('#ffffff');
    expect(onAccent('#e93a9a')).toBe('#ffffff');
  });

  // Where the desktop themes land: a bright accent with white on it is pale
  // text on a pale fill.
  it('writes near black on a bright accent', () => {
    expect(onAccent('#3dd425')).toBe('#0d0d0d');
    expect(onAccent('#c88800')).toBe('#0d0d0d');
    expect(onAccent('#3daee9')).toBe('#0d0d0d');
  });

  it('falls back to white for anything that is not a colour', () => {
    expect(onAccent('rebeccapurple')).toBe('#ffffff');
  });
});

describe('the light mode chrome', () => {
  it('is tinted on the desktop palettes, the ones whose desktops tint their own', () => {
    const tinted = BUILTIN_PALETTES.filter((p) => p.chromeLight);
    expect(tinted.map((p) => p.id)).toEqual(['breeze', 'adwaita', 'macos']);
  });

  it("wears libadwaita's own sidebar_bg_color on Adwaita", () => {
    expect(BUILTIN_PALETTES.find((p) => p.id === 'adwaita')!.chromeLight).toBe('#ebebed');
  });

  // A light Plasma scheme publishes the tone it tints its own chrome with, and
  // that beats the one Breeze ships.
  it('takes the tone a light scheme published over the built-in\u2019s', () => {
    const f = asDesktopStandIn({
      id: 'plasma',
      name: 'Plasma',
      accent: '#3daee9',
      chrome_light: '#e8e6e3',
      source: 'desktop'
    });
    expect(f.chrome_light).toBe('#e8e6e3');
  });

  // The separators are drawn off the chrome rather than off a fixed grey, or a
  // tinted rail loses the lines a white one had.
  it('draws its lines off the tone the chrome is wearing', () => {
    expect(chromeBorder('#ffffff')).toBe('#e6e6e6');
    expect(chromeBorder('#eff0f1')).toBe('#d7d8d9');
    expect(chromeBorder('#ebebeb')).toBe('#d4d4d4');
  });

  it('follows the theme onto the border variable', () => {
    const p = resolvePalette({ id: 'breeze', name: 'Breeze', accent: '#17698f', chrome_light: '#eff0f1' })!;
    expect(paletteVars(p, false)['--lerd-chrome-border']).toBe('#d7d8d9');
  });

  it('stays white for a theme that does not ask for one', () => {
    const p = resolvePalette({ id: 'ocean', name: 'Ocean', accent: '#3b7ea1' })!;
    expect(paletteVars(p, false)['--lerd-chrome-light']).toBe('#ffffff');
  });

  it('follows the theme that does', () => {
    const p = resolvePalette({ id: 'macos', name: 'macOS', accent: '#62ba46', chrome_light: '#F3F4F6' })!;
    expect(paletteVars(p, false)['--lerd-chrome-light']).toBe('#f3f4f6');
  });
});
