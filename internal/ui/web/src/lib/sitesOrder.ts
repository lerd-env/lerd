import type { Site } from '$stores/sites';
import type { SitesSort } from '$stores/sitesSort';

// Orders the sites list for a display sort mode. 'recent' and 'used' read the
// request store's traffic figures, which cover the store's retention window and
// count only the requests the app actually served. A site with no traffic in the
// window sorts below every site that has some, and alphabetically among the
// others, so the untrafficked tail is stable rather than arbitrary.
export function sortSites(list: Site[], mode: SitesSort): Site[] {
  const out = [...list];
  switch (mode) {
    case 'alpha':
      return out.sort(byDomain);
    case 'recent':
      return out.sort(byTraffic((s) => s.last_request_at ?? 0));
    case 'used':
      return out.sort(byTraffic((s) => s.request_count ?? 0));
    case 'newest':
      return out.reverse();
    case 'manual':
    default:
      return out;
  }
}

function byDomain(a: Site, b: Site): number {
  return a.domain.localeCompare(b.domain);
}

function byTraffic(measure: (s: Site) => number) {
  return (a: Site, b: Site) => {
    const av = measure(a);
    const bv = measure(b);
    return av === bv ? byDomain(a, b) : bv - av;
  };
}
