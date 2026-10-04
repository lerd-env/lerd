<script lang="ts">
  import { PALETTE, customColor } from '$lib/customColors';
  import { portal } from '$lib/portal';
  import { m } from '../paraglide/messages.js';

  // A chart an app put in a tab of its own: line or bar with a series each,
  // or a pie of one series, drawn as SVG with a legend.

  interface Props {
    chart: string;
    labels: string[];
    series: Array<{ name: string; points: Record<string, number>; color?: string | null }>;
  }
  let { chart, labels, series: given }: Props = $props();
  const isPie = $derived(chart === 'pie' || chart === 'exploded-pie');
  // Each series carries a value per label; lined up on the chart's labels, a
  // label a series has no value for draws as nothing.
  const series = $derived(given.map((s) => ({ ...s, values: labels.map((l) => s.points?.[l] ?? null) })));

  // Drawn at the width it is given, so a chart spanning two columns fills them
  // instead of being scaled up; the height stays the same.
  let width = $state(520);
  const W = $derived(Math.max(width, 200));
  const H = 200;
  // A series without a colour of its own takes the palette's next one.
  const fill = (i: number) => customColor(series[i]?.color ?? PALETTE[i % PALETTE.length]).fill;
  const max = $derived(Math.max(1, ...series.flatMap((s) => s.values.map((v) => Number(v ?? 0)))));
  const x = (i: number) => (labels.length <= 1 ? W / 2 : (i / (labels.length - 1)) * (W - 40) + 20);
  // Bars sit in a band per label, side by side within it.
  const band = $derived((W - 40) / Math.max(labels.length, 1));
  const bandCenter = (i: number) => 20 + band * (i + 0.5);
  const barWidth = $derived(Math.max(4, (band * 0.8) / Math.max(series.length, 1)));
  const xs = (i: number) => (chart === 'bar' ? bandCenter(i) : x(i));
  const y = (v: number) => H - 20 - (v / max) * (H - 40);
  const pie = $derived.by(() => {
    const values = (series[0]?.values ?? []).map(Number);
    const total = values.reduce((a, b) => a + b, 0) || 1;
    let angle = -Math.PI / 2;
    return values.map((v, i) => {
      const a0 = angle;
      angle += (v / total) * Math.PI * 2;
      const large = angle - a0 > Math.PI ? 1 : 0;
      const p = (a: number) => `${90 + 80 * Math.cos(a)} ${90 + 80 * Math.sin(a)}`;
      const mid = (a0 + angle) / 2;
      // An exploded slice moves out along the line through its middle.
      const shift = chart === 'exploded-pie' ? 7 : 0;
      return { d: `M 90 90 L ${p(a0)} A 80 80 0 ${large} 1 ${p(angle)} Z`, dx: shift * Math.cos(mid), dy: shift * Math.sin(mid), color: customColor(PALETTE[i % PALETTE.length]).fill, label: labels[i] ?? '', value: v, share: v / total, mid };
    });
  });

  // The popover sits at the data it describes: beside the highest point of a
  // label on a line or bar chart, by the middle of a slice on a pie, and is
  // kept inside the window.
  let hover = $state<{ x: number; y: number; title: string; rows: Array<{ color: string; name: string; value: string }> } | null>(null);
  let pop = $state<HTMLElement | null>(null);
  let plot = $state<SVGSVGElement | null>(null);
  let piePlot = $state<SVGSVGElement | null>(null);
  const fmt = (v: number) => (Number.isInteger(v) ? String(v) : v.toFixed(2));
  // anchor is the point in the window the popover belongs to; it goes to the
  // right of it, or to the left when there is no room.
  function at(ax: number, ay: number) {
    const x = ax + 236 > window.innerWidth ? ax - 236 : ax + 12;
    return { x: Math.max(8, x), y: ay - 12 };
  }
  $effect(() => {
    if (!hover || !pop) return;
    const h = pop.offsetHeight;
    const y = Math.min(Math.max(8, hover.y), window.innerHeight - h - 8);
    if (y !== hover.y) hover = { ...hover, y };
  });
  // What the popover describes, so it can be placed again as the page scrolls:
  // it moves with its point until the point leaves the window.
  let target: { kind: 'label' | 'slice'; index: number } | null = null;
  $effect(() => {
    const follow = () => {
      if (!hover || !target) return;
      if (target.kind === 'label') showLabel(target.index);
      else showSlice(target.index);
    };
    window.addEventListener('scroll', follow, true);
    window.addEventListener('resize', follow);
    return () => {
      window.removeEventListener('scroll', follow, true);
      window.removeEventListener('resize', follow);
    };
  });
  function showLabel(i: number) {
    if (!plot) return;
    target = { kind: 'label', index: i };
    const box = plot.getBoundingClientRect();
    const present = series.map((s, si) => ({ color: fill(si), name: s.name, value: s.values[i] })).filter((r) => r.value !== null);
    const top = Math.min(...present.map((r) => y(Number(r.value))), H - 20);
    hover = { ...at(box.left + xs(i), box.top + top), title: labels[i] ?? '', rows: present.map((r) => ({ ...r, value: fmt(Number(r.value)) })) };
  }
  function showSlice(index: number) {
    const slice = pie[index];
    if (!piePlot || !slice) return;
    target = { kind: 'slice', index };
    const box = piePlot.getBoundingClientRect();
    const scale = box.width / 196;
    hover = {
      ...at(box.left + (98 + 50 * Math.cos(slice.mid) + slice.dx) * scale, box.top + (98 + 50 * Math.sin(slice.mid) + slice.dy) * scale),
      title: slice.label,
      rows: [{ color: slice.color, name: series[0]?.name ?? '', value: `${fmt(slice.value)} · ${Math.round(slice.share * 100)}%` }]
    };
  }
  const active = $derived(hover ? labels.indexOf(hover.title) : -1);
</script>

{#if isPie}
  <div class="flex items-center gap-6">
    <svg bind:this={piePlot} viewBox="-8 -8 196 196" class="w-40 h-40 shrink-0" role="img">
      {#each pie as s, i (i)}
        <path role="presentation" d={s.d} transform="translate({s.dx} {s.dy})" fill={s.color} class="transition-opacity {hover && hover.title !== s.label ? 'opacity-50' : ''}" onmouseenter={() => showSlice(i)} onmouseleave={() => (hover = null)} />
      {/each}
    </svg>
    <ul class="text-xs space-y-1">
      {#each pie as s, i (i)}<li class="flex items-center gap-2"><span class="w-2.5 h-2.5 rounded-sm" style="background: {s.color}"></span>{s.label} <span class="text-gray-400 tabular-nums">{s.value}</span></li>{/each}
    </ul>
  </div>
{:else if chart === 'line' || chart === 'bar'}
  <div bind:clientWidth={width}>
  <svg bind:this={plot} viewBox="0 0 {W} {H}" width={W} height={H} class="block" role="img">
    <line x1="20" y1={H - 20} x2={W - 20} y2={H - 20} class="stroke-gray-300 dark:stroke-white/20" />
    {#each labels as l, i (i)}<text x={xs(i)} y={H - 4} text-anchor="middle" class="fill-gray-500 text-[10px]">{l}</text>{/each}
    {#if active >= 0 && chart !== 'bar'}
      <line x1={xs(active)} y1="12" x2={xs(active)} y2={H - 20} class="stroke-gray-300 dark:stroke-white/25" stroke-dasharray="3 3" />
    {/if}
    {#each series as s, si (si)}
      {#if chart === 'bar'}
        {#each s.values as v, i (i)}
          {#if v !== null}
          <rect x={bandCenter(i) - (barWidth * series.length) / 2 + si * barWidth} y={y(Number(v))} width={barWidth - 2} height={H - 20 - y(Number(v))} rx="2" fill={fill(si)} class="pointer-events-none transition-opacity {active >= 0 && active !== i ? 'opacity-50' : ''}" />
          {/if}
        {/each}
      {:else}
        <polyline fill="none" stroke={fill(si)} stroke-width="2" points={s.values.map((v, i) => (v === null ? null : `${x(i)},${y(Number(v))}`)).filter(Boolean).join(' ')} />
        {#each s.values as v, i (i)}{#if v !== null}<circle cx={x(i)} cy={y(Number(v))} r={active === i ? 4.5 : 3} fill={fill(si)} class="pointer-events-none" />{/if}{/each}
      {/if}
    {/each}
    <!-- A column per label catches the pointer, so a label shows every series at once. -->
    {#each labels as l, i (i)}
      <rect role="presentation" x={xs(i) - (chart === 'bar' ? band : (W - 40) / Math.max(labels.length - 1, 1)) / 2} y="0" width={chart === 'bar' ? band : (W - 40) / Math.max(labels.length - 1, 1)} height={H - 20} fill="transparent" onmouseenter={() => showLabel(i)} onmouseleave={() => (hover = null)} />
    {/each}
  </svg>
  </div>
  {#if series.length > 1}
    <ul class="flex flex-wrap gap-3 text-xs mt-1">
      {#each series as s, si (si)}<li class="flex items-center gap-1.5"><span class="w-2.5 h-2.5 rounded-sm" style="background: {fill(si)}"></span>{s.name}</li>{/each}
    </ul>
  {/if}
{:else}
  <p class="text-xs text-gray-500 dark:text-gray-400">{m.requests_chartUnsupported({ type: chart })}</p>
{/if}

{#if hover}
  <div use:portal bind:this={pop} role="tooltip" class="fixed z-[9999] w-56 max-h-[calc(100vh-16px)] overflow-hidden pointer-events-none rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl text-xs" style="left: {hover.x}px; top: {hover.y}px">
    <div class="px-3 py-1.5 font-semibold text-gray-800 dark:text-gray-100 border-b border-gray-100 dark:border-lerd-border/60">{hover.title}</div>
    <ul class="px-3 py-1.5 space-y-1">
      {#each hover.rows as r (r.name)}
        <li class="flex items-center gap-2"><span class="w-2.5 h-2.5 rounded-sm shrink-0" style="background: {r.color}"></span><span class="flex-1 truncate text-gray-600 dark:text-gray-300">{r.name}</span><span class="font-mono tabular-nums text-gray-800 dark:text-gray-100">{r.value}</span></li>
      {/each}
    </ul>
  </div>
{/if}
