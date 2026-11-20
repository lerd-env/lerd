import { render, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import NginxConfigTab from './NginxConfigTab.svelte';
import { getSiteNginx } from '$stores/sites';

const calls: string[] = [];

describe('settings editors for lerd’s own web server', () => {
  beforeEach(() => {
    calls.length = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        calls.push(String(url));
        return Promise.resolve(new Response(JSON.stringify({ path: '/x', content: '', exists: false }), { status: 200 }));
      })
    );
  });

  // The global editor reads the web server's own settings file, not nginx's.
  it('loads the web settings when targeting lerd’s own server', async () => {
    render(NginxConfigTab, { props: { target: 'web' } });
    await waitFor(() => expect(calls.some((u) => u.includes('/api/web/config'))).toBe(true));
    expect(calls.some((u) => u.includes('/api/nginx/'))).toBe(false);
  });

  it('keeps reading nginx’s file by default', async () => {
    render(NginxConfigTab);
    await waitFor(() => expect(calls.some((u) => u.includes('/api/nginx/config'))).toBe(true));
  });

  // A site's settings for the Go server travel as the web scope.
  it('asks for the web scope of a site', async () => {
    await getSiteNginx('app.test', 'web');
    expect(calls.some((u) => u.includes('/nginx?scope=web'))).toBe(true);
  });
});
