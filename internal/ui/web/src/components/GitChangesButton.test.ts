import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';

const { openGitDiffModal } = vi.hoisted(() => ({ openGitDiffModal: vi.fn() }));
vi.mock('$stores/modals', () => ({ openGitDiffModal }));

import GitChangesButton from './GitChangesButton.svelte';

describe('GitChangesButton', () => {
  it('opens the diff of its own checkout', async () => {
    render(GitChangesButton, { props: { domain: 'acme.test', path: '/srv/acme-feature', branchLabel: 'feature' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Show uncommitted changes' }));
    expect(openGitDiffModal).toHaveBeenCalledWith('acme.test', '/srv/acme-feature', 'feature');
  });
});
