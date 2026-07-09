<script lang="ts">
  import ListPanel from '$components/ListPanel.svelte';
  import ActionButton from '$components/ActionButton.svelte';
  import DumpBridgeToggle from '$components/DumpBridgeToggle.svelte';
  import ProfilerToggle from '$components/ProfilerToggle.svelte';
  import EmptyState from '$components/EmptyState.svelte';
  import Icon from '$components/Icon.svelte';
  import SiteIcon from '$components/SiteIcon.svelte';
  import SiteIndicators from '$components/SiteIndicators.svelte';
  import SitesSectionHeader from '$components/SitesSectionHeader.svelte';
  import LoadingRow from '$components/LoadingRow.svelte';
  import { accessMode } from '$stores/accessMode';
  import { routeRest, goToTab } from '$stores/route';
  import { sites, sitesLoaded, type Site } from '$stores/sites';
  import { sitesSort, type SitesSort } from '$stores/sitesSort';
  import { status } from '$stores/status';
  import {
    UNGROUPED,
    createWorkspace,
    renameWorkspace,
    saveWorkspaceLayout,
    toggleWorkspaceCollapse,
    workspaceCollapse,
    type WorkspaceLayoutEntry
  } from '$stores/workspaces';
  import { openLinkModal, openWorkspaceDeleteModal } from '$stores/modals';
  import { get } from 'svelte/store';
  import { flushSync, untrack } from 'svelte';
  import { dndzone, SOURCES, TRIGGERS, type DndEvent } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';
  import { m } from '../paraglide/messages.js';

  // routeRest may carry a sub-tab (e.g. "<domain>/env"); match the domain segment
  // so the sidebar row stays highlighted when a sub-tab is deep-linked.
  const selected = $derived($routeRest.split('/')[0]);
  const active = $derived($sites.filter((s) => !s.paused));
  const paused = $derived($sites.filter((s) => s.paused));

  // The active secondaries pinned under a main, drawn from the given list. Single
  // source of truth for the grouping rule, used both for rendering and persisting.
  function secondariesOf(list: Site[], main: Site): Site[] {
    if (!main.group) return [];
    return list.filter((x) => !x.paused && x.group === main.group && x.group_subdomain);
  }
  function secondariesFor(s: Site): Site[] {
    return secondariesOf(active, s);
  }

  // The main (non-secondary) active rows, ordered per the chosen sort mode.
  // Only mains are sorted; secondaries always stay pinned under their main.
  const mains = $derived(active.filter((s) => !s.group_subdomain));
  const sortedMains = $derived.by(() => {
    const list = [...mains];
    switch ($sitesSort) {
      case 'alpha':
        return list.sort((a, b) => a.domain.localeCompare(b.domain));
      case 'recent':
        // Newest activity first; sites with no log activity sink to the bottom.
        return list.sort((a, b) => (b.latest_log_time || '').localeCompare(a.latest_log_time || ''));
      case 'newest':
        return list.reverse();
      case 'manual':
      default:
        return list;
    }
  });
  // Reordering is available whenever we can write (loopback), in any sort mode.
  // Dragging a site auto-switches the list into manual mode (see persistRowDrop).
  const canReorder = $derived($accessMode.loopback);

  // Collapse key for the paused block. Leading space, like UNGROUPED, so it can
  // never collide with a workspace name (the server trims those).
  const PAUSED = ' paused';

  // Workspaces are display-only sections. With none configured the list renders
  // exactly as it did before: one zone, no headers.
  let wsOrder = $state<string[]>([]);
  const hasWorkspaces = $derived(wsOrder.length > 0);
  const sectionKeys = $derived([...wsOrder, UNGROUPED]);

  function sectionOf(s: Site): string {
    return s.workspace && wsOrder.includes(s.workspace) ? s.workspace : UNGROUPED;
  }

  // Active secondaries whose main isn't active (e.g. the main is paused) render
  // on their own below the sections so they never disappear.
  const orphanSecondaries = $derived.by(() => {
    const covered = new Set<string>();
    for (const main of sortedMains) for (const sec of secondariesFor(main)) covered.add(sec.domain);
    return active.filter((s) => s.group_subdomain && !covered.has(s.domain));
  });

  // Drag-and-drop via svelte-dnd-action, in two layers. Rows move within and
  // between section zones; workspace headers reorder the sections themselves.
  // Each row item is a main carrying its secondaries, so a whole group moves and
  // animates together. Dragging a row flips the list into manual mode, seeded
  // from whatever order is shown at drag start.
  type DndItem = { id: string; site: Site };
  const FLIP_MS = 180;
  // Unique per instance so the desktop and mobile lists aren't connected zones.
  // Every section shares this one type so a row can cross between them.
  const suffix = Math.random().toString(36).slice(2, 9);
  const dndType = 'sites-' + suffix;
  const wsDndType = 'workspaces-' + suffix;

  let dragDisabled = $state(true);
  // Workspace headers are draggable outright: svelte-dnd-action only starts a
  // drag after 3px of movement, so a plain click still toggles the section.
  let headerDragging = $state(false);
  let savingLayout = $state(false);
  let zones = $state<Record<string, DndItem[]>>({});
  let wsItems = $state<{ id: string }[]>([]);

  // True from the moment a row is picked up until the drop has settled. It
  // outlives dragDisabled, which svelte-dnd-action needs flipped back on the
  // first finalize of a cross-zone drop, and it keeps the live snapshot from
  // resyncing the zones out from under the drag.
  let rowDragActive = $state(false);

  // A collapsed section stays collapsed through a drag. Its zone is simply not
  // mounted, so it is not a drop target and nothing can unmount mid-drag; its
  // members are still carried through every layout we persist.
  function isCollapsed(key: string): boolean {
    return $workspaceCollapse.includes(key);
  }

  $effect(() => {
    // Resync from the server whenever we're not mid-drag and no write is in
    // flight, so a live status push never yanks the list out from under a drag
    // or reverts an order we just persisted optimistically.
    if (!dragDisabled || rowDragActive || headerDragging || savingLayout) return;
    const names = $status.workspaces ?? [];
    const sorted = sortedMains;
    untrack(() => {
      if (names.length !== wsOrder.length || names.some((n, i) => n !== wsOrder[i])) {
        wsOrder = [...names];
      }
      if (names.length !== wsItems.length || names.some((n, i) => n !== wsItems[i].id)) {
        wsItems = names.map((n) => ({ id: n }));
      }
      syncZones(sorted);
    });
  });

  // Rebuild each section's items from the sorted source. When only the row data
  // changed (same order, e.g. a live status push) refresh in place so the dnd
  // zones aren't rebuilt on every snapshot.
  function syncZones(sorted: Site[]) {
    const next: Record<string, Site[]> = {};
    for (const key of sectionKeys) next[key] = [];
    for (const s of sorted) (next[sectionOf(s)] ??= []).push(s);

    for (const key of sectionKeys) {
      const want = next[key] ?? [];
      const cur = zones[key] ?? [];
      const sameOrder = want.length === cur.length && want.every((s, i) => s.domain === cur[i].id);
      if (sameOrder) {
        want.forEach((s, i) => {
          if (cur[i].site !== s) cur[i].site = s;
        });
      } else {
        zones[key] = want.map((s) => ({ id: s.domain, site: s }));
      }
    }
    for (const key of Object.keys(zones)) {
      if (!sectionKeys.includes(key)) delete zones[key];
    }
  }

  // Set on the first consider, so a press on the grip that never became a drag
  // (a plain click) can put the drag state back instead of leaving it latched.
  let dragStarted = false;

  function startDrag(e: MouseEvent | TouchEvent) {
    if (e instanceof MouseEvent && e.button !== 0) return;
    e.preventDefault();
    dragStarted = false;
    dragDisabled = false;
    rowDragActive = true;
    const settle = () => {
      if (dragStarted) return; // a real drag ends on finalize instead
      dragDisabled = true;
      rowDragActive = false;
    };
    window.addEventListener('mouseup', settle, { once: true });
    window.addEventListener('touchend', settle, { once: true });
    // svelte-dnd-action only attaches its drag listener while enabled; flush the
    // state change now so the listener catches this same press as it bubbles up.
    flushSync();
  }

  // Real (non-delegated) listener on the handle. Svelte 5 delegates mousedown to
  // the document root, which would run after the event already passed the item;
  // attaching directly here lets flushSync wire up the drag before it bubbles up.
  function dragHandle(node: HTMLElement) {
    const onDown = (e: Event) => startDrag(e as MouseEvent | TouchEvent);
    node.addEventListener('mousedown', onDown);
    node.addEventListener('touchstart', onDown, { passive: false });
    return {
      destroy() {
        node.removeEventListener('mousedown', onDown);
        node.removeEventListener('touchstart', onDown);
      }
    };
  }

  // Keeps a press on a row from reaching the workspace zone that wraps it,
  // which is always live and would otherwise drag the whole workspace. The
  // row's own grip listener has already run by the time this fires.
  function stopDragBubbling(node: HTMLElement) {
    const stop = (e: Event) => e.stopPropagation();
    node.addEventListener('mousedown', stop);
    node.addEventListener('touchstart', stop, { passive: true });
    return {
      destroy() {
        node.removeEventListener('mousedown', stop);
        node.removeEventListener('touchstart', stop);
      }
    };
  }

  function rowConsider(key: string, e: CustomEvent<DndEvent<DndItem>>) {
    zones[key] = e.detail.items;
    dragStarted = true;
    const { source, trigger } = e.detail.info;
    if (source === SOURCES.KEYBOARD && trigger === TRIGGERS.DRAG_STOPPED) dragDisabled = true;
  }
  function rowFinalize(key: string, e: CustomEvent<DndEvent<DndItem>>) {
    zones[key] = e.detail.items;
    if (e.detail.info.source === SOURCES.POINTER) dragDisabled = true;
    schedulePersist();
  }

  // A drop across sections finalizes the source zone and the target zone in the
  // same tick. Coalesce them so the layout is built once, from both, and one
  // write goes out instead of two racing ones.
  let persistTimer: ReturnType<typeof setTimeout> | undefined;
  function schedulePersist() {
    clearTimeout(persistTimer);
    persistTimer = setTimeout(() => {
      rowDragActive = false;
      persistRowDrop();
    }, 0);
  }

  function headerConsider(e: CustomEvent<DndEvent<{ id: string }>>) {
    wsItems = e.detail.items;
    headerDragging = true;
  }
  function headerFinalize(e: CustomEvent<DndEvent<{ id: string }>>) {
    wsItems = e.detail.items;
    headerDragging = false;
    persistHeaderDrop();
  }

  // The workspace list implied by the current zones. Sites that never appear in
  // a zone (paused, orphan secondaries) keep the membership the server gave
  // them, so a drag never silently ungroups them.
  function layoutFromZones(order: string[]): WorkspaceLayoutEntry[] {
    const all = get(sites);
    const moved = new Map<string, string>();
    for (const key of [...order, UNGROUPED]) {
      const workspace = key === UNGROUPED ? '' : key;
      for (const item of zones[key] ?? []) {
        if (item.site.name) moved.set(item.site.name, workspace);
        for (const sec of secondariesOf(all, item.site)) {
          if (sec.name) moved.set(sec.name, workspace);
        }
      }
    }
    const members = new Map<string, string[]>(order.map((n) => [n, []]));
    for (const s of all) {
      if (!s.name) continue;
      const workspace = moved.has(s.name) ? moved.get(s.name)! : (s.workspace ?? '');
      members.get(workspace)?.push(s.name);
    }
    return order.map((name) => ({ name, sites: members.get(name) ?? [] }));
  }

  // The flat registry order implied by the current zones: sections top to
  // bottom, each main followed by its pinned secondaries. Anything not on
  // screen (paused, orphans) trails behind in its existing order.
  function siteOrderFromZones(order: string[]): Site[] {
    const all = get(sites);
    const byDomain = new Map(all.map((s) => [s.domain, s]));
    const next: Site[] = [];
    const used = new Set<string>();
    for (const key of [...order, UNGROUPED]) {
      for (const item of zones[key] ?? []) {
        const main = byDomain.get(item.id);
        if (!main || used.has(main.domain)) continue;
        next.push(main);
        used.add(main.domain);
        for (const sec of secondariesOf(all, main)) {
          if (!used.has(sec.domain)) {
            next.push(sec);
            used.add(sec.domain);
          }
        }
      }
    }
    for (const s of all) if (!used.has(s.domain)) next.push(s);
    return next;
  }

  // A row drag changes membership and the manual order, so it persists both in
  // one write. Optimistic; the KindSites/KindStatus pushes reconcile to server
  // truth, and a rejected write puts the previous order straight back.
  async function persistRowDrop() {
    const prevSites = get(sites);
    const prevSort = get(sitesSort);
    const layout = layoutFromZones(wsOrder);
    const ordered = siteOrderFromZones(wsOrder);

    const workspaceOf = new Map<string, string>();
    for (const ws of layout) for (const name of ws.sites) workspaceOf.set(name, ws.name);

    savingLayout = true;
    sitesSort.set('manual'); // dragging is what enables manual ordering
    sites.set(ordered.map((s) => ({ ...s, workspace: s.name ? (workspaceOf.get(s.name) ?? '') : '' })));

    const order = ordered.map((s) => s.name).filter((n): n is string => Boolean(n));
    const res = await saveWorkspaceLayout(layout, order);
    savingLayout = false;
    if (!res.ok) {
      sites.set(prevSites);
      sitesSort.set(prevSort);
      console.error('workspace layout failed:', res.error);
    }
  }

  // Reordering the sections only moves whole blocks, so the registry order is
  // left alone and sites.yaml is never rewritten.
  async function persistHeaderDrop() {
    const prevOrder = [...wsOrder];
    const order = wsItems.map((w) => w.id);
    savingLayout = true;
    wsOrder = order;
    const res = await saveWorkspaceLayout(layoutFromZones(order));
    savingLayout = false;
    if (!res.ok) {
      wsOrder = prevOrder;
      wsItems = prevOrder.map((n) => ({ id: n }));
      console.error('workspace reorder failed:', res.error);
    }
  }

  function select(s: Site) {
    goToTab('sites', s.domain);
  }

  // ── workspace create / rename / delete ──────────────────────────────────────

  let addingWorkspace = $state(false);
  let newWorkspaceName = $state('');
  let renamingKey = $state<string | null>(null);
  let renameValue = $state('');
  let menuKey = $state<string | null>(null);

  async function submitNewWorkspace() {
    const name = newWorkspaceName.trim();
    if (!name) return;
    addingWorkspace = false;
    newWorkspaceName = '';
    const res = await createWorkspace(name);
    if (!res.ok) console.error('create workspace failed:', res.error);
  }

  function startRename(key: string) {
    menuKey = null;
    renamingKey = key;
    renameValue = key;
  }

  async function submitRename() {
    const key = renamingKey;
    const next = renameValue.trim();
    renamingKey = null;
    if (!key || !next || next === key) return;
    const res = await renameWorkspace(key, next);
    if (!res.ok) console.error('rename workspace failed:', res.error);
  }

  function removeWorkspace(key: string) {
    menuKey = null;
    openWorkspaceDeleteModal({ name: key, siteCount: zones[key]?.length ?? 0 });
  }

  // ── sort menu ───────────────────────────────────────────────────────────────

  const sortOptions: Array<{ value: SitesSort; label: string }> = $derived([
    { value: 'recent', label: m.sites_sort_recent() },
    { value: 'alpha', label: m.sites_sort_alpha() },
    { value: 'newest', label: m.sites_sort_newest() }
  ]);

  let sortMenuOpen = $state(false);
  let overlayEl: HTMLDivElement | undefined = $state();

  function pickSort(v: SitesSort) {
    sitesSort.set(v);
    sortMenuOpen = false;
  }
  function onOverlayDocClick(e: MouseEvent) {
    if (overlayEl && !overlayEl.contains(e.target as Node)) {
      sortMenuOpen = false;
      addingWorkspace = false;
    }
  }
  function onOverlayKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      sortMenuOpen = false;
      addingWorkspace = false;
    }
  }
  $effect(() => {
    if (!sortMenuOpen && !addingWorkspace) return;
    document.addEventListener('mousedown', onOverlayDocClick);
    document.addEventListener('keydown', onOverlayKey);
    return () => {
      document.removeEventListener('mousedown', onOverlayDocClick);
      document.removeEventListener('keydown', onOverlayKey);
    };
  });

  // The section menus are separate popovers; one document listener closes them.
  $effect(() => {
    if (!menuKey) return;
    const close = () => (menuKey = null);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') menuKey = null;
    };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', onKey);
    };
  });
</script>

{#snippet actions()}
  {#if $accessMode.loopback}
    <DumpBridgeToggle />
    <ProfilerToggle />
    <ActionButton title={m.sites_linkNew()} tone="accent" onclick={openLinkModal}>
      <Icon name="plus" class="w-3.5 h-3.5" />
    </ActionButton>
  {/if}
{/snippet}

{#snippet parkHint()}
  {@html m.sites_emptyHint({ cmd: '<code class="bg-gray-100 dark:bg-white/5 px-1 rounded-sm">lerd park</code>' })}
{/snippet}

{#snippet overlayControls()}
  <!-- Spans the panel so the popovers can size to the column rather than to the
       button they hang off; the strip itself stays click-through. -->
  <div
    bind:this={overlayEl}
    class="absolute bottom-3 left-3 right-3 z-20 flex items-center justify-end gap-2 pointer-events-none"
  >
    {#if addingWorkspace}
      <div
        class="absolute bottom-full left-0 right-0 mb-2 rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl p-2 pointer-events-auto"
      >
        <!-- svelte-ignore a11y_autofocus -->
        <input
          autofocus
          bind:value={newWorkspaceName}
          onkeydown={(e) => e.key === 'Enter' && submitNewWorkspace()}
          placeholder={m.workspaces_namePlaceholder()}
          class="w-full px-2 py-1.5 text-xs rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-bg text-gray-800 dark:text-gray-200 focus:outline-none focus:border-lerd-red"
        />
        <div class="flex justify-end gap-1.5 mt-2">
          <button
            type="button"
            onclick={() => (addingWorkspace = false)}
            class="px-2 py-1 text-xs text-gray-500 hover:text-gray-700 dark:hover:text-gray-300">{m.common_cancel()}</button
          >
          <button
            type="button"
            onclick={submitNewWorkspace}
            class="px-2 py-1 text-xs font-medium rounded-md bg-lerd-red hover:bg-lerd-redhov text-white">{m.common_add()}</button
          >
        </div>
      </div>
    {/if}

    {#if sortMenuOpen}
      <div
        role="menu"
        class="absolute bottom-full right-0 mb-2 min-w-44 max-w-full rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl py-1 pointer-events-auto"
      >
        {#each sortOptions as opt (opt.value)}
          <button
            type="button"
            role="menuitemradio"
            aria-checked={$sitesSort === opt.value}
            onclick={() => pickSort(opt.value)}
            class="w-full flex items-center gap-2 px-3 py-1.5 text-left text-xs transition-colors hover:bg-gray-50 dark:hover:bg-white/5 {$sitesSort ===
            opt.value
              ? 'text-lerd-red font-semibold'
              : 'text-gray-700 dark:text-gray-200'}"
          >
            <span class="w-3 h-3 shrink-0">
              {#if $sitesSort === opt.value}<Icon name="check" class="w-3 h-3" />{/if}
            </span>
            <span class="flex-1">{opt.label}</span>
          </button>
        {/each}
      </div>
    {/if}

    {#if $accessMode.loopback}
      <button
        type="button"
        onclick={() => ((addingWorkspace = !addingWorkspace), (sortMenuOpen = false))}
        title={m.workspaces_add()}
        aria-label={m.workspaces_add()}
        aria-expanded={addingWorkspace}
        class="pointer-events-auto flex items-center justify-center w-7 h-7 rounded-full bg-gray-100 dark:bg-white/10 border border-gray-200 dark:border-white/15 backdrop-blur-sm text-gray-800 dark:text-gray-200 hover:border-lerd-red hover:text-lerd-red transition-colors"
      >
        <Icon name="plus" class="w-3.5 h-3.5" />
      </button>
    {/if}

    <button
      type="button"
      onclick={() => ((sortMenuOpen = !sortMenuOpen), (addingWorkspace = false))}
      title={m.sites_sort_label()}
      aria-haspopup="menu"
      aria-expanded={sortMenuOpen}
      aria-label={m.sites_sort_label()}
      class="pointer-events-auto flex items-center justify-center w-7 h-7 rounded-full bg-gray-100 dark:bg-white/10 border border-gray-200 dark:border-white/15 backdrop-blur-sm text-gray-800 dark:text-gray-200 hover:border-lerd-red hover:text-lerd-red transition-colors"
    >
      <Icon name="sort" class="w-3.5 h-3.5" />
    </button>
  </div>
{/snippet}

{#snippet siteRow(s: Site, grouped = false)}
  <button
    onclick={() => select(s)}
    class="group relative w-full flex items-center gap-2 px-3 py-2.5 text-left transition-colors border-b border-gray-50 dark:border-lerd-border/50 {selected ===
    s.domain
      ? 'bg-lerd-red/10 text-lerd-red'
      : 'text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/3'}"
  >
    {#if canReorder && !grouped}
      <span
        role="button"
        tabindex="-1"
        aria-label={m.sites_sort_reorder()}
        use:dragHandle
        onclick={(e) => e.stopPropagation()}
        onkeydown={(e) => e.stopPropagation()}
        class="absolute left-2 top-1/2 -translate-y-1/2 z-10 flex items-center justify-center w-6 h-6 rounded-md border border-gray-200 dark:border-white/15 bg-gray-100 dark:bg-lerd-card text-gray-800 dark:text-gray-200 hover:text-lerd-red hover:border-lerd-red cursor-grab active:cursor-grabbing opacity-0 group-hover:opacity-100 transition-opacity"
      >
        <Icon name="grip" class="w-4 h-4" />
      </span>
    {/if}
    {#if grouped}
      <Icon name="group" class="w-3.5 h-3.5 shrink-0 text-gray-400 dark:text-gray-500" />
    {/if}
    <span class="relative shrink-0 w-4 h-4 flex items-center justify-center">
      <SiteIcon site={s} />
    </span>
    <span class="flex-1 text-sm truncate">{s.domain}</span>
    {#if s.tls}
      <svg class="w-3 h-3 shrink-0 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/>
      </svg>
    {/if}
    <SiteIndicators site={s} />
  </button>
{/snippet}

{#snippet sectionRows(key: string)}
  <!-- An empty section keeps a little height so it stays a drop target.
       Rows swallow mousedown here: the workspace zone wrapping this section is
       always live, and a press that reached it would drag the whole workspace
       instead of the row. The grip's own listener has already run by then, so
       row dragging is unaffected. -->
  <section
    class="{hasWorkspaces && (zones[key]?.length ?? 0) === 0 ? 'min-h-[1.75rem]' : ''} {key === UNGROUPED &&
    hasWorkspaces
      ? 'border-t border-gray-100 dark:border-lerd-border'
      : ''}"
    use:stopDragBubbling
    use:dndzone={{ items: zones[key] ?? [], type: dndType, flipDurationMs: FLIP_MS, dragDisabled, dropTargetStyle: {} }}
    onconsider={(e) => rowConsider(key, e)}
    onfinalize={(e) => rowFinalize(key, e)}
  >
    {#each zones[key] ?? [] as item (item.id)}
      <div animate:flip={{ duration: FLIP_MS }}>
        {@render siteRow(item.site, false)}
        {#each secondariesFor(item.site) as sec (sec.domain)}
          {@render siteRow(sec, true)}
        {/each}
      </div>
    {/each}
  </section>
{/snippet}

{#snippet workspaceMenu(key: string)}
  <div class="relative">
    <button
      type="button"
      onmousedown={(e) => e.stopPropagation()}
      onclick={(e) => (e.stopPropagation(), (menuKey = menuKey === key ? null : key))}
      aria-haspopup="menu"
      aria-expanded={menuKey === key}
      aria-label={m.workspaces_sectionMenu()}
      title={m.workspaces_sectionMenu()}
      class="flex items-center justify-center w-5 h-5 rounded text-gray-400 hover:text-lerd-red transition-colors"
    >
      <Icon name="more" class="w-3.5 h-3.5" />
    </button>
    {#if menuKey === key}
      <div
        role="menu"
        tabindex="-1"
        onmousedown={(e) => e.stopPropagation()}
        class="absolute right-0 top-full mt-1 z-30 min-w-40 rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl py-1"
      >
        <button
          type="button"
          role="menuitem"
          onclick={() => startRename(key)}
          class="w-full px-3 py-1.5 text-left text-xs text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-white/5"
          >{m.workspaces_rename()}</button
        >
        <button
          type="button"
          role="menuitem"
          onclick={() => removeWorkspace(key)}
          class="w-full px-3 py-1.5 text-left text-xs text-gray-700 dark:text-gray-200 hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-600 dark:hover:text-red-400"
          >{m.workspaces_delete()}</button
        >
      </div>
    {/if}
  </div>
{/snippet}

{#snippet workspaceSection(key: string)}
  {#snippet menu()}{@render workspaceMenu(key)}{/snippet}
  {#if renamingKey === key}
    <div class="px-2 py-1.5 border-t border-gray-100 dark:border-lerd-border">
      <!-- svelte-ignore a11y_autofocus -->
      <input
        autofocus
        bind:value={renameValue}
        onblur={submitRename}
        onkeydown={(e) => (e.key === 'Enter' ? submitRename() : e.key === 'Escape' ? (renamingKey = null) : null)}
        class="w-full px-2 py-1 text-xs rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-bg text-gray-800 dark:text-gray-200 focus:outline-none focus:border-lerd-red"
      />
    </div>
  {:else}
    <SitesSectionHeader
      label={key}
      count={zones[key]?.length ?? 0}
      collapsed={isCollapsed(key)}
      ontoggle={() => toggleWorkspaceCollapse(key)}
      draggable={canReorder && wsOrder.length > 1}
      trailing={canReorder ? menu : undefined}
    />
  {/if}
  {#if !isCollapsed(key)}
    {@render sectionRows(key)}
  {/if}
{/snippet}

<ListPanel title={m.sites_title()} {actions} overlay={$sitesLoaded && $sites.length > 0 ? overlayControls : undefined}>
  {#if !$sitesLoaded}
    <LoadingRow />
  {:else if $sites.length === 0}
    <EmptyState title={m.sites_empty()} hint={parkHint} size="sm" />
  {:else}
    <div
      use:dndzone={{
        items: wsItems,
        type: wsDndType,
        flipDurationMs: FLIP_MS,
        dragDisabled: !canReorder || wsOrder.length < 2,
        dropTargetStyle: {}
      }}
      onconsider={headerConsider}
      onfinalize={headerFinalize}
    >
      {#each wsItems as ws (ws.id)}
        <div animate:flip={{ duration: FLIP_MS }}>
          {@render workspaceSection(ws.id)}
        </div>
      {/each}
    </div>

    <!-- Sites in no workspace trail the sections, unlabelled. With no
         workspaces at all this is the whole list, exactly as it looked before. -->
    {@render sectionRows(UNGROUPED)}

    {#each orphanSecondaries as s (s.domain)}
      {@render siteRow(s, true)}
    {/each}

    {#if paused.length > 0}
      <SitesSectionHeader
        label={m.sites_paused()}
        count={paused.length}
        collapsed={isCollapsed(PAUSED)}
        ontoggle={() => toggleWorkspaceCollapse(PAUSED)}
      />
      {#if !isCollapsed(PAUSED)}
        {#each paused as s (s.domain)}
          <button
            onclick={() => select(s)}
            class="w-full flex items-center gap-2 px-3 py-2 text-left transition-colors border-t border-gray-50 dark:border-lerd-border/50 {selected === s.domain
              ? 'bg-lerd-red/10 text-lerd-red'
              : 'text-gray-400 dark:text-gray-500 hover:bg-gray-50 dark:hover:bg-white/3'}"
          >
            <svg class="w-3 h-3 shrink-0 opacity-60" fill="currentColor" viewBox="0 0 24 24">
              <path d="M6 5h4v14H6zM14 5h4v14h-4z"/>
            </svg>
            <span class="flex-1 text-sm truncate">{s.domain}</span>
          </button>
        {/each}
      {/if}
    {/if}
  {/if}
</ListPanel>
