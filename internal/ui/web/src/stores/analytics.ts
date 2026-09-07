import { apiFetch, apiJson } from '$lib/api';

export interface RouteStat {
  route: string;
  method: string;
  example: string;
  p50_millis: number;
  p95_millis: number;
  recent_p95_millis?: number;
  multiplier: number;
  samples: number;
}

export interface LatencyBucket {
  upper_millis: number; // 0 = open-ended top bucket
  count: number;
}

export interface StatusCounts {
  c2xx: number;
  c3xx: number;
  c4xx: number;
  c5xx: number;
}

export interface ThroughputPoint {
  at_millis: number;
  count: number;
}

export interface RecentRequest {
  at_millis: number;
  method: string;
  route: string;
  uri: string;
  status: number;
  millis: number;
  cold: boolean;
}

export interface Analytics {
  site: string;
  range: string;
  samples: number;
  cold_starts: number;
  median_millis: number;
  p95_millis: number;
  status: StatusCounts;
  distribution: LatencyBucket[];
  throughput: ThroughputPoint[];
  routes: RouteStat[];
  recent: RecentRequest[];
  // Routes the user has silenced: nothing new is recorded on them and nothing
  // already stored for them appears above.
  excluded: string[];
}

export type TimeRange = '15m' | '1h' | '24h' | '7d';
export const TIME_RANGES: TimeRange[] = ['15m', '1h', '24h', '7d'];

// loadSiteAnalytics fetches the request-timing analytics for a site over a window,
// scoped to a worktree branch when given.
export async function loadSiteAnalytics(
  domain: string,
  range: TimeRange,
  branch = ''
): Promise<Analytics> {
  const params = new URLSearchParams({ range });
  if (branch) params.set('branch', branch);
  return apiJson<Analytics>(`/api/sites/${encodeURIComponent(domain)}/analytics?${params.toString()}`);
}

// removeRecorded drops recorded history: one request when atMillis is given (how
// the recent list identifies a row), otherwise the route's whole history. Setting
// exclude also stops lerd recording the route from here on.
export async function removeRecorded(
  domain: string,
  body: { route: string; branch?: string; at_millis?: number; uri?: string; exclude?: boolean }
): Promise<void> {
  const res = await apiFetch(`/api/sites/${encodeURIComponent(domain)}/analytics/remove`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  if (!res.ok) throw new Error((await res.text()) || res.statusText);
}

// unexcludeRoute puts a silenced route back under observation. Its old requests
// were never recorded, so it reappears only as new traffic arrives.
export async function unexcludeRoute(domain: string, route: string, branch = ''): Promise<void> {
  const params = new URLSearchParams({ route });
  if (branch) params.set('branch', branch);
  const res = await apiFetch(
    `/api/sites/${encodeURIComponent(domain)}/analytics/excludes?${params.toString()}`,
    { method: 'DELETE' }
  );
  if (!res.ok) throw new Error((await res.text()) || res.statusText);
}
