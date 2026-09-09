<script lang="ts">
  // One selectable option in a runtime picker: a radio dot, a title and a
  // description. Shared by the worker-mode and PHP-runtime panes so the two
  // pickers cannot drift apart visually.
  interface Props {
    selected: boolean;
    disabled?: boolean;
    accent: 'emerald' | 'sky';
    title: string;
    description: string;
    onclick: () => void;
  }
  let { selected, disabled = false, accent, title, description, onclick }: Props = $props();

  const border = $derived(
    accent === 'emerald'
      ? 'border-emerald-500 dark:border-emerald-400 bg-emerald-50 dark:bg-emerald-500/10 ring-1 ring-emerald-500/20'
      : 'border-sky-500 dark:border-sky-400 bg-sky-50 dark:bg-sky-500/10 ring-1 ring-sky-500/20'
  );
  const dot = $derived(
    accent === 'emerald' ? 'border-emerald-500 dark:border-emerald-400' : 'border-sky-500 dark:border-sky-400'
  );
  const fill = $derived(accent === 'emerald' ? 'bg-emerald-500 dark:bg-emerald-400' : 'bg-sky-500 dark:bg-sky-400');
  const titleColor = $derived(
    selected
      ? accent === 'emerald'
        ? 'text-emerald-900 dark:text-emerald-200'
        : 'text-sky-900 dark:text-sky-200'
      : 'text-gray-800 dark:text-gray-200'
  );
</script>

<button
  type="button"
  {onclick}
  {disabled}
  aria-pressed={selected}
  class="w-full text-left flex items-start gap-3 p-3 rounded-sm border-2 transition-colors disabled:opacity-60 disabled:cursor-not-allowed {selected
    ? border
    : 'border-gray-200 dark:border-lerd-border hover:border-gray-300 dark:hover:border-white/20 hover:bg-gray-50 dark:hover:bg-white/3'}"
>
  <span
    class="mt-0.5 inline-flex items-center justify-center w-4 h-4 rounded-full border-2 shrink-0 {selected
      ? dot
      : 'border-gray-300 dark:border-gray-500'}"
  >
    {#if selected}
      <span class="w-2 h-2 rounded-full {fill}"></span>
    {/if}
  </span>
  <span class="flex-1">
    <span class="block text-sm font-medium {titleColor}">{title}</span>
    <span class="block text-xs text-gray-500 dark:text-gray-400 mt-0.5">{description}</span>
  </span>
</button>
