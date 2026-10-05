// When something happened within a request: how long after it started, and
// the time of day, for a call the page sent or an event the request ran.

export function sinceStart(ts: string | undefined, started: string | undefined): string {
  const n = Date.parse(ts ?? '') - Date.parse(started ?? '');
  if (isNaN(n)) return '';
  const ms = Math.max(n, 0);
  return ms < 1000 ? `+${ms} ms` : `+${(ms / 1000).toFixed(ms < 10000 ? 2 : 1)} s`;
}

// clock is the time of day on a 24-hour clock to the millisecond, so calls a
// page sent close together still read in the order they went out, and the
// time fits one line.
export function clock(ts: string | undefined): string {
  const d = new Date(ts ?? '');
  return isNaN(d.getTime()) ? '' : d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3, hourCycle: 'h23' });
}
