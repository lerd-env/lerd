<script lang="ts">
  import { condense, type Waterfall, type WaterfallRow, type Layer } from '$lib/requestWaterfall';
  import Icon, { type IconName } from './Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import StructuredValue from './StructuredValue.svelte';
  import SourcePath from './SourcePath.svelte';
  import CallerSource from './CallerSource.svelte';
  import { customColor } from '$lib/customColors';
  import { portal } from '$lib/portal';
  import { m } from '../paraglide/messages.js';

  // A request's timeline: every span and moment on one chronological list, the
  // label beside its bar, a layer filter, a search and a popover per row.

  interface Props {
    waterfall: Waterfall;
  }
  let { waterfall }: Props = $props();

  const LAYERS: Array<{ id: Layer; label: () => string; bar: string; text: string; icon: IconName }> = [
    { id: 'browser', label: m.requests_layer_browser, bar: 'bg-cyan-400', text: 'text-cyan-700 dark:text-cyan-300', icon: 'globe' },
    { id: 'server', label: m.requests_layer_server, bar: 'bg-slate-400', text: 'text-slate-600 dark:text-slate-300', icon: 'services' },
    { id: 'app', label: m.requests_layer_app, bar: 'bg-blue-500', text: 'text-blue-700 dark:text-blue-300', icon: 'play' },
    { id: 'framework', label: m.requests_layer_framework, bar: 'bg-indigo-400', text: 'text-indigo-700 dark:text-indigo-300', icon: 'clock' },
    { id: 'query', label: m.requests_layer_query, bar: 'bg-amber-400', text: 'text-amber-700 dark:text-amber-300', icon: 'database' },
    { id: 'component', label: m.requests_layer_component, bar: 'bg-orange-400', text: 'text-orange-700 dark:text-orange-300', icon: 'cube' },
    { id: 'view', label: m.requests_layer_view, bar: 'bg-violet-400', text: 'text-violet-700 dark:text-violet-300', icon: 'eye' },
    { id: 'cache', label: m.requests_tab_cache, bar: 'bg-teal-400', text: 'text-teal-700 dark:text-teal-300', icon: 'bookmark' },
    { id: 'redis', label: m.requests_layer_redis, bar: 'bg-red-500', text: 'text-red-700 dark:text-red-300', icon: 'database' },
    { id: 'http', label: m.requests_tab_http, bar: 'bg-purple-500', text: 'text-purple-700 dark:text-purple-300', icon: 'globe' },
    { id: 'filesystem', label: m.requests_layer_filesystem, bar: 'bg-stone-400', text: 'text-stone-700 dark:text-stone-300', icon: 'download' },
    { id: 'log', label: m.requests_layer_log, bar: 'bg-emerald-500', text: 'text-emerald-700 dark:text-emerald-300', icon: 'docs' },
    { id: 'dump', label: m.requests_layer_dump, bar: 'bg-fuchsia-400', text: 'text-fuchsia-700 dark:text-fuchsia-300', icon: 'clipboard' },
    { id: 'error', label: m.requests_layer_error, bar: 'bg-rose-500', text: 'text-rose-700 dark:text-rose-300', icon: 'alert' }
  ];
  const layerStyle = Object.fromEntries(LAYERS.map((l) => [l.id, l])) as Record<Layer, (typeof LAYERS)[number]>;
  // A row the app wrote is drawn in the colour it chose and filtered by its
  // category, which become filters of their own beside lerd's layers.
  function styleOf(r: WaterfallRow) {
    if (r.layer !== 'custom') return layerStyle[r.layer];
    const c = customColor(r.color);
    return { id: 'custom' as Layer, label: () => r.category ?? '', bar: c.bar, text: c.text, icon: 'cube' as IconName };
  }
  const filterKey = (r: WaterfallRow) => (r.layer === 'custom' ? `cat:${r.category}` : r.layer);
  const categories = $derived(
    [...new Map(waterfall.rows.filter((r) => r.layer === 'custom').map((r) => [r.category ?? '', r])).values()].map((r) => ({ key: `cat:${r.category}`, label: r.category ?? '', bar: customColor(r.color).bar }))
  );

  let hidden = $state<Record<string, boolean>>({});
  let condensed = $state(true);
  let search = $state('');
  let hover = $state<{ r: WaterfallRow; i: number; x: number; y: number } | null>(null);
  let pop = $state<HTMLElement | null>(null);
  // The row the popover belongs to, and where along it the click landed.
  let anchor: HTMLElement | null = null;
  let offsetX = 0;
  // Under its row, or above it when there is no room underneath, and moving
  // with the row as the page scrolls until the row leaves the window, where
  // the popover stays at the edge.
  function layout() {
    if (!hover || !anchor) return;
    const row = anchor.getBoundingClientRect();
    const h = pop?.offsetHeight ?? 0;
    let y = row.bottom + 4;
    if (y + h > window.innerHeight - 8) y = row.top - h - 4;
    y = Math.min(Math.max(8, y), window.innerHeight - h - 8);
    const x = Math.min(Math.max(row.left + offsetX, 8), window.innerWidth - 392);
    if (x !== hover.x || y !== hover.y) hover = { ...hover, x, y };
  }
  $effect(() => {
    if (hover && pop) layout();
  });
  $effect(() => {
    const follow = () => layout();
    window.addEventListener('scroll', follow, true);
    window.addEventListener('resize', follow);
    return () => {
      window.removeEventListener('scroll', follow, true);
      window.removeEventListener('resize', follow);
    };
  });

  const total = $derived(waterfall.total);
  const present = $derived(LAYERS.filter((l) => waterfall.rows.some((r) => r.layer === l.id)));
  // A folded row opens in place into the rows it stands for, each clickable
  // like any other, and a second click on it folds them back.
  let expanded = $state<Record<string, boolean>>({});
  const groupKey = (r: WaterfallRow) => `${r.layer}:${r.start}:${r.items?.length ?? 0}`;
  const rows = $derived.by(() => {
    const needle = search.toLowerCase();
    const keep = waterfall.rows.filter((r) => !hidden[filterKey(r)] && (!needle || r.label.toLowerCase().includes(needle)));
    if (!condensed) return keep;
    return condense(keep, total).flatMap((r) => (r.items && expanded[groupKey(r)] ? [r, ...r.items.map((it) => ({ ...it, nested: true }))] : [r]));
  });

  const pct = (n: number) => (n / total) * 100;
  // lanes puts each folded item on the first lane it does not overlap, so
  // calls that ran at once stack instead of hiding behind one another.
  function lanes(items: WaterfallRow[]): { of: number[]; count: number } {
    const ends: number[] = [];
    const of = items.map((it) => {
      let l = ends.findIndex((end) => end <= it.start);
      if (l < 0) l = ends.push(0) - 1;
      ends[l] = Math.max(it.end, it.start + 0.001);
      return l;
    });
    return { of, count: Math.max(ends.length, 1) };
  }
  const fmt = (n: number) => `${n < 1 ? '<1' : n < 10 ? n.toFixed(1) : Math.round(n)} ms`;
  const pos = (n: number) => `${n < 10 ? n.toFixed(1) : Math.round(n)} ms`;
  const took = (r: WaterfallRow) => (r.end > r.start ? fmt(r.end - r.start) : '');
  // A click opens a row's details under it and a second click, a click
  // elsewhere or Escape closes them.
  function toggle(e: MouseEvent, r: WaterfallRow, i: number) {
    e.stopPropagation();
    if (r.items) {
      hover = null;
      expanded = { ...expanded, [groupKey(r)]: !expanded[groupKey(r)] };
      return;
    }
    if (hover?.i === i) {
      hover = null;
      return;
    }
    anchor = e.currentTarget as HTMLElement;
    offsetX = e.clientX - 192 - anchor.getBoundingClientRect().left;
    hover = { r, i, x: 0, y: 0 };
    layout();
  }
  function close(e: Event) {
    if (e instanceof KeyboardEvent && e.key !== 'Escape') return;
    hover = null;
  }
</script>

<div class="rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card text-xs overflow-hidden">
  <div class="flex items-center gap-2 flex-wrap px-3 py-2 border-b border-gray-100 dark:border-lerd-border/60">
    <span class="inline-flex items-center gap-1.5 font-semibold text-gray-800 dark:text-gray-100"><Icon name="clock" class="w-3.5 h-3.5" />{m.requests_timeline()}</span>
    <label class="ml-2 inline-flex items-center gap-1.5 text-gray-500 dark:text-gray-400 cursor-pointer select-none">
      <input type="checkbox" class="rounded-sm border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card text-lerd-red focus:ring-lerd-red" bind:checked={condensed} />
      {m.requests_condense()}
    </label>
    <div class="flex items-center gap-1 ml-auto" role="group" aria-label={m.requests_show()}>
      {#each categories as c (c.key)}
        {@const on = !hidden[c.key]}
        <button type="button" aria-pressed={on} use:tooltip={c.label} aria-label={c.label} onclick={() => (hidden = { ...hidden, [c.key]: on })} class="w-6 h-6 inline-flex items-center justify-center rounded-md transition-colors {on ? `${c.bar} text-white` : 'bg-gray-100 dark:bg-white/5 text-gray-400'}">
          <Icon name="cube" class="w-3.5 h-3.5" />
        </button>
      {/each}
      {#each present as l (l.id)}
        {@const on = !hidden[l.id]}
        <button type="button" aria-pressed={on} use:tooltip={l.label()} aria-label={l.label()} onclick={() => (hidden = { ...hidden, [l.id]: on })} class="w-6 h-6 inline-flex items-center justify-center rounded-md transition-colors {on ? `${l.bar} text-white` : 'bg-gray-100 dark:bg-white/5 text-gray-400'}">
          <Icon name={l.icon} class="w-3.5 h-3.5" />
        </button>
      {/each}
    </div>
    <input class="text-xs px-2 py-1 rounded-md border border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-white/5 w-40" placeholder={m.requests_filter()} bind:value={search} />
  </div>

  <div class="px-3 py-2 space-y-px">
    <div class="relative h-5 rounded-sm bg-gray-300 dark:bg-white/15 text-[10px] text-gray-700 dark:text-gray-200 flex items-center justify-center font-medium">{m.requests_totalTime({ time: fmt(total) })}</div>
    {#each rows as r, i (i)}
      {@const s = styleOf(r)}
      {@const left = pct(r.start)}
      {@const width = r.end > r.start ? Math.max(pct(r.end - r.start), 0.4) : 0}
      {@const edge = r.items ? Math.max(...r.items.map((it) => pct(it.end))) : left + width}
      {@const place = edge < 68 ? 'after' : left > 32 ? 'before' : 'inside'}
      {@const stack = r.items ? lanes(r.items) : null}
      <button type="button" aria-expanded={r.items ? Boolean(expanded[groupKey(r)]) : hover?.i === i} style={stack && stack.count > 1 ? `height: ${8 + stack.count * 5}px` : ''} class="relative block w-full h-6 rounded-sm text-left cursor-pointer {hover?.i === i ? 'ring-1 ring-gray-400 dark:ring-white/30' : ''} {'nested' in r ? 'bg-gray-100/70 dark:bg-white/[0.06]' : i % 2 ? 'bg-gray-50 dark:bg-white/[0.03]' : ''}" onclick={(e) => toggle(e, r, i)}>
        {#if r.items}
          {@const lane = stack ?? lanes(r.items)}
          {#each r.items as it, j (j)}
            {@const w = it.end > it.start ? Math.max(pct(it.end - it.start), 0.4) : 0}
            <span
              class="absolute rounded-sm {s.bar} {w ? '' : 'w-[3px]'} {lane.count > 1 ? 'opacity-55' : ''}"
              style="left: {pct(it.start)}%; {w ? `width: ${w}%;` : ''} {lane.count > 1 ? `top: ${4 + lane.of[j] * 5}px; height: 4px` : 'top: 6px; bottom: 6px'}"
            ></span>
          {/each}
        {:else}
          <span class="absolute top-1 bottom-1 rounded-sm {s.bar} {width ? '' : 'w-[3px]'}" style="left: {left}%; {width ? `width: ${width}%` : ''}"></span>
        {/if}
        <span class="absolute inset-y-0 flex items-center gap-1 whitespace-nowrap {place === 'inside' && r.items ? `px-1.5 rounded-sm bg-white/90 dark:bg-lerd-card/90 ${s.text}` : place === 'inside' ? 'text-white px-1.5' : `max-w-[60%] ${s.text}`}" style={place === 'after' ? `left: calc(${edge}% + 8px)` : place === 'before' ? `right: calc(${100 - left}% + 8px)` : `left: ${left}%; max-width: ${width}%;${r.items ? ' top: 50%; bottom: auto; transform: translateY(-50%); height: 20px;' : ''}`}>
          <Icon name={s.icon} class="w-3 h-3 shrink-0" />
          {#if r.warn || r.items?.some((it) => it.warn)}<Icon name="alert" class="w-3 h-3 shrink-0 text-rose-500" />{/if}
          {#if r.items}<Icon name="chevron" class="w-3 h-3 shrink-0 transition-transform {expanded[groupKey(r)] ? '' : '-rotate-90'}" />{/if}
          <span class="truncate {r.layer === 'query' && !r.items ? 'font-mono' : ''}">{r.items ? `${s.label()} · ${m.requests_events({ count: r.items.length })}` : r.label}</span>
          {#if !r.items && took(r)}<span class="shrink-0 opacity-70">{took(r)}</span>{/if}
        </span>
      </button>
    {/each}
  </div>
</div>

{#if hover}
  {@const s = styleOf(hover.r)}
  <div use:portal bind:this={pop} role="dialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={close} class="fixed z-[9999] w-96 max-h-[calc(100vh-16px)] overflow-y-auto rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl text-xs overflow-hidden" style="left: {hover.x}px; top: {hover.y}px">
    <div class="flex items-center gap-1.5 px-3 py-2 font-semibold border-b border-gray-100 dark:border-lerd-border/60 {s.text}"><Icon name={s.icon} class="w-3.5 h-3.5" />{s.label()}</div>
      {#if hover.r.code}
        <pre class="px-3 py-2 font-mono text-[11px] whitespace-pre-wrap break-words text-gray-700 dark:text-gray-200 max-h-40 overflow-y-auto">{hover.r.code}</pre>
      {:else}
        <p class="px-3 py-2 break-words text-gray-700 dark:text-gray-200">{hover.r.label.slice(0, 600)}</p>
      {/if}
      {#if hover.r.source}
        <div class="border-t border-gray-100 dark:border-lerd-border/60 px-3 py-2 text-[11px]"><CallerSource file={hover.r.source.file} line={hover.r.source.line} trace={hover.r.source.trace} muted={false} nested /></div>
      {/if}
      {#each hover.r.sections ?? [] as sec (sec.title)}
        <div class="border-t border-gray-100 dark:border-lerd-border/60 px-3 py-2">
          <div class="text-[10px] uppercase tracking-wide text-gray-400 mb-1">{sec.title}</div>
          <div class="grid grid-cols-[minmax(0,7rem)_minmax(0,1fr)] gap-x-3 gap-y-0.5 max-h-40 overflow-y-auto">
            {#each Object.entries(sec.values) as [k, v] (k)}
              <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400 truncate" title={k}>{k}</span>
              {#if /^\/[^\s]+$/.test(v)}
                <span class="text-[11px] min-w-0"><SourcePath file={v} short /></span>
              {:else}
                <StructuredValue value={v} class="text-gray-800 dark:text-gray-200 min-w-0" />
              {/if}
            {/each}
          </div>
        </div>
      {/each}
      <div class="grid grid-cols-3 border-t border-gray-100 dark:border-lerd-border/60 text-center">
        <div class="py-1.5"><div class="font-semibold text-gray-800 dark:text-gray-100">{took(hover.r) || '·'}</div><div class="text-[10px] text-gray-400">{m.requests_stat_duration()}</div></div>
        <div class="py-1.5 border-l border-gray-100 dark:border-lerd-border/60"><div class="font-semibold text-gray-800 dark:text-gray-100">{pos(hover.r.start)}</div><div class="text-[10px] text-gray-400">{m.requests_stat_start()}</div></div>
        <div class="py-1.5 border-l border-gray-100 dark:border-lerd-border/60"><div class="font-semibold text-gray-800 dark:text-gray-100">{hover.r.note && !took(hover.r) ? hover.r.note : pos(hover.r.end)}</div><div class="text-[10px] text-gray-400">{hover.r.note && !took(hover.r) ? m.requests_stat_kind() : m.requests_stat_end()}</div></div>
      </div>
  </div>
{/if}

<svelte:window onclick={close} onkeydown={close} />
