import type { DumpEvent } from '$lib/dumpsStream';

// A port of internal/reqstats.NormalizeRoute: the query and fragment drop and
// id-like segments collapse to ":id", so the Debug tab groups requests under
// the same route keys the timing view lists. Keep the two in step.
export function normalizeRoute(method: string, uri: string): string {
  let path = uri.split(/[?#]/)[0] || '/';
  path = path
    .split('/')
    .map((s) => (s !== '' && isIdSegment(s) ? ':id' : s))
    .join('/');
  if (path.length > 1) path = path.replace(/\/+$/, '');
  if (path === '') path = '/';
  const m = method.trim().toUpperCase();
  return m ? `${m} ${path}` : path;
}

function isIdSegment(s: string): boolean {
  if (/^\d+$/.test(s)) return true;
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(s)) return true;
  return s.length >= 12 && /^[0-9a-f]+$/i.test(s);
}

const METHODS = new Set(['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS']);

// routeQuery reads a search shaped like "GET /path" as that route, or "".
export function routeQuery(q: string): string {
  const m = /^\s*([A-Za-z]+)\s+(\/\S*)\s*$/.exec(q);
  if (!m || !METHODS.has(m[1].toUpperCase())) return '';
  return normalizeRoute(m[1], m[2]);
}

// routeOf is the route an event ran under: a web request's "METHOD /uri"
// context, a browser event's page as a GET of its path, or "" for a CLI run.
export function routeOf(ev: DumpEvent): string {
  const req = ev.ctx.request ?? '';
  const m = /^([A-Z]+) (\/\S*)$/.exec(req);
  if (m) return normalizeRoute(m[1], m[2]);
  if (ev.ctx.type !== 'browser') return '';
  try {
    const u = new URL(req);
    return normalizeRoute('GET', u.pathname + u.search);
  } catch {
    return '';
  }
}
