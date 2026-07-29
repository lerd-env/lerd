import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import Harness from './Dropdown.test.svelte';

async function tick() {
  await new Promise((r) => setTimeout(r, 0));
}

function openMenu(): HTMLElement {
  const menu = document.querySelector('[role="listbox"]') as HTMLElement | null;
  if (!menu) throw new Error('menu did not open');
  return menu;
}

describe('Dropdown', () => {
  it('renders trigger with label + value', () => {
    render(Harness, { props: { label: 'PHP', value: '8.3', options: ['8.2', '8.3'], onchange: () => {} } });
    expect(screen.getByRole('button', { name: /PHP 8\.3/ })).toBeInTheDocument();
  });

  it('falls back to placeholder when value is empty', () => {
    render(Harness, { props: { value: '', options: ['a'], placeholder: 'Choose…', onchange: () => {} } });
    expect(screen.getByRole('button', { name: /Choose…/ })).toBeInTheDocument();
  });

  it('shows the label of an option whose value is empty', () => {
    render(Harness, {
      props: {
        value: '',
        options: [
          { value: '', label: 'main (current)' },
          { value: 'demo/gui', label: 'demo/gui' }
        ],
        placeholder: 'Choose…',
        onchange: () => {}
      }
    });
    expect(screen.getByRole('button', { name: /main \(current\)/ })).toBeInTheDocument();
  });

  it('opens menu on click and lists options', async () => {
    render(Harness, { props: { value: '8.3', options: ['8.2', '8.3', '8.4'], label: 'PHP', onchange: () => {} } });
    screen.getByRole('button', { name: /PHP 8\.3/ }).click();
    await tick();
    const items = Array.from(openMenu().querySelectorAll('[role="option"]')).map((b) => b.textContent?.trim());
    expect(items).toEqual(['PHP 8.2', 'PHP 8.3', 'PHP 8.4']);
  });

  it('supports object options with description', async () => {
    render(Harness, {
      props: {
        value: 'mysql',
        options: [
          { value: 'mysql', label: 'MySQL', description: 'Most popular' },
          { value: 'pgsql', label: 'PostgreSQL', description: 'Postgres 18' }
        ],
        onchange: () => {}
      }
    });
    screen.getByRole('button').click();
    await tick();
    const menu = openMenu();
    expect(menu.textContent).toContain('Most popular');
    expect(menu.textContent).toContain('Postgres 18');
  });

  it('fires onchange when an option is picked', async () => {
    const onchange = vi.fn();
    render(Harness, { props: { value: '8.3', options: ['8.2', '8.3', '8.4'], label: 'PHP', onchange } });
    screen.getByRole('button').click();
    await tick();
    const buttons = Array.from(openMenu().querySelectorAll('button')) as HTMLButtonElement[];
    buttons.find((b) => b.textContent?.trim() === 'PHP 8.4')!.click();
    expect(onchange).toHaveBeenCalledWith('8.4');
  });

  it('does not fire onchange when picking current value', async () => {
    const onchange = vi.fn();
    render(Harness, { props: { value: '8.3', options: ['8.2', '8.3'], label: 'PHP', onchange } });
    screen.getByRole('button').click();
    await tick();
    const buttons = Array.from(openMenu().querySelectorAll('button')) as HTMLButtonElement[];
    buttons.find((b) => b.textContent?.trim() === 'PHP 8.3')!.click();
    expect(onchange).not.toHaveBeenCalled();
  });

  it('opens via ArrowDown keydown on trigger', async () => {
    render(Harness, { props: { value: '', options: ['a', 'b', 'c'], onchange: () => {} } });
    const trigger = screen.getByRole('button');
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    await tick();
    expect(trigger.getAttribute('aria-expanded')).toBe('true');
  });

  it('Enter selects the highlighted option', async () => {
    const onchange = vi.fn();
    render(Harness, { props: { value: 'a', options: ['a', 'b', 'c'], onchange } });
    const trigger = screen.getByRole('button');
    // ArrowDown on closed menu opens + advances past the selected item ('a')
    // to the next ('b'), mirroring native <select> behaviour.
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    await tick();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    expect(onchange).toHaveBeenCalledWith('b');
  });

  it('type-ahead jumps highlight to matching prefix', async () => {
    render(Harness, { props: { value: '', options: ['alpha', 'beta', 'beta-2', 'gamma'], onchange: () => {} } });
    const trigger = screen.getByRole('button');
    trigger.click();
    await tick();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'b', bubbles: true }));
    await tick();
    const highlighted = document.querySelector('[data-highlighted="true"]');
    expect(highlighted?.textContent?.trim()).toBe('beta');
  });

  it('Escape closes the menu', async () => {
    render(Harness, { props: { value: '', options: ['a', 'b'], onchange: () => {} } });
    const trigger = screen.getByRole('button');
    trigger.click();
    await tick();
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    await tick();
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
  });

  it('disabled options are skipped by keyboard nav', async () => {
    const onchange = vi.fn();
    render(Harness, {
      props: {
        value: 'a',
        options: [
          { value: 'a', label: 'A' },
          { value: 'b', label: 'B', disabled: true },
          { value: 'c', label: 'C' }
        ],
        onchange
      }
    });
    const trigger = screen.getByRole('button');
    // From 'a', ArrowDown should land on 'C' (skipping disabled 'B').
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    await tick();
    const highlighted = document.querySelector('[data-highlighted="true"]');
    expect(highlighted?.textContent?.trim()).toBe('C');
  });

  it('exposes correct ARIA roles', () => {
    render(Harness, { props: { value: 'a', options: ['a', 'b'], onchange: () => {} } });
    const trigger = screen.getByRole('button');
    expect(trigger.getAttribute('aria-haspopup')).toBe('listbox');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
  });

  it('marks the current value with aria-selected and red text class', async () => {
    render(Harness, { props: { value: 'b', options: ['a', 'b', 'c'], onchange: () => {} } });
    screen.getByRole('button').click();
    await tick();
    const selected = document.querySelector('[role="option"][aria-selected="true"]');
    expect(selected?.textContent?.trim()).toBe('b');
    expect(selected?.className).toMatch(/text-lerd-red/);
  });

  it('renders inherited dashed-violet border when inherited prop set', () => {
    render(Harness, { props: { value: '8.3', options: ['8.3'], inherited: true, onchange: () => {} } });
    expect(screen.getByRole('button').className).toMatch(/border-dashed/);
  });

  it('full width fills container when width="full"', () => {
    render(Harness, { props: { value: 'a', options: ['a'], width: 'full', onchange: () => {} } });
    expect(screen.getByRole('button').className).toMatch(/w-full/);
  });
});
