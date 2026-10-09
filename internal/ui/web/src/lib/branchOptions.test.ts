import { describe, it, expect } from 'vitest';
import { branchOptions } from './branchOptions';

const now = 1_000_000_000_000;
const daysAgo = (d: number) => now / 1000 - d * 86_400;

describe('branchOptions', () => {
  it('lists local before remote, each with its last commit age', () => {
    expect(branchOptions(['dev'], ['origin/x'], { dev: daysAgo(2), 'origin/x': daysAgo(9) }, now)).toEqual([
      { value: 'dev', label: 'dev', description: '2d ago' },
      { value: 'origin/x', label: 'origin/x', description: 'remote · 9d ago' }
    ]);
  });

  it('puts the most recently committed branch first, local or remote', () => {
    const got = branchOptions(['old', 'fresh'], ['origin/mid'], { old: daysAgo(30), fresh: daysAgo(1), 'origin/mid': daysAgo(5) }, now);
    expect(got.map((o) => o.value)).toEqual(['fresh', 'origin/mid', 'old']);
  });

  it('leaves the age out when git gave none', () => {
    expect(branchOptions(['dev'], ['origin/x'], {}, now).map((o) => o.description)).toEqual(['', 'remote']);
  });
});
