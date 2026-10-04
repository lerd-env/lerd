<script lang="ts">
  // A row of headline numbers at the top of a request tab, each with an
  // optional colour dot tying it to the timeline layer it counts.

  interface Props {
    stats: Array<{ label: string; value: string | number; dot?: string; tone?: string }>;
  }
  let { stats }: Props = $props();
</script>

<div class="flex flex-wrap rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card overflow-hidden">
  {#each stats as s (s.label)}
    <div class="px-4 py-2.5 border-r border-gray-100 dark:border-lerd-border/60 last:border-r-0 min-w-[6.5rem]">
      <div class="text-lg leading-tight tabular-nums {s.tone ?? 'text-sky-600 dark:text-sky-300'}">{s.value}</div>
      <div class="flex items-center gap-1 text-[10px] uppercase tracking-wide text-gray-500 dark:text-gray-400">{#if s.dot}<span class="w-1.5 h-1.5 rounded-full {s.dot}"></span>{/if}{s.label}</div>
    </div>
  {/each}
</div>
