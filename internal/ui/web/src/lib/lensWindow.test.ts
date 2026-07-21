import { describe, it, expect } from 'vitest';
import { windowGroups, LENS_PAGE } from './lensWindow';

interface G {
  key: string;
  rows: number[];
}

const g = (key: string, n: number): G => ({ key, rows: Array.from({ length: n }, (_, i) => i) });
const rowsOf = (x: G) => x.rows;

describe('windowGroups', () => {
  it('keeps every group when the budget covers them all', () => {
    const w = windowGroups([g('a', 3), g('b', 2)], rowsOf, LENS_PAGE);
    expect(w.pages.map((p) => p.group.key)).toEqual(['a', 'b']);
    expect(w.shown).toBe(5);
    expect(w.total).toBe(5);
  });

  it('drops groups past the budget but still counts their rows', () => {
    const w = windowGroups([g('a', 4), g('b', 4), g('c', 4)], rowsOf, 4);
    expect(w.pages.map((p) => p.group.key)).toEqual(['a']);
    expect(w.shown).toBe(4);
    expect(w.total).toBe(12);
  });

  it('truncates the group straddling the budget and reports its real size', () => {
    const w = windowGroups([g('a', 3), g('b', 10)], rowsOf, 5);
    expect(w.pages[1].rows).toHaveLength(2);
    expect(w.pages[1].total).toBe(10);
    expect(w.shown).toBe(5);
    expect(w.total).toBe(13);
  });

  it('does not copy rows when a group fits whole', () => {
    const group = g('a', 3);
    const w = windowGroups([group], rowsOf, 5);
    expect(w.pages[0].rows).toBe(group.rows);
  });

  it('handles an empty group list', () => {
    const w = windowGroups<G, number>([], rowsOf, LENS_PAGE);
    expect(w.pages).toEqual([]);
    expect(w.shown).toBe(0);
    expect(w.total).toBe(0);
  });

  it('skips empty groups without spending budget', () => {
    const w = windowGroups([g('a', 0), g('b', 2)], rowsOf, 2);
    expect(w.pages.map((p) => p.group.key)).toEqual(['a', 'b']);
    expect(w.shown).toBe(2);
  });
});
