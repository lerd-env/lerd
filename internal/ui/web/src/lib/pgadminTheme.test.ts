import { describe, it, expect, beforeEach } from 'vitest';
import { pgadminPalette, repaintPgadmin, themePgadminDocument } from './pgadminTheme';

function sheetWith(css: string): Document {
  const frame = document.createElement('iframe');
  document.body.appendChild(frame);
  const doc = frame.contentDocument!;
  const style = doc.createElement('style');
  style.textContent = css;
  doc.head.appendChild(style);
  return doc;
}

const rule = (doc: Document) => (doc.styleSheets[0].cssRules[0] as CSSStyleRule).style;

describe('pgadminPalette', () => {
  beforeEach(() => {
    document.documentElement.style.cssText = '';
    document.documentElement.style.setProperty('--lerd-bg', '#282a36');
    document.documentElement.style.setProperty('--lerd-card', '#343746');
    document.documentElement.style.setProperty('--lerd-accent', '#bd93f9');
  });

  it('puts the theme under pgAdmin design: its page, its panels, its primary', () => {
    const p = pgadminPalette(true);
    expect(p['#1e1e1e']).toBe('#282a36');
    expect(p['#282828']).toBe('#343746');
    expect(p['#333333']).toBe('#343746');
    expect(p['#234d6e']).toBe('#bd93f9');
    // Its lightest grey is the tone it writes text in, which stays readable.
    expect(p['#d4d4d4']).toBe('#e5e7eb');
    // White is the label on a filled button, and dimming it only costs contrast
    // against a fill the theme has already chosen.
    expect(p['#ffffff']).toBeUndefined();
    // The blue its icons are drawn in comes onto the accent with everything else.
    expect(p['#1b71b5']).toBe('#bd93f9');
    // PostgreSQL's own blue is the elephant, and a logo keeps the colour it is.
    expect(p['#336791']).toBeUndefined();
  });

  it('follows a palette change without being told how it is stored', () => {
    document.documentElement.style.setProperty('--lerd-accent', '#ff2d20');
    expect(pgadminPalette(true)['#234d6e']).toBe('#ff2d20');
  });
});

describe('repaintPgadmin', () => {
  beforeEach(() => {
    document.documentElement.style.cssText = '';
    document.documentElement.style.setProperty('--lerd-bg', '#282a36');
    document.documentElement.style.setProperty('--lerd-card', '#343746');
    document.documentElement.style.setProperty('--lerd-accent', '#bd93f9');
  });

  it('rewrites the colours wherever MUI wrote them, hex or rgb', () => {
    const doc = sheetWith('.mui-x { background-color: #1e1e1e; color: rgb(212, 212, 212); }');
    repaintPgadmin(doc, true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#282a36');
    expect(rule(doc).getPropertyValue('color')).toBe('#e5e7eb');
  });

  it('leaves pgAdmin its own design in light mode', () => {
    const doc = sheetWith('.mui-x { background-color: #1e1e1e; }');
    repaintPgadmin(doc, false);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#1e1e1e');
  });

  it('paints a second palette from pgAdmin colours, not from the first palette', () => {
    const doc = sheetWith('.mui-x { background-color: #1e1e1e; }');
    repaintPgadmin(doc, true);
    document.documentElement.style.setProperty('--lerd-bg', '#101010');
    repaintPgadmin(doc, true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#101010');
  });
});

describe('themePgadminDocument', () => {
  beforeEach(() => {
    document.head.innerHTML = '';
    document.documentElement.style.cssText = '';
  });

  it('puts the theme page behind the app and rewrites the one style tag', () => {
    document.documentElement.style.setProperty('--lerd-bg', '#141618');
    themePgadminDocument(document, true);
    expect(document.getElementById('lerd-pgadmin-theme')!.textContent).toContain(
      'background: #141618'
    );
    themePgadminDocument(document, false);
    expect(document.querySelectorAll('#lerd-pgadmin-theme').length).toBe(1);
    expect(document.getElementById('lerd-pgadmin-theme')!.textContent).toBe('');
  });
});
