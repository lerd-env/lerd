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

  it('darkens a desktop accent picked for dark surfaces until it reads on white', () => {
    const p = resolvePalette({
      id: 'omarchy',
      name: 'Omarchy (catppuccin)',
      accent: '#89b4fa',
      accent_dark: '#89b4fa',
      source: 'desktop'
    })!;
    const onWhite = 1.05 / (luminance(parseHex(p.accent)!) + 0.05);
    expect(onWhite).toBeGreaterThanOrEqual(4.5);
    expect(p.accentDark).toBe('#89b4fa');
  });

  it('leaves a desktop accent that already reads on white alone', () => {
    const p = resolvePalette({ id: 'omarchy', name: 'Omarchy', accent: '#1e66f5', source: 'desktop' })!;
    expect(p.accent).toBe('#1e66f5');
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
  // The declared tones are the theme's look; paletteVars nudges them to AA as
  // text where they fall short, which is what this checks (see 'accent text').
  it('keep both accent tones readable on the surface they land on', () => {
    for (const p of BUILTIN_PALETTES) {
      expect(contrast(p.accent, '#ffffff')).toBeGreaterThanOrEqual(3);
      expect(contrast(p.accentDark, p.bg)).toBeGreaterThanOrEqual(3);
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
      accent: '#2f6a89',
      accent_dark: '#7fb6d4'
    })!;
    expect(paletteVars(p, false)['--lerd-accent']).toBe('#2f6a89');
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

// Breeze lifts a focused window's header off the window colour, so a theme that
// publishes that tone moves the chrome with focus and one that does not holds.
describe('the chrome under focus', () => {
  const plasma = resolvePalette({
    id: 'breeze',
    name: 'Breeze',
    accent: '#3daee9',
    card: '#202326',
    chrome_light: '#eff0f1',
    chrome_active: '#292c30',
    chrome_light_active: '#dee0e2',
    source: 'desktop'
  })!;

  it('wears the card while the window is in the background', () => {
    expect(paletteVars(plasma, true, false)['--lerd-header']).toBe('#202326');
    expect(paletteVars(plasma, false, false)['--lerd-header-light']).toBe('#eff0f1');
  });

  it('lifts to the focused header tone while the window has focus', () => {
    expect(paletteVars(plasma, true, true)['--lerd-header']).toBe('#292c30');
    expect(paletteVars(plasma, false, true)['--lerd-header-light']).toBe('#dee0e2');
    // The sidebar between the rail and the page is not header, so it holds.
    expect(paletteVars(plasma, false, true)['--lerd-chrome-light']).toBe('#eff0f1');
  });

  // The focused header is the tone Breeze draws its borders in, so the frame's
  // lines have to step off it or they vanish.
  it('keeps the frame lines visible on the lifted header', () => {
    expect(paletteVars(plasma, true, false)['--lerd-header-border']).toBe(plasma.border);
    const lifted = paletteVars(plasma, true, true)['--lerd-header-border'];
    expect(lifted).not.toBe('#292c30');
    expect(lifted).not.toBe(plasma.border);
    expect(paletteVars(plasma, false, true)['--lerd-header-border']).toBe(chromeBorder('#dee0e2'));
  });

  it('holds still for a theme without a focused tone', () => {
    const p = resolvePalette({ id: 'ocean', name: 'Ocean', accent: '#3b7ea1', card: '#141b1f' })!;
    expect(paletteVars(p, true, true)['--lerd-header']).toBe('#141b1f');
    expect(paletteVars(p, false, true)['--lerd-header-light']).toBe('#ffffff');
  });

  it('refuses a focused tone that is not a plain hex colour', () => {
    const p = resolvePalette({ id: 'x', name: 'X', accent: '#112233', chrome_active: 'url(evil)' })!;
    expect(p.chromeActive).toBeUndefined();
  });
});

// Muted text is Tailwind's gray-400 on the dark surfaces and gray-500 on the light
// ones. A tinted theme lifts its cards toward the grey until the small print no
// longer reads, so each theme gets the step of grey its own surfaces need.
describe('muted text', () => {
  const lifted = (card: string) => {
    const [r, g, b] = parseHex(card)!;
    const up = (c: number) => Math.round(c + (255 - c) * 0.08);
    return `#${[up(r), up(g), up(b)].map((c) => c.toString(16).padStart(2, '0')).join('')}`;
  };

  it('reads on every dark theme card, including a lifted pill on it', () => {
    for (const p of BUILTIN_PALETTES) {
      const grey = paletteVars(p, true)['--color-gray-400'];
      expect(contrast(grey, lifted(p.card)), p.id).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('reads on every light theme chrome and on the gray-100 pills', () => {
    for (const p of BUILTIN_PALETTES) {
      const grey = paletteVars(p, false)['--color-gray-500'];
      expect(contrast(grey, p.chromeLight ?? '#ffffff'), p.id).toBeGreaterThanOrEqual(4.5);
      expect(contrast(grey, '#f3f4f6'), p.id).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('leaves the default theme dark grey exactly as Tailwind ships it', () => {
    expect(paletteVars(paletteById(BUILTIN_PALETTES, DEFAULT_PALETTE_ID), true)['--color-gray-400']).toBe('#99a1af');
  });
});

// Links, active tabs and selected rows are accent text, so the accent the page
// actually wears has to reach AA on the card behind it in every theme.
describe('accent text', () => {
  it('reads on the white card in light mode', () => {
    for (const p of BUILTIN_PALETTES) {
      expect(contrast(paletteVars(p, false)['--lerd-accent'], '#ffffff'), p.id).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('reads on the theme card in dark mode', () => {
    for (const p of BUILTIN_PALETTES) {
      expect(contrast(paletteVars(p, true)['--lerd-accent'], p.card), p.id).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('keeps an accent that already passes exactly as declared', () => {
    const lerd = paletteById(BUILTIN_PALETTES, DEFAULT_PALETTE_ID);
    expect(paletteVars(lerd, true)['--lerd-accent']).toBe(lerd.accentDark);
    expect(paletteVars(lerd, true)['--lerd-accent-hover']).toBe(lerd.accentHoverDark);
  });

  it('keeps the hover a visible step away from a nudged accent', () => {
    const lerd = paletteById(BUILTIN_PALETTES, DEFAULT_PALETTE_ID);
    const vars = paletteVars(lerd, false);
    expect(vars['--lerd-accent']).not.toBe(lerd.accent);
    expect(contrast(vars['--lerd-accent-hover'], vars['--lerd-accent'])).toBeGreaterThan(1.2);
  });
});

describe('light surfaces', () => {
  const p = resolvePalette({
    id: 'latte',
    name: 'Latte',
    accent: '#1e66f5',
    bg_light: '#e3e4e8',
    card_light: '#eff1f5',
    border_light: '#dce0e8'
  })!;

  it('moves the light greys a light theme declares', () => {
    const vars = paletteVars(p, false);
    expect(vars['--color-gray-50']).toBe('#e3e4e8');
    expect(vars['--color-white']).toBe('#eff1f5');
    expect(vars['--color-gray-200']).toBe('#dce0e8');
    expect(vars['--lerd-chrome-light']).toBe('#eff1f5');
  });

  it('puts the standard greys back in dark mode and for a theme without them', () => {
    const plain = resolvePalette({ id: 'ocean', name: 'Ocean', accent: '#3b7ea1' })!;
    for (const vars of [paletteVars(p, true), paletteVars(plain, false)]) {
      expect(vars['--color-gray-50']).toBe('');
      expect(vars['--color-white']).toBe('');
      expect(vars['--color-gray-200']).toBe('');
    }
  });
});
