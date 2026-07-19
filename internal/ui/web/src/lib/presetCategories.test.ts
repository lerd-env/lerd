import { describe, it, expect } from 'vitest';
import {
  categoryOf,
  asCategory,
  groupByCategory,
  CATEGORY_ORDER,
  CATEGORY_LABELS
} from './presetCategories';
import type { Preset } from '$stores/presets';

function p(name: string, category: string): Preset {
  return { name, category } as Preset;
}

describe('categoryOf', () => {
  it('reads the category the preset declares', () => {
    expect(categoryOf(p('mysql', 'databases'))).toBe('databases');
    expect(categoryOf(p('redis', 'cache'))).toBe('cache');
    expect(categoryOf(p('opensearch-dashboards', 'admin'))).toBe('admin');
  });

  it('returns other when a preset declares nothing', () => {
    expect(categoryOf({ name: 'totally-new-thing' } as Preset)).toBe('other');
  });

  // A store preset from a newer schema than this build must not break the grid.
  it('returns other for a category this build has no section for', () => {
    expect(categoryOf(p('from-the-future', 'quantum'))).toBe('other');
  });
});

describe('asCategory', () => {
  it('passes through a known category and rejects anything else', () => {
    expect(asCategory('storage')).toBe('storage');
    expect(asCategory('nonsense')).toBe('other');
    expect(asCategory(undefined)).toBe('other');
  });
});

describe('groupByCategory', () => {
  it('groups presets and only returns non-empty categories in order', () => {
    const groups = groupByCategory([
      p('redis', 'cache'),
      p('mysql', 'databases'),
      p('mongo', 'databases'),
      p('mailpit', 'mail')
    ]);
    expect(groups.map((g) => g.key)).toEqual(['databases', 'cache', 'mail']);
    expect(groups[0].items.map((x) => x.name)).toEqual(['mongo', 'mysql']);
  });

  it('sorts presets alphabetically within a category', () => {
    const groups = groupByCategory([
      p('valkey', 'cache'),
      p('memcached', 'cache'),
      p('redis', 'cache')
    ]);
    expect(groups[0].items.map((x) => x.name)).toEqual(['memcached', 'redis', 'valkey']);
  });

  it('keeps the global category order regardless of input order', () => {
    const groups = groupByCategory([p('selenium', 'testing'), p('mysql', 'databases')]);
    const idxDb = CATEGORY_ORDER.indexOf('databases');
    const idxTest = CATEGORY_ORDER.indexOf('testing');
    expect(idxDb).toBeLessThan(idxTest);
    expect(groups.map((g) => g.key)).toEqual(['databases', 'testing']);
  });

  it('orders the messaging bucket between cache and search', () => {
    const groups = groupByCategory([
      p('opensearch', 'search'),
      p('soketi', 'messaging'),
      p('redis', 'cache')
    ]);
    expect(groups.map((g) => g.key)).toEqual(['cache', 'messaging', 'search']);
  });

  it('buckets an undeclared preset into other rather than dropping it', () => {
    const groups = groupByCategory([{ name: 'mystery' } as Preset]);
    expect(groups.map((g) => g.key)).toEqual(['other']);
  });

  it('returns an empty list for no presets', () => {
    expect(groupByCategory([])).toEqual([]);
  });
});

describe('CATEGORY_LABELS', () => {
  it('has a non-empty label for every category in CATEGORY_ORDER', () => {
    for (const key of CATEGORY_ORDER) {
      expect(typeof CATEGORY_LABELS[key]).toBe('function');
      expect(CATEGORY_LABELS[key]().length).toBeGreaterThan(0);
    }
  });
});
