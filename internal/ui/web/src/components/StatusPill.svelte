<script lang="ts" module>
  // 'asleep' is anything idle-suspend stopped: it leads with the moon, not a dot.
  export type PillTone = 'ok' | 'error' | 'warn' | 'muted' | 'asleep';
  // 'sm' is the compact pill dense settings rows use next to a title.
  export type PillSize = 'sm' | 'md';
</script>

<script lang="ts">
  import MoonMark from '$components/MoonMark.svelte';

  interface Props {
    tone: PillTone;
    label: string;
    title?: string;
    size?: PillSize;
    onclick?: () => void;
  }
  let { tone, label, title, size = 'md', onclick }: Props = $props();

  const sizeClass: Record<PillSize, string> = {
    sm: 'text-[10px] px-2 py-0.5',
    md: 'text-xs px-2.5 py-1'
  };

  const toneClass: Record<PillTone, string> = {
    ok: 'bg-emerald-100 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-500',
    error: 'bg-red-100 dark:bg-red-500/10 text-red-600 dark:text-red-400',
    warn: 'bg-yellow-100 dark:bg-yellow-500/10 text-yellow-700 dark:text-yellow-400',
    muted: 'bg-gray-100 dark:bg-white/5 text-gray-600 dark:text-gray-400',
    asleep: 'bg-sky-100 dark:bg-sky-500/10 text-sky-700 dark:text-sky-400'
  };

  const dotClass: Record<PillTone, string> = {
    ok: 'bg-emerald-500',
    error: 'bg-red-500',
    warn: 'bg-yellow-500',
    muted: 'bg-gray-400',
    asleep: ''
  };

  const pillClass = $derived(
    `inline-flex items-center gap-1.5 font-medium rounded-full ${sizeClass[size]} ${toneClass[tone]}`
  );
</script>

{#if onclick}
  <button
    type="button"
    {title}
    aria-label={title}
    {onclick}
    class="{pillClass} tabular-nums cursor-pointer hover:brightness-95 dark:hover:brightness-125"
  >
    {@render mark()}{label}
  </button>
{:else}
  <span {title} class={pillClass}>
    {@render mark()}{label}
  </span>
{/if}

{#snippet mark()}
  {#if tone === 'asleep'}
    <MoonMark class="w-3 h-3" />
  {:else}
    <span class="w-1.5 h-1.5 rounded-full {dotClass[tone]}"></span>
  {/if}
{/snippet}
