import { describe, it, expect, beforeEach } from 'vitest';
import {
  themeMeilisearchDocument,
  repaintMeilisearch,
  watchMeilisearchRules
} from './meilisearchTheme';

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

describe('themeMeilisearchDocument', () => {
  beforeEach(() => {
    document.head.innerHTML = '';
    document.documentElement.style.cssText = '';
  });

  const css = () => document.getElementById('lerd-meilisearch-theme')!.textContent!;

  it('puts the theme page colour behind a page the app never styles', () => {
    document.documentElement.style.setProperty('--lerd-bg', '#141618');
    themeMeilisearchDocument(document, true);
    expect(css()).toContain('background: #141618');
    expect(css()).toContain('color-scheme: dark');
  });

  it('rewrites the one style tag on a mode change', () => {
    themeMeilisearchDocument(document, true);
    themeMeilisearchDocument(document, false);
    expect(document.querySelectorAll('#lerd-meilisearch-theme').length).toBe(1);
    expect(css()).toContain('color-scheme: light');
  });
});

describe('repaintMeilisearch', () => {
  beforeEach(() => {
    document.documentElement.style.cssText = '';
    document.documentElement.style.setProperty('--lerd-bg', '#141618');
    document.documentElement.style.setProperty('--lerd-card', '#202326');
    document.documentElement.style.setProperty('--lerd-accent', '#3daee9');
  });

  // The page it paints with its lightest grey has to land on the theme's page,
  // which is the whole point: the frame stops being a slab of another colour.
  it('lands its page grey exactly on the theme page colour', () => {
    const doc = sheetWith('.x { background-color: rgb(250, 251, 254); }');
    repaintMeilisearch(doc, true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#141618');
  });

  it('lands its panels on the theme card colour', () => {
    const doc = sheetWith('.x { background-color: rgb(255, 255, 255); }');
    repaintMeilisearch(doc, true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#202326');
  });

  // Its stylesheet writes white as the keyword, which no hex or rgb() match
  // would have caught.
  it('lands the white keyword on the theme card colour', () => {
    const doc = sheetWith('.x { background-color: white; }');
    repaintMeilisearch(doc, true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#202326');
  });

  it('puts the theme accent where its own was', () => {
    const doc = sheetWith('.x { color: rgb(228, 19, 89); }');
    repaintMeilisearch(doc, true);
    expect(rule(doc).color).toBe('#3daee9');
  });

  // Its darkest grey is text, which on a dark page has to read light.
  it('turns its text tone around for dark mode', () => {
    const doc = sheetWith('.x { color: rgb(57, 72, 110); }');
    repaintMeilisearch(doc, true);
    expect(rule(doc).color).toBe('#e5e7eb');
  });

  it('leaves its text tone alone in light mode', () => {
    const doc = sheetWith('.x { color: rgb(57, 72, 110); }');
    repaintMeilisearch(doc, false);
    expect(rule(doc).color).toBe('#39486e');
  });

  it('applies a second theme to its colours, not to the first theme', () => {
    const doc = sheetWith('.x { color: rgb(228, 19, 89); }');
    repaintMeilisearch(doc, true);
    document.documentElement.style.setProperty('--lerd-accent', '#fe8019');
    repaintMeilisearch(doc, true);
    expect(rule(doc).color).toBe('#fe8019');
  });

  it('leaves a colour of its own it does not use for chrome', () => {
    const doc = sheetWith('.x { color: rgb(220, 38, 38); }');
    repaintMeilisearch(doc, true);
    expect(rule(doc).color).toBe('rgb(220, 38, 38)');
  });
});

describe('watchMeilisearchRules', () => {
  beforeEach(() => {
    document.documentElement.style.cssText = '';
    document.documentElement.style.setProperty('--lerd-bg', '#141618');
    document.documentElement.style.setProperty('--lerd-accent', '#3daee9');
  });

  // Its components add a rule each as they mount, through the stylesheet rather
  // than the DOM, so a sweep alone leaves everything that mounts after it in
  // Meilisearch's own colours.
  it('repaints a rule inserted after the sweep', () => {
    const doc = sheetWith('.first { color: rgb(228, 19, 89); }');
    const win = doc.defaultView as Window & typeof globalThis;
    repaintMeilisearch(doc, true);
    watchMeilisearchRules(win, () => true);

    const sheet = doc.styleSheets[0];
    const at = sheet.insertRule('.late { background-color: rgb(250, 251, 254); }', sheet.cssRules.length);
    expect((sheet.cssRules[at] as CSSStyleRule).style.getPropertyValue('background-color')).toBe('#141618');
  });

  it('patches a realm once, however often it is asked', () => {
    const doc = sheetWith('.x { color: red; }');
    const win = doc.defaultView as Window & typeof globalThis;
    watchMeilisearchRules(win, () => true);
    const patched = win.CSSStyleSheet.prototype.insertRule;
    watchMeilisearchRules(win, () => true);
    expect(win.CSSStyleSheet.prototype.insertRule).toBe(patched);
  });
});
