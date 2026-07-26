<script lang="ts" module>
  export interface SegmentOption<T extends string = string> {
    value: T;
    label: string;
    title?: string;
    disabled?: boolean;
  }
</script>

<script lang="ts" generics="T extends string">
  import { tooltip } from '$lib/tooltip';

  interface Props {
    options: SegmentOption<T>[];
    value: T;
    label?: string;
    onchange: (value: T) => void;
  }
  let { options, value, label = '', onchange }: Props = $props();
</script>

<div
  role="group"
  aria-label={label}
  class="inline-flex shrink-0 gap-0.5 p-0.5 rounded-lg border border-gray-200 dark:border-lerd-border bg-gray-100 dark:bg-white/5"
>
  {#each options as opt (opt.value)}
    <button
      type="button"
      aria-pressed={opt.value === value}
      disabled={opt.disabled}
      use:tooltip={opt.title ?? ''}
      onclick={() => !opt.disabled && opt.value !== value && onchange(opt.value)}
      class="px-2 py-0.5 rounded-md text-[10px] font-semibold transition-colors disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:text-gray-500 dark:disabled:hover:text-gray-400 {opt.value === value
        ? 'bg-white dark:bg-lerd-card text-gray-800 dark:text-gray-100 shadow-xs'
        : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'}"
    >{opt.label}</button>
  {/each}
</div>
