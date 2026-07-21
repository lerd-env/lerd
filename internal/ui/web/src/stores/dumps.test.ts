import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { dumpGroups, dumps, filterSite, filterCtx, filterText, buildDumpGroups, status } from './dumps';
import { labelString } from '$lib/eventGroup';
import { wsMessage } from '$lib/ws';
import type { DumpEvent } from '$lib/dumpsStream';

function ev(over: Partial<DumpEvent> & { id: string; ts: string }): DumpEvent {
  return {
    v: 1,
    id: over.id,
    ts: over.ts,
    kind: 'dump',
    ctx: over.ctx ?? { type: 'fpm', site: 'acme', request: 'GET /' },
    src: over.src ?? { file: '/x.php', line: 1 },
    text: over.text,
    label: over.label
  };
}

describe('dumpGroups', () => {
  beforeEach(() => {
    dumps.set([]);
    filterSite.set('');
    filterCtx.set('');
    filterText.set('');
  });

  it('orders groups newest-first by most recent event in each group', () => {
    // Two distinct requests; the older request gets a later dump than the
    // newer request's earlier dump. Group sort must follow the latest
    // activity in each group, not the group's first dump.
    dumps.set([
      ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /a' } }),
      ev({ id: 'b', ts: '2026-05-10T12:00:05.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /b' } }),
      ev({ id: 'c', ts: '2026-05-10T12:00:10.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /a' } })
    ]);
    const groups = get(dumpGroups);
    expect(groups.map((g) => labelString(g.label))).toEqual(['[s] GET /a', '[s] GET /b']);
  });

  it('excludes non-dump kinds (queries share the same ring)', () => {
    dumps.set([
      ev({ id: 'd1', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /' } }),
      { ...ev({ id: 'q1', ts: '2026-05-10T12:00:01.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /' } }), kind: 'query' }
    ]);
    const groups = get(dumpGroups);
    const ids = groups.flatMap((g) => g.events.map((e) => e.id));
    expect(ids).toEqual(['d1']);
  });

  it('orders events within a group newest-first', () => {
    dumps.set([
      ev({ id: 'one',   ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /' } }),
      ev({ id: 'two',   ts: '2026-05-10T12:00:01.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /' } }),
      ev({ id: 'three', ts: '2026-05-10T12:00:02.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /' } })
    ]);
    const groups = get(dumpGroups);
    expect(groups[0].events.map((e) => e.id)).toEqual(['three', 'two', 'one']);
  });

  it('group ts reflects latest event timestamp', () => {
    dumps.set([
      ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /' } }),
      ev({ id: 'b', ts: '2026-05-10T12:00:05.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /' } })
    ]);
    const groups = get(dumpGroups);
    expect(groups[0].ts).toBe('2026-05-10T12:00:05.000Z');
  });

  it('applies site filter', () => {
    dumps.set([
      ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 'one' } }),
      ev({ id: 'b', ts: '2026-05-10T12:00:01.000Z', ctx: { type: 'fpm', site: 'two' } })
    ]);
    filterSite.set('one');
    const groups = get(dumpGroups);
    expect(groups.length).toBe(1);
    expect(groups[0].events[0].id).toBe('a');
  });

  it('applies ctx filter', () => {
    dumps.set([
      ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm' } }),
      ev({ id: 'b', ts: '2026-05-10T12:00:01.000Z', ctx: { type: 'cli' } })
    ]);
    filterCtx.set('cli');
    const groups = get(dumpGroups);
    expect(groups.length).toBe(1);
    expect(groups[0].events[0].id).toBe('b');
  });

  it('keeps a worktree request in its own group and tags the branch', () => {
    dumps.set([
      ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 'acme', request: 'GET /checkout', pid: 7 } }),
      ev({ id: 'b', ts: '2026-05-10T12:00:01.000Z', ctx: { type: 'fpm', site: 'acme', request: 'GET /checkout', pid: 7, branch: 'feature-x' } })
    ]);
    const groups = get(dumpGroups);
    expect(groups.length).toBe(2);
    const labels = groups.map((g) => labelString(g.label));
    expect(labels).toContain('[acme] GET /checkout');
    expect(labels).toContain('[acme@feature-x] GET /checkout');
  });

  it('groups one request\'s dumps together even when their rids differ', () => {
    // Without the lerd_devtools extension the bridge stamps a fresh rid per
    // dump() call; the Dumps tab must still show one card per request, not
    // one per dump.
    dumps.set([
      ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /', pid: 9, rid: 'vol-1' } }),
      ev({ id: 'b', ts: '2026-05-10T12:00:01.000Z', ctx: { type: 'fpm', site: 's', request: 'GET /', pid: 9, rid: 'vol-2' } })
    ]);
    const groups = get(dumpGroups);
    expect(groups.length).toBe(1);
    expect(groups[0].events.map((e) => e.id).sort()).toEqual(['a', 'b']);
  });

  it('hides [site] prefix when hideSitePrefix is set', () => {
    const groups = buildDumpGroups(
      [ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 'whitewaters', request: 'GET /x' } })],
      'whitewaters',
      '',
      '',
      true
    );
    expect(groups[0].label.site).toBe('');
    expect(labelString(groups[0].label)).toBe('GET /x');
  });

  it('keeps [site] prefix when hideSitePrefix is false', () => {
    const groups = buildDumpGroups(
      [ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', ctx: { type: 'fpm', site: 'whitewaters', request: 'GET /x' } })],
      '',
      '',
      '',
      false
    );
    expect(groups[0].label.site).toBe('whitewaters');
  });

  it('applies free-text search across label, text, file', () => {
    dumps.set([
      ev({ id: 'a', ts: '2026-05-10T12:00:00.000Z', text: 'Apple pie' }),
      ev({ id: 'b', ts: '2026-05-10T12:00:01.000Z', text: 'Banana bread', src: { file: '/banana.php', line: 1 } }),
      ev({ id: 'c', ts: '2026-05-10T12:00:02.000Z', label: 'banana_label' })
    ]);
    filterText.set('banana');
    const groups = get(dumpGroups);
    const ids = groups.flatMap((g) => g.events.map((e) => e.id)).sort();
    expect(ids).toEqual(['b', 'c']);
  });
});

describe('dumps status WS sync', () => {
  beforeEach(() => {
    status.set(null);
  });

  it('updates status when a dumps_status WS frame arrives', () => {
    wsMessage.set({
      type: 'dumps_status',
      dumps_status: {
        enabled: true,
        passthrough: false,
        listening: true,
        addr: 'unix:/tmp/x',
        count: 0,
        subscribers: 0,
        last_ts: ''
      }
    });
    expect(get(status)?.enabled).toBe(true);
  });

  it('ignores WS frames without a dumps_status payload', () => {
    status.set({
      enabled: false,
      passthrough: false,
      listening: false,
      addr: '',
      count: 0,
      subscribers: 0,
      last_ts: ''
    });
    wsMessage.set({ type: 'sites' });
    expect(get(status)?.enabled).toBe(false);
  });
});
