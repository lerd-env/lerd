<script lang="ts">
  import { condense, type Waterfall, type WaterfallRow, type Layer } from '$lib/requestWaterfall';
  import Icon, { type IconName } from './Icon.svelte';
  import SourcePath from './SourcePath.svelte';
  import { tooltip } from '$lib/tooltip';
  import { portal } from '$lib/portal';
  import { m } from '../paraglide/messages.js';

  // A request's timeline: everything it did on one chronological list, the
  // label beside its bar, a layer filter, a search and a popover per row.

  interface Props {
    waterfall: Waterfall;
  }
  let { waterfall }: Props = $props();

  const LAYERS: Array<{ id: Layer; label: () => string; bar: string; text: string; icon: IconName }> = [
    { id: 'request', label: m.timeline_layer_request, bar: 'bg-blue-500', text: 'text-blue-700 dark:text-blue-300', icon: 'play' },
    { id: 'query', label: m.debug_tab_queries, bar: 'bg-amber-400', text: 'text-amber-700 dark:text-amber-300', icon: 'database' },
    { id: 'view', label: m.debug_tab_views, bar: 'bg-violet-400', text: 'text-violet-700 dark:text-violet-300', icon: 'eye' },
    { id: 'cache', label: m.debug_tab_cache, bar: 'bg-teal-400', text: 'text-teal-700 dark:text-teal-300', icon: 'bookmark' },
    { id: 'http', label: m.debug_tab_http, bar: 'bg-sky-400', text: 'text-sky-700 dark:text-sky-300', icon: 'globe' },
    { id: 'job', label: m.debug_tab_jobs, bar: 'bg-indigo-400', text: 'text-indigo-700 dark:text-indigo-300', icon: 'clock' },
    { id: 'mail', label: m.debug_tab_mail, bar: 'bg-pink-400', text: 'text-pink-700 dark:text-pink-300', icon: 'upload' },
    { id: 'message', label: m.debug_tab_messages, bar: 'bg-cyan-400', text: 'text-cyan-700 dark:text-cyan-300', icon: 'bell' },
    { id: 'event', label: m.debug_tab_events, bar: 'bg-slate-400', text: 'text-slate-600 dark:text-slate-300', icon: 'cube' },
    { id: 'log', label: m.debug_tab_logs, bar: 'bg-emerald-500', text: 'text-emerald-700 dark:text-emerald-300', icon: 'docs' },
    { id: 'dump', label: m.debug_tab_dumps, bar: 'bg-fuchsia-400', text: 'text-fuchsia-700 dark:text-fuchsia-300', icon: 'clipboard' },
    { id: 'error', label: m.debug_tab_exceptions, bar: 'bg-rose-500', text: 'text-rose-700 dark:text-rose-300', icon: 'alert' }
  ];
  const styleOf = Object.fromEntries(LAYERS.map((l) => [l.id, l])) as Record<Layer, (typeof LAYERS)[number]>;

  let hidden = $state<Record<string, boolean>>({});
  let condensed = $state(true);
  let search = $state('');
  let open = $state<{ r: WaterfallRow; i: number; x: number; y: number } | null>(null);
  let pop = $state<HTMLElement | null>(null);
  // The row the popover belongs to, and where along it the click landed.
  let anchor: HTMLElement | null = null;
  let offsetX = 0;
  // Under its row, or above it when there is no room underneath, kept inside
  // the window as the page scrolls.
  function layout() {
    if (!open || !anchor) return;
    const row = anchor.getBoundingClientRect();
    const h = pop?.offsetHeight ?? 0;
    let y = row.bottom + 4;
    if (y + h > window.innerHeight - 8) y = row.top - h - 4;
    y = Math.min(Math.max(8, y), window.innerHeight - h - 8);
    const x = Math.min(Math.max(row.left + offsetX, 8), window.innerWidth - 392);
    if (x !== open.x || y !== open.y) open = { ...open, x, y };
  }
  $effect(() => {
    if (open && pop) layout();
  });

  const total = $derived(waterfall.total);
  const present = $derived(LAYERS.filter((l) => waterfall.rows.some((r) => r.layer === l.id)));
  // A folded row opens in place into the rows it stands for, and a second
  // click folds them back.
  let expanded = $state<Record<string, boolean>>({});
  const foldKey = (r: WaterfallRow) => `${r.layer}:${r.start}:${r.items?.length ?? 0}`;
  const rows = $derived.by(() => {
    const needle = search.toLowerCase();
    const keep = waterfall.rows.filter((r) => !hidden[r.layer] && (!needle || r.label.toLowerCase().includes(needle)));
    if (!condensed) return keep;
    return condense(keep).flatMap((r) => (r.items && expanded[foldKey(r)] ? [r, ...r.items.map((it) => ({ ...it, nested: true }))] : [r]));
  });

  const pct = (n: number) => (n / total) * 100;
  const fmt = (n: number) => `${n < 1 ? '<1' : n < 10 ? n.toFixed(1) : Math.round(n)} ms`;
  const pos = (n: number) => `${n < 10 ? n.toFixed(1) : Math.round(n)} ms`;
  const took = (r: WaterfallRow) => (r.end > r.start ? fmt(r.end - r.start) : '');
  function toggle(e: MouseEvent, r: WaterfallRow, i: number) {
    e.stopPropagation();
    if (r.items) {
      open = null;
      expanded = { ...expanded, [foldKey(r)]: !expanded[foldKey(r)] };
      return;
    }
    if (open?.i === i) {
      open = null;
      return;
    }
    anchor = e.currentTarget as HTMLElement;
    offsetX = e.clientX - 192 - anchor.getBoundingClientRect().left;
    open = { r, i, x: 0, y: 0 };
    layout();
  }
  function close(e: Event) {
    if (e instanceof KeyboardEvent && e.key !== 'Escape') return;
    open = null;
  }
</script>

<svelte:window onclick={close} onkeydown={close} onresize={layout} />

<div class="h-full overflow-y-auto p-3" onscroll={layout}>
  {#if waterfall.rows.length === 0}
    <p class="p-6 text-center text-xs text-gray-500 dark:text-gray-400">{m.timeline_empty()}</p>
  {:else}
    <div class="rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card text-xs overflow-hidden">
      <div class="flex items-center gap-2 flex-wrap px-3 py-2 border-b border-gray-100 dark:border-lerd-border/60">
        <label class="inline-flex items-center gap-1.5 text-gray-500 dark:text-gray-400 cursor-pointer select-none">
          <input type="checkbox" class="rounded-sm border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card text-lerd-red focus:ring-lerd-red" bind:checked={condensed} />
          {m.timeline_condense()}
        </label>
        <div class="flex items-center gap-1 ml-auto" role="group" aria-label={m.timeline_layers()}>
          {#each present as l (l.id)}
            {@const on = !hidden[l.id]}
            <button type="button" aria-pressed={on} use:tooltip={l.label()} aria-label={l.label()} onclick={() => (hidden = { ...hidden, [l.id]: on })} class="w-6 h-6 inline-flex items-center justify-center rounded-md transition-colors {on ? `${l.bar} text-white` : 'bg-gray-100 dark:bg-white/5 text-gray-400'}">
              <Icon name={l.icon} class="w-3.5 h-3.5" />
            </button>
          {/each}
        </div>
        <input class="text-xs px-2 py-1 rounded-md border border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-white/5 w-40" placeholder={m.timeline_filter()} aria-label={m.timeline_filter()} bind:value={search} />
      </div>

      <div class="px-3 py-2 space-y-px">
        <div class="h-5 rounded-sm bg-gray-300 dark:bg-white/15 text-[10px] text-gray-700 dark:text-gray-200 flex items-center justify-center font-medium">{m.timeline_total({ time: fmt(total) })}</div>
        {#each rows as r, i (i)}
          {@const s = styleOf[r.layer]}
          {@const left = pct(r.start)}
          {@const width = r.end > r.start ? Math.max(pct(r.end - r.start), 0.4) : 0}
          {@const edge = r.items ? Math.max(...r.items.map((it) => pct(it.end))) : left + width}
          {@const place = edge < 68 ? 'after' : left > 32 ? 'before' : 'inside'}
          <button type="button" aria-expanded={r.items ? Boolean(expanded[foldKey(r)]) : open?.i === i} class="relative block w-full h-6 rounded-sm text-left cursor-pointer {open?.i === i ? 'ring-1 ring-gray-400 dark:ring-white/30' : ''} {'nested' in r ? 'bg-gray-100/70 dark:bg-white/[0.06]' : i % 2 ? 'bg-gray-50 dark:bg-white/[0.03]' : ''}" onclick={(e) => toggle(e, r, i)}>
            {#if r.items}
              {#each r.items as it, j (j)}
                {@const w = it.end > it.start ? Math.max(pct(it.end - it.start), 0.4) : 0}
                <span class="absolute top-1.5 bottom-1.5 rounded-sm {s.bar} {w ? '' : 'w-[3px]'}" style="left: {pct(it.start)}%; {w ? `width: ${w}%` : ''}"></span>
              {/each}
            {:else}
              <span class="absolute top-1 bottom-1 rounded-sm {s.bar} {width ? '' : 'w-[3px]'}" style="left: {left}%; {width ? `width: ${width}%` : ''}"></span>
            {/if}
            <span class="absolute inset-y-0 flex items-center gap-1 whitespace-nowrap {place === 'inside' ? 'text-white px-1.5' : `max-w-[60%] ${s.text}`}" style={place === 'after' ? `left: calc(${edge}% + 8px)` : place === 'before' ? `right: calc(${100 - left}% + 8px)` : `left: ${left}%; max-width: ${width}%`}>
              <Icon name={s.icon} class="w-3 h-3 shrink-0" />
              {#if r.items}<Icon name="chevron" class="w-3 h-3 shrink-0 transition-transform {expanded[foldKey(r)] ? '' : '-rotate-90'}" />{/if}
              <span class="truncate {r.layer === 'query' ? 'font-mono' : ''}">{r.items ? m.timeline_events({ count: r.items.length }) : r.label}</span>
              {#if !r.items && took(r)}<span class="shrink-0 opacity-70">{took(r)}</span>{/if}
            </span>
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>

{#if open}
  {@const s = styleOf[open.r.layer]}
  <div use:portal bind:this={pop} role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={close} class="fixed z-[9999] w-96 max-h-[calc(100vh-16px)] overflow-y-auto rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl text-xs" style="left: {open.x}px; top: {open.y}px">
    <div class="flex items-center gap-1.5 px-3 py-2 font-semibold border-b border-gray-100 dark:border-lerd-border/60 {s.text}"><Icon name={s.icon} class="w-3.5 h-3.5" />{s.label()}</div>
    {#if open.r.code}
      <pre class="px-3 py-2 font-mono text-[11px] whitespace-pre-wrap break-words text-gray-700 dark:text-gray-200 max-h-40 overflow-y-auto">{open.r.code}</pre>
    {:else}
      <p class="px-3 py-2 break-words text-gray-700 dark:text-gray-200">{open.r.label.slice(0, 600)}</p>
    {/if}
    {#if open.r.source}
      <div class="border-t border-gray-100 dark:border-lerd-border/60 px-3 py-2 text-[11px]"><SourcePath file={open.r.source.file} line={open.r.source.line} /></div>
    {/if}
    <div class="grid grid-cols-3 border-t border-gray-100 dark:border-lerd-border/60 text-center">
      <div class="py-1.5"><div class="font-semibold text-gray-800 dark:text-gray-100">{took(open.r) || '·'}</div><div class="text-[10px] text-gray-400">{m.timeline_stat_duration()}</div></div>
      <div class="py-1.5 border-l border-gray-100 dark:border-lerd-border/60"><div class="font-semibold text-gray-800 dark:text-gray-100">{pos(open.r.start)}</div><div class="text-[10px] text-gray-400">{m.timeline_stat_start()}</div></div>
      <div class="py-1.5 border-l border-gray-100 dark:border-lerd-border/60"><div class="font-semibold text-gray-800 dark:text-gray-100">{open.r.note && !took(open.r) ? open.r.note : pos(open.r.end)}</div><div class="text-[10px] text-gray-400">{open.r.note && !took(open.r) ? m.timeline_stat_kind() : m.timeline_stat_end()}</div></div>
    </div>
  </div>
{/if}
