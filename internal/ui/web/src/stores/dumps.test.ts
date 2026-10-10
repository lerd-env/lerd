import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { status, flashDump, lastFlashId } from './dumps';
import { wsMessage } from '$lib/ws';

describe('flashDump', () => {
  afterEach(() => vi.useRealTimers());

  it('highlights a dump that just arrived, then lets it go', () => {
    vi.useFakeTimers();
    flashDump('d1');
    expect(get(lastFlashId)).toBe('d1');
    vi.advanceTimersByTime(3000);
    expect(get(lastFlashId)).toBe('');
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
