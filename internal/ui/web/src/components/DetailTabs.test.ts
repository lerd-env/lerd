import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import Harness from './DetailTabs.test.svelte';

describe('DetailTabs', () => {
  it('renders visible tabs and omits hidden ones', () => {
    render(Harness, {
      props: {
        active: 'a',
        tabs: [
          { id: 'a', label: 'A' },
          { id: 'b', label: 'B', hidden: true },
          { id: 'c', label: 'C' }
        ],
        onchange: () => {}
      }
    });
    expect(screen.getByText('A')).toBeInTheDocument();
    expect(screen.getByText('C')).toBeInTheDocument();
    expect(screen.queryByText('B')).not.toBeInTheDocument();
  });

  it('draws a divider only where the group changes', () => {
    const { container } = render(Harness, {
      props: {
        active: 'a',
        tabs: [
          { id: 'a', label: 'A', group: 'one' },
          { id: 'b', label: 'B', group: 'two' },
          { id: 'h', label: 'H', group: 'three', hidden: true },
          { id: 'c', label: 'C', group: 'two' },
          { id: 'd', label: 'D', group: 'four' }
        ],
        onchange: () => {}
      }
    });
    expect(container.querySelectorAll('[data-tab-divider]').length).toBe(2);
  });

  it('hides the bar when only one tab is visible', () => {
    render(Harness, {
      props: {
        active: 'a',
        tabs: [
          { id: 'a', label: 'Solo' },
          { id: 'b', label: 'Hidden', hidden: true }
        ],
        onchange: () => {}
      }
    });
    expect(screen.queryByText('Solo')).not.toBeInTheDocument();
  });

  it('renders the actions snippet alongside the tabs', () => {
    render(Harness, {
      props: {
        active: 'a',
        tabs: [{ id: 'a', label: 'A' }, { id: 'b', label: 'B' }],
        onchange: () => {},
        withActions: true
      }
    });
    expect(screen.getByText('Toggle')).toBeInTheDocument();
  });

  it('keeps the bar when a single tab is visible but actions are given', () => {
    render(Harness, {
      props: {
        active: 'a',
        tabs: [{ id: 'a', label: 'Solo' }],
        onchange: () => {},
        withActions: true
      }
    });
    expect(screen.queryByText('Solo')).not.toBeInTheDocument();
    expect(screen.getByText('Toggle')).toBeInTheDocument();
  });

  it('marks the active tab with the accent border', () => {
    render(Harness, {
      props: { active: 'b', tabs: [{ id: 'a', label: 'A' }, { id: 'b', label: 'B' }], onchange: () => {} }
    });
    expect(screen.getByText('B').className).toMatch(/border-lerd-red|text-lerd-red/);
    expect(screen.getByText('A').className).not.toMatch(/border-lerd-red/);
  });

  it('emits onchange with the clicked id', () => {
    const onchange = vi.fn();
    render(Harness, {
      props: { active: 'a', tabs: [{ id: 'a', label: 'A' }, { id: 'b', label: 'B' }], onchange }
    });
    screen.getByText('B').click();
    expect(onchange).toHaveBeenCalledWith('b');
  });

  describe('as a tab list', () => {
    const tabs = [
      { id: 'a', label: 'Overview' },
      { id: 'b', label: 'Logs' },
      { id: 'c', label: 'Env' }
    ];

    it('marks the active tab selected and keeps only it in the Tab order', () => {
      render(Harness, { props: { active: 'b', tabs, onchange: () => {} } });
      expect(screen.getByRole('tablist')).toBeInTheDocument();
      const logs = screen.getByRole('tab', { name: 'Logs' });
      expect(logs).toHaveAttribute('aria-selected', 'true');
      expect(logs).toHaveAttribute('tabindex', '0');
      expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('tabindex', '-1');
    });

    it('moves with the arrow keys, wrapping at the ends', async () => {
      const onchange = vi.fn();
      render(Harness, { props: { active: 'c', tabs, onchange } });
      await fireEvent.keyDown(screen.getByRole('tab', { name: 'Env' }), { key: 'ArrowRight' });
      expect(onchange).toHaveBeenLastCalledWith('a');
      await fireEvent.keyDown(screen.getByRole('tab', { name: 'Env' }), { key: 'ArrowLeft' });
      expect(onchange).toHaveBeenLastCalledWith('b');
    });

    it('jumps to the first and last tab with Home and End', async () => {
      const onchange = vi.fn();
      render(Harness, { props: { active: 'b', tabs, onchange } });
      await fireEvent.keyDown(screen.getByRole('tab', { name: 'Logs' }), { key: 'End' });
      expect(onchange).toHaveBeenLastCalledWith('c');
      await fireEvent.keyDown(screen.getByRole('tab', { name: 'Logs' }), { key: 'Home' });
      expect(onchange).toHaveBeenLastCalledWith('a');
    });
  });
});
