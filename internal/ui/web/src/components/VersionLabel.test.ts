import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import VersionLabel from './VersionLabel.svelte';
import { version } from '$stores/version';

function setVersion(current: string) {
  version.set({ current, latest: '', hasUpdate: false, checked: true, checking: false, changelog: '' });
}

describe('VersionLabel', () => {
  let writeText: ReturnType<typeof vi.fn>;
  beforeEach(() => {
    writeText = vi.fn(async () => {});
    Object.assign(navigator, { clipboard: { writeText } });
  });

  it('prints a release version as is', () => {
    setVersion('1.35.0');
    render(VersionLabel);
    expect(screen.getByText('v1.35.0')).toBeTruthy();
  });

  // The full describe string wraps into four lines in the narrow rail.
  it('shows a dev badge and copies the commit on click', async () => {
    setVersion('1.35.0-48-ga75062ab-dirty');
    render(VersionLabel);
    const badge = screen.getByRole('button', { name: /a75062ab/ });
    expect(badge.textContent?.trim()).toBe('dev');
    expect(screen.getByText('v1.35.0')).toBeTruthy();
    badge.click();
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith('a75062ab'));
  });

  // A beta carries nothing worth copying, so its badge is just a label.
  it('shows a plain beta badge', () => {
    setVersion('1.35.0-beta.4');
    render(VersionLabel);
    expect(screen.getByText('beta')).toBeTruthy();
    expect(screen.getByText('v1.35.0')).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
  });
});
