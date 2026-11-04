<script lang="ts" module>
  export interface BatchItem {
    name: string;
    label: string;
    // The version line the row shows until its update starts.
    target: string;
  }
  export interface BatchProgress {
    message: string;
    error?: boolean;
  }
  export interface BatchResult {
    name: string;
    ok: boolean;
    error?: string;
  }
</script>

<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import Toggle from './Toggle.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    // What had an update when the dialog opened. Kept fixed so a row does not
    // vanish the moment its own update clears the badge.
    items: BatchItem[];
    title: string;
    body: string;
    confirmLabel: string;
    progress?: Record<string, BatchProgress | undefined>;
    icon?: Snippet<[BatchItem]>;
    run: (names: string[], onresult: (r: BatchResult) => void) => Promise<BatchResult[]>;
    onclose: () => void;
    ondone: (results: BatchResult[]) => void;
  }
  let { items, title, body, confirmLabel, progress = {}, icon, run, onclose, ondone }: Props = $props();

  let on = $state<Record<string, boolean>>(
    Object.fromEntries(untrack(() => items).map((i) => [i.name, true]))
  );
  let updating = $state(false);
  // Live progress is cleared once an update ends, so its outcome is kept here
  // for the row to show while the rest of the batch runs.
  let outcome = $state<Record<string, BatchResult>>({});
  const selected = $derived(items.filter((i) => on[i.name]).map((i) => i.name));

  async function start() {
    updating = true;
    try {
      ondone(await run(selected, (r) => (outcome[r.name] = r)));
    } finally {
      updating = false;
      onclose();
    }
  }
</script>

<ConfirmModal
  open
  {title}
  {body}
  {confirmLabel}
  loading={updating}
  disabled={selected.length === 0}
  onconfirm={start}
  {onclose}
>
  {#snippet extra()}
    <div class="max-h-[50vh] overflow-y-auto rounded-lg border border-gray-200 dark:border-lerd-border divide-y divide-gray-100 dark:divide-lerd-border">
      {#each items as item (item.name)}
        {@const live = progress[item.name]}
        {@const result = outcome[item.name]}
        <div class="flex items-center gap-3 px-3 py-2.5">
          {#if icon}{@render icon(item)}{/if}
          <div class="flex-1 min-w-0">
            <div class="text-sm text-gray-800 dark:text-gray-100 truncate">{item.label}</div>
            {#if live}
              <div class="truncate text-xs {live.error ? 'text-red-500' : 'text-gray-500 dark:text-gray-400'}">{live.message}</div>
            {:else if result}
              <div class="truncate text-xs {result.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-500'}">
                {result.ok ? m.services_updateAll_rowDone() : result.error}
              </div>
            {:else}
              <div class="truncate font-mono text-xs text-emerald-600 dark:text-emerald-400">{item.target}</div>
            {/if}
          </div>
          <Toggle on={on[item.name]} title={item.label} disabled={updating} onclick={() => (on[item.name] = !on[item.name])} />
        </div>
      {/each}
    </div>
  {/snippet}
</ConfirmModal>
