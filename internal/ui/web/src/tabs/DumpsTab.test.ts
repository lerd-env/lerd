import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, cleanup, waitFor, fireEvent } from '@testing-library/svelte';
import { get } from 'svelte/store';
import DumpsTab from './DumpsTab.svelte';
import { filterSite, filterCtx, filterText, status } from '../stores/dumps';
import { debugSearch } from '../stores/debugLens';
import { installFakeLensApi, type FakeLensApi } from '../lib/fakeLensApi';
import type { DumpEvent } from '../lib/dumpEvent';

function ev(over: Partial<DumpEvent> & { id: string }): DumpEvent {
  return {
    v: 1,
    id: over.id,
    ts: over.ts ?? '2026-05-10T12:00:00.000Z',
    kind: 'dump',
    ctx: over.ctx ?? { type: 'fpm', site: 'whitewaters', request: 'GET /' },
    src: over.src ?? { file: '/app/foo.php', line: 12 },
    text: over.text ?? 'array:1 [\n  "k" => "v"\n]\n'
  };
}

describe('DumpsTab', () => {
  let api: FakeLensApi;

  beforeEach(() => {
    api = installFakeLensApi();
    filterSite.set('');
    filterCtx.set('');
    filterText.set('');
    debugSearch.set('');
    status.set({ enabled: true, passthrough: false, listening: true, addr: 'unix:/tmp/x', count: 0, subscribers: 0, last_ts: '' });
  });

  afterEach(() => {
    cleanup();
    api.restore();
  });

  it('asks lerd-ui for the site dumps and renders them without the site prefix', async () => {
    api.events.push(
      ev({ id: 'a', ctx: { type: 'fpm', site: 'whitewaters', request: 'GET /matched' } }),
      ev({ id: 'b', ctx: { type: 'fpm', site: 'otherone', request: 'GET /excluded' } })
    );
    const { container } = render(DumpsTab, { siteScope: 'whitewaters' });
    await waitFor(() => expect(container.textContent).toContain('GET /matched'));
    expect(container.textContent).not.toContain('GET /excluded');
    expect(container.textContent).not.toContain('[whitewaters]');
    expect(api.requests.some((u) => u.startsWith('/api/dumps/groups') && u.includes('kind=dump') && u.includes('site=whitewaters'))).toBe(true);
  });

  it('shows the empty state when the site has no dumps', async () => {
    api.events.push(ev({ id: 'a', ctx: { type: 'fpm', site: 'someone-else', request: 'GET /' } }));
    const { container } = render(DumpsTab, { siteScope: 'whitewaters' });
    await waitFor(() => expect(container.textContent).toMatch(/Waiting for dumps/));
  });

  it('does not mutate global filterSite when scoped', async () => {
    filterSite.set('previously-selected');
    render(DumpsTab, { siteScope: 'whitewaters' });
    await new Promise((r) => setTimeout(r, 20));
    expect(get(filterSite)).toBe('previously-selected');
  });

  it('shows an Enable button when the bridge is off and nothing was captured', async () => {
    api.enabled = false;
    status.set({ enabled: false, passthrough: false, listening: true, addr: 'unix:/tmp/x', count: 0, subscribers: 0, last_ts: '' });
    const { container } = render(DumpsTab, { siteScope: 'whitewaters' });
    await waitFor(() => expect(container.textContent).toMatch(/Enable debug bridge/));
    expect(container.textContent).toMatch(/Debug bridge is disabled/);
  });

  it('holds one page of requests and reads the next from where it ended', async () => {
    for (let i = 0; i < 35; i++) {
      api.events.push(ev({ id: `e${i}`, ctx: { type: 'fpm', site: 'whitewaters', request: `GET /r${i}`, pid: i } }));
    }
    const { container, getByRole } = render(DumpsTab, { siteScope: 'whitewaters' });
    await waitFor(() => expect(container.querySelectorAll('section').length).toBe(30));
    await fireEvent.click(getByRole('button', { name: /Load more/ }));
    await waitFor(() => expect(container.querySelectorAll('section').length).toBe(35));
    expect(api.requests.some((u) => u.includes('before='))).toBe(true);
  });

  it('sends the search to lerd-ui and shows what it answers', async () => {
    api.events.push(
      ev({ id: 'a', ctx: { type: 'fpm', site: 'whitewaters', request: 'GET /invoice', pid: 1 } }),
      ev({ id: 'b', ctx: { type: 'fpm', site: 'whitewaters', request: 'GET /cart', pid: 2 } })
    );
    const { container } = render(DumpsTab, { siteScope: 'whitewaters' });
    await waitFor(() => expect(container.querySelectorAll('section').length).toBe(2));
    debugSearch.set('invoice');
    await waitFor(() => expect(container.querySelectorAll('section').length).toBe(1));
    expect(api.requests.some((u) => u.includes('q=invoice'))).toBe(true);
  });

  it('reads the page again when lerd-ui says a dump arrived', async () => {
    const { container } = render(DumpsTab, { siteScope: 'whitewaters' });
    await waitFor(() => expect(container.textContent).toMatch(/Waiting for dumps/));
    api.events.push(ev({ id: 'live', ctx: { type: 'fpm', site: 'whitewaters', request: 'GET /live' } }));
    api.nudge('dump', 'whitewaters');
    await waitFor(() => expect(container.textContent).toContain('GET /live'), { timeout: 2000 });
  });
});
