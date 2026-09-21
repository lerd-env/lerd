<script lang="ts">
  import { untrack } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import { m } from '../../paraglide/messages.js';
  import type { WorkerOption } from '$stores/sites';

  // Edits the values a worker's framework definition exposes through its
  // tune_command (Laravel's queue worker: queue, tries, timeout), persisted to
  // .lerd.yaml. The fields come from the definition, so a framework that
  // declares different ones gets them here without a change in the UI.
  interface Props {
    open: boolean;
    label: string;
    options: WorkerOption[];
    onclose: () => void;
    onsave: (values: Record<string, string>) => void;
  }
  let { open, label, options, onclose, onsave }: Props = $props();

  let values = $state<Record<string, string>>({});

  // Seed only on the open transition (options read untracked), so a live sites
  // push mid-edit doesn't overwrite what the user is typing.
  $effect(() => {
    if (!open) return;
    const seeded: Record<string, string> = {};
    for (const o of untrack(() => options)) seeded[o.name] = o.value || '';
    values = seeded;
  });

  function save() {
    const out: Record<string, string> = {};
    for (const o of options) out[o.name] = (values[o.name] || '').trim();
    onsave(out);
    onclose();
  }
</script>

<Modal {open} {onclose} title={m.sites_controls_workerOptionsTitle({ label })} size="sm">
  <div class="px-5 py-4 space-y-3">
    {#each options as o (o.name)}
      <label class="block text-sm text-gray-600 dark:text-gray-400" for={'worker-option-' + o.name}>
        {o.name}
      </label>
      <input
        id={'worker-option-' + o.name}
        type="text"
        bind:value={values[o.name]}
        placeholder={o.default}
        spellcheck="false"
        autocomplete="off"
        onkeydown={(e) => e.key === 'Enter' && save()}
        class="w-full text-sm font-mono px-2.5 py-1.5 rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card text-gray-800 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-lerd-red"
      />
    {/each}
    <p class="text-[11px] text-gray-400 dark:text-gray-500">
      {m.sites_controls_workerOptionsHint()}
    </p>
  </div>
  {#snippet footer()}
    <button
      type="button"
      onclick={onclose}
      class="text-xs px-3 py-1.5 rounded-sm border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5 transition-colors"
    >{m.common_cancel()}</button>
    <button
      type="button"
      onclick={save}
      class="text-xs px-3 py-1.5 rounded-sm bg-lerd-red hover:bg-lerd-redhov text-lerd-onred transition-colors"
    >{m.common_save()}</button>
  {/snippet}
</Modal>
