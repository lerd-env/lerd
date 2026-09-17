import { describe, it, expect, beforeEach } from 'vitest';
import { rememberRustfsTheme, themeRustfsDocument } from './rustfsTheme';

describe('rememberRustfsTheme', () => {
  beforeEach(() => localStorage.clear());

  // The console reads this as it boots, which is after the frame's load event,
  // so the value has to be waiting for it.
  it('records the mode the console should boot in', () => {
    rememberRustfsTheme(true);
    expect(localStorage.getItem('active_theme')).toBe('dark');
    rememberRustfsTheme(false);
    expect(localStorage.getItem('active_theme')).toBe('light');
  });
});

describe('themeRustfsDocument', () => {
  function framed(): Document {
    const frame = document.createElement('iframe');
    document.body.appendChild(frame);
    return frame.contentDocument!;
  }

  it('moves the class its own dark palette is keyed on', () => {
    const doc = framed();
    themeRustfsDocument(doc, true);
    expect(doc.documentElement.classList.contains('dark')).toBe(true);
    expect(doc.documentElement.classList.contains('light')).toBe(false);
    themeRustfsDocument(doc, false);
    expect(doc.documentElement.classList.contains('light')).toBe(true);
    expect(doc.documentElement.classList.contains('dark')).toBe(false);
  });

  it('points its surfaces at the theme, in whichever palette is showing', () => {
    document.documentElement.style.setProperty('--lerd-bg', '#272822');
    document.documentElement.style.setProperty('--lerd-card', '#31322c');
    document.documentElement.style.setProperty('--lerd-accent', '#f92672');
    const doc = framed();
    themeRustfsDocument(doc, true);
    const css = doc.getElementById('lerd-rustfs-theme')!.textContent!;
    expect(css).toContain('--background: #272822');
    expect(css).toContain('--sidebar: #31322c');
    expect(css).toContain('--primary: #f92672');
  });

  it('uses the dashboard greys in light mode, and one style tag throughout', () => {
    const doc = framed();
    themeRustfsDocument(doc, true);
    themeRustfsDocument(doc, false);
    expect(doc.querySelectorAll('#lerd-rustfs-theme').length).toBe(1);
    expect(doc.getElementById('lerd-rustfs-theme')!.textContent).toContain('--background: #f9fafb');
  });
});
