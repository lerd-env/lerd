<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { startDumpsStream, stopDumpsStream, clearDumps } from '$stores/dumps';
  import { debugEvents } from '$stores/debugEvents';
  import { loadRequests, nestRequests, type RequestSummary } from '$stores/requests';
  import EmptyState from '$components/EmptyState.svelte';
  import RequestDetail from '$components/RequestDetail.svelte';
  import Modal from '$components/Modal.svelte';
  import { tooltip } from '$lib/tooltip';
  import Icon from '$components/Icon.svelte';
  import ClassName from '$components/ClassName.svelte';
  import { m } from '../paraglide/messages.js';

  // Every request lerd saw, one row each, with the requests a page sent listed
  // under it; opening one shows everything it did.

  interface Props {
    siteScope?: string;
  }
  let { siteScope = '' }: Props = $props();

  let list = $state<RequestSummary[]>([]);
  let selected = $state('');
  let search = $state('');
  let kind = $state<'all' | 'pages' | 'api' | 'cli'>('all');
  let onlyProblems = $state(false);
  // The dashboard keeps a mobile and a desktop copy of every view mounted, so
  // only the copy on screen fetches.
  let root = $state<HTMLElement | null>(null);

  async function refresh() {
    if (root && root.offsetParent === null) return;
    try {
      list = await loadRequests(siteScope);
    } catch {
      /* keep the last list */
    }
  }

  // New events can change any request, so the list follows the stream, at
  // most a couple of times a second.
  let timer: ReturnType<typeof setTimeout> | null = null;
  const unsub = debugEvents.subscribe(() => {
    if (timer) return;
    timer = setTimeout(() => {
      timer = null;
      void refresh();
    }, 500);
  });
  onMount(() => {
    startDumpsStream();
    void refresh();
  });
  onDestroy(() => {
    unsub();
    stopDumpsStream();
    if (timer) clearTimeout(timer);
  });

  const KIND: Record<string, (r: RequestSummary) => boolean> = {
    all: () => true,
    pages: (r) => r.type === 'page',
    api: (r) => r.type === 'fetch' || r.type === 'xhr' || r.type === 'request',
    cli: (r) => r.type === 'cli' || r.type === 'worker' || r.type === 'job'
  };
  const rows = $derived.by(() => {
    const needle = search.toLowerCase();
    const keep = list.filter(
      (r) =>
        KIND[kind](r) &&
        (!onlyProblems || r.problems.length > 0) &&
        (!needle || `${r.method ?? ''} ${r.uri ?? ''} ${r.command ?? ''} ${r.worker ?? ''} ${r.job ?? ''}`.toLowerCase().includes(needle))
    );
    return nestRequests(keep);
  });
  const problemCount = $derived(list.filter((r) => r.problems.length > 0).length);

  const METHOD: Record<string, string> = {
    GET: 'text-emerald-600 dark:text-emerald-300',
    POST: 'text-sky-600 dark:text-sky-300',
    PUT: 'text-amber-600 dark:text-amber-300',
    PATCH: 'text-amber-600 dark:text-amber-300',
    DELETE: 'text-rose-600 dark:text-rose-300'
  };
  function statusTone(s?: number): string {
    if (!s) return 'text-gray-400';
    if (s >= 500) return 'text-rose-600 dark:text-rose-300';
    if (s >= 400) return 'text-amber-600 dark:text-amber-300';
    return 'text-emerald-600 dark:text-emerald-300';
  }
  const CHIP = 'text-[11px] px-1.5 py-px rounded-full whitespace-nowrap';
  const BAD = 'bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300';
  const WARN = 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300';
  const PLAIN = 'bg-gray-100 dark:bg-white/10 text-gray-600 dark:text-gray-300';
  function problemTone(p: string): string {
    return p === 'N+1' || p === 'slow query' || p === '4xx' ? WARN : BAD;
  }
  function countChips(r: RequestSummary): string[] {
    const c = r.counts;
    const out: string[] = [];
    const order: Array<[string, (n: number) => string]> = [
      ['query', (n) => m.requests_count_queries({ count: n })],
      ['dump', (n) => m.requests_count_dumps({ count: n })],
      ['log', (n) => m.requests_count_logs({ count: n })],
      ['component', (n) => m.requests_count_components({ count: n })],
      ['mail', (n) => m.requests_count_mail({ count: n })],
      ['job', (n) => m.requests_count_jobs({ count: n })],
      ['http', (n) => m.requests_count_http({ count: n })]
    ];
    for (const [k, f] of order) if (c[k]) out.push(f(c[k]));
    return out;
  }
  function localTime(ts: string): string {
    const d = new Date(ts);
    return isNaN(d.getTime()) ? ts : d.toLocaleTimeString();
  }
  function label(r: RequestSummary): string {
    if (r.type === 'job') return r.job ?? 'job';
    if (r.type === 'cli' || r.type === 'worker') return r.worker || r.command || 'cli';
    return r.uri || r.rid;
  }
</script>

<Modal open={selected !== ''} title={m.requests_detail_title()} size="full" onclose={() => (selected = '')}>
  {#key selected}<RequestDetail rid={selected} onopen={(rid) => (selected = rid)} />{/key}
</Modal>

<div bind:this={root} class="flex flex-col h-full overflow-hidden">
    <div class="flex items-center gap-2 px-3 py-3 border-b border-gray-200 dark:border-lerd-border flex-wrap">
      <input class="text-xs px-2 py-1 rounded-sm border border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card flex-1 min-w-[140px]" placeholder={m.requests_search()} bind:value={search} />
      <div role="group" aria-label={m.requests_kind()} class="flex rounded-sm border border-gray-300 dark:border-lerd-border overflow-hidden text-xs">
        {#each [['all', m.requests_kind_all()], ['pages', m.requests_kind_pages()], ['api', m.requests_kind_api()], ['cli', m.requests_kind_cli()]] as [id, text] (id)}
          <button type="button" aria-pressed={kind === id} onclick={() => (kind = id as typeof kind)} class="px-2 py-1 border-l first:border-l-0 border-gray-300 dark:border-lerd-border {kind === id ? 'bg-gray-100 dark:bg-white/10 text-gray-800 dark:text-gray-100' : 'text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-white/5'}">{text}</button>
        {/each}
      </div>
      <label class="inline-flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400 cursor-pointer select-none">
        <input type="checkbox" class="rounded-sm border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card text-lerd-red focus:ring-lerd-red" bind:checked={onlyProblems} />
        {m.requests_onlyProblems()}
      </label>
      <button type="button" class="text-xs rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5" onclick={async () => { await clearDumps(); list = []; }}>{m.common_clear()}</button>
    </div>

    <div class="flex-1 overflow-y-auto">
      {#if rows.length === 0}
        <EmptyState title={m.debug_waiting_title()}>
          {#snippet hint()}{m.requests_waiting()}{/snippet}
        </EmptyState>
      {:else}
        <ul class="divide-y divide-gray-100 dark:divide-lerd-border/60">
          {#each rows as r (r.rid)}
            <li>
              <button type="button" onclick={() => (selected = r.rid)} class="w-full text-left grid grid-cols-[3.5rem_minmax(0,1fr)_3rem_4.5rem] sm:grid-cols-[3.5rem_minmax(0,1fr)_3rem_4.5rem_minmax(0,18rem)_4.5rem] gap-3 items-center px-3 py-2 text-xs hover:bg-gray-50 dark:hover:bg-white/5 {r.depth ? 'bg-gray-50/60 dark:bg-white/[0.02]' : ''}">
                <span class="font-mono text-[11px] {METHOD[r.method ?? ''] ?? 'text-violet-600 dark:text-violet-300'}">{r.type === 'job' ? 'JOB' : r.type === 'cli' || r.type === 'worker' ? 'CLI' : (r.method ?? '·')}</span>
                <span class="flex items-center gap-2 min-w-0 {r.depth ? 'pl-5' : ''}">
                  {#if r.depth}<span aria-hidden="true" class="text-gray-300 dark:text-gray-600">└</span>{/if}
                  <ClassName value={label(r)} class="font-mono truncate text-gray-800 dark:text-gray-100" />
                  {#if r.operation}<span class="font-mono truncate text-[11px] text-pink-700 dark:text-pink-300" title={(r.operations ?? []).join('\n')}>{r.operation}</span>{/if}
                  <span class="{CHIP} {PLAIN}">{r.type}</span>
                  {#if r.operation}<span class="shrink-0 text-pink-600 dark:text-pink-300" use:tooltip={'GraphQL'} aria-label="GraphQL"><Icon name="graphql" class="w-3.5 h-3.5" /></span>{/if}
                  {#if r.parent?.cross_origin && r.parent.site}<span class="{CHIP} bg-sky-100 dark:bg-sky-900/40 text-sky-700 dark:text-sky-300">{m.requests_from({ site: r.parent.site })}</span>{/if}
                </span>
                {#if r.type === 'job'}
                  <span class="font-mono text-[11px] truncate {r.job_status === 'failed' || r.job_status === 'errored' ? 'text-rose-600 dark:text-rose-300' : 'text-emerald-600 dark:text-emerald-300'}">{r.job_status ?? '·'}</span>
                {:else}
                  <span class="font-mono {statusTone(r.status)}">{r.status || '·'}</span>
                {/if}
                <span class="font-mono text-gray-600 dark:text-gray-300">{r.time_ms ? `${Math.round(r.time_ms)} ms` : '·'}</span>
                <span class="hidden sm:flex gap-1 flex-wrap">
                  {#each r.problems as p (p)}<span class="{CHIP} {problemTone(p)}">{p}</span>{/each}
                  {#each countChips(r) as c (c)}<span class="{CHIP} {PLAIN}">{c}</span>{/each}
                </span>
                <span class="hidden sm:block text-right font-mono text-[11px] text-gray-400">{localTime(r.started)}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
    <div class="px-3 py-2 border-t border-gray-200 dark:border-lerd-border text-[11px] text-gray-500 dark:text-gray-400">
      {m.requests_footer({ count: list.length, problems: problemCount })}
    </div>
  </div>
