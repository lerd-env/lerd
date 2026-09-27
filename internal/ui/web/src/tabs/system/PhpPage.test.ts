import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import PhpPage from './PhpPage.svelte';

// The version's own tabs fetch and stream; the header is all this is about.
vi.mock('./PhpDetail.svelte', async () => ({ default: (await import('../services/ServiceDetail.stub.svelte')).default }));
import { phpVersions } from '$stores/phpVersions';

describe('PhpPage', () => {
  beforeEach(() => {
    globalThis.fetch = (async () => new Response('{}', { status: 200 })) as unknown as typeof fetch;
    phpVersions.set(['8.4', '8.3']);
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
});
