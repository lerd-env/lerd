<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.envDuplicates);
  const duplicates = $derived(target?.duplicates ?? []);

  // Which occurrence to keep per key, as a line number. Nothing is chosen up
  // front: lerd cannot know which value the project meant, and defaulting to one
  // would make a guess look like a recommendation.
  let keep = $state<Record<string, number>>({});
  let seeded = false;
  $effect(() => {
    if (target && !seeded) {
      keep = {};
      seeded = true;
    }
  });

  const resolved = $derived(duplicates.filter((d) => keep[d.key] !== undefined));

  function confirm() {
    if (!target || resolved.length === 0) return;
    target.onResolve(resolved.map((d) => ({ key: d.key, line: keep[d.key] })));
    closeModal();
  }
</script>

<Modal open title={m.envEditor_duplicateModalTitle()} onclose={closeModal} size="md">
  <div class="px-5 py-4 space-y-3">
    {#if !target}
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.common_loading()}</p>
    {:else}
      <p class="text-sm text-gray-700 dark:text-gray-300">
        {m.envEditor_duplicateModalBody({ file: target.file })}
      </p>

      <div class="max-h-80 overflow-y-auto space-y-4 -mx-1 px-1">
        {#each duplicates as dupe (dupe.key)}
          <div class="space-y-1">
            <p class="font-mono text-xs font-semibold text-gray-800 dark:text-gray-200">
              {dupe.key}
            </p>
            {#each dupe.occurrences as occ (occ.line)}
              <label
                class="flex items-center gap-2 py-1 px-1.5 rounded-sm hover:bg-gray-50 dark:hover:bg-white/5 cursor-pointer"
              >
                <input
                  type="radio"
                  name={'dupe-' + dupe.key}
                  value={occ.line}
                  checked={keep[dupe.key] === occ.line}
                  onchange={() => (keep = { ...keep, [dupe.key]: occ.line })}
                  class="border-gray-300 dark:border-lerd-border shrink-0"
                />
                <span class="text-[10px] text-gray-400 dark:text-gray-500 shrink-0 tabular-nums">
                  {m.envEditor_duplicateLine({ n: occ.line + 1 })}
                </span>
                <span
                  class="font-mono text-xs text-gray-600 dark:text-gray-300 truncate"
                  title={occ.value}
                >
                  {occ.value || "''"}
                </span>
              </label>
            {/each}
          </div>
        {/each}
      </div>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={closeModal}>{m.common_cancel()}</DetailButton>
    {#if target}
      <DetailButton tone="primary" onclick={confirm} disabled={resolved.length === 0}>
        {m.envEditor_duplicateKeepSelected({ n: resolved.length })}
      </DetailButton>
    {/if}
  {/snippet}
</Modal>
