import { describe, it, expect, vi } from 'vitest';
import { scrollbarInPadding } from './scrollbarInPadding';

function box(o: { offsetW?: number; clientW?: number; offsetH?: number; clientH?: number; style?: Partial<CSSStyleDeclaration> }) {
  const el = document.createElement('div');
  Object.assign(el.style, o.style ?? {});
  document.body.appendChild(el);
  Object.defineProperty(el, 'offsetWidth', { value: o.offsetW ?? 0, configurable: true });
  Object.defineProperty(el, 'clientWidth', { value: o.clientW ?? 0, configurable: true });
  Object.defineProperty(el, 'offsetHeight', { value: o.offsetH ?? 0, configurable: true });
  Object.defineProperty(el, 'clientHeight', { value: o.clientH ?? 0, configurable: true });
  return el;
}

describe('scrollbarInPadding', () => {
  // A classic 10px bar comes out of the right padding, so the content keeps its place.
  it('takes a vertical bar out of the right padding', () => {
    const el = box({ offsetW: 400, clientW: 390, style: { paddingRight: '16px' } });
    scrollbarInPadding(el, 'y');
    expect(el.style.scrollbarGutter).toBe('stable');
    expect(el.style.paddingRight).toBe('6px');
  });

  // The sideways bar sits in the gap below the row instead of growing it.
  it('gives a horizontal bar and its padding back to the gap below', () => {
    const el = box({ offsetH: 60, clientH: 50, style: { paddingBottom: '6px' } });
    scrollbarInPadding(el, 'x');
    expect(el.style.overflowX).toBe('scroll');
    expect(el.style.marginBottom).toBe('-16px');
  });

  // macOS overlay bars take no room, so nothing is taken back.
  it('leaves spacing alone when the bar floats', () => {
    const el = box({ offsetW: 400, clientW: 400, style: { paddingRight: '16px' } });
    scrollbarInPadding(el, 'y');
    expect(el.style.paddingRight).toBe('16px');
  });

  // A pane mounted while hidden measures a 0px bar; it must correct itself once
  // shown, starting again from its own padding rather than the adjusted one.
  it('re-measures from the original padding when the area resizes', () => {
    let fire = () => {};
    vi.stubGlobal('ResizeObserver', class {
      constructor(cb: () => void) { fire = cb; }
      observe() {}
      disconnect() {}
    });
    let clientW = 400;
    const el = box({ offsetW: 400, style: { paddingRight: '16px' } });
    Object.defineProperty(el, 'clientWidth', { get: () => clientW });
    scrollbarInPadding(el, 'y');
    expect(el.style.paddingRight).toBe('16px');

    clientW = 390;
    fire();
    expect(el.style.paddingRight).toBe('6px');
    fire();
    expect(el.style.paddingRight).toBe('6px');
    vi.unstubAllGlobals();
  });
});
