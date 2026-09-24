import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import GitStatusBadge from './GitStatusBadge.svelte';

const clean = { staged: 0, modified: 0, untracked: 0, conflicted: 0, ahead: 0, behind: 0 };

describe('GitStatusBadge', () => {
  it('shows one * and spells the changes out for the tooltip', () => {
    render(GitStatusBadge, { props: { status: { ...clean, untracked: 3, modified: 11, ahead: 1 } } });
    const badge = screen.getByLabelText('3 untracked · 11 modified · 1 ahead');
    expect(badge.textContent?.replace(/\s/g, '')).toBe('*⇡');
  });

  it('renders nothing for a clean tree', () => {
    const { container } = render(GitStatusBadge, { props: { status: clean } });
    expect(container.textContent?.trim()).toBe('');
  });
});
