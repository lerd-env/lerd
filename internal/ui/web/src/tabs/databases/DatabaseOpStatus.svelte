<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '$components/Icon.svelte';
  import type { ImportIssue } from '$stores/databases';

  interface Props {
    tone: 'busy' | 'done' | 'warn' | 'error';
    message: string;
    // 0..1 while a measurable operation runs; omitted for one that isn't.
    percent?: number | null;
    // What the engine complained about, when it complained but carried on.
    issues?: ImportIssue[];
    // Anything the caller hangs off the status line, such as a link to the
    // engine's complaints in full.
    children?: Snippet;
  }
  let { tone, message, percent = null, issues = [], children }: Props = $props();

  const icon = $derived(tone === 'busy' ? 'spinner' : tone === 'done' ? 'check' : 'warn');
  const color = $derived(
    tone === 'error'
      ? 'text-red-500'
      : tone === 'warn'
        ? 'text-amber-600 dark:text-amber-400'
        : tone === 'done'
          ? 'text-emerald-600 dark:text-emerald-400'
          : 'text-gray-500 dark:text-gray-400'
  );
</script>

<div class="space-y-1" role="status" aria-live="polite">
  <p class="flex items-start gap-1.5 text-xs {color}">
    <Icon name={icon} class="w-3.5 h-3.5 shrink-0 mt-px {tone === 'busy' ? 'animate-spin' : ''}" />
    <span class="min-w-0 break-words">{message}</span>
  </p>
  {#if tone === 'busy' && percent !== null}
    <div class="h-1 rounded-full bg-gray-100 dark:bg-white/10 overflow-hidden">
      <div
        class="h-full rounded-full bg-lerd-red transition-[width] duration-150"
        style="width: {Math.round(percent * 100)}%"
      ></div>
    </div>
  {/if}
  {#if children}
    <div class="pl-5">{@render children()}</div>
  {/if}
  {#if issues.length > 0}
    <ul class="pl-5 space-y-0.5 text-[11px] text-gray-500 dark:text-gray-400">
      {#each issues as issue (issue.message)}
        <li class="break-words tabular-nums">{issue.count}× {issue.message}</li>
      {/each}
    </ul>
  {/if}
</div>
