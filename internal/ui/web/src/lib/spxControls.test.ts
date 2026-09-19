import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  isSpxReportView,
  setSpxConfigHidden,
  padSpxControlPanel,
  themeSpxDocument,
  fetchSpxReportCount
} from './spxControls';

describe('isSpxReportView', () => {
  it('is true for the single-report analysis URL', () => {
    expect(isSpxReportView('http://h/_spx/?SPX_UI_URI=/report.html&key=abc')).toBe(true);
  });

  it('is false for the control panel list URL', () => {
    expect(isSpxReportView('http://h/_spx/?SPX_UI_URI=/')).toBe(false);
  });
});

describe('setSpxConfigHidden', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('hides and restores the Configuration form', () => {
    document.body.innerHTML = '<form id="config"><fieldset></fieldset></form>';
    const form = document.getElementById('config')!;

    expect(setSpxConfigHidden(document, true)).toBe(true);
    expect(form.style.display).toBe('none');

    setSpxConfigHidden(document, false);
    expect(form.style.display).toBe('');
  });

  it('returns false when the Configuration form is absent', () => {
    document.body.innerHTML = '<div>nothing here</div>';
    expect(setSpxConfigHidden(document, true)).toBe(false);
  });

  it('hides the form in a separate realm, as the SPX iframe is', () => {
    // The SPX page is an iframe: its elements belong to the iframe's own JS
    // realm, so the helper must not rely on this document's HTMLElement.
    document.body.innerHTML = '<iframe></iframe>';
    const idoc = document.querySelector('iframe')!.contentDocument!;
    idoc.body.innerHTML = '<form id="config"><fieldset></fieldset></form>';
    const form = idoc.getElementById('config') as HTMLElement;

    expect(setSpxConfigHidden(idoc, true)).toBe(true);
    expect(form.style.display).toBe('none');
  });
});

describe('padSpxControlPanel', () => {
  beforeEach(() => {
    document.head.innerHTML = '';
  });

  it('adds a padding style tag and is idempotent', () => {
    padSpxControlPanel(document);
    padSpxControlPanel(document);
    expect(document.querySelectorAll('#lerd-spx-pad').length).toBe(1);
    expect(document.getElementById('lerd-spx-pad')!.textContent).toContain('padding');
  });
});

describe('themeSpxDocument', () => {
  beforeEach(() => {
    document.head.innerHTML = '';
  });

  it('points SPX\'s own variables at lerd\'s dark surfaces', () => {
    themeSpxDocument(document, true);
    const css = document.getElementById('lerd-spx-theme')!.textContent!;
    expect(css).toContain('--gradient-end: var(--lerd-bg)');
    expect(css).toContain('--border-color: var(--lerd-border)');
  });

  // The palette holds the dark surfaces only, so reusing them in light mode is
  // what kept the profiler dark whatever the dashboard was wearing.
  it('leaves the dark surfaces out of light mode', () => {
    themeSpxDocument(document, false);
    const css = document.getElementById('lerd-spx-theme')!.textContent!;
    expect(css).not.toContain('var(--lerd-bg)');
    expect(css).not.toContain('var(--lerd-card)');
    expect(css).toContain('--gradient-begin: #ffffff');
    // The accent is not a surface: it has a tone for each mode already.
    expect(css).toContain('--hover-color: var(--lerd-accent)');
  });

  it('rewrites the one style tag when the mode changes', () => {
    themeSpxDocument(document, true);
    themeSpxDocument(document, false);
    expect(document.querySelectorAll('#lerd-spx-theme').length).toBe(1);
    expect(document.getElementById('lerd-spx-theme')!.textContent).toContain('#ffffff');
  });

  it('lands after the sheet it has to outrank', () => {
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = '?SPX_UI_URI=/css/main.css';
    document.head.appendChild(link);
    themeSpxDocument(document, true);
    expect(document.head.lastElementChild!.id).toBe('lerd-spx-theme');
  });
});

describe('fetchSpxReportCount', () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('returns the number of reports from the metadata endpoint', async () => {
    globalThis.fetch = vi.fn(
      async () =>
        new Response(JSON.stringify({ results: [{ key: 'a' }, { key: 'b' }] }), { status: 200 })
    ) as unknown as typeof fetch;
    expect(await fetchSpxReportCount()).toBe(2);
  });

  it('returns null when the request fails', async () => {
    globalThis.fetch = vi.fn(async () => new Response('nope', { status: 500 })) as unknown as typeof fetch;
    expect(await fetchSpxReportCount()).toBeNull();
  });

  it('returns null when the payload has no results array', async () => {
    globalThis.fetch = vi.fn(
      async () => new Response(JSON.stringify({}), { status: 200 })
    ) as unknown as typeof fetch;
    expect(await fetchSpxReportCount()).toBeNull();
  });
});
