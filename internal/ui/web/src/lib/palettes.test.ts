import { describe, expect, it } from 'vitest';
import { luminance, parseHex } from './brandTint';
import {
  BUILTIN_PALETTES,
  DEFAULT_PALETTE_ID,
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
