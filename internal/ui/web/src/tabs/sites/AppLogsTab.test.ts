import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { flushSync } from 'svelte';
import Harness from './AppLogsTab.test.svelte';
import SiteHarness from './AppLogsTabSite.test.svelte';
import type { Site } from '$stores/sites';

function siteWith(extra: Partial<Site> = {}): Site {
  return {
    name: 'whitewaters',
    domain: 'theregistry.test',
    branch: 'main',
    ...extra
  } as Site;
}

describe('AppLogsTab', () => {
  const realFetch = globalThis.fetch;
  let calls: string[];

  beforeEach(() => {
    calls = [];
    globalThis.fetch = vi.fn(async (url: string) => {
      calls.push(url);
      return new Response(JSON.stringify({ files: [], entries: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      });
    }) as unknown as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it('re-fetches the file list when branch prop changes', async () => {
    const { rerender } = render(Harness, {
      props: { site: siteWith(), branch: '' }
    });

    // Allow the initial $effect to fire and the awaited fetch to resolve.
    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    await Promise.resolve();

    const parentCalls = calls.filter((u) => u.startsWith('/api/app-logs/theregistry.test'));
    expect(parentCalls.length).toBeGreaterThan(0);
    expect(parentCalls.some((u) => u.includes('branch='))).toBe(false);

    // Now switch to a worktree branch. The effect must re-fire so the
    // dropdown is scoped to the worktree path, not the parent's.
    calls.length = 0;
    await rerender({ site: siteWith(), branch: 'main' });
    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    await Promise.resolve();

    const wtCalls = calls.filter((u) => u.startsWith('/api/app-logs/theregistry.test'));
    expect(wtCalls.length).toBeGreaterThan(0);
    expect(wtCalls.some((u) => /[?&]branch=main(&|$)/.test(u))).toBe(true);
  });

  // Every snapshot replaces the site objects, so an effect keyed on the object
  // rather than on the domain reloads the list under the user and puts a
  // scrolled log back at the top.
  it('does not re-fetch when the store hands it a new site object', async () => {
    const { rerender } = render(SiteHarness, { props: { site: siteWith() } });

    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    await Promise.resolve();
    expect(calls.length).toBeGreaterThan(0);

    calls.length = 0;
    await rerender({ site: siteWith() });
    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    await Promise.resolve();

    expect(calls).toEqual([]);
  });

  it('re-fetches when switching back from worktree to parent', async () => {
    const { rerender } = render(Harness, {
      props: { site: siteWith(), branch: 'feat-x' }
    });

    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    await Promise.resolve();

    calls.length = 0;
    await rerender({ site: siteWith(), branch: '' });
    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    await Promise.resolve();

    const parentCalls = calls.filter((u) => u.startsWith('/api/app-logs/theregistry.test'));
    expect(parentCalls.length).toBeGreaterThan(0);
    expect(parentCalls.every((u) => !u.includes('branch='))).toBe(true);
  });

  // The tab has no stream of its own: it used to refresh only because every
  // websocket snapshot invalidated its effect. It polls on its own now.
  describe('polling', () => {
    function serveOneFile() {
      globalThis.fetch = vi.fn(async (url: string) => {
        calls.push(url);
        const body = url.includes('/laravel.log') ? { entries: [] } : { files: [{ name: 'laravel.log', size: 10 }] };
        return new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' }
        });
      }) as unknown as typeof fetch;
    }
    const entryCalls = () => calls.filter((u) => u.includes('/laravel.log')).length;

    beforeEach(() => {
      serveOneFile();
      vi.useFakeTimers();
    });
    afterEach(() => {
      vi.useRealTimers();
    });

    it('re-fetches the entries every 5 seconds', async () => {
      render(SiteHarness, { props: { site: siteWith() } });
      await vi.advanceTimersByTimeAsync(0);
      const initial = entryCalls();

      await vi.advanceTimersByTimeAsync(5000);
      expect(entryCalls()).toBe(initial + 1);
      await vi.advanceTimersByTimeAsync(5000);
      expect(entryCalls()).toBe(initial + 2);
    });

    it('does not poll a suspended site', async () => {
      render(SiteHarness, { props: { site: siteWith({ idle_suspended: true }) } });
      await vi.advanceTimersByTimeAsync(0);
      const initial = entryCalls();

      await vi.advanceTimersByTimeAsync(15000);
      expect(entryCalls()).toBe(initial);
    });

    it('stops polling once the tab is closed', async () => {
      const { unmount } = render(SiteHarness, { props: { site: siteWith() } });
      await vi.advanceTimersByTimeAsync(0);

      unmount();
      const afterClose = entryCalls();
      await vi.advanceTimersByTimeAsync(15000);
      expect(entryCalls()).toBe(afterClose);
    });
  });

  it('clears logs only after the confirmation modal is confirmed', async () => {
    const methodCalls: string[] = [];
    globalThis.fetch = vi.fn(async (url: string, init?: RequestInit) => {
      methodCalls.push((init?.method || 'GET') + ' ' + url);
      const seg = url.match(/\/api\/app-logs\/[^/?]+(?:\/([^?]+))?/)?.[1];
      if (seg === 'clear') {
        return new Response(JSON.stringify({ ok: true, files_cleared: 1, bytes_cleared: 2048 }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' }
        });
      }
      if (seg) {
        return new Response(JSON.stringify({ entries: [] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' }
        });
      }
      return new Response(JSON.stringify({ files: [{ name: 'laravel.log', size: 2048 }] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      });
    }) as unknown as typeof fetch;

    render(Harness, { props: { site: siteWith(), branch: '' } });
    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    await Promise.resolve();

    // Opening the modal must not delete anything on its own.
    const btn = (await screen.findByTitle(/reclaim disk/i)) as HTMLButtonElement;
    btn.click();
    flushSync();
    expect(methodCalls.some((c) => c.includes('/clear'))).toBe(false);

    // The modal's confirm button is what executes the delete.
    const confirm = (await screen.findByRole('button', { name: 'Clear logs' })) as HTMLButtonElement;
    confirm.click();
    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    await Promise.resolve();

    expect(methodCalls.some((c) => c.startsWith('POST') && c.includes('/clear'))).toBe(true);
  });
});
