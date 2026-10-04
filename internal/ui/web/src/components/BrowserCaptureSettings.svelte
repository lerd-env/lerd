<script lang="ts">
  import { onMount } from 'svelte';
  import LensToggle from './LensToggle.svelte';
  import Toggle from './Toggle.svelte';
  import BrowserEventsEditor from './BrowserEventsEditor.svelte';
  import {
    loadSiteBrowserCapture,
    saveSiteBrowserCapture,
    loadBrowserCapturePresets,
    applyBrowserCapturePreset,
    type BrowserCaptureSettings,
    type BrowserCapturePreset
  } from '$stores/browserCapture';
  import { m } from '../paraglide/messages.js';

  // One site's browser capture settings. A switch or checkbox saves at once; the
  // route and events save on their own buttons, since a half-typed value is not
  // one to apply. Everything below the site switch is locked while it is off.

  interface Props {
    site: string;
  }
  let { site }: Props = $props();

  let s = $state<BrowserCaptureSettings | null>(null);
  let route = $state('');
  let presets = $state<BrowserCapturePreset[]>([]);
  // Added and detected presets are always shown, in the order lerd sends them
  // (detected first, then by label); the rest wait behind "Show n more".
  let showAllPresets = $state(false);
  const relevant = $derived(presets.filter((p) => p.applied || p.detected));
  const shownPresets = $derived(showAllPresets ? presets : relevant);
  const hiddenCount = $derived(presets.length - relevant.length);
  let error = $state('');
  let saving = $state(false);
  let showAdvanced = $state(false);

  const CARD = 'rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card p-3 flex flex-col gap-2';
  const CAPTION = 'text-[10px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400';
  const PILL_ON = 'border-emerald-500/40 bg-emerald-50 dark:bg-emerald-900/30 text-emerald-700 dark:text-emerald-300';
  const PILL_OFF = 'border-gray-300 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/5';

  onMount(async () => {
    try {
      s = await loadSiteBrowserCapture(site);
      route = s.route;
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
      route = s.route;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }

  async function applyPreset(name: string, add: boolean) {
    saving = true;
    error = '';
    try {
      presets = await applyBrowserCapturePreset(site, name, add);
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
  {@const locked = saving || !cur.enabled}
  <div class="text-xs max-h-[75vh] overflow-y-auto">
    <div class="flex items-center gap-3 px-5 py-3 border-b border-gray-100 dark:border-lerd-border bg-gray-50 dark:bg-white/[0.02]">
      <div class="flex-1 min-w-0 text-gray-500 dark:text-gray-400">{cur.source === 'lerd.yaml' ? m.browser_settings_savedProject() : m.browser_settings_savedRegistry()}</div>
      <span class="text-gray-500 dark:text-gray-400">{m.browser_settings_enabled()}</span>
      <Toggle on={cur.enabled} tone="emerald" disabled={saving} title={m.browser_settings_enabled()} onclick={() => save({ ...cur, enabled: !cur.enabled })} />
    </div>

    {#if !cur.enabled}
      <div class="mx-5 mt-4 flex items-center gap-3 rounded-md border border-amber-500/40 bg-amber-50 dark:bg-amber-900/20 px-3 py-2.5 text-amber-800 dark:text-amber-200">
        <span class="flex-1">{m.browser_settings_off()}</span>
        <button type="button" disabled={saving} onclick={() => save({ ...cur, enabled: true })} class="rounded-md border border-amber-500/50 px-3 py-1 hover:bg-amber-100 dark:hover:bg-amber-900/40 disabled:opacity-50">{m.browser_settings_turnOn()}</button>
      </div>
    {/if}

    <fieldset disabled={locked} class="px-5 py-4 space-y-6 {cur.enabled ? '' : 'opacity-50'}">
      <section class="space-y-2.5">
        <div class="flex flex-wrap items-baseline gap-x-2">
          <h3 class="text-xs font-semibold text-gray-800 dark:text-gray-100">{m.browser_settings_report()}</h3>
          <span class="text-gray-500 dark:text-gray-400">{m.browser_settings_reportHint()}</span>
        </div>
        <div class="grid gap-2.5 grid-cols-1 sm:grid-cols-3">
          <div class={CARD}>
            <span class={CAPTION}>{m.browser_settings_console()}</span>
            {#each ['error', 'warn'] as level (level)}
              <LensToggle label={`console.${level}`} checked={cur.console.includes(level)} disabled={locked} onchange={(v) => save({ ...cur, console: toggleIn(cur.console, level, v) })} />
            {/each}
          </div>
          <div class={CARD}>
            <span class={CAPTION}>{m.browser_settings_network()}</span>
            <div class="flex flex-wrap gap-1.5">
              {#each [['4xx', '4xx'], ['5xx', '5xx'], ['failed', m.browser_settings_noResponse()]] as [cls, label] (cls)}
                {@const on = cur.network.includes(cls)}
                <button type="button" aria-pressed={on} disabled={locked} onclick={() => save({ ...cur, network: toggleIn(cur.network, cls, !on) })} class="font-mono rounded-md border px-2.5 py-1 {on ? PILL_ON : PILL_OFF}">{label}</button>
              {/each}
            </div>
            <LensToggle label={m.browser_settings_resourcesLong()} checked={cur.resources} disabled={locked} onchange={(v) => save({ ...cur, resources: v })} />
          </div>
          <div class={CARD}>
            <span class={CAPTION}>{m.browser_group_page()}</span>
            <LensToggle label={m.browser_settings_navigationLong()} checked={cur.navigation} disabled={locked} onchange={(v) => save({ ...cur, navigation: v })} />
          </div>
        </div>
      </section>

      {#if presets.length}
        <section class="space-y-2.5">
          <div class="flex flex-wrap items-baseline gap-x-2">
            <h3 class="text-xs font-semibold text-gray-800 dark:text-gray-100">{m.browser_settings_presets()}</h3>
            <span class="text-gray-500 dark:text-gray-400">{m.browser_settings_presetsHint()}</span>
          </div>
          {#if shownPresets.length}
            <div class="grid gap-2.5 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
              {#each shownPresets as p (p.name)}
                <div class="{CARD} {p.applied ? 'border-emerald-500/40 bg-emerald-50/60 dark:bg-emerald-900/10' : ''}">
                  <div class="flex items-center gap-2">
                    <span class="font-semibold text-gray-800 dark:text-gray-100">{p.label}</span>
                    {#if p.detected}<span class="text-[10px] rounded-sm px-1.5 py-0.5 bg-gray-100 dark:bg-white/10 text-gray-500 dark:text-gray-400">{m.browser_preset_detected()}</span>{/if}
                    <button type="button" aria-pressed={p.applied} aria-label={`${p.applied ? m.browser_preset_added() : m.browser_preset_add()} ${p.label}`} disabled={locked} onclick={() => applyPreset(p.name, !p.applied)} class="ml-auto rounded-md border px-2.5 py-1 {p.applied ? PILL_ON : PILL_OFF}">{p.applied ? m.browser_preset_added() : m.browser_preset_add()}</button>
                  </div>
                  <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400 break-words">{contents(p)}</span>
                </div>
              {/each}
            </div>
          {/if}
          {#if hiddenCount > 0 || showAllPresets}
            <button type="button" class="text-gray-500 dark:text-gray-400 underline hover:text-gray-800 dark:hover:text-gray-100" onclick={() => (showAllPresets = !showAllPresets)}>
              {showAllPresets ? m.browser_presets_showLess() : m.browser_presets_showMore({ count: hiddenCount })}
            </button>
          {/if}
        </section>
      {/if}

      <BrowserEventsEditor saved={cur.events} {presets} saving={locked} onsave={(ev) => save({ ...cur, events: ev })} />

      <section class="border-t border-gray-200 dark:border-lerd-border pt-3 space-y-3">
        <button type="button" aria-expanded={showAdvanced} onclick={() => (showAdvanced = !showAdvanced)} class="flex items-center gap-1.5 text-xs font-semibold text-gray-800 dark:text-gray-100">
          <svg class="w-3 h-3 transition-transform {showAdvanced ? '' : '-rotate-90'}" fill="none" stroke="currentColor" stroke-width="2.5" viewBox="0 0 24 24"><path stroke-linecap="round" d="M6 9l6 6 6-6" /></svg>
          {m.browser_settings_advanced()}
        </button>
        {#if showAdvanced}
          <div class="grid gap-4 grid-cols-1 sm:grid-cols-2 items-end">
            <form class="space-y-1.5" onsubmit={(e) => { e.preventDefault(); void save({ ...cur, route }); }}>
              <label class="block text-gray-500 dark:text-gray-400" for="browser-capture-route">{m.browser_settings_routeHint()}</label>
              <div class="flex gap-2">
                <input id="browser-capture-route" class="flex-1 font-mono px-2 py-1 rounded-md border border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card" bind:value={route} disabled={locked} />
                {#if route !== cur.route}
                  <button type="submit" disabled={locked} class="rounded-md px-3 py-1 bg-emerald-600 text-white hover:bg-emerald-700 disabled:opacity-50">{m.common_save()}</button>
                {/if}
              </div>
            </form>
            <div class="pb-1.5"><LensToggle label={m.browser_settings_verbose()} checked={cur.verbose} disabled={locked} onchange={(v) => save({ ...cur, verbose: v })} /></div>
          </div>
        {/if}
      </section>

      {#if error}<p class="text-rose-600 dark:text-rose-400 break-all">{error}</p>{/if}
    </fieldset>
  </div>
{:else if error}
  <p class="px-3 py-2 text-xs text-rose-600 dark:text-rose-400 break-all">{error}</p>
{/if}
