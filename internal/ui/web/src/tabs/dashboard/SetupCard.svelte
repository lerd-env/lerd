<script lang="ts">
  import { onMount } from 'svelte';
  import { setupSteps, setupDone, startSetup, finishSetup, type SetupStepId } from '$stores/setup';
  import { openLinkModal, openPresetModal } from '$stores/modals';
  import { openDocs } from '$stores/dashboard';
  import { toggleStartOnDashboardOpen } from '$stores/autostart';
  import { saveIdle, idleTimeoutMinutes } from '$stores/idle';
  import { accessMode } from '$stores/accessMode';
  import { enableNotifications } from '$lib/notify';
  import { desktopPalette, useSuggestedTheme } from '$stores/palettes';
  import { m } from '../../paraglide/messages.js';

  // How long "You're all set" stays before the slot goes back to the Lerd card.
  const CELEBRATE_MS = 3000;
  const RING = 2 * Math.PI * 15;

  const copy: Record<SetupStepId, { title: () => string; hint: () => string; cta: () => string; primary?: boolean }> = {
    site: { title: m.setup_site_title, hint: m.setup_site_hint, cta: m.onboarding_link_cta, primary: true },
    service: { title: m.setup_service_title, hint: m.setup_service_hint, cta: m.onboarding_service_cta },
    notify: { title: m.setup_notify_title, hint: m.setup_notify_hint, cta: m.notify_banner_enable },
    theme: { title: m.setup_theme_title, hint: m.setup_theme_hint, cta: () => m.setup_theme_cta({ name: $desktopPalette?.name ?? '' }) },
    open: { title: m.setup_open_title, hint: m.setup_open_hint, cta: m.setup_turnOn },
    idle: { title: m.setup_idle_title, hint: m.setup_idle_hint, cta: m.setup_turnOn }
  };

  const actions: Record<SetupStepId, () => void> = {
    site: openLinkModal,
    service: openPresetModal,
    notify: () => void enableNotifications(),
    theme: useSuggestedTheme,
    open: () => void toggleStartOnDashboardOpen(true),
    idle: () => void saveIdle(true, $idleTimeoutMinutes)
  };

  const doneCount = $derived($setupSteps.filter((s) => s.done).length);
  let celebrating = $state(false);
  let wasDone = false;

  onMount(() => {
    // Setup that finished while the dashboard was closed has nobody to show the
    // moment to, so the slot goes straight back to the Lerd card.
    if ($setupDone) {
      finishSetup();
      return;
    }
    startSetup();
    wasDone = false;
    return setupDone.subscribe((done) => {
      if (!done || wasDone) return;
      wasDone = true;
      celebrating = true;
      setTimeout(finishSetup, CELEBRATE_MS);
    });
  });
</script>

<!-- Sized like DashboardCard so it takes the Lerd card's grid slot exactly. -->
<div class="flex flex-col min-h-[280px] max-h-[340px] xl:min-h-0 xl:max-h-none bg-white dark:bg-lerd-card bg-linear-to-br from-lerd-red/8 via-transparent to-transparent dark:from-lerd-red/12 border border-lerd-red/25 dark:border-lerd-red/35 rounded-xl overflow-hidden">
  {#if celebrating}
    <div class="flex-1 flex flex-col items-center justify-center gap-2 px-6 text-center lerd-panel-in" role="status">
      <span class="w-12 h-12 rounded-full bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 grid place-items-center">
        <svg class="w-6 h-6" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg>
      </span>
      <p class="text-base font-semibold text-gray-900 dark:text-white">{m.setup_done_title()}</p>
      <p class="text-xs text-gray-500 dark:text-gray-400 max-w-[30ch]">{m.setup_done_body()}</p>
    </div>
  {:else}
    <div class="shrink-0 flex items-center gap-3 px-3 pt-3 pb-2.5">
      <svg class="w-11 h-11 shrink-0" viewBox="0 0 36 36" aria-hidden="true">
        <circle cx="18" cy="18" r="15" fill="none" stroke-width="3.5" class="stroke-gray-200 dark:stroke-lerd-border"/>
        <circle cx="18" cy="18" r="15" fill="none" stroke-width="3.5" stroke-linecap="round" class="stroke-lerd-red transition-[stroke-dashoffset] duration-300 -rotate-90 origin-center"
          stroke-dasharray={RING} stroke-dashoffset={RING - (RING * doneCount) / $setupSteps.length}/>
        <text x="18" y="22" text-anchor="middle" class="fill-gray-800 dark:fill-gray-100 text-[10.5px] font-semibold tabular-nums">{doneCount}/{$setupSteps.length}</text>
      </svg>
      <div class="min-w-0">
        <p class="text-sm font-semibold text-gray-900 dark:text-white">{m.setup_title()}</p>
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.setup_left({ count: String($setupSteps.length - doneCount) })}</p>
      </div>
      <button
        type="button"
        onclick={finishSetup}
        title={m.onboarding_dismiss()}
        aria-label={m.onboarding_dismiss()}
        class="ml-auto self-start w-7 h-7 inline-flex items-center justify-center rounded-sm text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-white/10 transition-colors"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
      </button>
    </div>

    <ol class="flex-1 min-h-0 overflow-y-auto px-3">
      {#each $setupSteps as step (step.id)}
        {@const c = copy[step.id]}
        <li class="grid grid-cols-[18px_1fr_auto] items-center gap-2.5 border-t border-gray-100 dark:border-lerd-border {step.done ? 'py-1.5' : 'py-2'}">
          {#if step.done}
            <span class="w-[18px] h-[18px] rounded-full bg-emerald-500 text-white grid place-items-center">
              <svg class="w-3 h-3" fill="none" stroke="currentColor" stroke-width="3.5" viewBox="0 0 24 24" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7"/></svg>
            </span>
            <span class="text-sm text-gray-400 dark:text-gray-500 line-through">{c.title()}</span>
          {:else}
            <span class="w-[18px] h-[18px] rounded-full border-[1.5px] border-gray-300 dark:border-gray-600"></span>
            <span class="min-w-0">
              <span class="block text-sm font-semibold text-gray-800 dark:text-gray-100">{c.title()}</span>
              <span class="block text-xs text-gray-500 dark:text-gray-400">{c.hint()}</span>
            </span>
            {#if $accessMode.localControl}
              <button
                type="button"
                onclick={actions[step.id]}
                class="shrink-0 inline-flex items-center px-2.5 py-1 rounded-md text-xs font-medium transition-colors {c.primary ? 'bg-lerd-red hover:bg-lerd-redhov text-lerd-onred' : 'bg-gray-100 hover:bg-gray-200 dark:bg-white/10 dark:hover:bg-white/20 text-gray-700 dark:text-gray-200'}"
              >{c.cta()}</button>
            {:else}
              <span class="text-[11px] text-gray-400 dark:text-gray-500 max-w-[12ch] text-right">{m.onboarding_loopbackOnly()}</span>
            {/if}
          {/if}
        </li>
      {/each}
    </ol>

    <div class="shrink-0 flex items-center gap-2 px-3 py-2.5 border-t border-gray-100 dark:border-lerd-border text-xs text-gray-500 dark:text-gray-400">
      <button type="button" onclick={openDocs} class="font-medium text-lerd-red hover:text-lerd-redhov transition-colors">{m.onboarding_docs()}</button>
    </div>
  {/if}
</div>
