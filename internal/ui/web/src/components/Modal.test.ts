import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import Harness from './Modal.test.svelte';

describe('Modal', () => {
  it('renders title and body when open', () => {
    render(Harness, { props: { open: true, title: 'Hello', onclose: () => {} } });
    expect(screen.getByText('Hello')).toBeInTheDocument();
    expect(screen.getByTestId('body')).toBeInTheDocument();
  });

  // An editor sizes itself from its box; a scrolling wrapper would leave it no height.
  it('lets a tall body fill the panel', () => {
    const { unmount } = render(Harness, { props: { open: true, title: 'X', onclose: () => {}, tall: true } });
    expect(screen.getByTestId('body').parentElement!.className).toContain('flex-1');
    unmount();
    render(Harness, { props: { open: true, title: 'X', onclose: () => {} } });
    expect(screen.getByTestId('body').parentElement!.className).toContain('overflow-y-auto');
  });

  it('renders nothing when closed', () => {
    render(Harness, { props: { open: false, title: 'Hello', onclose: () => {} } });
    expect(screen.queryByText('Hello')).not.toBeInTheDocument();
  });

  it('invokes onclose when the backdrop is clicked', () => {
    const onclose = vi.fn();
    const { container } = render(Harness, { props: { open: true, title: 'X', onclose } });
    const backdrop = container.querySelector('button.absolute.inset-0') as HTMLButtonElement;
    backdrop.click();
    expect(onclose).toHaveBeenCalledOnce();
  });

  it('invokes onclose when Escape is pressed', () => {
    const onclose = vi.fn();
    render(Harness, { props: { open: true, title: 'X', onclose } });
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(onclose).toHaveBeenCalledOnce();
  });

  // Escape closes the innermost open thing: an open dropdown first, the
  // dialog only on the next press.
  it('leaves itself open when Escape only closes a dropdown inside it', async () => {
    const onclose = vi.fn();
    render(Harness, { props: { open: true, title: 'T', onclose, withDropdown: true } });
    await fireEvent.click(screen.getByRole('button', { name: /Alpha/ }));
    expect(screen.getByRole('listbox')).toBeTruthy();
    await fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).toBeNull();
    expect(onclose).not.toHaveBeenCalled();
    await fireEvent.keyDown(document, { key: 'Escape' });
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it('leaves itself open when Escape only closes a popover inside it', async () => {
    const onclose = vi.fn();
    render(Harness, { props: { open: true, title: 'T', onclose, withPopover: true } });
    await fireEvent.click(screen.getByRole('button', { name: 'Open' }));
    await fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByText('panel item')).toBeNull();
    expect(onclose).not.toHaveBeenCalled();
    await fireEvent.keyDown(document, { key: 'Escape' });
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  // The panel lives on <body>, outside the dialog's own focus trap.
  it('takes keyboard focus into a popover inside it and gives it back on close', async () => {
    render(Harness, { props: { open: true, title: 'T', onclose: () => {}, withPopover: true } });
    const trigger = screen.getByRole('button', { name: 'Open' });
    trigger.focus();
    await fireEvent.click(trigger);
    const item = screen.getByRole('button', { name: 'panel item' });
    expect(document.activeElement).toBe(item);
    await fireEvent.keyDown(item, { key: 'Tab' });
    expect(document.activeElement).toBe(item);
    await fireEvent.keyDown(item, { key: 'Escape' });
    expect(document.activeElement).toBe(trigger);
  });

  it('renders footer slot when provided', () => {
    render(Harness, { props: { open: true, title: 'X', onclose: () => {}, withFooter: true } });
    expect(screen.getByTestId('footer')).toBeInTheDocument();
  });

  it('renders header actions beside the title when provided', () => {
    render(Harness, { props: { open: true, title: 'X', onclose: () => {}, withActions: true } });
    expect(screen.getByText('X').parentElement?.contains(screen.getByTestId('actions'))).toBe(true);
  });

  it('applies the xl width class for the large editor modal', () => {
    const { container } = render(Harness, {
      props: { open: true, title: 'X', onclose: () => {}, size: 'xl' }
    });
    expect(container.querySelector('.max-w-5xl')).toBeInTheDocument();
  });

  // A long snapshot list ran past the bottom of the window with no way to reach
  // the rest. The panel stops at the viewport and its body scrolls instead.
  it('caps the panel at the viewport and scrolls the body', () => {
    render(Harness, { props: { open: true, title: 'X', onclose: () => {}, withFooter: true } });
    // jsdom lays nothing out, so the classes that do the work are pinned: the
    // column lets the body shrink below its content, and the ends never do.
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveClass('flex', 'flex-col', 'max-h-[calc(100dvh-2rem)]');
    const scroller = screen.getByTestId('body').parentElement!;
    expect(scroller).toHaveClass('min-h-0', 'overflow-y-auto');
    const header = screen.getByText('X').parentElement!;
    const footer = screen.getByTestId('footer').parentElement!;
    expect(header).toHaveClass('shrink-0');
    expect(footer).toHaveClass('shrink-0');
    expect(scroller.contains(header)).toBe(false);
    expect(scroller.contains(footer)).toBe(false);
  });

  it('defaults to the md width class', () => {
    const { container } = render(Harness, { props: { open: true, title: 'X', onclose: () => {} } });
    expect(container.querySelector('.max-w-lg')).toBeInTheDocument();
  });

  // A dump filename is one unbreakable word. Without these the title runs past
  // the header and under the close button.
  it('breaks a long unspaced title instead of overflowing the header', () => {
    const { container } = render(Harness, {
      props: {
        open: true,
        title: 'main_ep_floral_union_a2140s70_aws_eu_central_1_pg_laravel_cloud-2026_07_24-dump.sql',
        onclose: () => {}
      }
    });
    const heading = container.querySelector('h3') as HTMLElement;
    expect(heading.className).toContain('min-w-0');
    expect(heading.className).toContain('break-words');
  });

  it('keeps the close button at full size next to a long title', () => {
    const { container } = render(Harness, { props: { open: true, title: 'X', onclose: () => {} } });
    const close = container.querySelector('h3 + button') as HTMLElement;
    expect(close.className).toContain('shrink-0');
  });

  it('is announced as a dialog named by its title', () => {
    render(Harness, { props: { open: true, title: 'Manage domains', onclose: () => {} } });
    const panel = screen.getByRole('dialog', { name: 'Manage domains' });
    expect(panel).toHaveAttribute('aria-modal', 'true');
  });

  it('takes focus on open and hands it back on close', async () => {
    const trigger = document.createElement('button');
    document.body.appendChild(trigger);
    trigger.focus();
    const { rerender } = render(Harness, { props: { open: true, title: 'X', onclose: () => {} } });
    expect(screen.getByRole('dialog').contains(document.activeElement)).toBe(true);
    await rerender({ open: false, title: 'X', onclose: () => {} });
    expect(document.activeElement).toBe(trigger);
    trigger.remove();
  });
});
