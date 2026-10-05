<script lang="ts">
  import { onMount } from 'svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import { debugbarSettings, loadDebugbarSettings, saveDebugbarSettings, type DebugbarSettings } from '$stores/debugbar';
  import { m } from '../paraglide/messages.js';

  // How the debug bar looks on the sites that show it. Which sites do is set
  // per site, in its Debug tab.
  let { disabled = false }: { disabled?: boolean } = $props();
  onMount(loadDebugbarSettings);

  function set<K extends keyof DebugbarSettings>(key: K, value: DebugbarSettings[K]) {
    if ($debugbarSettings) void saveDebugbarSettings({ ...$debugbarSettings, [key]: value });
  }
</script>

{#if $debugbarSettings}
  {@const s = $debugbarSettings}
  <div class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 text-xs text-gray-600 dark:text-gray-300">
    <span>{m.debugbar_settings_style()}</span>
    <SegmentedControl label={m.debugbar_settings_style()} value={s.style} onchange={(v) => set('style', v)} options={[{ value: 'dock', label: m.debugbar_settings_dock(), disabled }, { value: 'compact', label: m.debugbar_settings_compact(), disabled }]} />
    <span class={s.style === 'compact' ? '' : 'opacity-50'}>{m.debugbar_settings_edge()}</span>
    <SegmentedControl label={m.debugbar_settings_edge()} value={s.edge} onchange={(v) => set('edge', v)} options={[{ value: 'bottom', label: m.debugbar_settings_bottom(), disabled: disabled || s.style !== 'compact' }, { value: 'top', label: m.debugbar_settings_top(), disabled: disabled || s.style !== 'compact' }]} />
    <span>{m.debugbar_settings_corner()}</span>
    <SegmentedControl label={m.debugbar_settings_corner()} value={s.corner} onchange={(v) => set('corner', v)} options={[{ value: 'top-left', label: '↖', title: m.debugbar_settings_topLeft(), disabled }, { value: 'top-right', label: '↗', title: m.debugbar_settings_topRight(), disabled }, { value: 'bottom-left', label: '↙', title: m.debugbar_settings_bottomLeft(), disabled }, { value: 'bottom-right', label: '↘', title: m.debugbar_settings_bottomRight(), disabled }]} />
    <span>{m.debugbar_settings_theme()}</span>
    <SegmentedControl label={m.debugbar_settings_theme()} value={s.theme} onchange={(v) => set('theme', v)} options={[{ value: 'auto', label: m.debugbar_settings_system(), disabled }, { value: 'light', label: m.debugbar_settings_light(), disabled }, { value: 'dark', label: m.debugbar_settings_dark(), disabled }]} />
  </div>
{/if}
