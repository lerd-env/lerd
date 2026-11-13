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

  // A sentence wraps at the width it asks for; a short label keeps the window's.
  it('wraps a long label at its maxWidth', () => {
    const { btn } = mount({ label: 'Sync a site with its servers', maxWidth: 280 });
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    expect(tip()?.style.maxWidth).toBe('280px');
    btn.dispatchEvent(new MouseEvent('mouseleave'));
    const other = mount('Enable HTTPS');
    other.btn.dispatchEvent(new MouseEvent('mouseenter'));
    expect(tip()?.style.maxWidth).toBe(window.innerWidth - 12 + 'px');
  });

  // Focus handed back after a mouse click is not a request for the tooltip.
  it('shows on keyboard focus only', () => {
    const { btn } = mount('Text size');
    const visible = (v: boolean) => (btn.matches = ((s: string) => s === ':focus-visible' && v) as typeof btn.matches);
    visible(false);
    btn.dispatchEvent(new FocusEvent('focusin'));
    expect(tip()?.style.opacity ?? '0').toBe('0');
    visible(true);
    btn.dispatchEvent(new FocusEvent('focusin'));
    expect(tip()?.style.opacity).toBe('1');
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
