<script lang="ts">
  import { dashboardIconSvg } from '$lib/dashboardIcons';
  import { tintFor, type CategoryKey } from '$lib/presetCategories';

  interface Props {
    name: string;
    category?: CategoryKey;
    compact?: boolean;
  }
  let { name, category, compact = false }: Props = $props();

  const tint = $derived(tintFor(name, category));
  const box = $derived(compact ? 'w-8 h-8' : 'w-9 h-9 transition-transform group-hover:scale-105');
  const glyph = $derived(compact ? 'w-4 h-4' : 'w-5 h-5');
</script>

<span class="shrink-0 inline-flex items-center justify-center rounded-lg {tint} {box}">
  <svg class={glyph} fill="none" stroke="currentColor" viewBox="0 0 24 24">{@html dashboardIconSvg(name)}</svg>
</span>
