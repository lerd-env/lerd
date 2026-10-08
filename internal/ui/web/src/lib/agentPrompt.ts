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
