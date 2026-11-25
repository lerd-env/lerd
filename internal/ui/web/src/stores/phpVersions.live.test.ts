import { it, expect, vi } from 'vitest';
import { get } from 'svelte/store';

// A link or a resumed site installs a PHP version without this page asking, and
// the list was only read once, so the new version never showed.
it('reloads the installed versions when the live status gains or loses one', async () => {
  let installed = ['8.4', '8.5'];
  const fetches = vi.fn(async () => new Response(JSON.stringify(installed)));
  globalThis.fetch = fetches as unknown as typeof fetch;
  const { applyStatus } = await import('./status');
  const { phpVersions, loadPhpVersions } = await import('./phpVersions');
  const push = (versions: string[]) =>
    applyStatus({ php_fpms: versions.map((version) => ({ version, running: true })) }, () => {});

  await loadPhpVersions();
  push(['8.4', '8.5']);
  const loads = fetches.mock.calls.length;
  push(['8.5', '8.4']);
  expect(fetches.mock.calls.length).toBe(loads);

  installed = ['8.3', '8.4', '8.5'];
  push(installed);
  await vi.waitFor(() => expect(get(phpVersions)).toEqual(['8.3', '8.4', '8.5']));

  installed = ['8.4', '8.5'];
  push(installed);
  await vi.waitFor(() => expect(get(phpVersions)).toEqual(['8.4', '8.5']));
});
