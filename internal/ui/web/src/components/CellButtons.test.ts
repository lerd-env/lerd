import { render, fireEvent, screen } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import CellButtons, { type CellAction } from './CellButtons.svelte';

function setup(over: Partial<CellAction> = {}) {
  const add = vi.fn();
  const actions: CellAction[] = [
    { icon: 'plus', label: 'Add', onclick: add, ...over },
    { icon: 'sort', label: 'Sort', onclick: vi.fn(), menu: true }
  ];
  const { container } = render(CellButtons, { props: { actions } });
  const cells = container.querySelector('[data-expanded]') as HTMLElement;
  return { add, cells, dots: container.querySelector('[data-cell-dots]') as HTMLElement };
}

const pointer = (el: Element | Document, type: string, pointerType: string) => {
  const e = new Event(type, { bubbles: type === 'pointerdown' });
  Object.defineProperty(e, 'pointerType', { value: pointerType });
  return fireEvent(el, e);
};

describe('CellButtons', () => {
  it('starts merged behind the dots', () => {
    const { cells, dots } = setup();
    expect(cells.dataset.expanded).toBe('false');
    expect(dots).toBeTruthy();
  });

  it('splits under the mouse and merges when it leaves', async () => {
    const { cells } = setup();
    await pointer(cells, 'pointerenter', 'mouse');
    expect(cells.dataset.expanded).toBe('true');
    await pointer(cells, 'pointerleave', 'mouse');
    expect(cells.dataset.expanded).toBe('false');
  });

  // A finger fires pointerenter just before its click; splitting on it would
  // put a real button under the tap that was meant for the dots.
  it('splits on a tap, not on the touch that precedes it, and merges on a tap outside', async () => {
    const { cells, dots, add } = setup();
    await pointer(cells, 'pointerenter', 'touch');
    expect(cells.dataset.expanded).toBe('false');
    await fireEvent.click(dots);
    expect(cells.dataset.expanded).toBe('true');
    expect(add).not.toHaveBeenCalled();
    await pointer(document, 'pointerdown', 'touch');
    expect(cells.dataset.expanded).toBe('false');
  });

  it('splits for keyboard focus and merges once focus moves on', async () => {
    const { cells } = setup();
    screen.getByRole('button', { name: 'Add' }).focus();
    await Promise.resolve();
    expect(cells.dataset.expanded).toBe('true');
    screen.getByRole('button', { name: 'Add' }).blur();
    await Promise.resolve();
    expect(cells.dataset.expanded).toBe('false');
  });

  it('stays split while an action has something open', () => {
    expect(setup({ open: true }).cells.dataset.expanded).toBe('true');
  });

  it('runs the action', async () => {
    const { add } = setup();
    await fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    expect(add).toHaveBeenCalledOnce();
  });

  it('never merges a lone action', () => {
    const { container } = render(CellButtons, {
      props: { actions: [{ icon: 'sort', label: 'Sort', onclick: vi.fn() }] }
    });
    expect(container.querySelector('[data-cell-dots]')).toBeNull();
    expect((container.querySelector('[data-expanded]') as HTMLElement).dataset.expanded).toBe('true');
  });

  it('names its actions with the shared tooltip, not the browser one', () => {
    setup();
    const btn = screen.getByRole('button', { name: 'Sort' });
    expect(btn).not.toHaveAttribute('title');
    btn.dispatchEvent(new MouseEvent('mouseenter'));
    expect(document.querySelector('[role="tooltip"]')).toHaveTextContent('Sort');
  });
});
