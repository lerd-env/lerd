import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { trackWindowIdle } from './windowIdle';

const root = document.documentElement;
let stop: (() => void) | null = null;

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event('visibilitychange'));
}

// jsdom never has focus; a real window the user is looking at does.
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
});

afterEach(() => {
  vi.restoreAllMocks();
  stop?.();
  stop = null;
  setHidden(false);
  delete root.dataset.windowIdle;
});

describe('trackWindowIdle', () => {
  it('starts active in a focused window', () => {
    stop = trackWindowIdle();
    expect(root.dataset.windowIdle).toBeUndefined();
  });

  it('marks the page idle on blur and active again on focus', () => {
    stop = trackWindowIdle();
    window.dispatchEvent(new Event('blur'));
    expect(root.dataset.windowIdle).toBe('');
    window.dispatchEvent(new Event('focus'));
    expect(root.dataset.windowIdle).toBeUndefined();
  });

  it('marks the page idle while it is hidden', () => {
    stop = trackWindowIdle();
    setHidden(true);
    expect(root.dataset.windowIdle).toBe('');
  });

  it('stops tracking once released', () => {
    stop = trackWindowIdle();
    stop();
    stop = null;
    window.dispatchEvent(new Event('blur'));
    expect(root.dataset.windowIdle).toBeUndefined();
  });
});
