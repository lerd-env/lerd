import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import MobileHeader from './MobileHeader.svelte';
import { version } from '$stores/version';

describe('MobileHeader', () => {
  it('keeps the bell beside the theme switcher on the right', () => {
    const { container } = render(MobileHeader);
    // justify-between spreads direct children, so a third one lands mid-bar.
    const bar = container.firstElementChild!;
    expect(bar.children).toHaveLength(2);
    expect(bar.lastElementChild!.querySelectorAll('button').length).toBeGreaterThanOrEqual(2);
  });

  // Same badge as the rail, so a dev build does not print its describe string here either.
  it('shows the release and a dev badge for a dev build', () => {
    version.set({ current: '1.35.0-48-ga75062ab-dirty', latest: '', hasUpdate: false, checked: true, checking: false, changelog: '' });
    render(MobileHeader);
    expect(screen.getByText('v1.35.0')).toBeTruthy();
    expect(screen.getByRole('button', { name: /a75062ab/ }).textContent?.trim()).toBe('dev');
  });
});
