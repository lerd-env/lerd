import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fontSize, FONT_MAX, FONT_MIN } from './editorFont';

const fresh = async () => {
  vi.resetModules();
  return (await import('./editorFont')).editorFontSize;
};

describe('fontSize', () => {
  it('reads a stored size and falls back to the default', () => {
    expect(fontSize('14')).toBe(14);
    expect(fontSize(null)).toBe(12);
    expect(fontSize('big')).toBe(12);
  });

  it('clamps a hand-edited value to the supported range', () => {
    expect(fontSize(String(FONT_MAX + 4))).toBe(FONT_MAX);
    expect(fontSize(String(FONT_MIN - 4))).toBe(FONT_MIN);
  });
});

describe('editorFontSize', () => {
  beforeEach(() => localStorage.clear());

  it('starts from the default with nothing stored', async () => {
    expect(get(await fresh())).toBe(12);
  });

  it('keeps the size an older build saved for tinker alone', async () => {
    localStorage.setItem('tinker:fontSize', '16');
    expect(get(await fresh())).toBe(16);
  });

  it('prefers the shared size over the old tinker one', async () => {
    localStorage.setItem('tinker:fontSize', '16');
    localStorage.setItem('lerd-editor-font-size', '18');
    expect(get(await fresh())).toBe(18);
  });

  it('remembers a change for the next load', async () => {
    (await fresh()).set(15);
    expect(get(await fresh())).toBe(15);
  });

  it('keeps working in memory when the browser refuses to store it', async () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('full', 'QuotaExceededError');
    });
    const store = await fresh();
    store.set(15);
    expect(get(store)).toBe(15);
    setItem.mockRestore();
  });

  it('ignores a corrupt stored size', async () => {
    localStorage.setItem('lerd-editor-font-size', 'huge');
    expect(get(await fresh())).toBe(12);
  });
});
