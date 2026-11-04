import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import PhpPage from './PhpPage.svelte';

// The version's own tabs fetch and stream; the header is all this is about.
vi.mock('./PhpDetail.svelte', async () => ({ default: (await import('../services/ServiceDetail.stub.svelte')).default }));
import { phpVersions, phpVersionsLoaded } from '$stores/phpVersions';
import { routeRest } from '$stores/route';
import { status } from '$stores/status';
import { accessMode } from '$stores/accessMode';

describe('PhpPage', () => {
  beforeEach(() => {
    globalThis.fetch = (async () => new Response('{}', { status: 200 })) as unknown as typeof fetch;
    phpVersions.set(['8.4', '8.3']);
    phpVersionsLoaded.set(true);
    status.update((s) => ({ ...s, php_fpms: [] }));
    accessMode.set({ localControl: true, local: true, lanExposed: false, checked: true });
  });

  it('hides Update all while every version is current', () => {
    render(PhpPage);
    expect(screen.queryByRole('button', { name: 'Update all' })).toBeNull();
  });

  it('opens the dialog on the versions that have an update', async () => {
    status.update((s) => ({
      ...s,
      php_fpms: [
        { version: '8.4', running: true, update_available: true },
        { version: '8.3', running: true }
      ] as never
    }));
    render(PhpPage);

    await fireEvent.click(screen.getByRole('button', { name: 'Update all' }));

    expect(screen.getByText('PHP 8.4')).toBeInTheDocument();
    expect(screen.queryByText('PHP 8.3')).toBeNull();
  });

  it('leaves Update all to the viewer with control', () => {
    accessMode.set({ localControl: false, local: false, lanExposed: true, checked: true });
    status.update((s) => ({ ...s, php_fpms: [{ version: '8.4', running: true, update_available: true }] as never }));
    render(PhpPage);
    expect(screen.queryByRole('button', { name: 'Update all' })).toBeNull();
  });

  // The page heads like every other system page: one title and its action in
  // the strip, the versions under it on the page.
  it('heads the page with the title and the install action, versions below', () => {
    const { container } = render(PhpPage);
    const header = container.querySelector('.page-header')!;
    expect(header.textContent).toContain('PHP');
    expect(header.contains(screen.getByRole('button', { name: /install php/i }))).toBe(true);
    expect(header.contains(screen.getByText('8.4'))).toBe(false);
  });

  // A notification opens #system/php-8.4 on a fresh page, before the list has
  // arrived; an empty list then is not "nothing installed".
  it('keeps a php deep link while the versions are still loading', async () => {
    phpVersions.set([]);
    phpVersionsLoaded.set(false);
    location.hash = 'system/php-8.4';
    routeRest.set('php-8.4');
    render(PhpPage, { props: { initialVersion: '8.4' } });
    await Promise.resolve();
    expect(location.hash).toBe('#system/php-8.4');
    phpVersions.set(['8.5', '8.4']);
    phpVersionsLoaded.set(true);
    await Promise.resolve();
    expect(location.hash).toBe('#system/php-8.4');
  });
});
