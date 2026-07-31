<script lang="ts">
  import { lan, toggleLANServices } from '$stores/lan';
  import { accessMode } from '$stores/accessMode';
  import Toggle from '$components/Toggle.svelte';
  import SettingsCard from '$components/SettingsCard.svelte';
  import { m } from '../../paraglide/messages.js';

  // nested renders the setting as a sub-section of the LAN exposure card, with
  // a rule separating it from the exposure controls above. Standalone brings
  // its own card, for disabled-DNS mode where there is no LAN card to sit in.
  interface Props {
    nested?: boolean;
  }

  let { nested = false }: Props = $props();

  // The setting publishes nothing while lerd is loopback-only, so it stays out
  // of the way entirely. An already enabled setting keeps showing, so that
  // re-exposing does not silently republish databases nobody remembered.
  const hidden = $derived(!$lan.exposed && !$lan.servicesEnabled);
</script>

{#snippet body()}
  <div class="flex items-center justify-between gap-3 mb-2">
    <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.system_lan_services_title()}</span>
    {#if $accessMode.localControl}
      <Toggle
        on={$lan.servicesEnabled}
        loading={$lan.servicesLoading}
        disabled={!$lan.loaded}
        title={m.system_lan_services_title()}
        onclick={() => toggleLANServices(!$lan.servicesEnabled)}
      />
    {:else}
      <span class="inline-flex items-center gap-1.5 text-[10px] font-medium px-2 py-0.5 rounded-full {$lan.servicesReachable
        ? 'bg-emerald-100 dark:bg-emerald-500/15 text-emerald-700 dark:text-emerald-400'
        : $lan.servicesEnabled
          ? 'bg-amber-100 dark:bg-amber-500/15 text-amber-700 dark:text-amber-400'
          : 'bg-gray-100 dark:bg-white/5 text-gray-500 dark:text-gray-400'}">
        <span class="w-1.5 h-1.5 rounded-full {$lan.servicesReachable
          ? 'bg-emerald-500'
          : $lan.servicesEnabled
            ? 'bg-amber-500'
            : 'bg-gray-400'}"></span>
        {$lan.servicesReachable
          ? m.system_lan_services_enabled()
          : $lan.servicesEnabled
            ? m.system_remote_status_inert()
            : m.system_lan_loopback()}
      </span>
    {/if}
  </div>
  <p class="text-xs text-gray-500 dark:text-gray-400">
    {m.system_lan_services_description()}
  </p>
  {#if $lan.servicesEnabled && !$lan.servicesReachable}
    <p class="text-xs text-amber-600 dark:text-amber-400 mt-2">
      {m.system_lan_services_inactive()}
    </p>
  {/if}
  {#if $lan.servicesError}
    <p class="text-xs text-red-500 mt-2">{$lan.servicesError}</p>
  {/if}
  {#if $lan.servicesEnabled}
    <p class="text-xs text-amber-700 dark:text-amber-300 bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-500/30 rounded-lg px-3 py-2 mt-3">
      {m.system_lan_services_warning()}
    </p>
  {/if}
{/snippet}

{#if !hidden}
  {#if nested}
    <div class="mt-4 pt-4 border-t border-gray-100 dark:border-lerd-border">
      {@render body()}
    </div>
  {:else}
    <SettingsCard>
      {@render body()}
    </SettingsCard>
  {/if}
{/if}
