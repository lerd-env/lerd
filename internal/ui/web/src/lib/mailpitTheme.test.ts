import { describe, it, expect, beforeEach } from 'vitest';
import { rememberMailpitTheme, themeMailpitDocument } from './mailpitTheme';

describe('rememberMailpitTheme', () => {
  beforeEach(() => localStorage.clear());

  // Mailpit reads this key when its app boots, which is after the frame's load
  // event, so the value has to be waiting for it rather than applied to it.
  it('records the mode Mailpit should boot in', () => {
    rememberMailpitTheme(true);
    expect(localStorage.getItem('mp-theme')).toBe('dark');
    rememberMailpitTheme(false);
    expect(localStorage.getItem('mp-theme')).toBe('light');
  });
});

describe('themeMailpitDocument', () => {
  beforeEach(() => {
    document.head.innerHTML = '';
    document.documentElement.style.cssText = '';
  });

  it('paints Bootstrap\'s variables with lerd\'s dark surfaces and accent', () => {
    document.documentElement.style.setProperty('--lerd-bg', '#101014');
    document.documentElement.style.setProperty('--lerd-accent', '#ff2d20');
    themeMailpitDocument(document, true);
    const css = document.getElementById('lerd-mailpit-theme')!.textContent!;
    expect(css).toContain('--bs-body-bg: #101014');
    // Bootstrap builds its translucent surfaces from the parts, not the hex.
    expect(css).toContain('--bs-body-bg-rgb: 16, 16, 20');
    expect(css).toContain('--bs-primary: #ff2d20');
  });

  it('uses the dashboard greys in light mode', () => {
    document.documentElement.style.setProperty('--lerd-bg', '#101014');
    themeMailpitDocument(document, false);
    const css = document.getElementById('lerd-mailpit-theme')!.textContent!;
    expect(css).toContain('--bs-body-bg: #f9fafb');
    expect(css).not.toContain('#101014');
  });

  // The header is Bootstrap's primary by default, a band of brand colour where
  // the dashboard has a quiet rail.
  it('gives the header the raised surface rather than the accent', () => {
    themeMailpitDocument(document, true);
    const css = document.getElementById('lerd-mailpit-theme')!.textContent!;
    expect(css).toContain('.navbar.bg-primary');
    expect(css).not.toContain('.bg-primary { background-color: var(--lerd-accent)');
    expect(css).toMatch(/\.navbar\.bg-primary \{\s*background-color: #/);
  });

  it('rewrites the one style tag on a mode change', () => {
    themeMailpitDocument(document, true);
    themeMailpitDocument(document, false);
    expect(document.querySelectorAll('#lerd-mailpit-theme').length).toBe(1);
  });
});
