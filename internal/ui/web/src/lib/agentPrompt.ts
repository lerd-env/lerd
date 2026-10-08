import type { RecentRequest } from '$stores/analytics';

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
