import { describe, it, expect } from 'vitest';
import { sortSites } from './sitesOrder';
import type { Site } from '$stores/sites';

function site(domain: string, lastRequestAt?: number, requestCount?: number): Site {
  return {
    domain,
    last_request_at: lastRequestAt,
    request_count: requestCount
  } as unknown as Site;
}

// wibiway was hit most recently, admin has nearly three times its request count:
// the two orderings genuinely differ, which is why both modes exist.
const list = [
  site('admin.test', 1000, 300),
  site('wibiway.test', 5000, 110),
  site('quiet.test'),
  site('dormant.test')
];

describe('sortSites', () => {
  it('orders recent by the last request the app served', () => {
    expect(sortSites(list, 'recent').map((s) => s.domain)).toEqual([
      'wibiway.test',
      'admin.test',
      'dormant.test',
      'quiet.test'
    ]);
  });

  it('orders used by request count over the window', () => {
    expect(sortSites(list, 'used').map((s) => s.domain)).toEqual([
      'admin.test',
      'wibiway.test',
      'dormant.test',
      'quiet.test'
    ]);
  });

  it('sinks sites with no traffic below every site that has some', () => {
    const sorted = sortSites([site('zzz.test', 1, 1), site('aaa.test')], 'used');
    expect(sorted.map((s) => s.domain)).toEqual(['zzz.test', 'aaa.test']);
  });

  it('leaves alpha, newest and manual untouched by traffic', () => {
    expect(sortSites(list, 'alpha').map((s) => s.domain)).toEqual([
      'admin.test',
      'dormant.test',
      'quiet.test',
      'wibiway.test'
    ]);
    expect(sortSites(list, 'newest').map((s) => s.domain)).toEqual([
      'dormant.test',
      'quiet.test',
      'wibiway.test',
      'admin.test'
    ]);
    expect(sortSites(list, 'manual').map((s) => s.domain)).toEqual(list.map((s) => s.domain));
  });

  it('does not mutate the list it is given', () => {
    const input = [site('b.test', 1, 1), site('a.test', 9, 9)];
    sortSites(input, 'recent');
    expect(input.map((s) => s.domain)).toEqual(['b.test', 'a.test']);
  });
});
