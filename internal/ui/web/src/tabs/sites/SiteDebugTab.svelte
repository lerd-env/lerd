<script lang="ts">
  import DetailTabs, { type TabItem } from '$components/DetailTabs.svelte';
  import { onMount } from 'svelte';
  import DumpsTab from '$tabs/DumpsTab.svelte';
  import QueriesLens from '$components/QueriesLens.svelte';
  import KindLens from '$components/KindLens.svelte';
  import DebugDisabled from '$components/DebugDisabled.svelte';
  import BrowserLens from '$components/BrowserLens.svelte';
  import { browserCaptureEnabled, loadBrowserCaptureStatus } from '$stores/browserCapture';
  import { debugLens, type DebugLens } from '$stores/debugLens';
  import { refreshStatus } from '$stores/dumps';
  import { refreshDevtoolsStatus, debugCaptureEnabled } from '$stores/queries';
  import { countKinds, debugEvents } from '$stores/debugEvents';
  import { m } from '../../paraglide/messages.js';

  onMount(() => {
    void refreshStatus();
    void refreshDevtoolsStatus();
    void loadBrowserCaptureStatus();
  });

  interface Props {
    siteName?: string;
    framework?: string;
    domain?: string;
    branch?: string;
    // phpLenses is false for a site without PHP, which only has browser events.
    phpLenses?: boolean;
  }
  let { siteName = '', framework = '', domain = '', branch = '', phpLenses = true }: Props = $props();

  // Cache comes solely from the Laravel adapter, so it only applies to Laravel
  // sites; everything else is framework-agnostic (PDO and the Symfony
  // Mailer/Twig/EventDispatcher/Messenger/HttpClient seams cover every PHP app).
  const isLaravel = $derived(framework.toLowerCase() === 'laravel');
  const laravelOnly: DebugLens[] = ['cache'];
  const counts = $derived(countKinds($debugEvents, siteName));

  type Lens = DebugLens;
  const tabs = $derived<TabItem<Lens>[]>([
    { id: 'dumps', label: m.debug_tab_dumps(), count: counts['dump'] },
    { id: 'queries', label: m.debug_tab_queries(), count: counts['query'] },
    { id: 'jobs', label: m.debug_tab_jobs(), count: counts['job'] },
    { id: 'views', label: m.debug_tab_views(), count: counts['view'] },
    { id: 'mail', label: m.debug_tab_mail(), count: counts['mail'] },
    { id: 'cache', label: m.debug_tab_cache(), hidden: !isLaravel, count: counts['cache'] },
    { id: 'events', label: m.debug_tab_events(), count: counts['event'] },
    { id: 'http', label: m.debug_tab_http(), count: counts['http'] },
    { id: 'logs', label: m.debug_tab_logs(), count: counts['log'] },
    { id: 'exceptions', label: m.debug_tab_exceptions(), count: counts['exception'] },
    { id: 'messages', label: m.debug_tab_messages(), count: counts['message'] },
    { id: 'browser', label: m.debug_tab_browser(), count: counts['browser'] }
  ]);

  // A site without PHP keeps the lens bar, with Browser as its only lens.
  const browserOnly = $derived(tabs.filter((t) => t.id === 'browser'));

  // If the remembered lens isn't available for this framework, fall back.
  $effect(() => {
    if (!isLaravel && laravelOnly.includes($debugLens)) debugLens.set('queries');
  });
</script>

<div class="flex flex-col h-full overflow-hidden">
  {#if !phpLenses}
    <DetailTabs tabs={browserOnly} active="browser" onchange={() => {}} keepSingle />
    <div class="flex-1 min-h-0 overflow-hidden">
      <BrowserLens siteScope={siteName} />
    </div>
  {:else if !$debugCaptureEnabled && !$browserCaptureEnabled}
    <DebugDisabled />
  {:else}
    <DetailTabs {tabs} active={$debugLens} onchange={(id) => debugLens.set(id)} />
    <div class="flex-1 min-h-0 overflow-hidden">
      <!-- Browser capture has its own switch, so its lens shows while debug capture is off. -->
      {#if $debugLens === 'browser'}
        <BrowserLens siteScope={siteName} />
      {:else if !$debugCaptureEnabled}
        <DebugDisabled />
      {:else if $debugLens === 'dumps'}
        <DumpsTab siteScope={siteName} />
      {:else if $debugLens === 'queries'}
        <QueriesLens siteScope={siteName} />
      {:else}
        <KindLens kind={$debugLens as 'jobs' | 'views' | 'mail' | 'cache' | 'events' | 'http' | 'logs' | 'exceptions' | 'messages'} siteScope={siteName} />
      {/if}
    </div>
  {/if}
</div>
