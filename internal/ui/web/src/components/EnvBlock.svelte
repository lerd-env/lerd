<script lang="ts">
  import CopyButton from './CopyButton.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    vars: Record<string, string>;
    label?: string;
  }
  let { vars, label = '.env' }: Props = $props();

  const entries = $derived(
    Object.keys(vars)
      .sort()
      .map((k) => [k, vars[k]] as const)
  );
  const text = $derived(entries.map(([k, v]) => `${k}=${v}`).join('\n'));
</script>

<div class="rounded-xl border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card overflow-hidden">
  <div class="flex items-center justify-between gap-2 px-4 py-2.5 border-b border-gray-100 dark:border-lerd-border/60">
    <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{label}</span>
    <CopyButton {text} label={m.common_copy()} />
  </div>
  <div class="divide-y divide-gray-100 dark:divide-lerd-border/60">
    {#each entries as [key, value] (key)}
      <div class="flex items-start gap-3 px-4 py-2 hover:bg-gray-50 dark:hover:bg-white/3 transition-colors">
        <code class="w-44 shrink-0 truncate font-mono text-xs text-gray-500 dark:text-gray-400" title={key}>{key}</code>
        <code class="min-w-0 flex-1 break-all font-mono text-xs text-gray-800 dark:text-gray-100">{value}</code>
        <CopyButton text={`${key}=${value}`} label={m.common_copy()} tone="faint" class="mt-0.5" />
      </div>
    {/each}
  </div>
</div>
