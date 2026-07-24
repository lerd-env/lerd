<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import type { ImportIssue } from '$stores/databases';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    title: string;
    issues: ImportIssue[];
    // Distinct complaints dropped past the cap, so a trimmed list never reads
    // as the whole of what went wrong.
    omitted?: number;
    onclose: () => void;
  }
  let { title, issues, omitted = 0, onclose }: Props = $props();
</script>

<Modal open {title} {onclose} size="lg">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-600 dark:text-gray-300">{m.databases_importIssuesBody()}</p>
    <ul class="divide-y divide-gray-100 dark:divide-lerd-border/60 rounded-lg border border-gray-100 dark:border-lerd-border max-h-[55vh] overflow-y-auto">
      {#each issues as issue (issue.message)}
        <li class="flex items-start gap-3 px-3 py-2">
          <span class="shrink-0 tabular-nums text-xs font-semibold text-amber-600 dark:text-amber-400">{issue.count}×</span>
          <code class="min-w-0 break-words text-xs text-gray-700 dark:text-gray-300">{issue.message}</code>
        </li>
      {/each}
    </ul>
    {#if omitted > 0}
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {m.databases_importIssuesMore({ count: omitted })}
      </p>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={onclose}>{m.common_close()}</DetailButton>
  {/snippet}
</Modal>
