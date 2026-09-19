// A dashboard is now reached at a mount path ending in a slash, while a deep
// link is written the way the upstream writes it, which may lead with one.
// Joining them blind leaves a doubled slash, and a path with an empty segment is
// not the path the app routes on.
export function joinDashboardPath(dashboard: string, extra?: string | null): string {
  if (!extra) return dashboard;
  if (dashboard.endsWith('/') && extra.startsWith('/')) return dashboard + extra.slice(1);
  if (!dashboard.endsWith('/') && !extra.startsWith('/') && !extra.startsWith('?')) {
    return dashboard + '/' + extra;
  }
  return dashboard + extra;
}
