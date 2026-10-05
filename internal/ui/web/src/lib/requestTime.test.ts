import { describe, it, expect } from 'vitest';
import { sinceStart, clock } from './requestTime';

describe('sinceStart', () => {
  it('reads milliseconds under a second and seconds after', () => {
    expect(sinceStart('2026-10-05T10:00:00.250Z', '2026-10-05T10:00:00.000Z')).toBe('+250 ms');
    expect(sinceStart('2026-10-05T10:00:01.234Z', '2026-10-05T10:00:00.000Z')).toBe('+1.23 s');
    expect(sinceStart('2026-10-05T10:00:42.000Z', '2026-10-05T10:00:00.000Z')).toBe('+42.0 s');
  });
  it('says nothing without both times', () => {
    expect(sinceStart(undefined, '2026-10-05T10:00:00.000Z')).toBe('');
  });
});

describe('clock', () => {
  it('gives the time of day, or nothing for no time', () => {
    expect(clock('2026-10-05T10:00:00.042Z')).toMatch(/^\d{2}.\d{2}.\d{2}.042$/);
    expect(clock(undefined)).toBe('');
  });
});
