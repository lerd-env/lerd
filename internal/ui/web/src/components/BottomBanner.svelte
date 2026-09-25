<script lang="ts">
  import type { Snippet } from 'svelte';

  // The card the dashboard asks a one-off question with: an icon, a line of
  // context, one action and a dismiss. App stacks these along the bottom edge.
  let {
    icon,
    title,
    subtitle,
    actionLabel,
    dismissLabel,
    onAction,
    onDismiss
  }: {
    icon: Snippet;
    title: string;
    subtitle: string;
    actionLabel: string;
    dismissLabel: string;
    onAction: () => void;
    onDismiss: () => void;
  } = $props();
</script>

<div class="w-full rounded-lg border-l-4 border border-sky-300 dark:border-sky-500/40 border-l-sky-500 bg-white/85 dark:bg-lerd-card/80 backdrop-blur-md shadow-2xl">
  <div class="flex items-center gap-3 px-3 py-2.5">
    <span class="w-5 h-5 shrink-0 text-sky-600 dark:text-sky-400" aria-hidden="true">{@render icon()}</span>
    <div class="flex-1 min-w-0">
      <p class="text-xs font-semibold text-sky-900 dark:text-sky-200">{title}</p>
      <p class="text-[11px] text-sky-700 dark:text-sky-300/80 mt-0.5">{subtitle}</p>
    </div>
    <button
      onclick={onAction}
      class="shrink-0 inline-flex items-center text-xs font-medium bg-sky-600 hover:bg-sky-700 text-white rounded-sm px-3 py-1.5 transition-colors"
    >{actionLabel}</button>
    <button
      onclick={onDismiss}
      title={dismissLabel}
      aria-label={dismissLabel}
      class="shrink-0 text-sky-600/60 hover:text-sky-700 dark:text-sky-400/60 dark:hover:text-sky-300 transition-colors"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/>
      </svg>
    </button>
  </div>
</div>
