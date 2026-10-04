<script lang="ts">
  import type { BrowserCaptureEvent, BrowserCapturePreset } from '$stores/browserCapture';
  import { m } from '../paraglide/messages.js';

  // The DOM events a site reports, as a table. A row reads until it is edited;
  // an edit or a new row is checked as it is typed against the rules lerd
  // applies on save, and removing a row saves at once.

  interface Props {
    saved: BrowserCaptureEvent[];
    presets: BrowserCapturePreset[];
    saving: boolean;
    onsave: (events: BrowserCaptureEvent[]) => void;
  }
  let { saved, presets, saving, onsave }: Props = $props();

  let editing = $state<number | null>(null);
  let draft = $state<BrowserCaptureEvent>({ event: '', label: '', message: '' });
  $effect(() => {
    void saved;
    editing = null;
  });

  const EVENT_RE = /^[A-Za-z][A-Za-z0-9:._-]{0,99}$/;
  const PATH_RE = /^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)*$/;
  const badEvent = $derived(draft.event !== '' && !EVENT_RE.test(draft.event));
  const badPath = $derived(draft.message !== '' && !PATH_RE.test(draft.message));
  const canSave = $derived(draft.event !== '' && !badEvent && !badPath);

  // Which added preset an event came from, so a row says why it is there.
  function presetOf(event: string): string {
    return presets.find((p) => p.applied && p.events.some((e) => e.event === event))?.label ?? '';
  }

  function edit(i: number) {
    editing = i;
    draft = { ...(saved[i] ?? { event: '', label: '', message: '' }) };
  }
  function commit() {
    const next = saved.slice();
    if (editing === null || editing >= saved.length) next.push({ ...draft });
    else next[editing] = { ...draft };
    onsave(next);
  }

  const INPUT = 'w-full px-2 py-1 rounded-md border bg-white dark:bg-lerd-bg disabled:opacity-50';
  const OK = 'border-gray-300 dark:border-lerd-border';
  const BAD = 'border-rose-500 dark:border-rose-500';
  const COLS = 'grid grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1.2fr)_3.5rem] gap-3 items-center px-3';
</script>

<section class="space-y-2.5">
  <div class="flex flex-wrap items-baseline gap-x-2">
    <h3 class="text-xs font-semibold text-gray-800 dark:text-gray-100">{m.browser_settings_events()}</h3>
    <span class="text-gray-500 dark:text-gray-400">{m.browser_events_help()}</span>
    <a href="#docs/features/browser-capture" class="ml-auto underline text-gray-500 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-100">{m.browser_events_examples()}</a>
  </div>

  {#if saved.length || editing !== null}
    <div class="rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card overflow-hidden">
      <div class="{COLS} py-2 bg-gray-50 dark:bg-white/[0.03] text-[10px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
        <span>{m.browser_settings_eventName()}</span>
        <span>{m.browser_settings_eventLabel()}</span>
        <span>{m.browser_settings_eventMessage()}</span>
        <span></span>
      </div>
      {#each [...saved, ...(editing === saved.length ? [draft] : [])] as ev, i (i)}
        {#if editing === i}
          <form class="border-t border-gray-100 dark:border-lerd-border/60 bg-gray-50/60 dark:bg-white/[0.02] py-2.5 space-y-2" onsubmit={(e) => { e.preventDefault(); if (canSave) commit(); }}>
            <div class={COLS}>
              <input aria-label={m.browser_settings_eventName()} aria-invalid={badEvent} placeholder="my-app:error" class="{INPUT} font-mono {badEvent ? BAD : OK}" bind:value={draft.event} disabled={saving} />
              <input aria-label={m.browser_settings_eventLabel()} placeholder={m.browser_events_labelHint()} class="{INPUT} {OK}" bind:value={draft.label} disabled={saving} />
              <input aria-label={m.browser_settings_eventMessage()} aria-invalid={badPath} placeholder="detail.message" class="{INPUT} font-mono {badPath ? BAD : OK}" bind:value={draft.message} disabled={saving} />
              <span></span>
            </div>
            <div class="flex items-center justify-end gap-2 px-3">
              {#if badEvent || badPath}<span class="mr-auto text-rose-600 dark:text-rose-400">{m.browser_events_invalid()}</span>{/if}
              <button type="button" class="rounded-md px-3 py-1 text-gray-500 hover:bg-gray-100 dark:hover:bg-white/5" disabled={saving} onclick={() => (editing = null)}>{m.common_cancel()}</button>
              <button type="submit" class="rounded-md px-3 py-1 bg-emerald-600 text-white hover:bg-emerald-700 disabled:opacity-50" disabled={saving || !canSave}>{m.common_save()}</button>
            </div>
          </form>
        {:else}
          {@const from = presetOf(ev.event)}
          <div class="{COLS} py-2 border-t border-gray-100 dark:border-lerd-border/60">
            <span class="flex items-center gap-2 min-w-0">
              <span class="font-mono truncate text-gray-800 dark:text-gray-100">{ev.event}</span>
              {#if from}<span class="shrink-0 text-[10px] rounded-sm px-1.5 py-0.5 bg-gray-100 dark:bg-white/10 text-gray-500 dark:text-gray-400">{from}</span>{/if}
            </span>
            <span class="truncate text-gray-700 dark:text-gray-200">{ev.label || '—'}</span>
            <span class="font-mono truncate text-gray-500 dark:text-gray-400">{ev.message || '—'}</span>
            <span class="flex justify-end gap-0.5">
              <button type="button" aria-label={m.common_edit()} title={m.common_edit()} class={ICON} disabled={saving || editing !== null} onclick={() => edit(i)}>
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M4 20h4L19 9l-4-4L4 16v4z" /></svg>
              </button>
              <button type="button" aria-label={m.common_remove()} title={m.common_remove()} class="{ICON} hover:text-rose-600 dark:hover:text-rose-400" disabled={saving || editing !== null} onclick={() => onsave(saved.filter((_, j) => j !== i))}>
                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" d="M6 6l12 12M18 6L6 18" /></svg>
              </button>
            </span>
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  {#if editing === null}
    <button type="button" class="rounded-md border border-dashed border-gray-300 dark:border-lerd-border px-3 py-1 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/5" disabled={saving} onclick={() => edit(saved.length)}>+ {m.browser_settings_addEvent()}</button>
  {/if}
</section>

<script lang="ts" module>
  const ICON = 'w-6 h-6 flex items-center justify-center rounded-sm text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-white/5 disabled:opacity-40';
</script>
