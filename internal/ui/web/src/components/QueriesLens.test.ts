import { describe, it, expect, afterEach } from 'vitest';
import { render, cleanup, waitFor, fireEvent } from '@testing-library/svelte';
import QueriesLens from './QueriesLens.svelte';
import { installFakeLensApi, type FakeLensApi } from '$lib/fakeLensApi';
import type { DumpEvent } from '$lib/dumpEvent';
import { debugSearch } from '$stores/debugLens';

const query = (id: string): DumpEvent => ({
  v: 1,
  id,
  ts: '2026-05-10T12:00:00.000Z',
  kind: 'query',
  ctx: { type: 'fpm', site: 'shop', request: 'GET /cart', rid: 'r1' },
  src: { file: '/app/CartQuery.php', line: 1 },
  data: { sql: `select ${id}`, time_ms: 1, trace: [{ file: '/app/x.php', line: 1, func: 'loadCartItems' }] }
});

describe('QueriesLens opened rows', () => {
  let api: FakeLensApi;
  afterEach(() => {
    cleanup();
    api.restore();
  });

  // The list leaves out the call stack, so an opened row reads its event; a
  // closed one lets it go rather than holding every event ever opened.
  it('reads an opened row and lets it go once closed', async () => {
    api = installFakeLensApi([query('q1')]);
    const { findByText } = render(QueriesLens, { siteScope: 'shop' });
    const row = await findByText('select q1');
    const reads = () => api.requests.filter((u) => u.startsWith('/api/dumps/event')).length;

    await fireEvent.click(row);
    await waitFor(() => expect(reads()).toBe(1));
    await fireEvent.click(row);
    await fireEvent.click(row);
    await waitFor(() => expect(reads()).toBe(2));
  });
});

describe('QueriesLens rows that leave the page', () => {
  let api: FakeLensApi;
  afterEach(() => {
    cleanup();
    api.restore();
  });

  it('come back closed rather than open without their details', async () => {
    api = installFakeLensApi([query('q1')]);
    const { findByText, queryByText } = render(QueriesLens, { siteScope: 'shop' });
    await fireEvent.click(await findByText('select q1'));
    const stack = () => queryByText(/loadCartItems/);
    await waitFor(() => expect(stack()).not.toBeNull());

    debugSearch.set('nothing-matches');
    await waitFor(() => expect(queryByText('select q1')).toBeNull());
    debugSearch.set('');
    await findByText('select q1');
    // The source path is only drawn inside an opened row.
    expect(queryByText(/CartQuery/)).toBeNull();
  });
});
