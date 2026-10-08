import type { RecentRequest } from '$stores/analytics';
import type { AppLogEntry } from '$stores/appLogs';

// The prompt carries only what finds the request; the assistant reads the
// lenses themselves through lerd's MCP, so nothing pasted goes stale or gets cut.
export function requestAgentPrompt(domain: string, req: Pick<RecentRequest, 'method' | 'uri' | 'status'>, rid: string): string {
  return (
    `On my lerd site ${domain}, the request ${req.method} ${req.uri} returned ${req.status}. ` +
    `Read what happened in it with lerd's \`request\` tool (action "lenses", rid "${rid}"), ` +
    `then find and fix the root cause of the issues it reports.`
  );
}

// target is how site_doctor finds the checkout: `site "<domain>"`, or
// `path "<dir>"` for a worktree, which the site lookup would not reach.
export function doctorAgentPrompt(domain: string, target: string, checks: string[]): string {
  return (
    `On my lerd site ${domain}, the site doctor reports these checks not passing: ${checks.join(', ')}. ` +
    `Read them with lerd's \`diag\` tool (action "site_doctor", ${target}), ` +
    `fix their root causes, then run it again to confirm they pass.`
  );
}

// Only the entry's first line, cut short: it is there so the assistant can
// find the entry in the file. until ends the fetch at it, so it arrives last with what
// led up to it; since is exclusive and would drop the entry's own second.
export function logAgentPrompt(domain: string, target: string, file: string, entry: AppLogEntry): string {
  const first = (entry.message ?? '').split('\n')[0];
  const quote = first.length > 200 ? first.slice(0, 200) + '…' : first;
  const when = [entry.level, entry.date && `at ${entry.date}`].filter(Boolean).join(' ');
  return (
    `On my lerd site ${domain}, the app log ${file} has this entry${when ? ` (${when})` : ''}: "${quote}". ` +
    `Read it and what led up to it with lerd's \`logs\` tool (action "fetch", source "app:${file}", ${target}${entry.date ? `, until "${entry.date}"` : ''}), ` +
    `then find and fix the root cause.`
  );
}
