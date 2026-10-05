<script lang="ts">
  import HttpCall from './HttpCall.svelte';
  import { loadRequest, type RequestDetail, type QueryFinding } from '$stores/requests';
  import { buildWaterfall } from '$lib/requestWaterfall';
  import { inlineBindings } from '$lib/sqlInline';
  import { highlight } from '$lib/highlight';
  import { formatSql } from '$lib/sqlFormat';
  import { sinceStart, clock } from '$lib/requestTime';
  import type { DumpEvent } from '$lib/dumpsStream';
  import DetailTabs, { type TabItem } from './DetailTabs.svelte';
  import RequestTimeline from './RequestTimeline.svelte';
  import RequestStats from './RequestStats.svelte';
  import KeyValueTable from './KeyValueTable.svelte';
  import CopyButton from './CopyButton.svelte';
  import TraceBlock from './TraceBlock.svelte';
  import SourcePath from './SourcePath.svelte';
  import Icon from './Icon.svelte';
  import ClassName from './ClassName.svelte';
  import GraphQLType, { type GraphQLTypeInfo } from './GraphQLType.svelte';
  import CallerSource from './CallerSource.svelte';
  import StructuredValue from './StructuredValue.svelte';
  import CustomBlocks from './CustomBlocks.svelte';
  import { m } from '../paraglide/messages.js';

  // One request and everything that carried its id, a tab per kind of work,
  // with the page view that sent it and the requests it sent one click away.

  interface Props {
    rid: string;
    onopen: (rid: string) => void;
    // initialTab is the tab it opens on, the one a debug bar chip names.
    initialTab?: string;
  }
  let { rid, onopen, initialTab = 'performance' }: Props = $props();

  let d = $state<RequestDetail | null>(null);
  let missing = $state(false);
  // svelte-ignore state_referenced_locally
  let tab = $state(initialTab);
  let hiddenLevels = $state<Record<string, boolean>>({ debug: true });
  let querySearch = $state('');
  // Queries read on one line by default; formatted, each clause gets its own,
  // and the choice is remembered.
  let sqlFormatted = $state(readSqlFormat());
  function readSqlFormat(): boolean {
    try {
      return localStorage.getItem('lerd:sql-format') === 'on';
    } catch {
      return false;
    }
  }
  function toggleSqlFormat() {
    sqlFormatted = !sqlFormatted;
    try {
      localStorage.setItem('lerd:sql-format', sqlFormatted ? 'on' : 'off');
    } catch {}
  }

  async function load(id: string) {
    d = null;
    missing = false;
    try {
      d = await loadRequest(id);
    } catch {
      missing = true;
    }
  }
  $effect(() => {
    void load(rid);
  });

  const ev = (kind: string): DumpEvent[] => d?.events?.[kind] ?? [];
  const data = (e: DumpEvent) => (e.data ?? {}) as Record<string, any>;
  const ms = (n: number) => `${n < 1 ? '<1' : n < 10 ? n.toFixed(1) : Math.round(n)} ms`;

  const waterfall = $derived(d ? buildWaterfall(d, { dns: m.requests_phase_dns(), connect: m.requests_phase_connect(), wait: m.requests_phase_wait(), download: m.requests_phase_download(), dom: m.requests_phase_dom(), load: m.requests_phase_load(), bindings: m.requests_popover_bindings(), queries: m.requests_section_queries(), details: m.requests_popover_details(), state: m.requests_popover_state(), source: m.requests_popover_source(), timing: m.requests_popover_timing(), requestHeaders: m.http_requestHeaders(), responseHeaders: m.http_responseHeaders() }) : null);
  const http = $derived(data(ev('request')[0] ?? ({} as DumpEvent)));
  // Each route parameter as the request asked for it, masked where the rules
  // say, and the model it resolved to where one did.
  const routeParams = $derived(Object.entries((http.route_params ?? {}) as Record<string, { value?: string; model?: string; field?: string; key?: string; file?: string; line?: number }>));
  // The GraphQL operations the request body held, one per entry of a batch.
  const graphql = $derived(
    (http.graphql ?? []) as {
      type: string;
      name?: string;
      query: string;
      variables?: Record<string, unknown>;
      fields?: { alias?: string; name: string; type?: string; file?: string; line?: number; type_file?: string; type_line?: number; args?: Record<string, unknown>; data?: unknown }[];
      errors?: { message?: string; path?: (string | number)[] }[];
    }[]
  );
  const graphqlTypes = $derived((http.graphql_types ?? {}) as Record<string, GraphQLTypeInfo>);
  const spans = $derived(ev('span').map(data));
  const controller = $derived(spans.find((s) => s.label === 'Controller')?.name ?? '');
  // Who the request ran as, when the app or its framework said.
  const authUser = $derived(ev('auth').map(data)[0]);
  const middleware = $derived(ev('middleware').map(data)[0] as { global?: string[]; route?: string[]; sources?: Record<string, { file: string; line?: number }> } | undefined);
  const queries = $derived(ev('query'));
  const dbTime = $derived(queries.reduce((n, e) => n + Number(data(e).time_ms ?? 0), 0));
  const pageTiming = $derived(ev('browser').map(data).find((b) => b.type === 'timing')?.timing as Record<string, number> | undefined);
  const findings = $derived<QueryFinding[]>([...(d?.queries?.n_plus_one ?? []), ...(d?.queries?.slow ?? [])]);
  const slowSql = $derived(new Set((d?.queries?.slow ?? []).map((f) => f.sql)));
  const verbs = $derived.by(() => {
    const c: Record<string, number> = { select: 0, insert: 0, update: 0, delete: 0 };
    for (const e of queries) {
      const v = String(data(e).sql ?? '').trim().split(/\s/)[0].toLowerCase();
      if (v in c) c[v]++;
    }
    return c;
  });
  // Long lists render a page at a time; nothing is dropped, the rest is a click away.
  const PAGE = 200;
  let limits = $state<Record<string, number>>({});
  const cap = <T,>(key: string, list: T[]): T[] => list.slice(0, limits[key] ?? PAGE);
  // A query keeps the number of its place in the run when the list is filtered.
  const queryNo = $derived(new Map(queries.map((e, i) => [e.id, i + 1])));
  // An N+1 finding, once picked, narrows the list to the queries it repeats.
  let only = $state<QueryFinding | null>(null);
  const onlyIds = $derived(new Set(only?.ids ?? []));
  const shownQueries = $derived(queries.filter((e) => (!only || onlyIds.has(e.id)) && (!querySearch || String(data(e).sql ?? '').toLowerCase().includes(querySearch.toLowerCase()))));

  const logs = $derived(ev('log'));
  // Lines an app marked for the Performance tab through lerd/debug.
  const performanceNotes = $derived(logs.filter((e) => data(e).performance));
  const models = $derived.by(() => {
    const total: Record<string, Record<string, number>> = {};
    for (const e of ev('models')) {
      for (const [model, counts] of Object.entries((data(e).models ?? {}) as Record<string, Record<string, number>>)) {
        const row = (total[model] ??= {});
        for (const [action, n] of Object.entries(counts)) row[action] = (row[action] ?? 0) + Number(n);
      }
    }
    return Object.entries(total);
  });
  const MODEL_ACTIONS = ['retrieved', 'created', 'updated', 'deleted', 'restored'];
  // Where each model the app wrote is declared, so its name opens in the editor.
  const modelSources = $derived(Object.assign({}, ...ev('models').map((e) => (data(e).sources ?? {}) as Record<string, { file: string; line?: number }>)));
  const levels = $derived(Array.from(new Set(logs.map((e) => String(data(e).level ?? '')))));
  const shownLogs = $derived(logs.filter((e) => !hiddenLevels[String(data(e).level ?? '')]));
  // A failed call that reached a PHP request is listed with the requests the
  // page sent, and the load timing is on the timeline, so neither repeats here.
  const browserEvents = $derived(ev('browser').filter((e) => data(e).type !== 'request' && data(e).type !== 'timing' && !(data(e).type === 'network' && data(e).rid)));
  const session = $derived(ev('session').at(-1));
  const viewTime = (name: string) => spans.find((s) => s.label === 'View' && s.name === name)?.time_ms;
  const components = $derived.by(() => {
    const by = new Map<string, Record<string, any>[]>();
    for (const e of ev('component')) {
      const c = data(e);
      by.set(c.name, [...(by.get(c.name) ?? []), c]);
    }
    return [...by.entries()];
  });
  const cacheOps = $derived.by(() => {
    const c: Record<string, number> = {};
    for (const e of ev('cache')) c[data(e).op] = (c[data(e).op] ?? 0) + 1;
    return c;
  });
  const mails = $derived([...ev('mail'), ...ev('message')]);
  // Tabs the app built, one per id, its blocks in the order they were added.
  const customTabs = $derived.by(() => {
    const by = new Map<string, { title: string; columns: number; placement?: { position: 'before' | 'after'; tab: string }; blocks: Array<Record<string, any>> }>();
    for (const e of [...ev('tab')].sort((a, b) => Number(data(a).seq ?? 0) - Number(data(b).seq ?? 0))) {
      const t = data(e);
      const tab = by.get(t.id) ?? { title: String(t.title), columns: 1, blocks: [] as Array<Record<string, any>> };
      tab.columns = Number(t.columns ?? tab.columns) || 1;
      if (t.placement) tab.placement = t.placement;
      tab.blocks.push(t.block ?? {});
      by.set(t.id, tab);
    }
    return [...by.entries()].map(([id, t]) => ({ id: `custom:${id}`, ...t }));
  });

  const tabs = $derived<TabItem[]>([
    { id: 'performance', label: m.requests_tab_performance() },
    { id: 'request', label: m.requests_tab_request() },
    { id: 'graphql', label: 'GraphQL', count: graphql.length, hidden: !graphql.length },
    { id: 'exceptions', label: m.requests_section_exceptions(), count: ev('exception').length, hidden: !ev('exception').length },
    { id: 'database', label: m.requests_tab_database(), count: queries.length, hidden: !queries.length },
    { id: 'models', label: m.requests_tab_models(), count: models.length, hidden: !models.length },
    { id: 'views', label: m.requests_tab_views(), count: ev('view').length, hidden: !ev('view').length },
    { id: 'components', label: m.requests_section_components(), count: components.length, hidden: !components.length },
    { id: 'cache', label: m.requests_tab_cache(), count: ev('cache').length, hidden: !ev('cache').length },
    { id: 'redis', label: m.requests_layer_redis(), count: ev('redis').length, hidden: !ev('redis').length },
    { id: 'filesystem', label: m.requests_layer_filesystem(), count: ev('filesystem').length, hidden: !ev('filesystem').length },
    { id: 'events', label: m.requests_tab_events(), count: ev('event').length, hidden: !ev('event').length },
    { id: 'log', label: m.requests_section_logs(), count: logs.length, hidden: !logs.length },
    { id: 'dumps', label: m.requests_section_dumps(), count: ev('dump').length, hidden: !ev('dump').length },
    { id: 'mail', label: m.requests_tab_mail(), count: mails.length, hidden: !mails.length },
    { id: 'http', label: m.requests_tab_http(), count: ev('http').length, hidden: !ev('http').length },
    { id: 'jobs', label: m.requests_tab_jobs(), count: ev('job').length, hidden: !ev('job').length },
    { id: 'browser', label: m.requests_section_browser(), count: browserEvents.length, hidden: !browserEvents.length },
    { id: 'sent', label: m.requests_section_sent(), count: d?.children?.length ?? 0, hidden: !d?.children?.length }
  ]);
  // Custom tabs go after lerd's own unless one asked for a place beside a
  // built-in tab or another custom tab; an id nothing has stays at the end.
  const allTabs = $derived.by(() => {
    const list: TabItem[] = [...tabs];
    const placed = customTabs.filter((t) => !t.placement);
    for (const t of placed) list.push({ id: t.id, label: t.title });
    for (const t of customTabs.filter((t) => t.placement)) {
      const item = { id: t.id, label: t.title };
      const target = t.placement!.tab;
      const at = list.findIndex((x) => x.id === target || x.id === `custom:${target}`);
      if (at < 0) list.push(item);
      else list.splice(t.placement!.position === 'before' ? at : at + 1, 0, item);
    }
    return list;
  });

  function levelTone(level: string): string {
    if (['emergency', 'alert', 'critical', 'error'].includes(level)) return 'bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300';
    if (['warning', 'notice'].includes(level)) return 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300';
    if (level === 'debug') return 'bg-gray-100 dark:bg-white/10 text-gray-500 dark:text-gray-400';
    return 'bg-sky-100 dark:bg-sky-900/40 text-sky-700 dark:text-sky-300';
  }
  const offset = (ts: string) => (d ? sinceStart(ts, d.started) : '');
  // Where an outgoing call ran within the request, as fractions of its time:
  // it was reported as it finished, so it started its own time before that.
  function httpSpan(ts: string, took: number): [number, number] | undefined {
    const total = d?.time_ms ?? 0;
    if (!d || !total || !took) return undefined;
    const end = Date.parse(ts) - Date.parse(d.started);
    const clamp = (n: number) => Math.min(Math.max(n / total, 0), 1);
    return [clamp(end - took), clamp(end)];
  }
  function statusTone(s?: number): string {
    if (!s) return 'text-gray-400';
    if (s >= 500) return 'text-rose-600 dark:text-rose-300';
    if (s >= 400) return 'text-amber-600 dark:text-amber-300';
    return 'text-emerald-600 dark:text-emerald-300';
  }
  function bytes(n: number): string {
    return n >= 1048576 ? `${(n / 1048576).toFixed(1)} MB` : `${Math.round(n / 1024)} KB`;
  }
  const BOX = 'rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card overflow-hidden text-xs';
  const HEAD = 'px-3 py-2 text-[10px] uppercase tracking-wide text-gray-500 dark:text-gray-400 border-b border-gray-100 dark:border-lerd-border/60';
  const ROW = 'px-3 py-2 border-t first:border-t-0 border-gray-100 dark:border-lerd-border/60';
  const BADGE = 'text-[10px] px-1.5 py-0.5 rounded-sm';
</script>

<div class="flex flex-col flex-1 min-h-0 overflow-hidden">
  <div class="px-5 py-3 border-b border-gray-200 dark:border-lerd-border space-y-2">
    {#if d}
      <div class="flex items-center gap-3 flex-wrap">
        <span class="font-mono text-xs px-2 py-0.5 rounded-sm bg-gray-100 dark:bg-white/10">{d.type === 'job' ? 'JOB' : d.type === 'cli' || d.type === 'worker' ? 'CLI' : (d.method ?? '·')}</span>
        <h2 class="font-mono text-sm font-medium break-all text-gray-900 dark:text-white">{d.type === 'job' ? d.job : d.type === 'cli' || d.type === 'worker' ? (d.worker || d.command) : (d.uri || d.rid)}</h2>
        {#if d.job_status}<span class="font-mono text-xs {d.job_status === 'failed' || d.job_status === 'errored' ? 'text-rose-600 dark:text-rose-300' : 'text-emerald-600 dark:text-emerald-300'}">{d.job_status}</span>{/if}
        <!-- A GraphQL operation names the request better than the one route every operation shares. -->
        {#if d.route && !d.operation}<span class="font-mono text-xs text-gray-500 dark:text-gray-400" title={m.requests_route()}>{d.route}</span>{/if}
        {#if d.operation}<span class="inline-flex items-center gap-1 font-mono text-xs text-pink-700 dark:text-pink-300" title={(d.operations ?? []).join('\n')}><Icon name="graphql" class="w-3.5 h-3.5" />{d.operation}</span>{/if}
        {#if d.status}<span class="font-mono text-xs {statusTone(d.status)}">{d.status}</span>{/if}
        <span class="text-xs text-gray-500 dark:text-gray-400">{d.type} · {d.site}{d.branch ? `@${d.branch}` : ''}</span>
        <span class="ml-auto inline-flex items-center gap-1 text-[11px] text-gray-400 font-mono">rid {d.rid}<CopyButton text={d.rid} label="rid" tone="faint" /></span>
      </div>
      {#if controller}<div class="font-mono text-[11px] text-gray-500 dark:text-gray-400 truncate"><ClassName value={controller} /></div>{/if}
      {#if authUser}
        <div class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300 flex-wrap">
          <span class="text-gray-400">{m.requests_signedInAs()}</span>
          <span class="font-medium">{authUser.name ?? authUser.email ?? authUser.id}</span>
          {#if authUser.name && authUser.email}<span class="text-gray-500 dark:text-gray-400">{authUser.email}</span>{/if}
          <span class="font-mono text-[11px] text-gray-400">#{authUser.id}</span>
          {#if authUser.guard}<span class="text-[10px] px-1.5 py-0.5 rounded-sm bg-gray-100 dark:bg-white/10 text-gray-600 dark:text-gray-300">{authUser.guard}</span>{/if}
        </div>
      {/if}
      {#if d.problems.length}
        <div class="flex gap-1.5 flex-wrap">
          {#each d.problems as p (p)}<span class="text-[11px] px-2 py-0.5 rounded-full bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300">{p}</span>{/each}
        </div>
      {/if}
      {#if d.parent}
        <button type="button" onclick={() => onopen(d!.parent!.rid)} class="w-full text-left flex items-center gap-2 rounded-md border border-sky-500/30 bg-sky-50 dark:bg-sky-900/20 px-3 py-2 text-xs text-sky-800 dark:text-sky-200">
          <span class="flex-1">{m.requests_sentBy({ via: d.parent.via ?? 'fetch', url: d.parent.url ?? '', site: d.parent.site ?? '' })}{#if d.parent.cross_origin}, {m.requests_crossOrigin()}{/if}</span>
          <span>{m.requests_openPage()} →</span>
        </button>
      {/if}
    {/if}
  </div>

  {#if missing}
    <p class="px-5 py-4 text-xs text-gray-500 dark:text-gray-400">{m.requests_notFound()}</p>
  {:else if d && waterfall}
    <DetailTabs tabs={allTabs} active={tab} onchange={(id) => (tab = id)} snap />
    <div class="flex-1 overflow-y-auto px-5 py-4 space-y-4 bg-gray-50/50 dark:bg-transparent">
      {#if tab === 'performance'}
        <RequestStats
          stats={[
            { label: m.requests_stat_response(), value: d.time_ms ? ms(d.time_ms) : '·' },
            ...(http.memory_peak ? [{ label: m.requests_stat_memory(), value: bytes(http.memory_peak) }] : []),
            ...(d.time_ms ? [{ label: m.requests_layer_app(), value: ms(Math.max(d.time_ms - dbTime, 0)), dot: 'bg-blue-500' }] : []),
            ...(queries.length ? [{ label: m.requests_stat_db(), value: ms(dbTime), dot: 'bg-amber-400' }] : []),
            ...(d.queue_ms !== undefined ? [{ label: m.requests_stat_queue(), value: ms(d.queue_ms), dot: 'bg-slate-400' }] : []),
            ...(pageTiming?.loadEventEnd ? [{ label: m.requests_stat_browser(), value: ms(pageTiming.loadEventEnd), dot: 'bg-cyan-400' }] : [])
          ]}
        />
        {#if d.time_ms && queries.length}
          <div class="flex h-1.5 rounded-full overflow-hidden bg-gray-200 dark:bg-white/10" aria-hidden="true">
            <span class="bg-blue-500" style="width: {Math.max(0, 100 - (dbTime / d.time_ms) * 100)}%"></span>
            <span class="bg-amber-400" style="width: {Math.min(100, (dbTime / d.time_ms) * 100)}%"></span>
          </div>
        {/if}
        {#if performanceNotes.length}
          <div class={BOX}>
            {#each performanceNotes as e (e.id)}
              {@const l = data(e)}
              <div class="{ROW} flex items-start gap-2">
                <span class="shrink-0 {BADGE} {levelTone(l.level)}">{l.level}</span>
                <span class="flex-1 break-words">{l.message}</span>
                <span class="font-mono text-[11px] text-gray-400">{offset(e.ts)}</span>
              </div>
            {/each}
          </div>
        {/if}
        <RequestTimeline {waterfall} />
      {:else if tab === 'request'}
        <div class="{BOX} grid grid-cols-[8rem_minmax(0,1fr)]">
          {#each [[m.requests_field_method(), d.method], ['URI', d.uri], [m.requests_route(), d.route], [m.requests_field_controller(), controller], [m.requests_field_status(), d.status], [m.requests_field_origin(), http.origin]].filter(([, v]) => v) as [k, v] (k)}
            <span class="px-3 py-1.5 text-gray-500 dark:text-gray-400 border-t first:border-t-0 border-gray-100 dark:border-lerd-border/60">{k}</span>
            <span class="px-3 py-1.5 font-mono break-all border-t border-gray-100 dark:border-lerd-border/60">{v}</span>
          {/each}
        </div>
        {#if middleware && ((middleware.global?.length ?? 0) + (middleware.route?.length ?? 0))}
          <section class={BOX}>
            <h4 class="px-3 py-2 font-semibold text-gray-800 dark:text-gray-100 border-b border-gray-100 dark:border-lerd-border/60">{m.requests_middleware()} <span class="font-normal text-gray-400">{(middleware.global?.length ?? 0) + (middleware.route?.length ?? 0)}</span></h4>
            {#each [[m.requests_middlewareGlobal(), middleware.global ?? []], [m.requests_middlewareRoute(), middleware.route ?? []]] as [heading, list] (heading)}
              {#if list.length}
                <div class="{ROW} grid grid-cols-[8rem_minmax(0,1fr)] gap-3">
                  <span class="text-gray-500 dark:text-gray-400">{heading}</span>
                  <ol class="space-y-0.5">
                    {#each list as name, i (i)}
                      {@const source = middleware.sources?.[name]}
                      <li class="font-mono text-[11px] break-all">{#if source}<SourcePath file={source.file} line={source.line} label={name} bare dotted />{:else}<ClassName value={name} />{/if}</li>
                    {/each}
                  </ol>
                </div>
              {/if}
            {/each}
          </section>
        {/if}
        {#if routeParams.length}
          <section class={BOX}>
            <h4 class="px-3 py-2 font-semibold text-gray-800 dark:text-gray-100 border-b border-gray-100 dark:border-lerd-border/60">{m.requests_routeParams()} <span class="font-normal text-gray-400">{routeParams.length}</span></h4>
            {#each routeParams as [name, p] (name)}
              <div class="{ROW} grid grid-cols-[minmax(0,14rem)_minmax(0,1fr)_minmax(0,1fr)] gap-3">
                <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400 truncate" title={p.field ? `${name}:${p.field}` : name}>{name}{#if p.field}<span class="text-gray-400 dark:text-gray-500">:{p.field}</span>{/if}</span>
                <span class="font-mono text-[11px] text-gray-800 dark:text-gray-200 break-all">{p.value}</span>
                <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400 break-all">{#if p.model}{#if p.file}<SourcePath file={p.file} line={p.line} label={p.model} bare dotted />{:else}{p.model}{/if}{#if p.key !== undefined} <span class="text-gray-800 dark:text-gray-200">#{p.key}</span>{/if}{/if}</span>
              </div>
            {/each}
          </section>
        {/if}
        <KeyValueTable title={m.requests_kv_headers()} values={http.headers} />
        <KeyValueTable title={m.requests_kv_query()} values={http.query} />
        <KeyValueTable title={m.requests_kv_body()} values={http.body} />
        <KeyValueTable title={m.requests_kv_cookies()} values={http.cookies} />
        {#if session}<KeyValueTable title={`${m.requests_section_session()}${data(session).name ? ` · ${data(session).name}` : ''}`} values={data(session).data} />{/if}
        <KeyValueTable title={m.requests_kv_response()} values={http.response_headers} />
        {#if http.response_body}<KeyValueTable title={m.requests_kv_responseBody()} values={http.response_body} />{/if}
      {:else if tab === 'exceptions'}
        <div class="{BOX} border-rose-500/30">
          {#each ev('exception') as e (e.id)}
            <div class="{ROW} space-y-1">
              <div><ClassName value={String(data(e).type ?? '')} class="font-mono text-rose-600 dark:text-rose-300" /> {data(e).message}</div>
              <TraceBlock src={e.src} trace={data(e).frames ?? data(e).trace} />
            </div>
          {/each}
        </div>
      {:else if tab === 'database'}
        <RequestStats
          stats={[
            { label: m.requests_stat_queries(), value: d.queries?.query_count ?? queries.length },
            { label: m.requests_stat_slow(), value: d.queries?.slow?.length ?? 0, dot: 'bg-amber-400' },
            { label: 'N+1', value: d.queries?.n_plus_one?.length ?? 0, dot: 'bg-rose-500' },
            ...Object.entries(verbs).map(([k, v]) => ({ label: k.toUpperCase(), value: v })),
            { label: m.requests_stat_duration(), value: ms(dbTime) }
          ]}
        />
        {#if findings.length}
          <div class="{BOX} border-amber-500/30">
            {#each findings as f, i (i)}
              <div class="{ROW} space-y-1.5">
                <div class="flex items-center gap-2">
                  <span class="{BADGE} bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300">{f.count ? `N+1 ×${f.count}` : ms(f.time_ms ?? 0)}</span>
                  <span class="font-mono truncate flex-1">{f.fingerprint ?? f.sql}</span>
                  <span class="text-[11px] min-w-0 max-w-[45%]"><SourcePath file={f.caller.file} line={f.caller.line} short /></span>
                  {#if f.ids?.length}<button type="button" aria-pressed={only === f} onclick={() => (only = only === f ? null : f)} class="text-xs px-2 py-1 rounded-md border border-gray-200 dark:border-lerd-border whitespace-nowrap {only === f ? 'bg-gray-100 dark:bg-white/10 text-gray-800 dark:text-gray-100' : 'text-gray-500 dark:text-gray-400'}">{m.requests_nPlusOneShow()}</button>{/if}
                </div>
                {#if f.example_sql}
                  <div class="flex items-center gap-2">
                    <span class="text-gray-400 w-14 shrink-0">{m.requests_example()}</span>
                    <code class="flex-1 min-w-0 truncate font-mono rounded-sm bg-gray-50 dark:bg-white/5 px-2 py-1">{f.example_sql}</code>
                    <CopyButton text={f.example_sql} label={m.queries_copySql()} />
                    <CopyButton text={`EXPLAIN ${f.example_sql}`} label={m.requests_copyExplain()} />
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
        <div class={BOX}>
          <div class="flex items-center gap-2 {HEAD}">
            <span class="flex-1">{m.requests_section_queries()}</span>
            <button type="button" aria-pressed={sqlFormatted} onclick={toggleSqlFormat} class="normal-case tracking-normal text-xs px-2 py-1 rounded-md border border-gray-200 dark:border-lerd-border {sqlFormatted ? 'bg-gray-100 dark:bg-white/10 text-gray-800 dark:text-gray-100' : 'text-gray-500 dark:text-gray-400'}">{m.requests_sql_format()}</button>
            <input class="normal-case tracking-normal text-xs px-2 py-1 rounded-md border border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-white/5 w-48" placeholder={m.requests_filter()} bind:value={querySearch} />
          </div>
          {#each cap('queries', shownQueries) as e (e.id)}
            {@const q = data(e)}
            <div class="{ROW} grid grid-cols-[2.5rem_5rem_minmax(0,1fr)_auto_4rem_auto] gap-3 items-start {slowSql.has(q.sql) ? 'bg-amber-50 dark:bg-amber-900/15' : ''}">
              <span class="font-mono tabular-nums text-right text-gray-400 dark:text-gray-500">{queryNo.get(e.id)}</span>
              <span class="text-gray-500 dark:text-gray-400 truncate">{q.connection ?? ''}</span>
              <code class="font-mono break-words {sqlFormatted ? 'whitespace-pre-wrap' : ''} {slowSql.has(q.sql) ? 'text-amber-700 dark:text-amber-300' : ''}">{@html highlight(sqlFormatted ? formatSql(inlineBindings(q.sql, q.bindings)) : inlineBindings(q.sql, q.bindings), 'sql')}</code>
              <span class="text-[11px] min-w-0">{#if e.src?.file}<CallerSource file={e.src.file} line={e.src.line} trace={data(e).trace} />{/if}</span>
              <span class="font-mono text-right tabular-nums">{ms(Number(q.time_ms ?? 0))}</span>
              <CopyButton text={() => inlineBindings(q.sql, q.bindings)} label={m.queries_copySql()} />
            </div>
          {/each}{@render more('queries', (shownQueries).length)}
        </div>
      {:else if tab === 'models'}
        <RequestStats stats={MODEL_ACTIONS.map((a) => ({ label: a, value: models.reduce((n, [, c]) => n + (c[a] ?? 0), 0) }))} />
        <div class={BOX}>
          <div class="{HEAD} grid grid-cols-[minmax(0,1fr)_repeat(5,5rem)] gap-3"><span>{m.requests_col_model()}</span>{#each MODEL_ACTIONS as a (a)}<span class="text-right">{a}</span>{/each}</div>
          {#each cap('models', models) as [model, counts] (model)}
            <div class="{ROW} grid grid-cols-[minmax(0,1fr)_repeat(5,5rem)] gap-3 items-center">
              <span class="font-mono break-all">{#if modelSources[model]}<SourcePath file={modelSources[model].file} line={modelSources[model].line} label={model} bare dotted />{:else}<ClassName value={model} />{/if}</span>
              {#each MODEL_ACTIONS as a (a)}<span class="font-mono text-right tabular-nums {counts[a] ? '' : 'text-gray-300 dark:text-gray-600'}">{counts[a] ?? 0}</span>{/each}
            </div>
          {/each}{@render more('models', (models).length)}
        </div>
      {:else if tab === 'views'}
        <RequestStats stats={[{ label: m.requests_tab_views(), value: ev('view').length }, { label: m.requests_stat_duration(), value: ms(spans.filter((s) => s.label === 'View').reduce((n, s) => n + Number(s.time_ms ?? 0), 0)), dot: 'bg-violet-400' }]} />
        {#each cap('views', ev('view')) as e, i (e.id)}
          {@const v = data(e)}
          <div class={BOX}>
            <div class="flex items-center gap-2 px-3 py-2">
              <span class="font-mono tabular-nums text-gray-400 dark:text-gray-500">{i + 1}</span>
              <span class="font-mono font-medium text-violet-700 dark:text-violet-300">{v.name}</span>
              <span class="text-[11px] min-w-0 flex-1">{#if v.path}<SourcePath file={v.path} muted short />{/if}</span>
              {#if viewTime(v.name) !== undefined}<span class="font-mono tabular-nums">{ms(viewTime(v.name))}</span>{/if}
            </div>
            {#if v.data_preview && Object.keys(v.data_preview).length}
              <div class="flex flex-wrap gap-1.5 px-3 pb-2">
                {#each Object.entries(v.data_preview) as [k, val] (k)}<span class="font-mono text-[11px] rounded-sm bg-gray-100 dark:bg-white/5 px-1.5 py-0.5"><span class="text-gray-500">{k}</span> {val}</span>{/each}
              </div>
            {/if}
          </div>
        {/each}{@render more('views', (ev('view')).length)}
      {:else if tab === 'components'}
        {#each cap('components', components) as [name, phases] (name)}
          {@const at = phases.find((c) => c.file)}
          <div class={BOX}>
            <div class="flex items-center gap-2 px-3 py-2 border-b border-gray-100 dark:border-lerd-border/60">
              <span class="font-mono font-medium text-orange-700 dark:text-orange-300 min-w-0">{#if at}<SourcePath file={at.file} line={at.line} label={name} bare dotted />{:else}<ClassName value={name} />{/if}</span>
              <span class="ml-auto font-mono tabular-nums text-gray-500">{ms(phases.reduce((n, c) => n + Number(c.time_ms ?? 0), 0))}</span>
            </div>
            {#each phases as c, i (i)}
              <div class="{ROW} grid grid-cols-[7rem_minmax(0,1fr)_4rem] gap-3 items-start">
                <span class="justify-self-start {BADGE} {c.status === 'failed' ? 'bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300' : 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300'}">{c.phase}</span>
                <span class="min-w-0 space-y-1 text-gray-600 dark:text-gray-300">
                  {#each Object.entries(c.details ?? {}) as [k, v] (k)}<StructuredValue value={v} />{/each}
                  {#if c.state}<KeyValueTable title={m.requests_popover_state()} values={c.state} />{/if}
                  {#if c.exception}<span class="block text-rose-600 dark:text-rose-300">{c.exception}</span>{/if}
                </span>
                <span class="font-mono text-right tabular-nums">{ms(Number(c.time_ms ?? 0))}</span>
              </div>
            {/each}
          </div>
        {/each}{@render more('components', (components).length)}
      {:else if tab === 'cache'}
        <RequestStats stats={Object.entries(cacheOps).map(([k, v]) => ({ label: k, value: v, dot: k === 'miss' ? 'bg-amber-400' : k === 'hit' ? 'bg-emerald-500' : undefined }))} />
        <div class={BOX}>
          {#each cap('cache', ev('cache')) as e (e.id)}
            {@const c = data(e)}
            <div class="{ROW} grid grid-cols-[4rem_minmax(0,1fr)_auto_6rem_4.5rem] gap-3 items-center">
              <span class="justify-self-start {BADGE} {c.op === 'miss' ? 'bg-amber-100 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300' : c.op === 'hit' ? 'bg-emerald-100 dark:bg-emerald-900/40 text-emerald-700 dark:text-emerald-300' : 'bg-gray-100 dark:bg-white/10 text-gray-600 dark:text-gray-300'}">{c.op}</span>
              <span class="min-w-0 space-y-0.5"><span class="block font-mono break-all">{c.key}</span>{#if c.file}<span class="block text-[11px]"><SourcePath file={c.file} muted short /></span>{/if}</span>
              <span class="text-[11px] min-w-0">{#if e.src?.file}<CallerSource file={e.src.file} line={e.src.line} trace={c.trace} />{/if}</span>
              <span class="text-gray-500 dark:text-gray-400 truncate">{c.store}{c.connection ? ` · ${c.connection}` : ''}</span>
              <span class="font-mono text-[11px] text-gray-400 text-right">{offset(e.ts)}</span>
            </div>
          {/each}{@render more('cache', (ev('cache')).length)}
        </div>
      {:else if tab === 'redis'}
        <RequestStats stats={[{ label: m.requests_stat_count(), value: ev('redis').length }, { label: m.requests_stat_duration(), value: ms(ev('redis').reduce((n, e) => n + Number(data(e).time_ms ?? 0), 0)), dot: 'bg-red-500' }]} />
        <div class={BOX}>
          {#each cap('redis', ev('redis')) as e (e.id)}
            {@const r = data(e)}
            <div class="{ROW} grid grid-cols-[6rem_minmax(0,1fr)_auto_6rem_4rem] gap-3 items-start">
              <span class="font-mono font-medium text-red-700 dark:text-red-300">{r.command}</span>
              <span class="min-w-0"><StructuredValue value={r.args ?? ''} /></span>
              <span class="text-[11px] min-w-0">{#if e.src?.file}<CallerSource file={e.src.file} line={e.src.line} trace={data(e).trace} />{/if}</span>
              <span class="text-gray-500 dark:text-gray-400 truncate">{r.connection ?? ''}</span>
              <span class="font-mono text-right tabular-nums">{ms(Number(r.time_ms ?? 0))}</span>
            </div>
          {/each}{@render more('redis', (ev('redis')).length)}
        </div>
      {:else if tab === 'filesystem'}
        <RequestStats stats={[{ label: m.requests_stat_count(), value: ev('filesystem').length }, { label: m.requests_stat_duration(), value: ms(ev('filesystem').reduce((n, e) => n + Number(data(e).time_ms ?? 0), 0)), dot: 'bg-stone-400' }]} />
        <div class={BOX}>
          {#each cap('filesystem', ev('filesystem')) as e (e.id)}
            {@const f = data(e)}
            <div class="{ROW} grid grid-cols-[7rem_6rem_minmax(0,1fr)_auto_4rem] gap-3 items-start">
              <span class="justify-self-start {BADGE} {f.status === 'failed' ? 'bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300' : 'bg-stone-100 dark:bg-white/10 text-stone-700 dark:text-stone-300'}">{f.op}</span>
              <span class="text-gray-500 dark:text-gray-400 truncate">{f.disk ?? ''}</span>
              <span class="font-mono break-all">{f.path}{f.exception ? ` · ${f.exception}` : ''}</span>
              <span class="text-[11px] min-w-0">{#if e.src?.file}<CallerSource file={e.src.file} line={e.src.line} trace={data(e).trace} />{/if}</span>
              <span class="font-mono text-right tabular-nums">{ms(Number(f.time_ms ?? 0))}</span>
            </div>
          {/each}{@render more('filesystem', (ev('filesystem')).length)}
        </div>
      {:else if tab === 'events'}
        <div class={BOX}>
          {#each cap('events', ev('event')) as e (e.id)}
            <div class="{ROW} grid grid-cols-[minmax(0,1fr)_4.5rem] gap-3">
              <span class="font-mono break-all">{data(e).name}</span>
              <span class="font-mono text-[11px] text-gray-400 text-right">{offset(e.ts)}</span>
            </div>
          {/each}{@render more('events', (ev('event')).length)}
        </div>
      {:else if tab === 'log'}
        <div class="flex gap-1 flex-wrap" role="group" aria-label={m.requests_section_logs()}>
          {#each levels as l (l)}
            {@const on = !hiddenLevels[l]}
            <button type="button" aria-pressed={on} onclick={() => (hiddenLevels = { ...hiddenLevels, [l]: on })} class="text-[11px] px-2 py-0.5 rounded-full {on ? levelTone(l) : 'text-gray-400 line-through'}">{l} {logs.filter((e) => data(e).level === l).length}</button>
          {/each}
        </div>
        <div class={BOX}>
          {#each cap('logs', shownLogs) as e (e.id)}
            {@const l = data(e)}
            <div class="{ROW} grid grid-cols-[4rem_4.5rem_5rem_minmax(0,1fr)] gap-2 items-baseline">
              <span class="font-mono text-[11px] text-gray-400">{offset(e.ts)}</span>
              <span class="justify-self-start {BADGE} {levelTone(l.level)}">{l.level}</span>
              <span class="text-gray-500 dark:text-gray-400 truncate">{l.channel ?? ''}</span>
              <span class="space-y-1 min-w-0">
                <span class="block break-words">{l.message}</span>
                {#if l.context}<div class="rounded-sm bg-gray-50 dark:bg-white/5 px-2 py-1"><StructuredValue value={l.context} open /></div>{/if}
                {#if l.show_trace}<TraceBlock src={e.src} trace={l.trace} />{/if}
              </span>
            </div>
          {/each}{@render more('logs', (shownLogs).length)}
          {#if shownLogs.length < logs.length}
            <div class="{ROW} text-[11px] text-gray-400">{m.requests_hiddenLevels({ count: logs.length - shownLogs.length })}</div>
          {/if}
        </div>
      {:else if tab === 'dumps'}
        <div class={BOX}>
          {#each cap('dumps', ev('dump')) as e (e.id)}
            <pre class="{ROW} font-mono text-[11px] whitespace-pre-wrap break-all">{e.label ? `${e.label}: ` : ''}{e.text}</pre>
          {/each}{@render more('dumps', (ev('dump')).length)}
        </div>
      {:else if tab === 'mail'}
        <div class={BOX}>
          {#each cap('mails', mails) as e (e.id)}
            {@const x = data(e)}
            <div class="{ROW} space-y-0.5">
              <div class="flex items-center gap-2">
                <span class="{BADGE} bg-gray-100 dark:bg-white/10 text-gray-600 dark:text-gray-300">{e.kind === 'mail' ? 'mail' : x.channel}</span>
                <span class="font-medium flex-1 break-words">{x.subject || x.body || x.notification || ''}</span>
                <span class="font-mono text-[11px] text-gray-400">{offset(e.ts)}</span>
              </div>
              {#if x.to}<div class="text-[11px] text-gray-500 dark:text-gray-400 break-all">→ {Array.isArray(x.to) ? x.to.join(', ') : x.to}</div>{/if}
            </div>
          {/each}{@render more('mails', (mails).length)}
        </div>
      {:else if tab === 'http'}
        <div class={BOX}>
          {#each cap('http', ev('http')) as e (e.id)}
            {@const x = data(e)}
            {@const span = httpSpan(e.ts, Number(x.time_ms ?? 0))}
            <HttpCall call={x} tone={statusTone(x.status)} from={span?.[0]} to={span?.[1]} />
          {/each}{@render more('http', (ev('http')).length)}
        </div>
      {:else if tab === 'jobs'}
        <div class={BOX}>
          {#each cap('jobs', ev('job')) as e (e.id)}
            {@const x = data(e)}
            <div class="{ROW} grid grid-cols-[minmax(0,1fr)_6rem_4rem] gap-3">
              <ClassName value={String(x.class ?? '')} class="font-mono break-all" />
              <span class="{x.status === 'failed' ? 'text-rose-600 dark:text-rose-300' : 'text-gray-500 dark:text-gray-400'}">{x.status}</span>
              <span class="font-mono text-right tabular-nums">{x.time_ms ? ms(x.time_ms) : ''}</span>
            </div>
          {/each}{@render more('jobs', (ev('job')).length)}
        </div>
      {:else if tab === 'browser'}
        <div class={BOX}>
          {#each cap('browser', browserEvents) as e (e.id)}
            {@const b = data(e)}
            <div class="{ROW} flex items-start gap-2">
              <span class="shrink-0 {BADGE} bg-gray-100 dark:bg-white/10 text-gray-600 dark:text-gray-300">{b.type === 'console' ? `console.${b.level}` : b.type}</span>
              <span class="font-mono break-all flex-1">{b.message}</span>
              <span class="shrink-0 font-mono text-[11px] text-gray-400">{offset(e.ts)}</span>
            </div>
          {/each}{@render more('browser', (browserEvents).length)}
        </div>
      {:else if tab.startsWith('custom:')}
        {@const custom = customTabs.find((t) => t.id === tab)}
        <CustomBlocks blocks={custom?.blocks ?? []} columns={custom?.columns ?? 1} />
      {:else if tab === 'graphql'}
        {#each graphql as op, i (i)}
          <section class={BOX}>
            <div class="flex items-center gap-2 px-3 py-2 border-b border-gray-100 dark:border-lerd-border/60">
              <span class="{BADGE} bg-pink-100 dark:bg-pink-900/30 text-pink-700 dark:text-pink-300">{op.type}</span>
              <span class="font-mono font-semibold text-gray-800 dark:text-gray-100">{op.name ?? ''}</span>
              {#if op.errors?.length}<span class="{BADGE} bg-rose-100 dark:bg-rose-900/40 text-rose-700 dark:text-rose-300">{m.requests_graphql_errors({ count: op.errors.length })}</span>{/if}
            </div>
            {#each op.fields ?? [] as f, j (j)}
              <div class="{ROW} space-y-1.5">
                <div class="flex items-baseline gap-2 flex-wrap">
                  <span class="font-mono font-semibold text-gray-800 dark:text-gray-100">{#if f.alias}<span class="font-normal text-gray-400">{f.alias}: </span>{/if}{#if f.file}<SourcePath file={f.file} line={f.line} label={f.name} bare dotted />{:else}{f.name}{/if}</span>
                  {#if f.type}<GraphQLType type={f.type} types={graphqlTypes} />{/if}
                </div>
                {#if f.args && Object.keys(f.args).length}
                  <div class="grid grid-cols-[6rem_minmax(0,1fr)] gap-x-3 text-[11px]"><span class="text-gray-400">{m.requests_graphql_input()}</span><StructuredValue value={f.args} open /></div>
                {/if}
                {#if 'data' in f}
                  <div class="grid grid-cols-[6rem_minmax(0,1fr)] gap-x-3 text-[11px]"><span class="text-gray-400">{m.requests_graphql_response()}</span><StructuredValue value={f.data} /></div>
                {/if}
              </div>
            {/each}
            {#each op.errors ?? [] as err, k (k)}
              <div class="{ROW} text-rose-600 dark:text-rose-300 text-[11px]"><span class="font-mono">{(err.path ?? []).join('.')}</span> {err.message}</div>
            {/each}
            <details class="border-t border-gray-100 dark:border-lerd-border/60">
              <summary class="px-3 py-2 cursor-pointer text-[11px] text-gray-500 dark:text-gray-400 flex items-center gap-2">{m.requests_graphql_query()}<span class="ml-auto"><CopyButton text={op.query} label={m.common_copy()} /></span></summary>
              <pre class="px-3 pb-2 font-mono text-[12px] leading-[1.6] whitespace-pre-wrap break-words [font-variant-ligatures:none]">{@html highlight(op.query, 'graphql')}</pre>
              {#if op.variables}<KeyValueTable title={m.requests_graphql_variables()} values={op.variables} />{/if}
            </details>
          </section>
        {/each}
      {:else if tab === 'sent'}
        <div class={BOX}>
          {#each cap('sent', d.children ?? []) as c (c.rid)}
            <button type="button" onclick={() => onopen(c.rid)} class="{ROW} w-full text-left flex items-center gap-2 hover:bg-gray-50 dark:hover:bg-white/5">
              <span class="font-mono text-[11px] {statusTone(c.status)}">{c.status}</span>
              <span class="font-mono truncate flex-1">{c.url}</span>
              <span class="text-[11px] text-gray-400">{c.via}{c.duration_ms ? ` · ${Math.round(c.duration_ms)} ms` : ''}</span>
              <span class="text-[11px] font-mono tabular-nums whitespace-nowrap text-gray-500 dark:text-gray-400 text-right">{clock(c.at)}</span>
            </button>
          {/each}{@render more('sent', (d.children ?? []).length)}
        </div>
      {/if}
    </div>
  {/if}
</div>

{#snippet more(key: string, total: number)}
  {#if total > (limits[key] ?? PAGE)}
    <button type="button" class="w-full py-2 text-xs text-gray-500 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-100 hover:bg-gray-50 dark:hover:bg-white/5" onclick={() => (limits = { ...limits, [key]: (limits[key] ?? PAGE) + PAGE })}>{m.requests_showMore({ count: Math.min(total - (limits[key] ?? PAGE), PAGE), total: total - (limits[key] ?? PAGE) })}</button>
  {/if}
{/snippet}
