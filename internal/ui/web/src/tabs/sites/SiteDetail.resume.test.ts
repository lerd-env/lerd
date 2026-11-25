import { render, fireEvent, waitFor } from '@testing-library/svelte';
import { it, expect, vi } from 'vitest';

const notifyLocalFailure = vi.hoisted(() => vi.fn());
vi.mock('$lib/notify', async (orig) => ({ ...(await orig<typeof import('$lib/notify')>()), notifyLocalFailure }));

import SiteDetail from './SiteDetail.svelte';
import type { Site } from '$stores/sites';

// Resuming can fail, its PHP may have to be installed again, and the paused
// page would otherwise just sit there as if the button did nothing.
it('says why a site could not be resumed', async () => {
  globalThis.fetch = vi.fn(async (url: RequestInfo | URL) =>
    String(url).includes('/unpause')
      ? new Response(JSON.stringify({ ok: false, error: 'PHP 8.3 could not be brought up' }))
      : new Response('[]')
  ) as unknown as typeof fetch;
  const site = { name: 'shop', domain: 'shop.test', path: '/srv/shop', services: [], paused: true } as unknown as Site;
  const { getByRole } = render(SiteDetail, { props: { site } });
  await fireEvent.click(getByRole('button', { name: 'Resume site' }));
  await waitFor(() =>
    expect(notifyLocalFailure).toHaveBeenCalledWith('site_resume', 'shop.test', 'PHP 8.3 could not be brought up')
  );
});
