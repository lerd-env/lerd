<script lang="ts">
  import { onMount } from 'svelte';
  import LensToggle from './LensToggle.svelte';
  import Toggle from './Toggle.svelte';
  import BrowserEventsEditor from './BrowserEventsEditor.svelte';
  import {
    loadSiteBrowserCapture,
    saveSiteBrowserCapture,
    loadBrowserCapturePresets,
    setBrowserCapturePreset,
    type BrowserCaptureSettings,
    type BrowserCapturePreset
  } from '$stores/browserCapture';
  import { m } from '../paraglide/messages.js';

  // One site's browser capture settings. A switch or checkbox saves at once;
  // events save on their own button, since a half-typed value is not one to
  // apply. Options stay editable while the site is off, so they can be
  // picked before turning it on.

  interface Props {
    site: string;
  }
  let { site }: Props = $props();

  let s = $state<BrowserCaptureSettings | null>(null);
  let presets = $state<BrowserCapturePreset[]>([]);
  // Only the presets of libraries the project uses, plus any the site switched
  // on by hand; the CLI and MCP still list every preset.
  const shown = $derived(presets.filter((p) => p.detected || p.active));
  let error = $state('');
  let saving = $state(false);

  const SECTION = 'px-5 py-4 space-y-2 border-t border-gray-100 dark:border-lerd-border';
  const HEADING = 'text-xs font-semibold text-gray-800 dark:text-gray-100';
  const ROW = 'flex flex-col gap-1.5';
  const CAPTION = 'text-[10px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400';
  const PILL_ON = 'border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300';
  const PILL_OFF = 'border-gray-300 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/5';

  onMount(async () => {
    try {
      s = await loadSiteBrowserCapture(site);
      presets = await loadBrowserCapturePresets(site);
    } catch (e) {
      error = String(e);
    }
  });

  async function save(next: BrowserCaptureSettings) {
    saving = true;
    error = '';
    try {
      s = await saveSiteBrowserCapture(site, next);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }

  async function setPreset(name: string, on: boolean) {
    saving = true;
    error = '';
    try {
      presets = await setBrowserCapturePreset(site, name, on);
      s = await loadSiteBrowserCapture(site);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }

  // The events a preset adds, for its card.
  function contents(p: BrowserCapturePreset): string {
    return (p.events ?? []).map((e) => e.event).join(', ');
  }

  function toggleIn(list: string[], value: string, on: boolean): string[] {
    return on ? [...list.filter((v) => v !== value), value] : list.filter((v) => v !== value);
  }
</script>

{#if s}
  {@const cur = s}
  {@const locked = saving}
  <div class="max-h-[70vh] overflow-y-auto">
    <div class="px-5 py-3 flex items-center gap-3">
      <span class="flex-1 text-sm text-gray-700 dark:text-gray-300">{m.browser_settings_enabled()}</span>
      <Toggle on={cur.enabled} tone="emerald" disabled={saving} title={m.browser_settings_enabled()} onclick={() => save({ ...cur, enabled: !cur.enabled })} />
    </div>

    <fieldset disabled={locked}>
      <section class={SECTION}>
        <h3 class={HEADING}>{m.browser_settings_report()}</h3>
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.browser_settings_reportHint()}</p>
        <div class="space-y-2.5 pt-1">
          <div class={ROW}>
            <span class={CAPTION}>{m.browser_settings_console()}</span>
            <div class="flex flex-wrap gap-x-5 gap-y-1.5">
              {#each ['error', 'warn'] as level (level)}
                <LensToggle label={`console.${level}`} checked={cur.console.includes(level)} disabled={locked} onchange={(v) => save({ ...cur, console: toggleIn(cur.console, level, v) })} />
              {/each}
            </div>
          </div>
          <div class={ROW}>
            <span class={CAPTION}>{m.browser_settings_network()}</span>
            <div class="flex flex-wrap gap-1.5">
              {#each [['4xx', '4xx'], ['5xx', '5xx'], ['failed', m.browser_settings_noResponse()]] as [cls, label] (cls)}
                {@const on = cur.network.includes(cls)}
                <button type="button" aria-pressed={on} disabled={locked} onclick={() => save({ ...cur, network: toggleIn(cur.network, cls, !on) })} class="text-xs font-mono rounded-md border px-2.5 py-1 {on ? PILL_ON : PILL_OFF}">{label}</button>
              {/each}
            </div>
          </div>
          <div class={ROW}>
            <span class={CAPTION}>{m.browser_group_page()}</span>
            <LensToggle label={m.browser_settings_resourcesLong()} checked={cur.resources} disabled={locked} onchange={(v) => save({ ...cur, resources: v })} />
            <LensToggle label={m.browser_settings_navigationLong()} checked={cur.navigation} disabled={locked} onchange={(v) => save({ ...cur, navigation: v })} />
          </div>
        </div>
      </section>

      {#if shown.length}
        <section class={SECTION}>
          <h3 class={HEADING}>{m.browser_settings_presets()}</h3>
          <p class="text-xs text-gray-500 dark:text-gray-400">{m.browser_settings_presetsHint()}</p>
          <!-- Detected presets are on until switched off; the list scrolls rather than folding. -->
          <div class="max-h-44 overflow-y-auto -mx-1 px-1 space-y-2">
            {#each shown as p (p.name)}
              <label class="flex items-start gap-2 cursor-pointer select-none">
                <input type="checkbox" class="mt-0.5 rounded-sm border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card text-lerd-red focus:ring-lerd-red" checked={p.active} disabled={locked} onchange={(e) => setPreset(p.name, (e.currentTarget as HTMLInputElement).checked)} />
                <span class="flex-1 min-w-0">
                  <span class="flex items-center gap-1.5">
                    <span class="text-sm text-gray-700 dark:text-gray-300 truncate">{p.label}</span>
                  </span>
                  <span class="block text-xs font-mono text-gray-500 dark:text-gray-400 truncate">{contents(p)}</span>
                </span>
              </label>
            {/each}
          </div>
        </section>
      {/if}

      <div class={SECTION}>
        <BrowserEventsEditor saved={cur.events} saving={locked} onsave={(ev) => save({ ...cur, events: ev })} />
      </div>
    </fieldset>
  </div>
  {#if error}
    <div class="px-5 py-2 border-t border-gray-100 dark:border-lerd-border">
      <p class="text-xs text-red-500 break-all">{error}</p>
    </div>
  {/if}
{:else if error}
  <p class="px-5 py-4 text-xs text-red-500 break-all">{error}</p>
{/if}
