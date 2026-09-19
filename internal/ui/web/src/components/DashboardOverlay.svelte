<script lang="ts">
  import { dashboardOpen, closeDashboard } from '$stores/dashboard';
  import ServiceIcon from './ServiceIcon.svelte';
  import {
    profilerEnabled,
    loadProfilerStatus,
    setProfiler,
    clearProfilerData
  } from '$stores/profiler';
  import Icon from './Icon.svelte';
  import StatusDot from './StatusDot.svelte';
  import DocsViewer from './DocsViewer.svelte';
  import { docsLocation, docsSiteURL } from '$stores/docs';
  import {
    isSpxReportView,
    setSpxConfigHidden,
    padSpxControlPanel,
    themeSpxDocument,
    fetchSpxReportCount
  } from '$lib/spxControls';
  import { m } from '../paraglide/messages.js';
  import { syncEmbeddedTheme } from '$lib/embeddedTheme';
  import { joinDashboardPath } from '$lib/dashboardPath';
  import { rememberMailpitTheme, themeMailpitDocument } from '$lib/mailpitTheme';
  import {
    themeMeilisearchDocument,
    repaintMeilisearch,
    watchMeilisearchRules
  } from '$lib/meilisearchTheme';
  import { rememberRustfsTheme, themeRustfsDocument } from '$lib/rustfsTheme';
  import { repaintLightOnly, themeLightOnlyDocument } from '$lib/lightOnlyTheme';
  import {
    hasFrameDesign,
    repaintFrameDesign,
    watchFrameDesign,
    themeFrameDocument
  } from '$lib/frameThemes';

  const LIGHT_ONLY = ['phpmyadmin', 'mongo-express', 'rabbitmq'];

  let busy = $state(false);
  let clearing = $state(false);
  let iframeEl = $state<HTMLIFrameElement | null>(null);
  let entryHref = $state('');
  let canGoBack = $state(false);
  let isReportView = $state(false);
  // The SPX Configuration form is collapsed by default so the report list
  // gets the whole view; the header button restores it on demand.
  let configHidden = $state(true);

  const isProfiler = $derived($dashboardOpen?.name === 'profiler');
  // The documentation is served by the daemon out of the embedded pages, so it
  // renders in place of the iframe and only its lerd.sh twin opens in a tab.
  const isDocs = $derived($dashboardOpen?.name === 'docs');
  const docsHref = $derived(docsSiteURL($docsLocation?.route ?? ''));
  // Where the header's links point: the embedded frame's URL, or for the
  // documentation the same page on lerd.sh.
  const externalHref = $derived(
    isDocs
      ? docsHref
      : $dashboardOpen
        ? joinDashboardPath($dashboardOpen.dashboard, $dashboardOpen.extraPath)
        : ''
  );

  const headerBtnClass =
    'text-xs rounded-sm border border-gray-200 dark:border-lerd-border px-2 py-1 ' +
    'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 transition-colors';

  // Reset iframe-history tracking whenever a different dashboard opens.
  $effect(() => {
    $dashboardOpen;
    entryHref = '';
    canGoBack = false;
    isReportView = false;
    configHidden = true;
  });

  $effect(() => {
    if (isProfiler) void loadProfilerStatus();
  });

  // SPX never re-fetches its report list while the control panel page is open,
  // so new captures stay hidden. Poll and reload the iframe only once the
  // report count has actually grown.
  $effect(() => {
    if (!isProfiler || isReportView || !$profilerEnabled) return;
    let lastReportCount = -1;
    const id = setInterval(async () => {
      if (document.hidden) return;
      const count = await fetchSpxReportCount();
      if (count === null) return;
      if (lastReportCount >= 0 && count > lastReportCount) reloadIframe();
      lastReportCount = count;
    }, 4000);
    return () => clearInterval(id);
  });

  // iframeWindow returns the embedded window only when it is same-origin and
  // therefore drivable; cross-origin service dashboards return null.
  function iframeWindow(): Window | null {
    try {
      const w = iframeEl?.contentWindow ?? null;
      void w?.location.href; // throws for a cross-origin frame
      return w;
    } catch {
      return null;
    }
  }

  // The embedded page follows lerd rather than the browser, re-applied on every
  // navigation inside the frame because each one loads a fresh document.
  function applyTheme() {
    const w = iframeWindow();
    const dark = document.documentElement.classList.contains('dark');
    syncEmbeddedTheme(w?.document ?? null, dark);
    // SPX gates its light mode inside its stylesheet and paints from variables of
    // its own, so the switch alone leaves it wearing upstream's colours.
    if (isProfiler && w) themeSpxDocument(w.document, dark);
    if ($dashboardOpen?.name === 'mailpit') {
      rememberMailpitTheme(dark);
      // Mailpit is Bootstrap's own greys until its variables are told otherwise.
      if (w?.document) themeMailpitDocument(w.document, dark);
    }
    if ($dashboardOpen?.name === 'rustfs' && w?.document) {
      rememberRustfsTheme(dark);
      themeRustfsDocument(w.document, dark);
    }
    // None of these ships a dark design to switch to: phpMyAdmin carries none in
    // any of its four themes, Mongo Express sits on a Bootstrap 3 that predates
    // the idea, and RabbitMQ's management UI has one stylesheet and no second
    // half to it. So their own stylesheets are what gets turned around.
    if (LIGHT_ONLY.includes($dashboardOpen?.name ?? '') && w?.document) {
      repaintLightOnly(w.document, dark);
      themeLightOnlyDocument(w.document, dark);
    }
    // These take a dark design of their own as soon as the proxy answers their
    // question about the scheme. Dark is not themed though: left there they wear
    // their own greys inside an overlay wearing lerd's, so the design is laid
    // over the palette.
    const design = $dashboardOpen?.name ?? '';
    if (hasFrameDesign(design) && w?.document) {
      themeFrameDocument(w.document, dark);
      repaintFrameDesign(w.document, design, dark);
      watchFrameDesign(w, design, () => document.documentElement.classList.contains('dark'));
    }
    if ($dashboardOpen?.name === 'meilisearch' && w?.document) {
      themeMeilisearchDocument(w.document, dark);
      // The rules already there are swept; the ones its components add as they
      // mount are caught as they arrive.
      repaintMeilisearch(w.document, dark);
      watchMeilisearchRules(w, () => document.documentElement.classList.contains('dark'));
    }
  }

  // The switcher toggles a class for the mode and rewrites the palette as inline
  // custom properties, so both attributes are watched: an open dashboard then
  // keeps step with a theme change as well as a light/dark one, without the
  // overlay knowing how either is stored.
  $effect(() => {
    // Both of these read their own preference as they boot, which is after the
    // frame's load event, so the mode is written on open rather than once the
    // frame is there.
    const dark = document.documentElement.classList.contains('dark');
    if ($dashboardOpen?.name === 'mailpit') rememberMailpitTheme(dark);
    if ($dashboardOpen?.name === 'rustfs') rememberRustfsTheme(dark);
    const obs = new MutationObserver(() => applyTheme());
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style'] });
    return () => obs.disconnect();
  });

  function onIframeLoad() {
    const w = iframeWindow();
    applyTheme();
    const href = w?.location.href ?? '';
    if (href === '' || !w) {
      canGoBack = false;
      isReportView = false;
      return;
    }
    if (entryHref === '') entryHref = href;
    canGoBack = href !== entryHref;
    isReportView = isSpxReportView(href);
    if (!isReportView && isProfiler) {
      padSpxControlPanel(w.document);
      setSpxConfigHidden(w.document, configHidden);
    }
  }

  function toggleConfig() {
    configHidden = !configHidden;
    const w = iframeWindow();
    if (w) setSpxConfigHidden(w.document, configHidden);
  }

  function goBack() {
    iframeWindow()?.history.back();
  }

  function reloadIframe() {
    const w = iframeWindow();
    if (w) {
      w.location.reload();
    } else if (iframeEl) {
      iframeEl.src = iframeEl.src; // cross-origin: reload to its entry URL
    }
  }

  async function toggleProfiler() {
    if (busy) return;
    busy = true;
    try {
      await setProfiler(!$profilerEnabled);
    } finally {
      busy = false;
    }
  }

  // clearData wipes every captured SPX report, then reloads the embedded UI
  // so its report list reflects the now-empty data directory.
  async function clearProfilerReports() {
    if (clearing) return;
    clearing = true;
    try {
      await clearProfilerData();
      reloadIframe();
    } finally {
      clearing = false;
    }
  }
</script>

{#if $dashboardOpen}
  {@const d = $dashboardOpen}
  {@const iframeSrc = joinDashboardPath(d.dashboard, d.extraPath)}
  <div class="fixed top-0 right-0 left-0 bottom-16 md:left-14 md:bottom-0 z-30 flex flex-col bg-white dark:bg-lerd-bg">
    <div class="flex items-center justify-between px-3 py-3 border-b border-gray-200 dark:border-lerd-border shrink-0">
      <div class="flex items-center gap-3 min-w-0">
        {#if isProfiler}
          <button
            onclick={goBack}
            disabled={!canGoBack}
            title={m.common_back()}
            aria-label={m.common_back()}
            class="text-gray-400 enabled:hover:text-gray-700 dark:enabled:hover:text-gray-200 disabled:opacity-30 disabled:cursor-not-allowed transition-colors shrink-0"
          >
            <Icon name="back" />
          </button>
        {/if}
        <ServiceIcon name={d.name} icon={d.icon} bare compact />
        <span class="text-sm font-medium text-gray-900 dark:text-white truncate">{d.label || d.name}</span>
        <a
          href={externalHref}
          target="_blank"
          rel="noopener"
          class="font-mono text-[10px] text-sky-600 dark:text-sky-400 hover:underline truncate"
        >{externalHref}</a>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        {#if isProfiler}
          {#if !isReportView}
            <button
              onclick={toggleConfig}
              title={configHidden ? m.profiler_config_show() : m.profiler_config_hide()}
              class={headerBtnClass}
            >
              {configHidden ? m.profiler_config_show() : m.profiler_config_hide()}
            </button>
          {/if}
          <button
            onclick={clearProfilerReports}
            disabled={clearing}
            title={m.profiler_clear_title()}
            class="{headerBtnClass} disabled:opacity-50"
          >
            {clearing ? m.profiler_clear_busy() : m.profiler_clear()}
          </button>
          <button
            onclick={toggleProfiler}
            disabled={busy}
            aria-pressed={$profilerEnabled}
            class="flex items-center gap-1.5 text-xs rounded-sm border px-2 py-1 transition-colors disabled:opacity-50 {$profilerEnabled
              ? 'border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/20 text-emerald-700 dark:text-emerald-300 hover:border-emerald-500'
              : 'border-gray-200 dark:border-lerd-border text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'}"
          >
            {#if $profilerEnabled}
              <StatusDot color="emerald" size="xs" pulse />
            {/if}
            {busy ? m.profiler_busy() : $profilerEnabled ? m.profiler_disarm() : m.profiler_arm()}
          </button>
        {/if}
        {#if !isDocs}
          <button
            onclick={reloadIframe}
            title={m.common_refresh()}
            aria-label={m.common_refresh()}
            class="text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 transition-colors"
          >
            <Icon name="refresh" />
          </button>
        {/if}
        <a
          href={externalHref}
          target="_blank"
          rel="noopener"
          title={m.common_openInNewTab()}
          aria-label={m.common_openInNewTab()}
          class="text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 transition-colors"
        ><Icon name="external" /></a>
        <button
          onclick={closeDashboard}
          title={m.common_close()}
          aria-label={m.common_closeDashboard()}
          class="text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 transition-colors"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
          </svg>
        </button>
      </div>
    </div>
    {#if isDocs}
      <DocsViewer />
    {:else}
      <!-- Keyed on the source so switching dashboards tears the frame down and
           builds a new one. Re-pointing the old frame is a navigation, which
           runs the embedded app's beforeunload handler; an admin UI that
           registers one (pgAdmin) then blocks the swap behind a native confirm
           the overlay has no way to answer. -->
      {#key iframeSrc}
        <iframe
          bind:this={iframeEl}
          onload={onIframeLoad}
          src={iframeSrc}
          class="flex-1 w-full bg-white border-0"
          title={d.label || d.name}
        ></iframe>
      {/key}
    {/if}
  </div>
{/if}
