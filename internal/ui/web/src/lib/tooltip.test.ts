import { describe, it, expect, afterEach } from 'vitest';
import { tooltip } from './tooltip';

function mount(param: Parameters<typeof tooltip>[1]) {
  const btn = document.createElement('button');
  document.body.appendChild(btn);
  const handle = tooltip(btn, param);
  return { btn, handle };
}

function tip() {
  return document.querySelector('[role="tooltip"]') as HTMLElement | null;
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('tooltip action', () => {
  it('reveals a body-level tooltip with the label on hover', () => {
    const { btn } = mount('Enable HTTPS');
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    const t = tip();
    expect(t).not.toBeNull();
    expect(t?.parentElement).toBe(document.body);
    expect(t?.textContent).toContain('Enable HTTPS');
    expect(t?.style.opacity).toBe('1');
  });

  // A label wider than the window, such as a long branch name, wraps instead of running off it.
  it('never grows wider than the window', () => {
    const { btn } = mount('feature/' + 'x'.repeat(400));
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    const t = tip()!;
    expect(t.style.maxWidth).toBe(window.innerWidth - 12 + 'px');
    expect(t.style.overflowWrap).toBe('anywhere');
    expect(t.className).not.toContain('whitespace-nowrap');
  });

  it('hides on mouseleave', () => {
    const { btn } = mount('Reload');
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    btn.dispatchEvent(new MouseEvent('mouseleave'));
    expect(tip()?.style.opacity).toBe('0');
  });

  it('accepts a string or an options object', () => {
    const { btn } = mount({ label: 'Sites', placement: 'right' });
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    expect(tip()?.textContent).toContain('Sites');
  });

  it('shows nothing when the label is empty', () => {
    const { btn } = mount('');
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    expect(tip()?.style.opacity ?? '0').toBe('0');
  });
});
