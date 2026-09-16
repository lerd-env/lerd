// The logs lerd streams are plain text: fpm, nginx, dnsmasq, the watcher and
// any worker that isn't colouring its own output all write a level word and
// nothing else. One vocabulary covers them, so every viewer reads the same.
const RED = 'text-red-500';
const YELLOW = 'text-yellow-600 dark:text-yellow-400';
const BLUE = 'text-blue-600 dark:text-blue-400';
const GREEN = 'text-emerald-600 dark:text-emerald-500';

export function logHighlight(line: string): string | null {
  // PHP's own severities outrank the level word next to them: a PHP Warning is
  // a bug in the site, while an fpm WARNING is housekeeping.
  if (/PHP (Fatal|Parse)|Fatal error|PHP Warning/.test(line)) return RED;
  if (/PHP (Notice|Deprecated)/.test(line)) return YELLOW;
  if (/\b(error|crit|critical|alert|emerg|fatal|panic)\b/i.test(line)) return RED;
  if (/\bwarn(ing)?\b/i.test(line)) return YELLOW;
  if (/\bnotice\b/i.test(line)) return BLUE;
  // Access lines end the request in quotes and follow it with the status.
  const status = line.match(/"\s+(\d{3})\b/);
  if (status) {
    const code = Number(status[1]);
    if (code >= 500) return RED;
    if (code >= 400) return YELLOW;
    return GREEN;
  }
  return null;
}
