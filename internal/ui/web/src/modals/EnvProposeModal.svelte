<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import type { EnvProposeEntry } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.envPropose);
  const required = $derived((target?.entries ?? []).filter((e) => e.required));
  const optional = $derived((target?.entries ?? []).filter((e) => !e.required));

  // Seed the tick state once from the entries: required keys start on, optional
  // ones off, so the common case is one click to add exactly what the app needs.
  let checked = $state<Record<string, boolean>>({});
  let seeded = false;
  $effect(() => {
    if (target && !seeded) {
      const init: Record<string, boolean> = {};
      for (const e of target.entries) init[e.key] = e.required;
      checked = init;
      seeded = true;
    }
  });

  const selected = $derived((target?.entries ?? []).filter((e) => checked[e.key]).map((e) => e.key));

  function setAll(on: boolean) {
    const next: Record<string, boolean> = {};
    for (const e of target?.entries ?? []) next[e.key] = on;
    checked = next;
  }

  function confirm() {
    if (!target || selected.length === 0) return;
    target.onAdd(selected);
    closeModal();
  }
</script>

<Modal open title={m.envEditor_proposeModalTitle()} onclose={closeModal} size="md">
  <div class="px-5 py-4 space-y-3">
    {#if !target}
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.common_loading()}</p>
    {:else}
      <div class="flex items-start justify-between gap-3">
        <p class="text-sm text-gray-700 dark:text-gray-300">
          {m.envEditor_proposeModalBody({ file: target.file })}
        </p>
        <div class="flex items-center gap-2 shrink-0 pt-0.5">
          <button type="button" onclick={() => setAll(true)} class="text-[11px] text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200">
            {m.envEditor_proposeSelectAll()}
          </button>
          <span class="text-gray-300 dark:text-gray-600">·</span>
          <button type="button" onclick={() => setAll(false)} class="text-[11px] text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200">
            {m.envEditor_proposeSelectNone()}
          </button>
        </div>
      </div>

      <div class="max-h-80 overflow-y-auto space-y-4 -mx-1 px-1">
        {#snippet keyList(entries: EnvProposeEntry[], heading: string)}
          {#if entries.length > 0}
            <div class="space-y-1">
              <p class="text-[10px] font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">{heading}</p>
              {#each entries as e (e.key)}
                <label class="flex items-center gap-2 py-1 px-1.5 rounded-sm hover:bg-gray-50 dark:hover:bg-white/5 cursor-pointer">
                  <input
                    type="checkbox"
                    bind:checked={checked[e.key]}
                    class="rounded-sm border-gray-300 dark:border-lerd-border shrink-0"
                  />
                  <span class="font-mono text-xs text-gray-800 dark:text-gray-200 shrink-0">{e.key}</span>
                  <span class="font-mono text-xs text-gray-400 dark:text-gray-500 truncate" title={e.value}>
                    ={e.value}
                  </span>
                </label>
              {/each}
            </div>
          {/if}
        {/snippet}

        {@render keyList(required, m.envEditor_proposeRequiredHeading())}
        {@render keyList(optional, m.envEditor_proposeOptionalHeading())}
      </div>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={closeModal}>{m.common_cancel()}</DetailButton>
    {#if target}
      <DetailButton tone="primary" onclick={confirm} disabled={selected.length === 0}>
        {selected.length > 0 ? m.envEditor_proposeAddSelected({ n: selected.length }) : m.envEditor_proposeAddNone()}
      </DetailButton>
    {/if}
  {/snippet}
</Modal>
