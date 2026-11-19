import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, afterEach } from 'vitest';
import SettingToggle from './SettingToggle.svelte';
import { accessMode } from '$stores/accessMode';

const props = { titleOn: 'Turn it on', titleOff: 'Turn it off' };

describe('SettingToggle', () => {
  afterEach(() => accessMode.update((a) => ({ ...a, localControl: true })));

  it('offers the switch with the action it will take', async () => {
    accessMode.update((a) => ({ ...a, localControl: true }));
    const onclick = vi.fn();
    render(SettingToggle, { props: { ...props, on: false, onclick } });
    const btn = screen.getByTitle('Turn it on');
    await fireEvent.click(btn);
    expect(onclick).toHaveBeenCalledOnce();
  });

  it('shows a read-only state where this browser may not change it', () => {
    accessMode.update((a) => ({ ...a, localControl: false }));
    render(SettingToggle, { props: { ...props, on: true, onclick: vi.fn() } });
    expect(screen.queryByTitle('Turn it off')).toBeNull();
    expect(screen.getByText(/enabled/i)).toBeTruthy();
  });
});
