import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import type { DumpEvent } from '$lib/dumpsStream';
import { dumps } from './dumps';
import { showTests } from './debugLens';
import { debugEvents, hiddenTestCount, countKinds } from './debugEvents';

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
