<script lang="ts">
  import KeyValueTable from './KeyValueTable.svelte';
  import Icon from './Icon.svelte';
  import { m } from '../paraglide/messages.js';
  import { bytes } from '$lib/requestWaterfall';

  // One outgoing request a page or job made: a row with its method, URL,
  // status and time, opening into the phases it spent and both ends' headers.

  interface Timing {
    dns?: number;
    connect?: number;
    tls?: number;
    sent?: number;
    first_byte?: number;
  }
  interface Call {
    method?: string;
    url?: string;
    status?: number;
    reason?: string;
    failed?: boolean;
    time_ms?: number;
    timing?: Timing;
    request_size?: number;
    response_size?: number;
    request_headers?: Record<string, string>;
    response_headers?: Record<string, string>;
  }
  interface Props {
    call: Call;
    tone: string;
    // Where the call ran within its request, as fractions of the request's
    // time, drawn as a faint line under the row.
    from?: number;
    to?: number;
  }
  let { call, tone, from, to }: Props = $props();
  let open = $state(false);

  const fmt = (n: number) => `${n < 1 ? '<1' : n < 10 ? n.toFixed(1) : Math.round(n)} ms`;
  // curl reports each phase as time since the start, so each segment is the
  // gap to the phase before it.
  const phases = $derived.by(() => {
    const t = call.timing ?? {};
    const total = call.time_ms ?? 0;
    const marks: Array<[string, number | undefined, string]> = [
      [m.http_phase_dns(), t.dns, 'bg-slate-400'],
      [m.http_phase_connect(), t.connect, 'bg-amber-400'],
      [m.http_phase_tls(), t.tls, 'bg-violet-400'],
      [m.http_phase_wait(), t.first_byte, 'bg-sky-500'],
      [m.http_phase_download(), total || undefined, 'bg-emerald-500']
    ];
    let prev = 0;
    const out: Array<{ label: string; took: number; bar: string }> = [];
    for (const [label, at, bar] of marks) {
      if (at === undefined || at <= prev) continue;
      out.push({ label, took: at - prev, bar });
      prev = at;
    }
    return out;
  });
  const span = $derived(phases.reduce((n, p) => n + p.took, 0));
  const hasDetail = $derived(phases.length > 0 || Boolean(call.reason) || call.response_size !== undefined || Object.keys(call.request_headers ?? {}).length > 0 || Object.keys(call.response_headers ?? {}).length > 0);
</script>

<div class="border-t first:border-t-0 border-gray-100 dark:border-lerd-border/60">
  <button
    type="button"
    class="relative w-full px-3 py-2 grid grid-cols-[1rem_4rem_minmax(0,1fr)_3rem_4rem] gap-3 items-center text-left {hasDetail ? 'hover:bg-gray-50 dark:hover:bg-white/[0.03]' : 'cursor-default'}"
    aria-expanded={hasDetail ? open : undefined}
    disabled={!hasDetail}
    onclick={() => (open = !open)}
  >
    <span class="text-gray-400">{#if hasDetail}<Icon name="chevron" class="w-3 h-3 transition-transform {open ? '' : '-rotate-90'}" />{/if}</span>
    <span class="font-mono">{call.method}</span>
    <span class="font-mono break-all">{call.url}</span>
    <span class="font-mono text-right {tone}">{call.failed && !call.status ? '✕' : call.status}</span>
    <span class="font-mono text-right tabular-nums text-gray-500 dark:text-gray-400">{call.time_ms ? fmt(call.time_ms) : ''}</span>
    {#if from !== undefined && to !== undefined && to > from}
      <span class="absolute left-0 right-0 bottom-0 h-0.5 pointer-events-none" aria-hidden="true">
        <span class="absolute inset-y-0 rounded-full bg-purple-400/50" style="left: {from * 100}%; width: {Math.max((to - from) * 100, 0.6)}%"></span>
      </span>
    {/if}
  </button>
  {#if open}
    <div class="px-3 pb-3 pl-8 space-y-2">
      {#if phases.length}
        <div class="space-y-1.5">
          <div class="flex h-2 rounded-sm overflow-hidden">
            {#each phases as p (p.label)}
              <span class="{p.bar} h-full" style="width: {(p.took / span) * 100}%"></span>
            {/each}
          </div>
          <div class="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-gray-500 dark:text-gray-400">
            {#each phases as p (p.label)}
              <span class="inline-flex items-center gap-1.5"><span class="w-2 h-2 rounded-sm {p.bar}"></span>{p.label} <span class="tabular-nums text-gray-700 dark:text-gray-200">{fmt(p.took)}</span></span>
            {/each}
          </div>
        </div>
      {/if}
      {#if call.status || call.failed || call.request_size !== undefined || call.response_size !== undefined}
        <div class="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-gray-500 dark:text-gray-400">
          <span>{m.http_status()} <span class="font-mono {tone}">{call.status || '✕'}{call.reason ? ` ${call.reason}` : call.failed && !call.status ? ` ${m.http_failed()}` : ''}</span></span>
          {#if call.request_size !== undefined}<span>{m.http_sentSize()} <span class="tabular-nums text-gray-700 dark:text-gray-200">{bytes(call.request_size)}</span></span>{/if}
          {#if call.response_size !== undefined}<span>{m.http_received()} <span class="tabular-nums text-gray-700 dark:text-gray-200">{bytes(call.response_size)}</span></span>{/if}
        </div>
      {/if}
      <KeyValueTable title={m.http_requestHeaders()} values={call.request_headers} />
      <KeyValueTable title={m.http_responseHeaders()} values={call.response_headers} />
    </div>
  {/if}
</div>
