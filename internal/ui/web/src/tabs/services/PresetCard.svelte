<script lang="ts">
  import DetailButton from '$components/DetailButton.svelte';
  import Icon from '$components/Icon.svelte';
  import ServiceCardShell from '$components/ServiceCardShell.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import { type CategoryKey } from '$lib/presetCategories';
  import { installPresetAndOpen, presetAddLabel, type Preset } from '$stores/presets';
  import { serviceLabel } from '$stores/services';

  interface Props {
    preset: Preset;
    category?: CategoryKey;
  }
  let { preset, category }: Props = $props();

  async function add() {
    if (preset.installing) return;
    await installPresetAndOpen(preset);
  }
</script>

{#snippet plusIcon()}<Icon name="plus" class="w-4 h-4" />{/snippet}

<ServiceCardShell>
  <ServiceIcon name={preset.name} {category} />
  <div class="min-w-0 flex-1">
    <div class="text-sm font-semibold text-gray-900 dark:text-white truncate" title={serviceLabel(preset.name)}>{serviceLabel(preset.name)}</div>
    {#if preset.error}
      <p class="text-[11px] leading-snug text-red-500 truncate" title={preset.error}>{preset.error}</p>
    {:else if preset.description}
      <p class="text-[11px] leading-snug text-gray-500 dark:text-gray-400 truncate" title={preset.description}>{preset.description}</p>
    {/if}
  </div>
  <div class="shrink-0">
    <DetailButton
      tone="secondary"
      onclick={add}
      disabled={Boolean(preset.installing)}
      loading={Boolean(preset.installing)}
      title={preset.installingMessage || presetAddLabel(preset)}
      icon={plusIcon}
    />
  </div>
</ServiceCardShell>
