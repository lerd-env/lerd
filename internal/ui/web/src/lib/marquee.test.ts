import { describe, it, expect, afterEach } from 'vitest';
import { marquee } from './marquee';

// jsdom has no layout, so the widths a browser would measure are set by hand.
function mount(scrollWidth: number, clientWidth: number) {
  const host = document.createElement('button');
  const text = document.createElement('span');
  host.appendChild(text);
  document.body.appendChild(host);
  Object.defineProperty(text, 'scrollWidth', { value: scrollWidth });
  Object.defineProperty(text, 'clientWidth', { value: clientWidth });
  const handle = marquee(text);
  return { host, text, handle };
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('marquee action', () => {
  it('scrolls a clipped text by its overflow while its parent is hovered', () => {
    const { host, text } = mount(300, 180);
    host.dispatchEvent(new MouseEvent('mouseenter'));
    expect(text.classList.contains('lerd-marquee')).toBe(true);
    expect(text.style.getPropertyValue('--marquee-shift')).toBe('-120px');
  });

  it('stops and resets on mouseleave', () => {
    const { host, text } = mount(300, 180);
    host.dispatchEvent(new MouseEvent('mouseenter'));
    host.dispatchEvent(new MouseEvent('mouseleave'));
    expect(text.classList.contains('lerd-marquee')).toBe(false);
  });

  it('leaves a text that fits alone', () => {
    const { host, text } = mount(120, 180);
    host.dispatchEvent(new MouseEvent('mouseenter'));
    expect(text.classList.contains('lerd-marquee')).toBe(false);
  });

  it('stops listening once destroyed', () => {
    const { host, text, handle } = mount(300, 180);
    handle.destroy();
    host.dispatchEvent(new MouseEvent('mouseenter'));
    expect(text.classList.contains('lerd-marquee')).toBe(false);
  });
});
