import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { DumpEvent } from '$lib/dumpsStream';
import { dumps } from './dumps';
import { showTests } from './debugLens';
import { debugEvents, hiddenTestCount, countKinds, buildKindGroups } from './debugEvents';

function ev(id: string, test = false): DumpEvent {
  return {
    v: 1,
    id,
    ts: '2026-07-21T10:00:00.000Z',
    kind: 'dump',
    ctx: { type: 'cli', site: 'acme', test: test || undefined },
    src: { file: '/app/Http/Kernel.php', line: 10 },
    text: id
  };
}

describe('debugEvents', () => {
  beforeEach(() => {
    dumps.set([ev('a'), ev('t1', true), ev('b'), ev('t2', true)]);
    showTests.set(false);
  });

  it('hides test-run events by default', () => {
    expect(get(debugEvents).map((e) => e.id)).toEqual(['a', 'b']);
    expect(get(hiddenTestCount)).toBe(2);
  });

  it('includes them once the toggle is on', () => {
    showTests.set(true);
    expect(get(debugEvents).map((e) => e.id)).toEqual(['a', 't1', 'b', 't2']);
    expect(get(hiddenTestCount)).toBe(0);
  });

  it('keeps the tab counters agreeing with the visible list', () => {
    expect(countKinds(get(debugEvents), 'acme')['dump']).toBe(2);
    showTests.set(true);
    expect(countKinds(get(debugEvents), 'acme')['dump']).toBe(4);
  });
});

function job(id: string, status: string, worker = ''): DumpEvent {
  return {
    v: 1,
    id,
    ts: '2026-07-21T10:00:0' + id.slice(-1) + '.000Z',
    kind: 'job',
    ctx: { type: 'cli', site: 'acme', rid: id, worker: worker || undefined },
    src: { file: '/app/Jobs/SendInvoice.php', line: 20 },
    data: { class: 'App\\Jobs\\SendInvoice', status }
  };
}

describe('buildKindGroups', () => {
  it('keeps a worker\'s jobs on screen with worker capture off', () => {
    const events = [job('1', 'processed', 'queue:work'), job('2', 'failed', 'queue:work')];
    expect(buildKindGroups(events, 'job', '', '', false, '', false)).toHaveLength(2);
  });

  it('still hides a worker\'s other events with worker capture off', () => {
    const query = { ...job('3', 'processed', 'queue:work'), kind: 'query' } as DumpEvent;
    expect(buildKindGroups([query], 'query', '', '', false, '', false)).toHaveLength(0);
  });

  it('narrows to one job status when the filter is set', () => {
    const events = [job('4', 'processing'), job('5', 'failed')];
    const groups = buildKindGroups(events, 'job', '', '', false, '', true, 'failed');
    expect(groups).toHaveLength(1);
    expect(groups[0].events[0].id).toBe('5');
  });
});
