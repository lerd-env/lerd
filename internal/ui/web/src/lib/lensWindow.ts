// Shared row windowing for the Debug lenses. Every lens renders request
// groups whose rows come off one unbounded client buffer, so each of them
// slices the same way: fill a row budget group by group, truncating the
// group that straddles the edge.

// LENS_PAGE is both the initial window and the step each "load more" adds.
export const LENS_PAGE = 100;

export interface LensPage<G, R> {
  group: G;
  rows: R[];
  // total is the group's row count before truncation, so headers keep
  // reporting the real size of a request.
  total: number;
}

export interface LensWindow<G, R> {
  pages: LensPage<G, R>[];
  shown: number;
  total: number;
}

export function windowGroups<G, R>(groups: G[], rowsOf: (g: G) => R[], limit: number): LensWindow<G, R> {
  const pages: LensPage<G, R>[] = [];
  let shown = 0;
  let total = 0;
  for (const group of groups) {
    const rows = rowsOf(group);
    total += rows.length;
    if (shown >= limit) continue;
    const take = Math.min(rows.length, limit - shown);
    pages.push({ group, rows: take === rows.length ? rows : rows.slice(0, take), total: rows.length });
    shown += take;
  }
  return { pages, shown, total };
}
