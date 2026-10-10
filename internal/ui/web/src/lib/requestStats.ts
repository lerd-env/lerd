import type { DumpEvent } from './dumpEvent';

// RequestStats is what one request cost, in milliseconds and bytes. Every field
// but response is absent when nothing captured it.
export interface RequestStats {
  response: number;
  memory?: number;
  app?: number;
  database?: number;
  queue?: number;
}

type Timed = { time_ms?: number; memory_peak?: number; queue_ms?: number };
const round = (n: number) => Math.round(n * 1000) / 1000;

// requestStats splits a request's response time, as nginx measured it, using
// the summary PHP reports as the request ends and the queries it ran.
export function requestStats(events: DumpEvent[], response: number): RequestStats {
  const stats: RequestStats = { response };
  const queries = events.filter((ev) => ev.kind === 'query');
  const database = round(queries.reduce((sum, ev) => sum + Number((ev.data as Timed)?.time_ms ?? 0), 0));
  const php = events.find((ev) => ev.kind === 'request')?.data as Timed | undefined;
  if (php?.memory_peak) stats.memory = php.memory_peak;
  if (php?.time_ms !== undefined) stats.app = round(Math.max(0, php.time_ms - database));
  if (queries.length) stats.database = database;
  if (php?.queue_ms !== undefined) stats.queue = php.queue_ms;
  return stats;
}
