import { render, cleanup } from '@testing-library/svelte';
import { describe, it, expect, vi, afterEach } from 'vitest';
import Harness from './SiteHeader.test.svelte';
import type { Site } from '$stores/sites';

// isDesktop reads matchMedia whenever it gains its first subscriber, so each
// test sets the width it wants before rendering the header.
function width(desktop: boolean) {
  vi.stubGlobal('matchMedia', (q: string) => ({ matches: desktop, media: q, addEventListener() {}, removeEventListener() {} }));
}

const site = { domain: 'app.test', domains: ['app.test'], path: '/home/u/Code/app', php_version: '8.3', worktrees: [] } as unknown as Site;
const worktreeSite = { ...site, branch: 'main', worktrees: [{ branch: 'feat', domain: 'feat.app.test', path: '/home/u/Code/app-feat' }] } as unknown as Site;

async function header(props: { site: Site }) {
  return render(Harness, { props });
}

describe('SiteHeader back button', () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it('sits at the start of the worktree tabs on a phone', async () => {
    width(false);
    const { getByLabelText, getByText } = await header({ site: worktreeSite });
    const back = getByLabelText('Back');
    expect(back.parentElement).toContainElement(getByText('feat'));
  });

  it('sits at the start of the URL row when the site has no worktree tabs', async () => {
    width(false);
    const { getByLabelText } = await header({ site });
    expect(getByLabelText('Back')).toBeInTheDocument();
  });

  it('is not there on a desktop, which has the side panel', async () => {
    width(true);
    const { queryByLabelText } = await header({ site: worktreeSite });
    expect(queryByLabelText('Back')).not.toBeInTheDocument();
  });
});
