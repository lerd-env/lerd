<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import { loadRouteQueries, type RouteQueries } from '$stores/analytics';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    domain: string;
    route: string;
    branch?: string;
    open: boolean;
    onclose: () => void;
  }
  let { domain, route, branch = '', open, onclose }: Props = $props();

  let data = $state<RouteQueries | null>(null);
  let loading = $state(false);

  $effect(() => {
    if (!open || !route) return;
    const d = domain;
    const rt = route;
    const b = branch;
    loading = true;
    data = null;
    loadRouteQueries(d, rt, b)
      .then((r) => {
        if (d === domain && rt === route && b === branch) data = r;
      })
      .catch(() => {
        if (d === domain && rt === route && b === branch) data = null;
      })
      .finally(() => {
        if (d === domain && rt === route && b === branch) loading = false;
      });
  });

  function fmtMs(ms: number): string {
    if (ms >= 1000) return (ms / 1000).toFixed(1).replace(/\.0$/, '') + ' s';
    return Math.round(ms) + ' ms';
  }
  const hasEvidence = $derived((data?.evidence?.length ?? 0) > 0);
</script>

<Modal {open} {onclose} title={m.sites_timing_queriesFor({ route })} size="lg">
  {#if loading}
    <div class="py-10 text-center text-xs text-gray-400 dark:text-gray-500">{m.common_loading()}</div>
  {:else if !hasEvidence}
    <div class="py-10 text-center text-xs text-gray-500 dark:text-gray-400 leading-relaxed">
      {m.sites_timing_queriesNone()}
    </div>
  {:else if data}
    <div class="space-y-3">
      {#each data.evidence as ra (ra.rid ?? ra.request)}
        <div class="rounded-lg border border-gray-200 dark:border-lerd-border overflow-hidden">
          <div class="flex items-center justify-between gap-3 px-3 py-2 bg-gray-50 dark:bg-white/[0.03] border-b border-gray-100 dark:border-lerd-border">
            <span class="font-mono text-[11px] text-gray-700 dark:text-gray-200 truncate">{ra.request}</span>
            <span class="shrink-0 flex items-center gap-2 text-[11px] tabular-nums">
              <span class="text-gray-500 dark:text-gray-400">{m.sites_timing_queriesN({ n: ra.query_count })}</span>
              <span class="font-semibold {ra.total_time_ms >= 100 ? 'text-amber-600 dark:text-amber-400' : 'text-gray-600 dark:text-gray-300'}">{fmtMs(ra.total_time_ms)}</span>
            </span>
          </div>

          {#if ra.n_plus_one && ra.n_plus_one.length > 0}
            <div class="px-3 py-2 space-y-1.5">
              {#each ra.n_plus_one as f (f.fingerprint)}
                <div class="text-[11px]">
                  <div class="flex items-center gap-2">
                    <span class="shrink-0 font-semibold text-red-600 dark:text-red-400">{m.sites_timing_nPlusOne({ n: f.count })}</span>
                    <span class="text-gray-400 dark:text-gray-500 tabular-nums">{fmtMs(f.total_time_ms)}</span>
                    {#if f.caller.file}
                      <span class="text-gray-400 dark:text-gray-500 font-mono truncate">{f.caller.file}:{f.caller.line}</span>
                    {/if}
                  </div>
                  <pre class="mt-0.5 font-mono text-[10.5px] text-gray-600 dark:text-gray-300 whitespace-pre-wrap break-all">{f.sample_sql}</pre>
                </div>
              {/each}
            </div>
          {/if}

          {#if ra.slow && ra.slow.length > 0}
            <div class="px-3 py-2 space-y-1.5 {ra.n_plus_one && ra.n_plus_one.length > 0 ? 'border-t border-gray-100 dark:border-lerd-border' : ''}">
              {#each ra.slow as f, i (i)}
                <div class="text-[11px]">
                  <div class="flex items-center gap-2">
                    <span class="shrink-0 font-semibold text-amber-600 dark:text-amber-400">{m.sites_timing_slowQuery()}</span>
                    <span class="text-gray-400 dark:text-gray-500 tabular-nums">{fmtMs(f.time_ms)}</span>
                    {#if f.caller.file}
                      <span class="text-gray-400 dark:text-gray-500 font-mono truncate">{f.caller.file}:{f.caller.line}</span>
                    {/if}
                  </div>
                  <pre class="mt-0.5 font-mono text-[10.5px] text-gray-600 dark:text-gray-300 whitespace-pre-wrap break-all">{f.sql}</pre>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</Modal>
