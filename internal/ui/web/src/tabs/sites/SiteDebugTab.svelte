<script lang="ts">
  import DetailTabs from '$components/DetailTabs.svelte';
  import { onMount } from 'svelte';
  import DumpsTab from '$tabs/DumpsTab.svelte';
  import QueriesLens from '$components/QueriesLens.svelte';
  import KindLens from '$components/KindLens.svelte';
  import DebugDisabled from '$components/DebugDisabled.svelte';
  import BrowserLens from '$components/BrowserLens.svelte';
  import RequestTimeline from '$components/RequestTimeline.svelte';
  import ProfilePrompt from '$components/ProfilePrompt.svelte';
  import { profileKeyFor } from '$stores/profiler';
  import { writable } from 'svelte/store';
  import { apiJson } from '$lib/api';
  import type { DumpEvent } from '$lib/dumpsStream';
  import { buildWaterfall, type ServedRequest } from '$lib/requestWaterfall';
  import { routeQuery } from '$lib/route';
  import { debugLens, debugLensTabs, debugSearch, type DebugLens } from '$stores/debugLens';
  import { refreshStatus, startDumpsStream, stopDumpsStream } from '$stores/dumps';
  import { refreshDevtoolsStatus, debugCaptureEnabled } from '$stores/queries';
  import { countKinds, debugEvents, providePickRequest, scopeLensEvents } from '$stores/debugEvents';
  import Icon from '$components/Icon.svelte';
  import { modal } from '$stores/modals';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../../paraglide/messages.js';

  // The lenses hold the event stream open themselves; the timeline reads it
  // too, so the tab keeps it open while it is shown.
  onMount(() => {
    void refreshStatus();
    void refreshDevtoolsStatus();
    startDumpsStream();
    return stopDumpsStream;
  });

  interface Props {
    siteName?: string;
    framework?: string;
    domain?: string;
    branch?: string;
    // phpLenses is false for a site without PHP, which only has browser events.
    phpLenses?: boolean;
    // rid pins the lenses to one request, as a recent request's inspector
    // does; served is that request as nginx timed it, for the timeline.
    rid?: string;
    served?: ServedRequest;
    // origin is the site's scheme and host for profiling a request's route,
    // empty where SPX cannot run.
    origin?: string;
    // onleave runs before the view hands over to the Profiler, so a dialog
    // holding it can close itself.
    onleave?: () => void;
  }
  let { siteName = '', framework = '', domain = '', branch = '', phpLenses = true, rid = '', served, origin = '', onleave }: Props = $props();

  // Without a pinned request, a search that is exactly one of the site's
  // request ids picks that request, and a picked request gains a timeline.
  const picked = $derived.by(() => {
    const q = $debugSearch.trim();
    return !rid && q && $debugEvents.some((ev) => ev.ctx.rid === q && ev.ctx.site === siteName) ? q : '';
  });
  const scope = writable('');
  $effect(() => scope.set(rid || picked));
  // A pinned or picked request may predate what the stream replayed; the
  // server's ring still holds it.
  const fetched = writable<DumpEvent[]>([]);
  $effect(() => {
    const want = rid || picked;
    fetched.set([]);
    if (want) apiJson<DumpEvent[]>(`/api/dumps?${new URLSearchParams({ rid: want })}`).then((evs) => (want === (rid || picked) ? fetched.set(evs) : undefined), () => {});
  });
  // A search shaped like "GET /path" narrows the lenses to that whole route.
  const route = writable('');
  $effect(() => route.set(rid ? '' : routeQuery($debugSearch)));
  const events = scopeLensEvents(scope, fetched, route);
  let timeline = $state(Boolean(rid));
  // A request id clicked in a lens becomes the search and opens its timeline.
  providePickRequest((id) => {
    debugSearch.set(id);
    timeline = true;
  });
  const showTimeline = $derived(timeline && Boolean(rid || picked));
  const waterfall = $derived(showTimeline ? buildWaterfall($events, rid ? served : undefined) : null);
  // The request the timeline is about, as nginx logged it or as PHP reported it.
  // The SPX capture lerd stamped with this request's id, if it was profiled.
  let profileKey = $state('');
  $effect(() => {
    const want = rid || picked;
    profileKey = '';
    if (want) void profileKeyFor(want).then((k) => (want === (rid || picked) ? (profileKey = k) : undefined));
  });
  const requestLine = $derived(served?.label ?? $events.find((ev) => ev.ctx.type === 'fpm' && ev.ctx.request)?.ctx.request ?? '');

  // Cache comes solely from the Laravel adapter, so it only applies to Laravel
  // sites; everything else is framework-agnostic (PDO and the Symfony
  // Mailer/Twig/EventDispatcher/Messenger/HttpClient seams cover every PHP app).
  const isLaravel = $derived(framework.toLowerCase() === 'laravel');
  const laravelOnly: DebugLens[] = ['cache'];
  const counts = $derived(countKinds($events, siteName));

  const tabs = $derived([
    ...(rid || picked ? [{ id: 'timeline', label: m.debug_tab_timeline(), group: 'timeline' }] : []),
    ...debugLensTabs(counts, isLaravel, Boolean(rid))
  ]);
  const active = $derived(showTimeline ? 'timeline' : $debugLens);
  function pick(id: string) {
    timeline = id === 'timeline';
    if (!timeline) debugLens.set(id as DebugLens);
  }

  // A site without PHP keeps the lens bar, with Browser as its only lens.
  const browserOnly = $derived(tabs.filter((t) => t.id === 'browser'));

  // Full screen works like Tinker's: transient, and Escape leaves it only once
  // no modal is stacked on top, since the modal owns Escape first.
  let fullscreen = $state(false);
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && fullscreen && $modal.kind === null && !e.defaultPrevented) {
      e.preventDefault();
      fullscreen = false;
    }
  }

  // If the remembered lens isn't available for this framework, fall back.
  $effect(() => {
    if (!isLaravel && laravelOnly.includes($debugLens)) debugLens.set('queries');
  });
</script>

<svelte:window onkeydown={onKeydown} />

{#snippet fullscreenAction()}
  <!-- Full screen hides the site header, so name the site here. -->
  {#if !rid && fullscreen && domain}<span class="text-xs font-mono text-gray-600 dark:text-gray-300">{domain}</span>{/if}
  {#if !rid}<button
    type="button"
    onclick={() => (fullscreen = !fullscreen)}
    class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
    use:tooltip={fullscreen ? m.debug_exitFullscreenTitle() : m.debug_fullscreenTitle()}
    aria-label={fullscreen ? m.debug_exitFullscreenTitle() : m.debug_fullscreenTitle()}
  >
    <Icon name={fullscreen ? 'minimize' : 'maximize'} class="w-4 h-4" />
  </button>{/if}
{/snippet}

<div class="flex flex-col overflow-hidden {fullscreen ? 'fixed inset-0 z-50 bg-white dark:bg-lerd-bg' : 'h-full'}">
  {#if !$debugCaptureEnabled && !rid}
    <DebugDisabled />
  {:else if !phpLenses}
    <DetailTabs tabs={browserOnly} active="browser" onchange={() => {}} keepSingle actions={rid ? undefined : fullscreenAction} />
    <div class="flex-1 min-h-0 overflow-hidden">
      <BrowserLens siteScope={siteName} pinned={Boolean(rid)} />
    </div>
  {:else}
    <!-- One request has no filter or full screen, so its bar carries only tabs. -->
    <DetailTabs {tabs} {active} onchange={pick} actions={rid ? undefined : fullscreenAction} />
    <div class="flex-1 min-h-0 overflow-hidden">
      {#if waterfall}
        <div class="h-full flex flex-col">
          <div class="px-3 pt-3"><ProfilePrompt {origin} request={requestLine} {profileKey} {onleave} /></div>
          <div class="flex-1 min-h-0"><RequestTimeline {waterfall} /></div>
        </div>
      {:else if $debugLens === 'browser'}
        <BrowserLens siteScope={siteName} pinned={Boolean(rid)} />
      {:else if $debugLens === 'dumps'}
        <DumpsTab siteScope={siteName} pinned={Boolean(rid)} />
      {:else if $debugLens === 'queries'}
        <QueriesLens siteScope={siteName} pinned={Boolean(rid)} />
      {:else}
        <KindLens kind={$debugLens as 'jobs' | 'views' | 'mail' | 'cache' | 'events' | 'http' | 'logs' | 'exceptions' | 'messages'} siteScope={siteName} pinned={Boolean(rid)} />
      {/if}
    </div>
  {/if}
</div>
