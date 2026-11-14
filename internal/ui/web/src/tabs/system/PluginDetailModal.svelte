<script lang="ts">
  import type { Snippet } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Icon from '$components/Icon.svelte';
  import PluginIcon from '$components/PluginIcon.svelte';
  import PluginRequirements from '$components/PluginRequirements.svelte';
  import DetailTabs from '$components/DetailTabs.svelte';
  import Toggle from '$components/Toggle.svelte';
  import DOMPurify from 'dompurify';
  import { apiUrl } from '$lib/api';
  import { loadPluginDocs, loadPluginLogs, authorProfile, type PluginFailure, type PluginFeature, type PluginSchedule } from '$stores/plugins';
  import { m } from '../../paraglide/messages.js';

  // One plugin in full, from an installed or a catalog card; an installed one
  // also gets its docs and failure log. The caller brings the actions.
  interface Props {
    name: string;
    description?: string;
    version?: string;
    author?: string;
    icon?: string;
    images?: string[];
    repo?: string;
    commit?: string;
    adds?: string[];
    requires?: string[];
    missing?: string[];
    platforms?: string[];
    lerd?: string;
    tags?: string[];
    error?: string;
    // Set for an installed plugin: what it adds by name, and whether it has docs.
    installed?: boolean;
    // A newer approved commit, its version and GitHub's comparison with this one.
    update?: string;
    updateVersion?: string;
    changes?: string;
    features?: PluginFeature[];
    hasDocs?: boolean;
    // Set for an installed plugin that sends notifications, with the switch.
    notifications?: 'none' | 'pending' | 'allowed' | 'denied';
    onnotifications?: (allowed: boolean) => void;
    // Set for an installed plugin with schedules: the switch and each schedule.
    schedule?: 'none' | 'pending' | 'allowed' | 'denied';
    schedules?: PluginSchedule[];
    onschedule?: (allowed: boolean) => void;
    onclose: () => void;
    actions?: Snippet;
  }
  let { name, description, version, author, icon, images = [], repo, commit, adds = [], requires = [], missing = [], platforms = [], lerd, tags = [], error, installed = false, update, updateVersion, changes, features = [], hasDocs = false, notifications = 'none', onnotifications, schedule = 'none', schedules = [], onschedule, onclose, actions }: Props = $props();

  type Tab = 'overview' | 'features' | 'docs' | 'logs';
  let tab = $state<Tab>('overview');
  const tabs = $derived([
    { id: 'overview' as Tab, label: m.sites_tabs_overview() },
    { id: 'features' as Tab, label: m.system_plugins_tabFeatures(), hidden: !features.length && !adds.length },
    { id: 'docs' as Tab, label: m.system_plugins_docs(), hidden: !hasDocs },
    { id: 'logs' as Tab, label: m.services_tabs_logs(), hidden: !installed }
  ]);
  // Docs and logs load when their tab is first opened.
  let docs = $state<Promise<string> | null>(null);
  let logs = $state<Promise<PluginFailure[]> | null>(null);
  function selectTab(id: Tab) {
    tab = id;
    if (id === 'docs') docs ??= loadPluginDocs(name).then(docsHTML);
    if (id === 'logs') logs ??= loadPluginLogs(name);
  }
  // Plugin files are served by lerd-ui itself, which is not the dashboard's
  // origin when it is opened at lerd.localhost.
  function docsHTML(html: string): string {
    const t = document.createElement('template');
    t.innerHTML = DOMPurify.sanitize(html, { ADD_ATTR: ['target'] });
    for (const img of t.content.querySelectorAll<HTMLImageElement>('img[src^="/plugins/"]')) img.src = apiUrl(img.getAttribute('src')!);
    return t.innerHTML;
  }
  const FEATURE_LABELS: Record<string, () => string> = {
    command: m.system_plugins_feature_command,
    site_command: m.system_plugins_feature_site_command,
    mcp: m.system_plugins_feature_mcp,
    hook: m.system_plugins_feature_hook,
    page: m.system_plugins_feature_page,
    site_tab: m.system_plugins_feature_site_tab,
    overview_card: m.system_plugins_feature_overview_card,
    page_script: m.system_plugins_feature_page_script,
    provides: m.system_plugins_feature_provides,
    service: m.system_plugins_feature_service,
    framework: m.system_plugins_feature_framework,
    notifications: m.system_plugins_feature_notifications
  };

  const PLATFORM_NAMES: Record<string, string> = { linux: 'Linux', macos: 'macOS' };
  let shown = $state(0);
  let hovered = $state(false);
  const step = (by: number) => (shown = (shown + by + images.length) % images.length);
  // Arrow keys step the slider, unless a field has the focus.
  function onKey(e: KeyboardEvent) {
    if (images.length < 2 || (e.target instanceof Element && e.target.closest('input, textarea'))) return;
    if (e.key === 'ArrowLeft') step(-1);
    if (e.key === 'ArrowRight') step(1);
  }
  // Like the lineup on lerd.sh, the active thumbnail's bar fills over the slide
  // time and its end moves the slide on, so they never drift. Hover pauses it;
  // reduced motion has no fill and so no change.
  const SLIDE_MS = 5000;
  const repoLabel = $derived(repo?.replace(/^https:\/\/(www\.)?github\.com\//, '') ?? '');
</script>

{#snippet label(text: string)}
  <dt class="text-[10px] font-semibold uppercase tracking-wider text-gray-400 pt-0.5">{text}</dt>
{/snippet}

<svelte:window onkeydown={onKey} />

{#snippet heading()}
  <div class="flex items-center gap-3.5">
    <span class="shrink-0 flex items-center justify-center w-11 h-11 rounded-xl bg-gray-100 dark:bg-white/5 text-gray-700 dark:text-gray-200 ring-1 ring-gray-200/70 dark:ring-white/5">
      <PluginIcon {icon} />
    </span>
    <div class="min-w-0">
      <div class="flex items-baseline gap-2 min-w-0">
        <span class="truncate font-semibold text-gray-900 dark:text-white">{name}</span>
        {#if version}<span class="shrink-0 text-[11px] font-mono tabular-nums text-gray-400">v{version}</span>{/if}
        {#if author}
          {@const profile = authorProfile(repo)}
          {@const [before, after] = m.system_plugins_by({ author: '\u0000' }).split('\u0000')}
          <span class="shrink-0 truncate text-[11px] text-gray-400">
            {before}{#if profile}<a href={profile} target="_blank" rel="noopener noreferrer" class="hover:text-lerd-red underline-offset-2 hover:underline">{author}</a>{:else}{author}{/if}{after}
          </span>
        {/if}
      </div>
      {#if description}<p class="mt-0.5 text-xs leading-snug text-gray-500 dark:text-gray-400">{description}</p>{/if}
    </div>
  </div>
{/snippet}

<Modal open title={name} {heading} {onclose} size={images.length || hasDocs ? 'xl' : 'lg'} footer={actions}>
  <DetailTabs {tabs} active={tab} onchange={selectTab} />
  <div class="p-5 space-y-5">
    {#if tab === 'overview'}
    {#if update}
      <div class="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-emerald-200 dark:border-emerald-500/20 bg-emerald-50 dark:bg-emerald-500/5 px-3 py-2 text-xs">
        <span class="font-medium text-emerald-700 dark:text-emerald-300">{m.system_plugins_updateAvailable()}</span>
        <span class="font-mono tabular-nums text-gray-600 dark:text-gray-300">{version ?? ''} → {updateVersion || update.slice(0, 7)}</span>
        {#if changes}<a href={changes} target="_blank" rel="noopener noreferrer" class="ml-auto font-medium text-lerd-red hover:underline">{m.system_plugins_whatChanged()}</a>{/if}
      </div>
    {/if}
    {#if images.length}
      <figure class="space-y-2" onmouseenter={() => (hovered = true)} onmouseleave={() => (hovered = false)}>
        <!-- A fixed frame, so stepping between screenshots of other shapes never resizes the dialog. -->
        <div class="group/slider relative aspect-[16/10] w-full max-w-[88vh] mx-auto rounded-lg border border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-lerd-bg overflow-hidden">
          <img src={images[shown]} alt="" class="w-full h-full object-contain" />
          {#if images.length > 1}
            <button type="button" onclick={() => step(-1)} aria-label={m.common_pagePrev()}
              class="absolute left-2 top-1/2 -translate-y-1/2 w-9 h-9 flex items-center justify-center rounded-full bg-black/50 text-white backdrop-blur-sm opacity-70 group-hover/slider:opacity-100 hover:bg-black/70 transition">
              <Icon name="chevron" class="w-4 h-4 rotate-90" />
            </button>
            <button type="button" onclick={() => step(1)} aria-label={m.common_pageNext()}
              class="absolute right-2 top-1/2 -translate-y-1/2 w-9 h-9 flex items-center justify-center rounded-full bg-black/50 text-white backdrop-blur-sm opacity-70 group-hover/slider:opacity-100 hover:bg-black/70 transition">
              <Icon name="chevron" class="w-4 h-4 -rotate-90" />
            </button>
            <span class="absolute bottom-2 right-2 px-2 py-0.5 rounded-full bg-black/50 text-white text-[11px] tabular-nums backdrop-blur-sm">{shown + 1} / {images.length}</span>
          {/if}
        </div>
        {#if images.length > 1}
          <div class="flex gap-2 overflow-x-auto">
            {#each images as src, i (src)}
              <button type="button" onclick={() => (shown = i)} aria-label="{i + 1} / {images.length}" aria-pressed={shown === i}
                class="relative shrink-0 w-28 aspect-video rounded-md overflow-hidden border-2 {shown === i ? 'border-lerd-red' : 'border-transparent opacity-70 hover:opacity-100'}">
                <img {src} alt="" loading="lazy" class="w-full h-full object-cover object-left-top" />
                {#if shown === i}
                  {#key shown}
                    <span data-slide-bar class="slide-bar absolute left-0 bottom-0 h-0.5 w-full bg-lerd-red" class:paused={hovered} style:--slide-time="{SLIDE_MS}ms" onanimationend={() => step(1)}></span>
                  {/key}
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      </figure>
    {/if}

    {#if error}
      <p class="text-xs text-red-600 dark:text-red-400 break-words">{m.system_plugins_broken()}: {error}</p>
    {/if}

    <dl class="grid grid-cols-[7rem_1fr] gap-x-4 gap-y-2.5 text-xs">
      {@render label(m.system_plugins_source())}
      <dd class="min-w-0 text-gray-600 dark:text-gray-300">
        {#if repo}
          <a href={repo} target="_blank" rel="noopener noreferrer" class="hover:text-lerd-red break-all">{repoLabel}</a>
          {#if commit}<span class="ml-1.5 font-mono text-gray-400">{commit.slice(0, 7)}</span>{/if}
        {:else}
          {m.system_plugins_notListed()}
        {/if}
      </dd>
      {#if requires.length}
        {@render label(m.system_plugins_requires())}
        <dd><PluginRequirements {requires} {missing} bare /></dd>
      {/if}
      {#if platforms.length}
        {@render label(m.system_plugins_runsOn())}
        <dd class="text-gray-600 dark:text-gray-300">{platforms.map((p) => PLATFORM_NAMES[p] ?? p).join(', ')}</dd>
      {/if}
      {#if lerd}
        {@render label('Lerd')}
        <dd class="font-mono text-gray-600 dark:text-gray-300">{lerd}</dd>
      {/if}
      {#if installed && notifications !== 'none' && onnotifications}
        {@render label(m.system_plugins_notifications())}
        <dd class="flex items-center">
          <Toggle on={notifications === 'allowed'} title={m.system_plugins_notifications()} onclick={() => onnotifications?.(notifications !== 'allowed')} />
        </dd>
      {/if}
      {#if installed && schedule !== 'none' && onschedule}
        {@render label(m.system_plugins_schedule())}
        <dd class="space-y-1.5">
          <Toggle on={schedule === 'allowed'} title={m.system_plugins_schedule()} onclick={() => onschedule?.(schedule !== 'allowed')} />
          {#each schedules as s (s.name)}
            <div class="flex flex-wrap items-baseline gap-x-2 text-gray-600 dark:text-gray-300">
              <span class="font-mono">{s.name}</span>
              <span class="text-gray-400">{m.system_plugins_scheduleEvery({ every: s.every })}</span>
              {#if s.last_run}<span class="text-gray-400 tabular-nums" title={new Date(s.last_run).toLocaleString()}>{m.system_plugins_scheduleLast({ time: new Date(s.last_run).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) })}</span>{/if}
              {#if s.running}
                <span class="text-gray-500">{m.system_plugins_scheduleRunning()}</span>
              {:else if s.last_error}
                <span class="text-red-500" title={s.last_error}>{m.system_plugins_scheduleFailed({ error: s.last_error.split('\n')[0] })}</span>
              {:else if !s.last_run}
                <span class="text-gray-400">{m.system_plugins_scheduleNever()}</span>
              {/if}
            </div>
          {/each}
        </dd>
      {/if}
      {#if tags.length}
        {@render label(m.system_plugins_tags())}
        <dd class="flex flex-wrap gap-1">
          {#each tags as t (t)}<span class="text-[10px] px-1.5 py-0.5 rounded-sm border border-gray-200 dark:border-lerd-border text-gray-500 dark:text-gray-400">{t}</span>{/each}
        </dd>
      {/if}
    </dl>
    {:else if tab === 'features'}
      {#if features.length}
        <ul class="space-y-3 text-xs">
          {#each features as f, i (i)}
            <li class="min-w-0">
              <span class="text-[10px] uppercase tracking-wider text-gray-400">{FEATURE_LABELS[f.kind]?.() ?? f.kind}</span>
              {#if f.name}<span class="ml-1.5 font-mono text-gray-700 dark:text-gray-200">{f.name}</span>{/if}
              {#if f.detail}<p class="text-gray-500 dark:text-gray-400 break-words">{f.detail}</p>{/if}
            </li>
          {/each}
        </ul>
      {:else}
        <div class="flex flex-wrap gap-1">
          {#each adds as a (a)}<span class="text-[10px] font-mono px-1.5 py-0.5 rounded-sm bg-gray-100 dark:bg-white/5 text-gray-500 dark:text-gray-400">{a}</span>{/each}
        </div>
      {/if}
    {:else if tab === 'docs' && docs}
      {#await docs}
        <p class="text-xs text-gray-400">…</p>
      {:then html}
        <article class="docs-prose">{@html html}</article>
      {:catch err}
        <p class="text-xs text-red-600 dark:text-red-400">{err.message}</p>
      {/await}
    {:else if tab === 'logs' && logs}
      {#await logs}
        <p class="text-xs text-gray-400">…</p>
      {:then list}
        {#if list.length === 0}
          <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_plugins_noFailures()}</p>
        {:else}
          <ol class="space-y-3">
            {#each list as f, i (i)}
              <li class="rounded-lg border border-gray-200 dark:border-lerd-border">
                <div class="flex items-center gap-3 px-3 py-1.5 text-xs border-b border-gray-100 dark:border-lerd-border">
                  <span class="text-gray-400 tabular-nums">{new Date(f.at).toLocaleString()}</span>
                  <span class="font-mono text-gray-700 dark:text-gray-200 min-w-0 truncate">{f.surface} · {f.what}</span>
                  {#if f.report_url}
                    <a href={f.report_url} target="_blank" rel="noopener noreferrer" class="ml-auto shrink-0 font-medium text-lerd-red hover:underline">{m.system_plugins_report()}</a>
                  {/if}
                </div>
                <pre class="px-3 py-2 text-[11px] leading-relaxed whitespace-pre-wrap break-words text-red-700 dark:text-red-300 max-h-60 overflow-auto">{f.error}</pre>
              </li>
            {/each}
          </ol>
        {/if}
      {:catch err}
        <p class="text-xs text-red-600 dark:text-red-400">{err.message}</p>
      {/await}
    {/if}
  </div>
</Modal>

<style>
  .slide-bar {
    transform-origin: left;
    animation: plugin-slide var(--slide-time) linear forwards;
  }
  .slide-bar.paused {
    animation-play-state: paused;
  }
  @keyframes plugin-slide {
    from { transform: scaleX(0); }
    to { transform: scaleX(1); }
  }
  @media (prefers-reduced-motion: reduce) {
    .slide-bar { animation: none; transform: none; }
  }
</style>
