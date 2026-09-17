import { describe, it, expect } from 'vitest';
import { syncEmbeddedTheme } from './embeddedTheme';

function docWith(links: Array<{ media: string; href: string }>): Document {
  const d = document.implementation.createHTMLDocument('x');
  for (const l of links) {
    const el = d.createElement('link');
    el.rel = 'stylesheet';
    el.media = l.media;
    el.setAttribute('href', l.href);
    d.head.appendChild(el);
  }
  return d;
}

describe('syncEmbeddedTheme', () => {
  it('turns a media-gated dark sheet on when lerd is dark', () => {
    const d = docWith([
      { media: '', href: 'default.css' },
      { media: '(prefers-color-scheme: dark)', href: 'dark.css' }
    ]);
    syncEmbeddedTheme(d, true);
    expect(d.querySelector<HTMLLinkElement>('link[href="dark.css"]')?.media).toBe('all');
    // The unconditional sheet is left exactly as it was.
    expect(d.querySelector<HTMLLinkElement>('link[href="default.css"]')?.media).toBe('');
  });

  it('turns it off when lerd is light, even if the browser prefers dark', () => {
    const d = docWith([{ media: '(prefers-color-scheme: dark)', href: 'dark.css' }]);
    syncEmbeddedTheme(d, false);
    expect(d.querySelector<HTMLLinkElement>('link[href="dark.css"]')?.media).toBe('not all');
  });

  it('is repeatable, so a page that navigates keeps the theme it was given', () => {
    const d = docWith([{ media: '(prefers-color-scheme: dark)', href: 'dark.css' }]);
    syncEmbeddedTheme(d, true);
    syncEmbeddedTheme(d, false);
    syncEmbeddedTheme(d, true);
    expect(d.querySelector<HTMLLinkElement>('link[href="dark.css"]')?.media).toBe('all');
  });

  it('leaves a dashboard that gates nothing on the media query alone', () => {
    const d = docWith([{ media: 'screen', href: 'app.css' }]);
    syncEmbeddedTheme(d, true);
    expect(d.querySelector<HTMLLinkElement>('link[href="app.css"]')?.media).toBe('screen');
  });

  it('survives a document it cannot touch', () => {
    expect(() => syncEmbeddedTheme(null, true)).not.toThrow();
  });
});

describe('a page carrying both a light and a dark sheet', () => {
  function pair(): Document {
    const d = document.implementation.createHTMLDocument('x');
    for (const [media, href] of [
      ['(prefers-color-scheme: light)', 'light.css'],
      ['(prefers-color-scheme: dark)', 'dark.css']
    ]) {
      const el = d.createElement('link');
      el.rel = 'stylesheet';
      el.media = media;
      el.setAttribute('href', href);
      d.head.appendChild(el);
    }
    return d;
  }

  it('turns the light one off while the dark one is on', () => {
    const d = pair();
    syncEmbeddedTheme(d, true);
    expect(d.querySelector<HTMLLinkElement>('link[href="dark.css"]')?.media).toBe('all');
    expect(d.querySelector<HTMLLinkElement>('link[href="light.css"]')?.media).toBe('not all');
  });

  it('and the other way round', () => {
    const d = pair();
    syncEmbeddedTheme(d, false);
    expect(d.querySelector<HTMLLinkElement>('link[href="dark.css"]')?.media).toBe('not all');
    expect(d.querySelector<HTMLLinkElement>('link[href="light.css"]')?.media).toBe('all');
  });
});

describe('palette', () => {
  it('copies lerd\'s own custom properties into the embedded document', () => {
    const host = document.createElement('html');
    host.style.setProperty('--lerd-accent', '#00ff00');
    host.style.setProperty('--lerd-bg', '#111111');
    const d = document.implementation.createHTMLDocument('x');
    syncEmbeddedTheme(d, true, host);
    expect(d.documentElement.style.getPropertyValue('--lerd-accent')).toBe('#00ff00');
    expect(d.documentElement.style.getPropertyValue('--lerd-bg')).toBe('#111111');
  });

  it('leaves the embedded document alone when lerd declares nothing', () => {
    const host = document.createElement('html');
    const d = document.implementation.createHTMLDocument('x');
    syncEmbeddedTheme(d, false, host);
    expect(d.documentElement.style.getPropertyValue('--lerd-accent')).toBe('');
  });
});
