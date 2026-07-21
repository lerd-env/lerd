import type { DumpEvent } from '$lib/dumpsStream';

// Shared request-grouping primitives for every Debug lens (dumps, queries,
// jobs, views, …). Centralised so the branch rule can't drift between the
// per-lens stores again: events from a git worktree share the parent site's
// `ctx.site` and are told apart only by `ctx.branch`, so both the group key
// and the label have to fold the branch in.

// groupKey buckets events into one request: the per-request id when the
// lerd_devtools extension supplied one, else method+path+pid for web requests,
// else a 5s pid bucket for CLI invocations. site and branch lead both fallback
// keys so a worktree request never merges into the parent site's request.
//
// `dump` is the exception: dump()/dd() can fire without the extension (the
// pure-PHP bridge), and the collector's rid() then falls back to a fresh id
// per call, so trusting it would split one request's dumps into a card each.
// Group dumps by request instead. Every other kind is extension-emitted, so
// its rid is always the stable per-request one.
export function groupKey(ev: DumpEvent): string {
  if (ev.ctx.rid && ev.kind !== 'dump') return `rid:${ev.ctx.rid}`;
  const site = ev.ctx.site ?? '';
  const branch = ev.ctx.branch ?? '';
  if (ev.ctx.type === 'fpm') {
    return `fpm:${site}:${branch}:${ev.ctx.request ?? ''}:${ev.ctx.pid ?? ''}`;
  }
  const bucket = Math.floor(new Date(ev.ts).getTime() / 5000);
  return `cli:${site}:${branch}:${ev.ctx.pid ?? ''}:${bucket}`;
}

// GroupLabel is a group header split into its parts rather than one string,
// so the renderer can turn the site name into a link to that site's Debug tab
// while the rest stays plain text. site is empty when the surrounding UI
// already establishes it, or when the event carried no site at all.
export interface GroupLabel {
  site: string;
  branch: string;
  text: string;
}

// groupLabel describes the header of one request group. The bracketed chunk
// reads `[site@branch]`, or `[site]`, or `[branch]` alone when hideSitePrefix
// drops the site (within one site the branch is the only thing telling
// worktree events apart from the parent).
export function groupLabel(ev: DumpEvent, hideSitePrefix: boolean): GroupLabel {
  return {
    site: hideSitePrefix ? '' : (ev.ctx.site ?? ''),
    branch: ev.ctx.branch ?? '',
    text: labelText(ev)
  };
}

function labelText(ev: DumpEvent): string {
  if (ev.ctx.worker) return ev.ctx.worker;
  if (ev.ctx.type === 'fpm') return ev.ctx.request || '(request)';
  return `cli (pid ${ev.ctx.pid ?? '?'})`;
}

// labelString flattens a GroupLabel back to the single-string form, for
// callers that need one string rather than the rendered parts.
export function labelString(l: GroupLabel): string {
  const inner = l.site ? (l.branch ? `${l.site}@${l.branch}` : l.site) : l.branch;
  return inner ? `[${inner}] ${l.text}` : l.text;
}
