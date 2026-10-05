<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { loadRequest, type RequestDetail as Detail } from '$stores/requests';
  import RequestDetail from '$components/RequestDetail.svelte';
  import Modal from '$components/Modal.svelte';
  import { tooltip } from '$lib/tooltip';
  import { summarize, path, popoverPlace } from './chips';
  import { clock } from '$lib/requestTime';
  import type { BarConfig } from './config';
  import { m } from '../paraglide/messages.js';

  // The bar a site's page carries: the request behind the page in a row of
  // chips, each opening the dashboard's own request view on its tab.
  interface Props {
    rid: string;
    config: BarConfig;
  }
  let { rid: served, config }: Props = $props();
  // svelte-ignore state_referenced_locally
  let rid = $state(served || document.documentElement.getAttribute('data-lerd-page') || '');
  function follow(e: Event) {
    if (served) return;
    rid = (e as CustomEvent<string>).detail;
    void refresh(true);
  }

  let d = $state<Detail | null>(null);
  // A PHP page's request is in once its own request event is, the response
  // sent; until then the bar shows it is loading rather than half a request.
  // A page no PHP request served is in as soon as lerd has its page view.
  const loaded = $derived(!!d && (!served || !!d.events?.request?.length));
  const s = $derived(loaded && d ? summarize(d) : null);

  // ponytail: polls every 3s while the page is visible, for the requests the
  // page sends after it loads; a stream if it ever shows up in a profile.
  let timer: ReturnType<typeof setInterval> | undefined;
  async function refresh(always = false) {
    if (!rid) rid = document.documentElement.getAttribute('data-lerd-page') ?? '';
    if (!rid || (!always && d && document.visibilityState !== 'visible')) return;
    try {
      d = await loadRequest(rid);
    } catch {
      /* not reported yet, or it has left the ring: keep what was shown */
    }
    // Until the request is in, ask again soon rather than at the next poll.
    if (!loaded && tries++ < 40) setTimeout(() => refresh(true), 500);
  }
  let tries = 0;
  onMount(() => {
    void refresh(true);
    timer = setInterval(refresh, 3000);
    document.addEventListener('lerd:page', follow);
  });
  onDestroy(() => {
    clearInterval(timer);
    document.removeEventListener('lerd:page', follow);
  });

  const store = {
    get(key: string): string | null {
      try {
        return localStorage.getItem(key);
      } catch {
        return null;
      }
    },
    set(key: string, value: string | null) {
      try {
        if (value === null) localStorage.removeItem(key);
        else localStorage.setItem(key, value);
      } catch {}
    }
  };
  const CORNER_KEY = 'lerd:debugbar:corner';
  const OPEN_KEY = 'lerd:debugbar:open';
  const corners = ['bottom-right', 'bottom-left', 'top-right', 'top-left'];

  // Collapsed until someone opens it, then remembered per browser, like the
  // corner it was dragged to, which wins over lerd's setting.
  let min = $state(store.get(OPEN_KEY) !== '1');
  // svelte-ignore state_referenced_locally
  let corner = $state(corners.includes(store.get(CORNER_KEY) ?? '') ? store.get(CORNER_KEY)! : config.corner);
  const compact = $derived(config.style === 'compact');
  let bar: HTMLElement | null = $state(null);

  // morph animates the bar from where it was to where a change puts it.
  // The mark's tooltip waits while it moves, or a pointer it passes under
  // would leave the tooltip where it was.
  let moving = $state(false);
  function morph(change: () => void) {
    if (!bar) return change();
    const first = bar.getBoundingClientRect();
    change();
    requestAnimationFrame(() => {
      if (!bar || window.matchMedia?.('(prefers-reduced-motion: reduce)').matches || !bar.animate) return;
      const last = bar.getBoundingClientRect();
      if (!last.width || !last.height) return;
      moving = true;
      setTimeout(() => (moving = false), 300);
      bar.animate([{ transform: `translate(${first.left - last.left}px, ${first.top - last.top}px) scale(${first.width / last.width}, ${first.height / last.height})`, transformOrigin: '0 0' }, { transform: 'none', transformOrigin: '0 0' }], { duration: 280, easing: 'cubic-bezier(.2,.8,.2,1)' });
      bar.firstElementChild?.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 220, delay: 60, fill: 'backwards' });
    });
  }
  function setMin(v: boolean) {
    pop = '';
    morph(() => (min = v));
    store.set(OPEN_KEY, v ? null : '1');
  }

  // A compact strip takes its own room on the page, so the page's content
  // starts beside it rather than under it.
  $effect(() => {
    const strip = compact && !min;
    const h = strip && bar ? bar.getBoundingClientRect().height : 0;
    const root = document.documentElement.style;
    root.paddingTop = strip && config.edge === 'top' ? `${h}px` : '';
    root.paddingBottom = strip && config.edge === 'bottom' ? `${h}px` : '';
  });

  // The mark restores a minimised bar; dragging it moves it to the nearest corner.
  let drag: { x: number; y: number; dx: number; dy: number; moved: boolean } | null = null;
  let dragged = false;
  let dragPos = $state<{ left: number; top: number } | null>(null);
  function down(e: PointerEvent) {
    if (!min || !bar) return;
    const r = bar.getBoundingClientRect();
    drag = { x: e.clientX, y: e.clientY, dx: e.clientX - r.left, dy: e.clientY - r.top, moved: false };
    (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
  }
  function move(e: PointerEvent) {
    if (!drag) return;
    if (!drag.moved && Math.hypot(e.clientX - drag.x, e.clientY - drag.y) < 5) return;
    drag.moved = true;
    dragPos = { left: e.clientX - drag.dx, top: e.clientY - drag.dy };
  }
  function up(e: PointerEvent) {
    if (!drag) return;
    const moved = drag.moved;
    drag = null;
    dragPos = null;
    if (!moved) return;
    corner = `${e.clientY < innerHeight / 2 ? 'top' : 'bottom'}-${e.clientX < innerWidth / 2 ? 'left' : 'right'}`;
    store.set(CORNER_KEY, corner);
    dragged = true;
  }
  function markClick() {
    if (dragged) return void (dragged = false);
    if (min) setMin(false);
  }

  // The chip a view opened from, so the dialog grows out of it.
  let origin = $state<DOMRect | null>(null);
  let view = $state<{ rid: string; tab: string } | null>(null);
  function open(e: Event, tab: string, target = rid) {
    origin = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (pop) origin = (bar?.querySelector(`[data-pop="${pop}"]`) as HTMLElement | null)?.getBoundingClientRect() ?? origin;
    pop = '';
    view = { rid: target, tab };
  }

  // The two lists a chip opens, placed away from the edge the bar sits on.
  let pop = $state<'' | 'tabs' | 'children'>('');
  let popPos = $state('');
  function toggle(e: Event, which: 'tabs' | 'children') {
    if (pop === which) return void (pop = '');
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    popPos = popoverPlace(r, innerWidth, innerHeight);
    pop = which;
  }

  const INLINE = 2;
  const shownTabs = $derived(s ? s.appTabs.slice(0, s.appTabs.length > INLINE + 1 ? INLINE : INLINE + 1) : []);
  const moreTabs = $derived(s ? s.appTabs.slice(shownTabs.length) : []);
  const ms = (n: number) => `${n < 1 ? '<1' : n < 10 ? n.toFixed(1) : Math.round(n)} ms`;
  const pct = (n: number) => `${Math.max(n * 100, 0)}%`;
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && (pop = '')} onclick={() => (pop = '')} />

{#snippet icon(d: string)}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path {d} /></svg>{/snippet}

<!-- Until the request is in, only the mark shows, so the bar never vanishes
     while lerd-ui restarts or a page view has not been reported yet. -->
<div
    bind:this={bar}
    class="lbar c-{corner}"
    class:min
    class:compact
    class:edge-top={compact && config.edge === 'top'}
    class:dragging={!!dragPos}
    style={dragPos ? `left:${dragPos.left}px;top:${dragPos.top}px;right:auto;bottom:auto` : ''}
    role="region"
    aria-label={m.debugbar_label()}
  >
    <div class="lbar-in">
      <button class="mark" class:loading={!s} type="button" onpointerdown={down} onpointermove={move} onpointerup={up} onclick={markClick} aria-label={m.debugbar_show()} use:tooltip={min && !moving && !dragPos ? (s ? m.debugbar_show() : m.debugbar_waiting()) : ''}>L</button>
      {#if !s}
        <span class="chip loading" role="status"><span class="spin" aria-hidden="true"></span><span class="k">{m.debugbar_loading()}</span></span>
        <button class="icon-btn" type="button" onclick={(e) => { e.stopPropagation(); setMin(true); }} aria-label={m.debugbar_minimise()} use:tooltip={m.debugbar_minimise()}>{@render icon('M6 6l12 12M18 6L6 18')}</button>
      {:else}
      <button class="chip req" type="button" onclick={(e) => open(e, 'request')} use:tooltip={m.debugbar_openRequest()}>
        <span class="dot" class:err={s.failed}></span>{#if s.method}<span class="method">{s.method}</span>{/if}<span class="font-mono">{s.uri}</span>{#if s.status}<span class={s.status >= 400 ? 'err' : 'ok'}>{s.status}</span>{/if}
      </button>
      {#if s.timeMs}
      <button class="chip" type="button" onclick={(e) => open(e, 'performance')} use:tooltip={m.debugbar_time()}>
        {@render icon('M12 9v4l2 2M9 2h6M20 13a8 8 0 1 1-16 0 8 8 0 0 1 16 0z')}
        <span class="tabular-nums">{ms(s.timeMs)}</span>
        <span class="phase" aria-hidden="true"><i style="width:{pct(s.phases.server)};background:var(--slate)"></i><i style="width:{pct(s.phases.app)};background:var(--blue)"></i><i style="width:{pct(s.phases.db)};background:var(--yellow)"></i></span>
      </button>
      {/if}
      {#if s.memory}
        <button class="chip" type="button" onclick={(e) => open(e, 'performance')} use:tooltip={m.requests_stat_memory()}>{@render icon('M4 8a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2zM8 10v4M12 10v4M16 10v4')}<span class="tabular-nums">{s.memory}</span></button>
      {/if}
      {#if s.queries || s.views || s.cache || s.logs || s.appTabs.length}<span class="sep"></span>{/if}
      {#if s.queries}
        <button class="chip" type="button" onclick={(e) => open(e, 'database')} use:tooltip={m.requests_tab_database()}>{@render icon('M4 5c0 1.7 3.6 3 8 3s8-1.3 8-3-3.6-3-8-3-8 1.3-8 3zm0 0v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3')}<span class="tabular-nums">{s.queries}</span><span class="k lbl">{m.debugbar_queries()}</span>{#if s.nPlusOne}<span class="pill pill-amber">N+1</span>{/if}</button>
      {/if}
      {#if s.views}
        <button class="chip" type="button" onclick={(e) => open(e, 'views')} use:tooltip={m.requests_tab_views()}>{@render icon('M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12zm13 0a3 3 0 1 1-6 0 3 3 0 0 1 6 0z')}<span class="tabular-nums">{s.views}</span><span class="k lbl">{m.debugbar_views()}</span></button>
      {/if}
      {#if s.cache}
        <button class="chip" type="button" onclick={(e) => open(e, 'cache')} use:tooltip={m.requests_tab_cache()}>{@render icon('M6 3h12v18l-6-4-6 4z')}<span class="tabular-nums">{s.cache}</span><span class="k lbl">{m.debugbar_cache()}</span></button>
      {/if}
      {#if s.logs}
        <button class="chip" type="button" onclick={(e) => open(e, 'log')} use:tooltip={m.requests_section_logs()}>{@render icon('M4 6h16M4 12h16M4 18h10')}<span class="tabular-nums">{s.logs}</span><span class="k lbl">{m.debugbar_logs()}</span></button>
      {/if}
      {#each shownTabs as t (t.id)}
        <button class="chip" type="button" onclick={(e) => open(e, t.id)} use:tooltip={m.debugbar_appTab({ name: t.title })}>{@render icon('M12 2l9 5v10l-9 5-9-5V7z')}<span class="k">{t.title}</span></button>
      {/each}
      {#if moreTabs.length}
        <button class="chip" type="button" data-pop="tabs" aria-expanded={pop === 'tabs'} onclick={(e) => { e.stopPropagation(); toggle(e, 'tabs'); }} use:tooltip={pop === 'tabs' ? '' : m.debugbar_moreTabs({ count: moreTabs.length })}>{@render icon('M12 2l9 5v10l-9 5-9-5V7z')}<span class="k">+{moreTabs.length}</span></button>
      {/if}
      {#if s.user || s.children.length}<span class="sep"></span>{/if}
      {#if s.user}
        <button class="chip" type="button" onclick={(e) => open(e, 'request')} use:tooltip={m.debugbar_user()}>{@render icon('M16 8a4 4 0 1 1-8 0 4 4 0 0 1 8 0zM4 21c1.5-4 4.5-6 8-6s6.5 2 8 6')}<span>{s.user}</span></button>
      {/if}
      {#if s.children.length}
        <button class="chip" type="button" data-pop="children" aria-expanded={pop === 'children'} onclick={(e) => { e.stopPropagation(); toggle(e, 'children'); }} use:tooltip={pop === 'children' ? '' : m.debugbar_children()}>{@render icon('M7 7h11l-3-3M17 17H6l3 3')}<span class="tabular-nums">{s.children.length}</span><span class="k lbl">{m.debugbar_childrenLabel()}</span>{#if s.childFailures}<span class="pill pill-red">{m.debugbar_failed({ count: s.childFailures })}</span>{/if}</button>
      {/if}
      <button class="icon-btn" type="button" onclick={(e) => { e.stopPropagation(); setMin(true); }} aria-label={m.debugbar_minimise()} use:tooltip={m.debugbar_minimise()}>{@render icon('M6 6l12 12M18 6L6 18')}</button>
      {/if}
    </div>
  </div>

{#if s}
  {#if pop}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <div class="bar-pop" style={popPos} onclick={(e) => e.stopPropagation()}>
      {#if pop === 'tabs'}
        <h4>{m.debugbar_appTabs()}</h4>
        <div class="rows">
          {#each moreTabs as t (t.id)}
            <button class="bar-row" style="grid-template-columns:18px minmax(0,1fr)" type="button" onclick={(e) => open(e, t.id)}>{@render icon('M12 2l9 5v10l-9 5-9-5V7z')}<span>{t.title}</span></button>
          {/each}
        </div>
      {:else}
        <h4>{m.debugbar_childrenLabel()}</h4>
        <div class="rows">
          {#each s.children as c (c.rid)}
            <button class="bar-row" style="grid-template-columns:minmax(0,1fr) 36px 52px 92px" type="button" onclick={(e) => open(e, 'performance', c.rid)}>
              <span class="path">{path(c.url)}</span><span class={c.status >= 400 || !c.status ? 'err' : 'ok'}>{c.status || '—'}</span><span class="tabular-nums text-right">{c.ms ? ms(c.ms) : ''}</span><span class="tabular-nums text-right whitespace-nowrap" style="color:var(--muted)">{clock(c.at)}</span>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
{/if}

<Modal open={!!view} title={m.requests_detail_title()} size="full" {origin} onclose={() => (view = null)}>
  {#if view}
    {#key view.rid + view.tab}<RequestDetail rid={view.rid} initialTab={view.tab} onopen={(r) => (view = { rid: r, tab: 'performance' })} />{/key}
  {/if}
</Modal>
