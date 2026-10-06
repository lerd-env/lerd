<script lang="ts">
  import type { BrowserCaptureEvent } from '$stores/browserCapture';
  import DetailButton from '$components/DetailButton.svelte';
  import Icon from '$components/Icon.svelte';
  import { closeModal } from '$stores/modals';
  import { m } from '../paraglide/messages.js';

  // The DOM events a site reports, edited the way a site's domains are: a row
  // per event with edit and remove, and an add row at the bottom. Every change
  // saves at once, checked against the rules lerd applies on save.

  interface Props {
    saved: BrowserCaptureEvent[];
    saving: boolean;
    onsave: (events: BrowserCaptureEvent[]) => void;
  }
  let { saved, saving, onsave }: Props = $props();

  const EVENT_RE = /^[A-Za-z][A-Za-z0-9:._-]{0,99}$/;
  const PATH_RE = /^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)*$/;
  const valid = (e: BrowserCaptureEvent) => EVENT_RE.test(e.event) && (e.message === '' || PATH_RE.test(e.message));
  const typedBad = (e: BrowserCaptureEvent) =>
    (e.event !== '' && !EVENT_RE.test(e.event)) || (e.message !== '' && !PATH_RE.test(e.message));

  let added = $state<BrowserCaptureEvent>({ event: '', label: '', message: '' });
  let editIndex = $state(-1);
  let draft = $state<BrowserCaptureEvent>({ event: '', label: '', message: '' });
  $effect(() => {
    void saved;
    editIndex = -1;
  });

  function add() {
    const ev = { event: added.event.trim(), label: '', message: added.message.trim() };
    if (!valid(ev)) return;
    onsave([...saved, ev]);
    added = { event: '', label: '', message: '' };
  }
  function startEdit(i: number) {
    editIndex = i;
    draft = { ...saved[i] };
  }
  function saveEdit() {
    if (!valid(draft)) return;
    onsave(saved.map((e, i) => (i === editIndex ? { ...draft } : e)));
  }
  function onkey(e: KeyboardEvent, enter: () => void, escape?: () => void) {
    if (e.key === 'Enter') enter();
    if (e.key === 'Escape' && escape) escape();
  }

  const INPUT = 'min-w-0 text-sm font-mono bg-transparent border rounded-sm px-2 py-1 text-gray-700 dark:text-gray-300 placeholder-gray-400 dark:placeholder-gray-600 focus:outline-hidden';
  const OK = 'border-gray-200 dark:border-lerd-border focus:border-lerd-red/50';
  const BAD = 'border-rose-500';
</script>

<section class="space-y-2">
  <div class="flex flex-wrap items-baseline gap-x-2">
    <h3 class="text-xs font-semibold text-gray-800 dark:text-gray-100">{m.browser_settings_events()}</h3>
    <a href="#docs/features/browser-capture" onclick={closeModal} class="ml-auto text-xs underline text-gray-500 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-100">{m.browser_events_examples()}</a>
  </div>
  <p class="text-xs text-gray-500 dark:text-gray-400">{m.browser_events_help()}</p>

  {#each saved as ev, i (ev.event + ':' + i)}
    {#if editIndex !== i}
      <div class="flex items-center gap-2">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-1.5">
            <span class="text-sm font-mono text-gray-700 dark:text-gray-300 truncate">{ev.event}</span>
          </div>
          {#if ev.label || ev.message}
            <div class="text-xs text-gray-500 dark:text-gray-400 truncate">{ev.label}{ev.label && ev.message ? ' · ' : ''}<span class="font-mono">{ev.message}</span></div>
          {/if}
        </div>
        <button onclick={() => startEdit(i)} disabled={saving} class="text-gray-400 hover:text-lerd-red transition-colors disabled:opacity-50" title={m.common_edit()} aria-label="{m.common_edit()} {ev.event}">
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
          </svg>
        </button>
        <button onclick={() => onsave(saved.filter((_, j) => j !== i))} disabled={saving} class="text-gray-400 hover:text-red-500 transition-colors disabled:opacity-50" title={m.common_remove()} aria-label="{m.common_remove()} {ev.event}">
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
        </button>
      </div>
    {:else}
      {@const bad = typedBad(draft)}
      <div class="flex items-center gap-2">
        <div class="flex-1 min-w-0 grid grid-cols-3 gap-1.5">
          <input aria-label={m.browser_settings_eventName()} bind:value={draft.event} disabled={saving} onkeydown={(e) => onkey(e, saveEdit, () => (editIndex = -1))} class="{INPUT} {draft.event !== '' && !EVENT_RE.test(draft.event) ? BAD : OK}" />
          <input aria-label={m.browser_settings_eventLabel()} placeholder={m.browser_events_labelHint()} bind:value={draft.label} disabled={saving} onkeydown={(e) => onkey(e, saveEdit, () => (editIndex = -1))} class="{INPUT} font-sans {OK}" />
          <input aria-label={m.browser_settings_eventMessage()} placeholder="detail.message" bind:value={draft.message} disabled={saving} onkeydown={(e) => onkey(e, saveEdit, () => (editIndex = -1))} class="{INPUT} {draft.message !== '' && !PATH_RE.test(draft.message) ? BAD : OK}" />
        </div>
        <button onclick={saveEdit} disabled={saving || bad || draft.event === ''} class="text-emerald-500 hover:text-emerald-600 disabled:opacity-50" title={m.common_save()} aria-label={m.common_save()}>
          <Icon name="check" class="w-4 h-4" />
        </button>
        <button onclick={() => (editIndex = -1)} class="text-gray-400 hover:text-gray-600" title={m.common_cancel()} aria-label={m.common_cancel()}>
          <Icon name="close" class="w-4 h-4" />
        </button>
      </div>
    {/if}
  {/each}

  <div class="flex items-center gap-2 pt-1">
    <input aria-label={m.browser_settings_eventName()} placeholder={m.browser_settings_addEvent()} bind:value={added.event} disabled={saving} onkeydown={(e) => onkey(e, add)} class="flex-1 {INPUT} py-1.5 {added.event !== '' && !EVENT_RE.test(added.event.trim()) ? BAD : OK}" />
    <input aria-label={m.browser_settings_eventMessage()} placeholder="detail.message" bind:value={added.message} disabled={saving} onkeydown={(e) => onkey(e, add)} class="w-36 {INPUT} py-1.5 {added.message !== '' && !PATH_RE.test(added.message.trim()) ? BAD : OK}" />
    <DetailButton tone="primary" onclick={add} disabled={saving || !valid({ event: added.event.trim(), label: '', message: added.message.trim() })}>{m.common_add()}</DetailButton>
  </div>
  {#if typedBad({ event: added.event.trim(), label: '', message: added.message.trim() }) || (editIndex !== -1 && typedBad(draft))}
    <p class="text-xs text-rose-600 dark:text-rose-400">{m.browser_events_invalid()}</p>
  {/if}
</section>
