import { describe, it, expect, vi } from 'vitest';

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }));
vi.mock('$lib/api', () => ({ apiFetch, apiJson: vi.fn() }));

import { streamPull } from './worktree';

describe('streamPull', () => {
  it('sends the reviewed commit along with the chosen steps', async () => {
    apiFetch.mockResolvedValue(new Response('event: done\ndata: {"ok":true}\n\n', { headers: { 'Content-Type': 'text/event-stream' } }));
    await streamPull('acme.test', 'feature', 'abc123', { composer: true, js: false, migrate: false, snapshot: false }, () => {});
    const url = new URL(apiFetch.mock.calls[0][0], 'http://x');
    expect(url.pathname).toBe('/api/sites/pull');
    expect(url.searchParams.get('target')).toBe('abc123');
    expect(url.searchParams.get('composer')).toBe('1');
  });

  // Refused before streaming began, the server answers in JSON; its reason must reach the dialog.
  it('reports a refusal that came back as JSON', async () => {
    apiFetch.mockResolvedValue(new Response('{"error":"unknown worktree branch"}', { headers: { 'Content-Type': 'application/json' } }));
    const events: object[] = [];
    await streamPull('acme.test', 'gone', 'abc123', { composer: false, js: false, migrate: false, snapshot: false }, (e) => events.push(e));
    expect(events).toEqual([{ done: true, ok: false, error: 'unknown worktree branch' }]);
  });
});
