import { describe, it, expect, beforeEach } from 'vitest';
import {
  hasFrameDesign,
  repaintFrameDesign,
  watchFrameDesign,
  themeFrameDocument
} from './frameThemes';

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

// The palette the dashboard is meant to end up wearing.
const COBALT = { bg: '#193549', card: '#1f4662', accent: '#ffc600' };

beforeEach(() => {
  document.documentElement.style.cssText = '';
  document.documentElement.style.setProperty('--lerd-bg', COBALT.bg);
  document.documentElement.style.setProperty('--lerd-card', COBALT.card);
  document.documentElement.style.setProperty('--lerd-accent', COBALT.accent);
});

describe('hasFrameDesign', () => {
  it('knows the dashboards that bring a dark design of their own', () => {
    for (const name of ['pgadmin', 'kafka-ui', 'redisinsight']) {
      expect(hasFrameDesign(name)).toBe(true);
    }
    expect(hasFrameDesign('phpmyadmin')).toBe(false); // turned around instead
    expect(hasFrameDesign(undefined)).toBe(false);
  });
});

describe('repaintFrameDesign', () => {
  it('puts the page under Kafbat UI own darkest grey', () => {
    const doc = sheetWith('.nav { background-color: #0b0d0e; color: #e3e6e8; }');
    repaintFrameDesign(doc, 'kafka-ui', true);
    expect(rule(doc).getPropertyValue('background-color')).toBe(COBALT.bg);
    // Its lightest grey is the tone it writes text in, which stays light.
    const text = rule(doc).getPropertyValue('color');
    expect(parseInt(text.slice(1, 3), 16)).toBeGreaterThan(0x80);
  });

  it('gives a panel the theme card rather than a step along the scale', () => {
    const doc = sheetWith('.panel { background-color: #2f3639; }');
    repaintFrameDesign(doc, 'kafka-ui', true);
    expect(rule(doc).getPropertyValue('background-color')).toBe(COBALT.card);
  });

  it('carries RedisInsight own blue onto the accent, and leaves danger alone', () => {
    const doc = sheetWith('.a { color: rgb(0, 107, 180); border-color: #bd271e; }');
    repaintFrameDesign(doc, 'redisinsight', true);
    expect(rule(doc).getPropertyValue('color')).toBe(COBALT.accent);
    expect(rule(doc).getPropertyValue('border-color')).toBe('#bd271e');
  });

  it('leaves pgAdmin PostgreSQL blue where it is, the logo keeping its colour', () => {
    const doc = sheetWith('.elephant { fill: #336791; background-color: #234d6e; }');
    repaintFrameDesign(doc, 'pgadmin', true);
    expect(rule(doc).getPropertyValue('fill')).toBe('#336791');
    expect(rule(doc).getPropertyValue('background-color')).toBe(COBALT.accent);
  });

  it('hands each design back in light mode', () => {
    const doc = sheetWith('.nav { background-color: #0b0d0e; }');
    repaintFrameDesign(doc, 'kafka-ui', false);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#0b0d0e');
  });

  it('paints a second palette from the design, not from the first palette', () => {
    const doc = sheetWith('.nav { background-color: #0b0d0e; }');
    repaintFrameDesign(doc, 'kafka-ui', true);
    document.documentElement.style.setProperty('--lerd-bg', '#101010');
    repaintFrameDesign(doc, 'kafka-ui', true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#101010');
  });

  it('does nothing for a dashboard that brings no design of its own', () => {
    const doc = sheetWith('.x { background-color: #0b0d0e; }');
    repaintFrameDesign(doc, 'phpmyadmin', true);
    expect(rule(doc).getPropertyValue('background-color')).toBe('#0b0d0e');
  });
});

describe('watchFrameDesign', () => {
  it('paints a rule the app inserts after the sweep has run', () => {
    const frame = document.createElement('iframe');
    document.body.appendChild(frame);
    const win = frame.contentWindow as Window & typeof globalThis;
    const doc = frame.contentDocument!;
    const style = doc.createElement('style');
    doc.head.appendChild(style);
    watchFrameDesign(win, 'kafka-ui', () => true);
    style.sheet!.insertRule('.late { background-color: #0b0d0e; }', 0);
    expect((style.sheet!.cssRules[0] as CSSStyleRule).style.getPropertyValue('background-color'))
      .toBe(COBALT.bg);
  });
});

describe('themeFrameDocument', () => {
  it('puts the theme page behind the app and clears it again in light', () => {
    themeFrameDocument(document, true);
    expect(document.getElementById('lerd-frame-theme')!.textContent).toContain(
      `background: ${COBALT.bg}`
    );
    themeFrameDocument(document, false);
    expect(document.querySelectorAll('#lerd-frame-theme').length).toBe(1);
    expect(document.getElementById('lerd-frame-theme')!.textContent).toBe('');
  });
});
