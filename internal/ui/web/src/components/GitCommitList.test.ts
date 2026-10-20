import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';

const { loadGitCommits } = vi.hoisted(() => ({ loadGitCommits: vi.fn() }));
vi.mock('$lib/gitDiff', async (orig) => ({ ...(await orig<object>()), loadGitCommits }));

import GitCommitList from './GitCommitList.svelte';

describe('GitCommitList', () => {
  it('starts folded with the count on its heading, and opens on click', async () => {
    loadGitCommits.mockResolvedValue({ commits: [{ hash: 'abc1234def', subject: 'feat: x', time: Math.floor(Date.now() / 1000) - 3 * 3600 }], more: false });
    render(GitCommitList, { props: { domain: 'acme.test', path: '/srv/acme', direction: 'outgoing', onopen: vi.fn() } });

    const toggle = await screen.findByRole('button', { name: 'Commits · 1' });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('button', { name: /feat: x/ })).toBeNull();

    await fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('feat: x')).toBeVisible();
    expect(screen.getByText('3h ago')).toBeVisible();
  });
});
