<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal } from '$stores/modals';
  import {
    presets,
    presetsLoaded,
    installablePresets,
    availableVersions,
    installPresetAndOpen,
    loadPresets,
    presetAddLabel,
    matchesPresetQuery,
    type Preset
  } from '$stores/presets';
  import { m } from '../paraglide/messages.js';

  let query = $state('');
  const filtered = $derived($installablePresets.filter((p) => matchesPresetQuery(p, query)));

  onMount(() => {
    loadPresets();
  });

  function setSelectedVersion(name: string, tag: string) {
    presets.update((list) => list.map((p) => (p.name === name ? { ...p, selected_version: tag } : p)));
  }

  async function onInstall(p: Preset) {
    await installPresetAndOpen(p, { onSuccess: closeModal });
  }
</script>

<Modal open title={m.services_preset_title()} onclose={closeModal}>
  <div class="px-5 py-3 border-b border-gray-100 dark:border-lerd-border">
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.services_preset_subtitle()}</p>
  </div>
  {#if $presetsLoaded && $installablePresets.length > 0}
    <div class="px-5 py-2 border-b border-gray-100 dark:border-lerd-border">
      <input
        bind:value={query}
        type="text"
        placeholder={m.services_preset_searchPlaceholder()}
        class="w-full bg-transparent text-sm text-gray-800 dark:text-gray-100 placeholder:text-gray-400 dark:placeholder:text-gray-500 focus:outline-hidden"
        autocomplete="off"
        spellcheck="false"
      />
    </div>
  {/if}
  <div class="px-5 py-3 max-h-80 overflow-y-auto">
    {#if !$presetsLoaded}
      <div class="py-4 text-center text-xs text-gray-400">{m.common_loading()}</div>
    {:else if $installablePresets.length === 0}
      <div class="py-4 text-center text-xs text-gray-400">{m.services_preset_allInstalled()}</div>
    {:else if filtered.length === 0}
      <div class="py-4 text-center text-xs text-gray-400">{m.services_preset_noMatches()}</div>
    {:else}
      {#each filtered as p (p.name)}
        <div class="border border-gray-100 dark:border-lerd-border rounded-lg p-3 mb-2 last:mb-0">
          <div class="flex items-center gap-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{p.name}</span>
                {#if (p.installed_tags || []).length > 0}
                  <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded-sm bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400">
                    {m.services_preset_installedTag({ tags: (p.installed_tags || []).join(', ') })}
                  </span>
                {/if}
              </div>
              {#if p.description}
                <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">{p.description}</p>
              {/if}
              {#if p.image}
                <div class="text-[11px] text-gray-400 dark:text-gray-500 mt-1 font-mono truncate">{p.image}</div>
              {/if}
              {#if p.depends_on && p.depends_on.length > 0}
                <div class="text-[11px] text-gray-400 dark:text-gray-500 mt-1">{m.services_preset_dependsOn()} {p.depends_on.join(', ')}</div>
              {/if}
              {#if p.dashboard}
                <div class="text-[11px] text-gray-400 dark:text-gray-500 mt-0.5">{m.services_preset_dashboard()} {p.dashboard}</div>
              {/if}
              {#if (p.missing_deps || []).length > 0}
                <div class="text-[11px] text-amber-600 dark:text-amber-400 mt-1">{m.services_preset_installFirst({ deps: (p.missing_deps || []).join(', ') })}</div>
              {/if}
            </div>
            {#if (p.versions || []).length > 0}
              <div class="shrink-0">
                <Dropdown
                  value={p.selected_version ?? ''}
                  options={availableVersions(p).map((v) => ({ value: v.tag, label: v.label || v.tag }))}
                  onchange={(v) => setSelectedVersion(p.name, v)}
                />
              </div>
            {/if}
            <DetailButton
              tone="primary"
              onclick={() => onInstall(p)}
              disabled={Boolean(p.installing) || (p.missing_deps || []).length > 0}
              loading={Boolean(p.installing)}
            >
              {presetAddLabel(p)}
            </DetailButton>
          </div>
          {#if p.installing && p.installingMessage}
            <p class="text-[11px] text-gray-400 dark:text-gray-500 mt-2 font-mono truncate">{p.installingMessage}</p>
          {/if}
          {#if p.error}
            <p class="text-[11px] text-red-500 mt-2">{p.error}</p>
          {/if}
        </div>
      {/each}
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={closeModal}>{m.common_close()}</DetailButton>
  {/snippet}
</Modal>
