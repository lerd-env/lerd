import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import SystemHealthWidget from './SystemHealthWidget.svelte';
import { status, type ToolStatus } from '$stores/status';
import { accessMode } from '$stores/accessMode';
import { get } from 'svelte/store';

const tool = (over: Partial<ToolStatus>): ToolStatus => ({
  name: 'fnm',
  installed: 'v1.4.4',
  pinned: 'v1.4.4',
  present: true,
  update_available: false,
  ...over
});

const pending = tool({ name: 'composer', installed: '2.10.2', pinned: '2.10.3', update_available: true });

describe('SystemHealthWidget updates', () => {
  beforeEach(() => {
    status.set({ ...get(status), tools: [tool({})] });
    accessMode.set({ localControl: true, local: true, lanExposed: false, checked: true });
  });

  it('stays quiet while every tool is current', () => {
    render(SystemHealthWidget);
    expect(screen.queryByRole('button', { name: /Tools · / })).toBeNull();
  });

  it('counts the tools with an update and opens the update dialog', async () => {
    status.set({ ...get(status), tools: [tool({}), pending] });
    render(SystemHealthWidget);

    await fireEvent.click(screen.getByRole('button', { name: 'Tools · 1 update' }));

    expect(screen.getByText('2.10.2 → 2.10.3')).toBeInTheDocument();
  });

  it('tells a remote viewer about the update without offering to run it', () => {
    accessMode.set({ localControl: false, local: false, lanExposed: true, checked: true });
    status.set({ ...get(status), tools: [pending] });
    render(SystemHealthWidget);

    expect(screen.getByText('Tools · 1 update')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Tools · 1 update' })).toBeNull();
  });

  it('stays quiet while every PHP version is current', () => {
    status.set({ ...get(status), php_fpms: [{ version: '8.4', running: true }] as never });
    render(SystemHealthWidget);
    expect(screen.queryByText(/PHP · /)).toBeNull();
  });

  it('counts the PHP versions with an update and opens the update dialog', async () => {
    status.set({
      ...get(status),
      php_fpms: [
        { version: '8.3', running: true, update_available: true },
        { version: '8.4', running: true, update_available: true },
        { version: '8.5', running: true }
      ] as never
    });
    render(SystemHealthWidget);

    await fireEvent.click(screen.getByRole('button', { name: 'PHP · 2 updates' }));

    expect(screen.getByText('PHP 8.3')).toBeInTheDocument();
    expect(screen.queryByText('PHP 8.5')).toBeNull();
  });

  it('marks the version chip that has an update', () => {
    status.set({
      ...get(status),
      php_fpms: [
        { version: '8.3', running: true, update_available: true },
        { version: '8.5', running: true }
      ] as never
    });
    render(SystemHealthWidget);
    expect(screen.getAllByLabelText('Base image updated')).toHaveLength(1);
  });
});
