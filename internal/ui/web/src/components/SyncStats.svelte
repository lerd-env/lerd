<script lang="ts">
  import { m } from '../paraglide/messages.js';

  // Behind, ahead and changed files, as a pull, push or switch confirmation shows them.
  interface Props {
    behind: number;
    ahead: number;
    // null while the count is still being read.
    files: number | null;
    // Makes the files card open the full list of changed files.
    onfiles?: () => void;
    filesTitle?: string;
  }
  let { behind, ahead, files, onfiles, filesTitle }: Props = $props();
</script>

{#snippet stat(n: string, label: string, link = false)}
  <div class="h-full rounded-lg bg-gray-50 dark:bg-white/[0.03] border border-gray-100 dark:border-lerd-border px-3 py-2">
    <div class="text-lg font-semibold tabular-nums text-gray-800 dark:text-gray-100">{n}</div>
    <div class="text-[11px] {link ? 'text-lerd-red' : 'text-gray-500 dark:text-gray-400'}">{label}</div>
  </div>
{/snippet}

<div class="grid grid-cols-3 gap-2">
  {@render stat(String(behind), m.branchSwitch_statBehind())}
  {@render stat(String(ahead), m.branchSwitch_statAhead())}
  {#if onfiles && files}
    <button
      type="button"
      onclick={onfiles}
      title={filesTitle}
      class="text-left rounded-lg transition-colors hover:ring-1 hover:ring-lerd-red/40 focus-visible:ring-1 focus-visible:ring-lerd-red"
    >
      {@render stat(String(files), m.branchSwitch_statFiles(), true)}
    </button>
  {:else}
    {@render stat(files === null ? '…' : String(files), m.branchSwitch_statFiles())}
  {/if}
</div>
