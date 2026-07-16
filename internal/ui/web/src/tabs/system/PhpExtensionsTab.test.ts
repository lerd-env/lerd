import { render, screen, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import PhpExtensionsTab from './PhpExtensionsTab.svelte';

const { fetchPhpExtensions } = vi.hoisted(() => ({ fetchPhpExtensions: vi.fn() }));

vi.mock('$stores/phpVersions', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, fetchPhpExtensions };
});

function report(over: Record<string, unknown> = {}) {
  return {
    ok: true,
    report: {
      version: '8.4',
      built: true,
      needs_rebuild: false,
      extensions: { declared: [], has: null, cannot: null },
      packages: { declared: [], has: null, cannot: null },
      modules: [],
      ...over
    }
  };
}

describe('PhpExtensionsTab', () => {
  beforeEach(() => vi.resetAllMocks());

  it('lists the modules the image actually loads', async () => {
    fetchPhpExtensions.mockResolvedValue(report({ modules: ['bcmath', 'mongodb', 'opcache'] }));

    render(PhpExtensionsTab, { props: { version: '8.4' } });

    await waitFor(() => expect(screen.getByText('mongodb')).toBeTruthy());
    expect(screen.getByText('bcmath')).toBeTruthy();
    expect(screen.getByText('opcache')).toBeTruthy();
  });

  // The whole point: a declared entry this image did not load must not be shown
  // as present. It is rendered, but distinguished from what the image has.
  it('distinguishes a declared entry the image could not load', async () => {
    fetchPhpExtensions.mockResolvedValue(
      report({
        extensions: { declared: ['mongodb', 'swoole'], has: ['swoole'], cannot: ['mongodb'] },
        modules: ['swoole']
      })
    );

    render(PhpExtensionsTab, { props: { version: '7.4' } });

    await waitFor(() => expect(screen.getByTitle(/did not build/i)).toBeTruthy());
    // swoole is present, so it carries no "did not build" caveat.
    expect(screen.getByTitle(/did not build/i).textContent).toContain('mongodb');
  });

  it('tells the user to rebuild an image that predates the declared set', async () => {
    fetchPhpExtensions.mockResolvedValue(
      report({ needs_rebuild: true, extensions: { declared: ['mongodb'], has: null, cannot: null } })
    );

    render(PhpExtensionsTab, { props: { version: '8.1' } });

    await waitFor(() => expect(screen.getByText(/predates/i)).toBeTruthy());
  });

  it('claims nothing about a version with no image', async () => {
    fetchPhpExtensions.mockResolvedValue(report({ built: false, modules: [] }));

    render(PhpExtensionsTab, { props: { version: '8.2' } });

    await waitFor(() => expect(screen.getByText(/has no image yet/i)).toBeTruthy());
  });
});
