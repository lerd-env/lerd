<script lang="ts">
  import DetailTabs, { type TabItem } from '$components/DetailTabs.svelte';
  import { onMount } from 'svelte';
  import DumpsTab from '$tabs/DumpsTab.svelte';
  import QueriesLens from '$components/QueriesLens.svelte';
  import KindLens from '$components/KindLens.svelte';
  import DebugDisabled from '$components/DebugDisabled.svelte';
  import { debugLens, type DebugLens } from '$stores/debugLens';
  import { dumps, refreshStatus } from '$stores/dumps';
  import { refreshDevtoolsStatus, debugCaptureEnabled } from '$stores/queries';
  import { countKinds } from '$stores/debugEvents';
  import { m } from '../../paraglide/messages.js';

  onMount(() => {
    void refreshStatus();
    void refreshDevtoolsStatus();
  });

  interface Props {
    siteName?: string;
    framework?: string;
    domain?: string;
    branch?: string;
  }
  let { siteName = '', framework = '', domain = '', branch = '' }: Props = $props();

  // Cache comes solely from the Laravel adapter, so it only applies to Laravel
  // sites; everything else is framework-agnostic (PDO and the Symfony
  // Mailer/Twig/EventDispatcher/Messenger/HttpClient seams cover every PHP app).
  const isLaravel = $derived(framework.toLowerCase() === 'laravel');
  const laravelOnly: DebugLens[] = ['cache'];
  const counts = $derived(countKinds($dumps, siteName));

  type Lens = DebugLens;
  const tabs = $derived<TabItem<Lens>[]>([
    { id: 'dumps', label: m.debug_tab_dumps(), count: counts['dump'] },
    { id: 'queries', label: m.debug_tab_queries(), count: counts['query'] },
    { id: 'jobs', label: m.debug_tab_jobs(), count: counts['job'] },
    { id: 'views', label: m.debug_tab_views(), count: counts['view'] },
    { id: 'mail', label: m.debug_tab_mail(), count: counts['mail'] },
    { id: 'cache', label: m.debug_tab_cache(), hidden: !isLaravel, count: counts['cache'] },
    { id: 'events', label: m.debug_tab_events(), count: counts['event'] },
    { id: 'http', label: m.debug_tab_http(), count: counts['http'] }
  ]);

  // If the remembered lens isn't available for this framework, fall back.
  $effect(() => {
    if (!isLaravel && laravelOnly.includes($debugLens)) debugLens.set('queries');
  });
</script>

<div class="flex flex-col h-full overflow-hidden">
  {#if !$debugCaptureEnabled}
    <DebugDisabled />
  {:else}
    <DetailTabs {tabs} active={$debugLens} onchange={(id) => debugLens.set(id)} />
    <div class="flex-1 min-h-0 overflow-hidden">
      {#if $debugLens === 'dumps'}
        <DumpsTab siteScope={siteName} />
      {:else if $debugLens === 'queries'}
        <QueriesLens siteScope={siteName} />
      {:else}
        <KindLens kind={$debugLens as 'jobs' | 'views' | 'mail' | 'cache' | 'events' | 'http'} siteScope={siteName} />
      {/if}
    </div>
  {/if}
</div>
