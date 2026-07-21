import { describe, it, expect } from 'vitest';
import { groupKey, groupLabel, labelString } from './eventGroup';
import type { DumpEvent } from '$lib/dumpsStream';

function ev(over: Partial<DumpEvent['ctx']> & { ts?: string } = {}): DumpEvent {
  const { ts, ...ctx } = over;
  return {
    v: 1,
    id: 'x',
    ts: ts ?? '2026-05-10T12:00:00.000Z',
    kind: 'query',
    ctx: { type: 'fpm', site: 'acme', request: 'GET /checkout', pid: 7, ...ctx },
    src: { file: '/x.php', line: 1 }
  };
}

describe('groupKey', () => {
  it('separates a worktree request from the parent it shares site/request/pid with', () => {
    const parent = groupKey(ev({ branch: '' }));
    const worktree = groupKey(ev({ branch: 'feature-x' }));
    expect(parent).not.toBe(worktree);
  });

  it('keeps the per-request id as the boundary when present', () => {
    expect(groupKey(ev({ rid: 'r1', branch: 'feature-x' }))).toBe('rid:r1');
  });

  it('ignores rid for dump events (it can be volatile without the extension)', () => {
    // Two dumps in one request whose rids differ (collector new_id() fallback)
    // must still land in the same group, so dumps key on request, not rid.
    const a = groupKey({ ...ev({ rid: 'vol-1' }), kind: 'dump' });
    const b = groupKey({ ...ev({ rid: 'vol-2' }), kind: 'dump' });
    expect(a).toBe(b);
    expect(a.startsWith('rid:')).toBe(false);
  });

  it('folds branch into the cli bucket key too', () => {
    const parent = groupKey(ev({ type: 'cli', branch: '' }));
    const worktree = groupKey(ev({ type: 'cli', branch: 'feature-x' }));
    expect(parent).not.toBe(worktree);
  });
});

describe('groupLabel', () => {
  it('returns site and branch as separate parts', () => {
    expect(groupLabel(ev({ branch: 'feature-x' }), false)).toEqual({
      site: 'acme',
      branch: 'feature-x',
      text: 'GET /checkout'
    });
  });

  it('drops the site but keeps the branch when the site prefix is hidden', () => {
    expect(groupLabel(ev({ branch: 'feature-x' }), true)).toEqual({
      site: '',
      branch: 'feature-x',
      text: 'GET /checkout'
    });
  });

  it('names the worker command when the event came from a worker process', () => {
    expect(groupLabel(ev({ worker: 'queue:work' }), false).text).toBe('queue:work');
  });

  it('falls back to the pid for cli invocations', () => {
    expect(groupLabel(ev({ type: 'cli', request: '' }), false).text).toBe('cli (pid 7)');
  });

  it('flattens back to the bracketed single-string form', () => {
    expect(labelString(groupLabel(ev({ branch: 'feature-x' }), false))).toBe('[acme@feature-x] GET /checkout');
    expect(labelString(groupLabel(ev({ branch: '' }), false))).toBe('[acme] GET /checkout');
    expect(labelString(groupLabel(ev({ branch: 'feature-x' }), true))).toBe('[feature-x] GET /checkout');
    expect(labelString(groupLabel(ev({ branch: '', site: '' }), false))).toBe('GET /checkout');
  });
});
