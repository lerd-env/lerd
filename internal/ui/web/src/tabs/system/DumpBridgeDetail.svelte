<script lang="ts">
  import { onMount } from 'svelte';
  import DetailPanel from '$components/DetailPanel.svelte';
  import DetailHeader from '$components/DetailHeader.svelte';
  import DetailTabs, { type TabItem } from '$components/DetailTabs.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import DumpsTab from '$tabs/DumpsTab.svelte';
  import QueriesLens from '$components/QueriesLens.svelte';
  import KindLens from '$components/KindLens.svelte';
  import DebugDisabled from '$components/DebugDisabled.svelte';
  import { status as dumpsStatusValue, refreshStatus, togglePassthrough } from '$stores/dumps';
  import { refreshDevtoolsStatus, debugCaptureEnabled, setDebugCapture } from '$stores/queries';
  import { debugLens, type DebugLens } from '$stores/debugLens';
  import { sites } from '$stores/sites';
  import { countKinds, debugEvents } from '$stores/debugEvents';
  import { m } from '../../paraglide/messages.js';

  // The global view aggregates every site; Cache comes solely from the Laravel
  // adapter so its tab shows only when a linked site is Laravel. Everything else
  // is universal (PDO and the Symfony Mailer/Twig/EventDispatcher/Messenger/
  // HttpClient seams).
  const anyLaravel = $derived($sites.some((s) => (s.framework ?? '').toLowerCase() === 'laravel'));
  const laravelOnly: DebugLens[] = ['cache'];
  const counts = $derived(countKinds($debugEvents));

  type Lens = DebugLens;
  const tabs = $derived<TabItem<Lens>[]>([
    { id: 'dumps', label: m.debug_tab_dumps(), count: counts['dump'] },
    { id: 'queries', label: m.debug_tab_queries(), count: counts['query'] },
    { id: 'jobs', label: m.debug_tab_jobs(), count: counts['job'] },
    { id: 'views', label: m.debug_tab_views(), count: counts['view'] },
    { id: 'mail', label: m.debug_tab_mail(), count: counts['mail'] },
    { id: 'cache', label: m.debug_tab_cache(), hidden: !anyLaravel, count: counts['cache'] },
    { id: 'events', label: m.debug_tab_events(), count: counts['event'] },
    { id: 'http', label: m.debug_tab_http(), count: counts['http'] }
  ]);

  $effect(() => {
    if (!anyLaravel && laravelOnly.includes($debugLens)) debugLens.set('queries');
  });

  // One switch arms the whole window (debug bridge + devtools collector).
  let toggling = $state(false);
  async function flip() {
    if (toggling) return;
    toggling = true;
    try {
      await setDebugCapture(!$debugCaptureEnabled);
      await refreshStatus();
    } finally {
      toggling = false;
    }
  }

  let switchingPassthrough = $state(false);
  async function flipPassthrough() {
    if (switchingPassthrough) return;
    switchingPassthrough = true;
    try {
      await togglePassthrough(!$dumpsStatusValue?.passthrough);
      await refreshStatus();
    } finally {
      switchingPassthrough = false;
    }
  }

  onMount(() => {
    void refreshStatus();
    void refreshDevtoolsStatus();
  });
</script>

{#snippet pill()}
  <div class="flex items-center gap-2">
    {#if $debugCaptureEnabled}
      <StatusPill tone="ok" label={m.dumps_bridge_capturing()} />
      <DetailButton tone="secondary" disabled={toggling} loading={toggling} onclick={flip}>
        {m.common_disable()}
      </DetailButton>
    {:else}
      <StatusPill tone="muted" label={m.dumps_bridge_off()} />
      <DetailButton tone="success" disabled={toggling} loading={toggling} onclick={flip}>
        {m.common_enable()}
      </DetailButton>
    {/if}
  </div>
{/snippet}

<DetailPanel>
  <DetailHeader title={m.debug_title()} trailing={pill} />

  {#if !$debugCaptureEnabled}
    <DebugDisabled />
  {:else}
    <DetailTabs {tabs} active={$debugLens} onchange={(id) => debugLens.set(id)} />
    {#if $debugLens === 'dumps'}
    <div class="px-3 sm:px-5 py-2 space-y-2 shrink-0 text-xs text-gray-500 dark:text-gray-400">
      <p>
        {m.dumps_bridge_description()}
        {#if $dumpsStatusValue}
          {m.dumps_bridge_listener({ state: $dumpsStatusValue.listening ? m.dumps_bridge_listenerUp() : m.dumps_bridge_listenerDown(), addr: $dumpsStatusValue.addr })}
          {#if $dumpsStatusValue.count > 0}
            {m.dumps_bridge_buffered({ count: $dumpsStatusValue.count })}{#if $dumpsStatusValue.last_ts}, {m.dumps_bridge_bufferedLast({ time: new Date($dumpsStatusValue.last_ts).toLocaleTimeString() })}{/if}.
          {/if}
        {/if}
      </p>
      <div class="flex items-center gap-2 flex-wrap">
        <label class="inline-flex items-center gap-2 cursor-pointer select-none">
          <input
            type="checkbox"
            class="rounded-sm border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card text-lerd-red focus:ring-lerd-red"
            checked={Boolean($dumpsStatusValue?.passthrough)}
            disabled={switchingPassthrough}
            onchange={flipPassthrough}
          />
          <span>{m.dumps_bridge_passthrough()}</span>
        </label>
        {#if switchingPassthrough}
          <span class="text-[11px] text-amber-600 dark:text-amber-400">{m.dumps_bridge_passthroughRestarting()}</span>
        {:else}
          <span class="text-[11px] text-gray-400 dark:text-gray-500">{m.dumps_bridge_passthroughHint()}</span>
        {/if}
      </div>
    </div>
    <div class="flex-1 min-h-0 overflow-hidden">
      <DumpsTab />
    </div>
  {:else if $debugLens === 'queries'}
    <div class="px-3 sm:px-5 py-2 shrink-0 text-xs text-gray-500 dark:text-gray-400">
      <p>{m.queries_description()}</p>
    </div>
    <div class="flex-1 min-h-0 overflow-hidden">
      <QueriesLens />
    </div>
    {:else}
    <div class="flex-1 min-h-0 overflow-hidden">
      <KindLens kind={$debugLens as 'jobs' | 'views' | 'mail' | 'cache' | 'events' | 'http'} />
    </div>
    {/if}
  {/if}
</DetailPanel>
