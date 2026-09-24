<script lang="ts">
  import { coreDown } from '$stores/status';
  import { accessMode } from '$stores/accessMode';
  import { goToTab } from '$stores/route';
  import {
    lerdStart,
    lerdStarting,
    lerdStopping,
    lerdStartStep,
    lerdStartUnit,
    lerdStartDone,
    lerdStartTotal
  } from '$stores/lerdLifecycle';
  import { m } from '../../paraglide/messages.js';

  // The stage ids the start stream emits, mapped to their own message. A unit
  // name is shown as-is: it is the same identifier the CLI prints.
  const startStepLabel = $derived.by(() => {
    if ($lerdStartUnit) return $lerdStartUnit;
    switch ($lerdStartStep) {
      case 'preparing':
        return m.dashboard_hero_startStep_preparing();
      case 'images':
        return m.dashboard_hero_startStep_images();
      case 'units':
        return m.dashboard_hero_startStep_units();
      case 'dns':
        return m.dashboard_hero_startStep_dns();
      default:
        return '';
    }
  });

  const startButtonLabel = $derived.by(() => {
    if (!$lerdStarting) return m.dashboard_hero_startLerd();
    if ($lerdStartTotal > 0)
      return m.dashboard_hero_startingLerdCount({ done: $lerdStartDone, total: $lerdStartTotal });
    return m.dashboard_hero_startingLerd();
  });
</script>

{#if $coreDown.length > 0}
  <p class="inline-flex items-center gap-2 min-w-0 px-2.5 py-1 rounded-full border border-red-200 dark:border-red-500/30 bg-red-50 dark:bg-red-500/10 text-xs text-red-700 dark:text-red-300/80">
    <span class="relative inline-flex w-2 h-2 shrink-0">
      <span class="absolute inline-flex w-full h-full rounded-full bg-red-400 opacity-75 animate-ping"></span>
      <span class="relative inline-flex w-2 h-2 rounded-full bg-red-500"></span>
    </span>
    <span class="font-semibold text-red-900 dark:text-red-200">{m.dashboard_hero_coreDown({ components: $coreDown.join(', ') })}</span>
    <span aria-hidden="true">·</span>
    <span class="truncate">{$lerdStarting && startStepLabel ? startStepLabel : m.dashboard_hero_coreDownHint()}</span>
  </p>
  {#if $accessMode.localControl}
    <button
      onclick={lerdStart}
      disabled={$lerdStarting || $lerdStopping}
      class="shrink-0 inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-red-600 hover:bg-red-700 text-white disabled:opacity-50 transition-colors"
    >{startButtonLabel}</button>
  {/if}
  <button
    onclick={() => goToTab('system', 'lerd')}
    class="shrink-0 inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border border-red-300 dark:border-red-500/40 text-red-800 dark:text-red-200 hover:bg-red-100 dark:hover:bg-red-500/15 transition-colors"
  >{m.dashboard_hero_openSystem()}</button>
{/if}
