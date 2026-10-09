import { m } from '../paraglide/messages.js';

// relativeTime says how long before ref (both epoch ms) something happened, the
// short way the dashboard does: "just now", "5m ago", "3h ago", "2d ago".
export function relativeTime(at: number, ref: number): string {
  const diff = Math.max(0, ref - at);
  if (diff < 30_000) return m.time_now();
  const mins = Math.floor(diff / 60_000);
  if (mins < 60) return m.time_min_ago({ n: Math.max(1, mins) });
  const hours = Math.floor(mins / 60);
  if (hours < 24) return m.time_hour_ago({ n: hours });
  return m.time_day_ago({ n: Math.floor(hours / 24) });
}
