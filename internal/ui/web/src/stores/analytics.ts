import { apiJson } from '$lib/api';

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
