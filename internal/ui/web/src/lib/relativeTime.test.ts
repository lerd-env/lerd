import { describe, it, expect } from 'vitest';
import { relativeTime } from './relativeTime';

const now = 1_000_000_000_000;

describe('relativeTime', () => {
  it('rounds to the largest whole unit', () => {
    expect(relativeTime(now - 10_000, now)).toBe('just now');
    expect(relativeTime(now - 5 * 60_000, now)).toBe('5m ago');
    expect(relativeTime(now - 3 * 3_600_000, now)).toBe('3h ago');
    expect(relativeTime(now - 50 * 3_600_000, now)).toBe('2d ago');
  });
});
