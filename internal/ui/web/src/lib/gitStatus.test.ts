import { describe, it, expect } from 'vitest';
import { gitParts, gitMarker, checkoutFor, type GitStatus } from './gitStatus';

const clean: GitStatus = { staged: 0, modified: 0, untracked: 0, conflicted: 0, ahead: 0, behind: 0 };

describe('gitParts', () => {
  it('lists nothing for a clean tree', () => {
    expect(gitParts(clean)).toEqual([]);
  });

  it('lists each kind with its count, conflicts first', () => {
    const s = { staged: 1, modified: 2, untracked: 3, conflicted: 1, ahead: 2, behind: 1 };
    expect(gitParts(s).map((p) => `${p.kind}:${p.count}`)).toEqual([
      'conflicted:1', 'untracked:3', 'modified:2', 'staged:1', 'ahead:2', 'behind:1'
    ]);
  });
});

// Same marks as a minimal shell prompt: one * for any change, arrows for the upstream.
describe('gitMarker', () => {
  it('is empty for a clean tree', () => {
    expect(gitMarker(clean)).toEqual({ text: '', conflicted: false });
  });

  it('folds every kind of change into one *', () => {
    expect(gitMarker({ ...clean, untracked: 3, modified: 11 }).text).toBe('*');
    expect(gitMarker({ ...clean, staged: 1 }).text).toBe('*');
  });

  it('adds the upstream arrows', () => {
    expect(gitMarker({ ...clean, modified: 1, ahead: 2, behind: 1 }).text).toBe('*⇡⇣');
    expect(gitMarker({ ...clean, behind: 3 }).text).toBe('⇣');
  });

  it('flags a conflict', () => {
    expect(gitMarker({ ...clean, conflicted: 1 })).toEqual({ text: '*', conflicted: true });
  });
});

describe('checkoutFor', () => {
  const checkouts = [
    { ...clean, branch: 'staging', path: '/p/app', main: true, modified: 2 },
    { ...clean, branch: 'bugfix/job', path: '/p/app-bugfix-job', main: false, modified: 1 }
  ];

  it('finds the main checkout', () => {
    expect(checkoutFor(checkouts, { isMain: true, path: '' })?.modified).toBe(2);
  });

  // lerd shows "bugfix-job" for git's "bugfix/job", so the name can't be the key.
  it('finds a worktree by its path, not its branch name', () => {
    expect(checkoutFor(checkouts, { isMain: false, path: '/p/app-bugfix-job' })?.modified).toBe(1);
  });
});
