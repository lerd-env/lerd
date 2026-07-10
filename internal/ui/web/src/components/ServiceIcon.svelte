<script lang="ts">
  import { dashboardIconSvg } from '$lib/dashboardIcons';
  import { serviceMeta } from '$lib/serviceMeta';
  import { tintFor, asCategory } from '$lib/presetCategories';

  interface Props {
    name: string;
    category?: string;
    icon?: string;
    compact?: boolean;
  }
  let { name, category, icon, compact = false }: Props = $props();

  // A caller holding the preset or service passes its declared values; one
  // holding only a name resolves them through the registry.
  const meta = $derived($serviceMeta.get(name));
  const resolvedCategory = $derived(asCategory(category ?? meta?.category));
  const resolvedIcon = $derived(icon ?? meta?.icon);

  const tint = $derived(tintFor(resolvedCategory));
  const box = $derived(compact ? 'w-8 h-8' : 'w-9 h-9 transition-transform group-hover:scale-105');
  const glyph = $derived(compact ? 'w-4 h-4' : 'w-5 h-5');
</script>

<span class="shrink-0 inline-flex items-center justify-center rounded-lg {tint} {box}">
  <svg class={glyph} fill="none" stroke="currentColor" viewBox="0 0 24 24"
    >{@html dashboardIconSvg(name, resolvedIcon)}</svg
  >
</span>
