<script lang="ts">
  import DetailTabs from '$components/DetailTabs.svelte';
  import { onMount } from 'svelte';
  import DumpsTab from '$tabs/DumpsTab.svelte';
  import QueriesLens from '$components/QueriesLens.svelte';
  import KindLens from '$components/KindLens.svelte';
  import DebugDisabled from '$components/DebugDisabled.svelte';
  import BrowserLens from '$components/BrowserLens.svelte';
  import { debugLens, debugLensTabs, type DebugLens } from '$stores/debugLens';
  import { refreshStatus } from '$stores/dumps';
  import { refreshDevtoolsStatus, debugCaptureEnabled } from '$stores/queries';
  import { countKinds, debugEvents } from '$stores/debugEvents';
  import Icon from '$components/Icon.svelte';
  import { modal } from '$stores/modals';
  import { tooltip } from '$lib/tooltip';
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

  const tabs = $derived(debugLensTabs(counts, isLaravel));

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
  {#if fullscreen && domain}<span class="text-xs font-mono text-gray-600 dark:text-gray-300">{domain}</span>{/if}
  <button
    type="button"
    onclick={() => (fullscreen = !fullscreen)}
    class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
    use:tooltip={fullscreen ? m.debug_exitFullscreenTitle() : m.debug_fullscreenTitle()}
    aria-label={fullscreen ? m.debug_exitFullscreenTitle() : m.debug_fullscreenTitle()}
  >
    <Icon name={fullscreen ? 'minimize' : 'maximize'} class="w-4 h-4" />
  </button>
{/snippet}

<div class="flex flex-col overflow-hidden {fullscreen ? 'fixed inset-0 z-50 bg-white dark:bg-lerd-bg' : 'h-full'}">
  {#if !$debugCaptureEnabled}
    <DebugDisabled />
  {:else if !phpLenses}
    <DetailTabs tabs={browserOnly} active="browser" onchange={() => {}} keepSingle actions={fullscreenAction} />
    <div class="flex-1 min-h-0 overflow-hidden">
      <BrowserLens siteScope={siteName} />
    </div>
  {:else}
    <DetailTabs {tabs} active={$debugLens} onchange={(id) => debugLens.set(id)} actions={fullscreenAction} />
    <div class="flex-1 min-h-0 overflow-hidden">
      {#if $debugLens === 'browser'}
        <BrowserLens siteScope={siteName} />
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
