<script lang="ts">
  import Dropdown from './Dropdown.svelte';
  import { palette, palettes } from '$stores/theme';
  import type { Palette } from '$lib/palettes';
  import { m } from '../paraglide/messages.js';

  // The two lerd themes are named by a word the interface can translate; the
  // classic schemes and a user's own file carry proper names that stay as they
  // are in every language.
  const LOCALISED: Record<string, () => string> = {
    lerd: m.theme_palette_lerd,
    muted: m.theme_palette_muted
  };
  const label = (p: Palette) => LOCALISED[p.id]?.() ?? p.name;

  const options = $derived($palettes.map((p) => ({ value: p.id, label: label(p) })));
  const swatches = $derived(
    Object.fromEntries($palettes.map((p) => [p.id, `--swatch:${p.accent};--swatch-dark:${p.accentDark}`]))
  );
</script>

<Dropdown value={$palette} {options} onchange={(v) => palette.set(v)}>
  {#snippet optionIcon(value: string)}
    <span
      class="palette-swatch w-3 h-3 rounded-full shrink-0 border border-black/10 dark:border-white/15"
      style={swatches[value] || ''}
    ></span>
  {/snippet}
</Dropdown>
