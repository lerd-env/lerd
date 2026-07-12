import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import Harness from './WorktreeDBDropModal.test.svelte';

describe('WorktreeDBDropModal', () => {
  it('does not render anything when closed', () => {
    const { queryByText } = render(Harness, {
      props: { open: false, branch: 'feat-a', onclose: () => {}, onconfirm: () => {} }
    });
    expect(queryByText('Drop database')).toBeNull();
  });

  it('names the branch and the database that is about to go', () => {
    render(Harness, {
      props: {
        open: true,
        branch: 'feat-a',
        database: 'acme_feat_a',
        onclose: () => {},
        onconfirm: () => {}
      }
    });
    expect(screen.getByText(/The feat-a worktree goes back to sharing/)).toBeTruthy();
    expect(screen.getByText('acme_feat_a')).toBeTruthy();
  });

  it('drops nothing and closes when cancelled', async () => {
    const onconfirm = vi.fn();
    const onclose = vi.fn();
    render(Harness, { props: { open: true, branch: 'feat-a', onclose, onconfirm } });
    await fireEvent.click(screen.getByText('Cancel'));
    expect(onconfirm).not.toHaveBeenCalled();
    expect(onclose).toHaveBeenCalled();
  });

  it('emits onconfirm and closes when confirmed', async () => {
    const onconfirm = vi.fn();
    const onclose = vi.fn();
    render(Harness, { props: { open: true, branch: 'feat-a', onclose, onconfirm } });
    await fireEvent.click(screen.getByText('Drop database'));
    expect(onconfirm).toHaveBeenCalled();
    expect(onclose).toHaveBeenCalled();
  });
});
