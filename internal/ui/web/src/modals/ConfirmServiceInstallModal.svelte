<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import {
    presets,
    presetsLoaded,
    availableVersions,
    installPresetAndOpen,
    loadPresets,
    presetAddLabel,
    type Preset
  } from '$stores/presets';
  import { serviceLabel } from '$stores/services';
  import { m } from '../paraglide/messages.js';

  const name = $derived($modal.serviceInstall?.name ?? '');
  const label = $derived(serviceLabel(name));
  // /api/services/presets carries a row per bundled preset with its installed
  // state; a name with no row is a custom service the store cannot install.
  const preset = $derived($presets.find((p) => p.name === name));

  onMount(() => {
    loadPresets();
  });

  function setSelectedVersion(tag: string) {
    presets.update((list) => list.map((p) => (p.name === name ? { ...p, selected_version: tag } : p)));
  }

  async function onInstall(p: Preset) {
    await installPresetAndOpen(p, { onSuccess: closeModal });
  }

  function safeClose() {
    if (preset?.installing) return;
    closeModal();
  }
</script>

<Modal open title={label} onclose={safeClose} size="sm">
  <div class="px-5 py-4 space-y-3">
    {#if !$presetsLoaded}
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.common_loading()}</p>
    {:else if preset}
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.services_install_offer({ name: label })}</p>
      {#if preset.description}
        <p class="text-xs text-gray-500 dark:text-gray-400">{preset.description}</p>
      {/if}
      {#if preset.image}
        <div class="text-[11px] text-gray-400 dark:text-gray-500 font-mono truncate">{preset.image}</div>
      {/if}
      {#if (preset.versions || []).length > 0}
        <Dropdown
          value={preset.selected_version ?? ''}
          options={availableVersions(preset).map((v) => ({ value: v.tag, label: v.label || v.tag }))}
          onchange={(v) => setSelectedVersion(v)}
        />
      {/if}
      {#if (preset.missing_deps || []).length > 0}
        <p class="text-xs text-amber-600 dark:text-amber-400">
          {m.services_preset_installFirst({ deps: (preset.missing_deps || []).join(', ') })}
        </p>
      {/if}
      {#if preset.installing && preset.installingMessage}
        <p class="text-[11px] text-gray-400 dark:text-gray-500 font-mono truncate">{preset.installingMessage}</p>
      {/if}
      {#if preset.error}
        <p class="text-xs text-red-500">{preset.error}</p>
      {/if}
    {:else}
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.services_install_custom({ name })}</p>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={Boolean(preset?.installing)}>{m.common_close()}</DetailButton>
    {#if preset}
      <DetailButton
        tone="primary"
        onclick={() => onInstall(preset)}
        disabled={Boolean(preset.installing) || (preset.missing_deps || []).length > 0}
        loading={Boolean(preset.installing)}
      >
        {presetAddLabel(preset)}
      </DetailButton>
    {/if}
  {/snippet}
</Modal>
