import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import PackageSuggestions from './PackageSuggestions.svelte';
import { accessMode } from '$stores/accessMode';

const calls: string[] = [];

beforeEach(() => {
  calls.length = 0;
  location.hash = '';
  accessMode.update((a) => ({ ...a, localControl: true }));
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      calls.push(String(url));
      const packages = calls.some((c) => c.includes('/dismiss'))
        ? []
        : [{ name: 'lerd/debug', dev: true, reason: 'your own timeline rows', docs: 'https://lerd.sh/features/debug-package/' }];
      return new Response(JSON.stringify({ ok: true, packages }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    })
  );
});

afterEach(() => vi.unstubAllGlobals());

describe('PackageSuggestions', () => {
  it('offers a missing package with its reason', async () => {
    render(PackageSuggestions, { props: { domain: 'shop.test' } });
    expect(await screen.findByText('lerd/debug')).toBeInTheDocument();
    expect(screen.getByText('your own timeline rows')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Install' })).toBeInTheDocument();
  });

  it('opens lerd.sh docs in the built-in viewer', async () => {
    render(PackageSuggestions, { props: { domain: 'shop.test' } });
    await fireEvent.click(await screen.findByText('Docs'));
    expect(location.hash).toBe('#docs/features/debug-package');
  });

  it('stops offering a dismissed package', async () => {
    render(PackageSuggestions, { props: { domain: 'shop.test' } });
    await fireEvent.click(await screen.findByRole('button', { name: 'Don’t suggest again' }));
    await vi.waitFor(() => expect(screen.queryByText('lerd/debug')).not.toBeInTheDocument());
    expect(calls.some((c) => c.includes('/api/sites/shop.test/packages/dismiss?name=lerd%2Fdebug'))).toBe(true);
  });
});
