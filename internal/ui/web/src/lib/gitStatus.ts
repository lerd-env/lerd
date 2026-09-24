import { apiJson } from './api';

export interface GitStatus {
  staged: number;
  modified: number;
  untracked: number;
  conflicted: number;
  ahead: number;
  behind: number;
}

export interface GitCheckout extends GitStatus {
  branch: string;
  path: string;
  main: boolean;
}

// Worktrees match on path: lerd sanitises branch names ("bugfix/x" shows as
// "bugfix-x"), so the name git reports can't be compared with the tab's.
export function checkoutFor(checkouts: GitCheckout[], tab: { isMain: boolean; path: string }): GitCheckout | undefined {
  return checkouts.find((c) => (tab.isMain ? c.main : !c.main && c.path === tab.path));
}

export type GitPartKind = 'conflicted' | 'untracked' | 'modified' | 'staged' | 'ahead' | 'behind';

const ORDER: GitPartKind[] = ['conflicted', 'untracked', 'modified', 'staged', 'ahead', 'behind'];

// gitParts lists what is non-zero, for the tooltip.
export function gitParts(s: GitStatus): { kind: GitPartKind; count: number }[] {
  return ORDER.filter((k) => s[k] > 0).map((kind) => ({ kind, count: s[kind] }));
}

// gitMarker is what a minimal shell prompt prints after the branch: one * for
// any change to the tree, then arrows for commits ahead of or behind upstream.
export function gitMarker(s: GitStatus): { text: string; conflicted: boolean } {
  const dirty = s.conflicted + s.untracked + s.modified + s.staged > 0;
  const text = (dirty ? '*' : '') + (s.ahead > 0 ? '⇡' : '') + (s.behind > 0 ? '⇣' : '');
  return { text, conflicted: s.conflicted > 0 };
}

export async function loadGitStatus(domain: string): Promise<GitCheckout[]> {
  const res = await apiJson<{ checkouts: GitCheckout[] }>(`/api/sites/git-status?domain=${encodeURIComponent(domain)}`);
  return res.checkouts;
}
