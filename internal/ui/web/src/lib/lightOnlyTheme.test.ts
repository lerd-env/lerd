import { describe, it, expect, beforeEach } from 'vitest';
import { repaintLightOnly, themeLightOnlyDocument } from './lightOnlyTheme';

function sheetWith(css: string): Document {
  const frame = document.createElement('iframe');
  document.body.appendChild(frame);
  const doc = frame.contentDocument!;
  const style = doc.createElement('style');
  style.textContent = css;
  doc.head.appendChild(style);
  return doc;
}

const rule = (doc: Document, at = 0) => (doc.styleSheets[0].cssRules[at] as CSSStyleRule).style;

describe('repaintLightOnly', () => {
  beforeEach(() => {
    document.documentElement.style.cssText = '';
    document.documentElement.style.setProperty('--lerd-bg', '#101010');
    document.documentElement.style.setProperty('--lerd-card', '#1c1c1c');
  });

  it('turns a design the right way up: its page becomes the page, its text the text', () => {
    const doc = sheetWith('body { background-color: #ffffff; color: #212529; }');
    repaintLightOnly(doc, true);
    // White is the lightest thing the design has, so it lands on the page, and
    // the near-black it wrote text in comes back light enough to read.
    expect(rule(doc).getPropertyValue('background-color')).toBe('#101010');
    const text = rule(doc).getPropertyValue('color');
    expect(text).not.toBe('#212529');
    expect(parseInt(text.slice(1, 3), 16)).toBeGreaterThan(0x80);
  });

  it('leaves a colour that carries a hue saying what it says', () => {
    const doc = sheetWith('.alert-danger { background-color: #dc3545; border-color: #198754; }');
    repaintLightOnly(doc, true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#dc3545');
    expect(rule(doc).getPropertyValue('border-color')).toBe('#198754');
  });

  it('keeps the tone a filled button already chose for its own text', () => {
    const doc = sheetWith('.btn-primary { background-color: #0d6efd; color: #ffffff; }');
    repaintLightOnly(doc, true);
    expect(rule(doc).getPropertyValue('color')).toBe('#ffffff');
  });

  it('reaches the colours inside a media block', () => {
    const doc = sheetWith('@media screen { .card { background-color: #eeeeee; } }');
    const nested = ((doc.styleSheets[0].cssRules[0] as CSSMediaRule).cssRules[0] as CSSStyleRule)
      .style;
    repaintLightOnly(doc, true);
    expect(nested.getPropertyValue('background-color')).not.toBe('#eeeeee');
  });

  it('leaves a design alone where the browser cannot name its colours', () => {
    // jsdom has no canvas, so the names are unreadable here and the rule has to
    // come back untouched rather than half painted.
    const doc = sheetWith('.err { background-color: pink; border: 1px solid maroon; }');
    repaintLightOnly(doc, true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('pink');
  });

  it('lays a panel flat instead of carrying its sheen onto a dark one', () => {
    const doc = sheetWith('.card-header { background: linear-gradient(#eeeeee, #cccccc); }');
    repaintLightOnly(doc, true);
    const bg = rule(doc).getPropertyValue('background');
    expect(bg).not.toContain('gradient');
    expect(bg).toMatch(/^#[0-9a-f]{6}$/);
  });

  it('drops the shadow a light design lifts its text with', () => {
    const doc = sheetWith('.tab { color: #333333; text-shadow: 0 1px 0 #ffffff; }');
    repaintLightOnly(doc, true);
    expect(rule(doc).getPropertyValue('text-shadow')).toBe('none');
    repaintLightOnly(doc, false);
    expect(rule(doc).getPropertyValue('text-shadow')).toContain('#ffffff');
  });

  it('keeps a shadow under the panel rather than a halo around it', () => {
    const doc = sheetWith('.note { box-shadow: 0 0 10px #cccccc; }');
    repaintLightOnly(doc, true);
    const shadow = rule(doc).getPropertyValue('box-shadow');
    expect(shadow).toContain('0 0 10px');
    const painted = shadow.match(/#[0-9a-f]{6}/)![0];
    // Darker than the page it sits on, so it reads as a shadow on either side.
    expect(parseInt(painted.slice(1, 3), 16)).toBeLessThan(0x10);
  });

  it('leaves the sheet lerd itself wrote alone', () => {
    const frame = document.createElement('iframe');
    document.body.appendChild(frame);
    const doc = frame.contentDocument!;
    themeLightOnlyDocument(doc, true);
    const ours = doc.getElementById('lerd-light-only-theme')!.textContent;
    repaintLightOnly(doc, true);
    expect(doc.getElementById('lerd-light-only-theme')!.textContent).toBe(ours);
    expect((doc.styleSheets[0].cssRules[0] as CSSStyleRule).style.getPropertyValue('background'))
      .toContain('#');
  });

  it('hands the design back in light mode rather than painting a second theme', () => {
    const doc = sheetWith('body { background-color: #ffffff; color: #212529; }');
    repaintLightOnly(doc, true);
    repaintLightOnly(doc, false);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#ffffff');
    expect(rule(doc).getPropertyValue('color')).toBe('#212529');
  });

  it('paints a second palette from the design, not from the first palette', () => {
    const doc = sheetWith('body { background-color: #ffffff; }');
    repaintLightOnly(doc, true);
    document.documentElement.style.setProperty('--lerd-bg', '#2a2118');
    repaintLightOnly(doc, true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#2a2118');
  });
});

describe('themeLightOnlyDocument', () => {
  beforeEach(() => {
    document.head.innerHTML = '';
    document.documentElement.style.cssText = '';
  });

  const css = () => document.getElementById('lerd-light-only-theme')!.textContent!;

  it('puts the theme behind a page the design never styled', () => {
    document.documentElement.style.setProperty('--lerd-bg', '#141618');
    themeLightOnlyDocument(document, true);
    expect(css()).toContain('background: #141618');
    expect(css()).toContain('color-scheme: dark');
  });

  it('rewrites the one style tag on a mode change', () => {
    themeLightOnlyDocument(document, true);
    themeLightOnlyDocument(document, false);
    expect(document.querySelectorAll('#lerd-light-only-theme').length).toBe(1);
    expect(css()).not.toContain('color-scheme: dark');
  });
});
