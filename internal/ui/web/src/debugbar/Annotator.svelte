<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { apiFetch, apiJson } from '$lib/api';
  import { selectorFor, describeElement, notePlace } from './annotate';
  import { m } from '../paraglide/messages.js';

  // Notes pinned to elements of this page: the picker that makes one, the
  // markers that show the open ones where their elements are, and the box
  // that reads, edits, resolves or deletes one.
  export interface Note {
    id: string;
    url: string;
    selector: string;
    comment: string;
    tag?: string;
    text?: string;
  }
  interface Props {
    rid: string;
    picking: boolean;
    // The page's open notes, for the bar's list of them.
    list?: Note[];
  }
  let { rid, picking = $bindable(), list = $bindable([]) }: Props = $props();

  let notes = $state<Note[]>([]);
  const here = (n: Note) => {
    try {
      return new URL(n.url).pathname === location.pathname;
    } catch {
      return false;
    }
  };
  const pageNotes = $derived(notes.filter(here));
  $effect(() => {
    list = pageNotes;
  });

  async function load() {
    try {
      notes = await apiJson<Note[]>('/api/annotations');
    } catch {
      /* not reachable from here: no markers */
    }
  }

  // ponytail: polls with the bar so a note an assistant resolves leaves the page; a stream if it ever matters.
  let timer: ReturnType<typeof setInterval> | undefined;
  onMount(() => {
    void load();
    timer = setInterval(() => document.visibilityState === 'visible' && load(), 4000);
    addEventListener('scroll', place, { passive: true, capture: true });
    addEventListener('resize', place);
  });
  onDestroy(() => {
    clearInterval(timer);
    stopPicking();
    removeEventListener('scroll', place, { capture: true });
    removeEventListener('resize', place);
  });

  // The picker: what the pointer is over, outlined, until a click picks it.
  let hover = $state<{ el: Element; r: DOMRect } | null>(null);
  let target = $state<Element | null>(null);
  const ours = (e: Event) => e.composedPath().some((n) => (n as Element).tagName === 'LERD-DEBUGBAR');
  function over(e: PointerEvent) {
    if (target || ours(e)) return void (hover = null);
    const el = document.elementFromPoint(e.clientX, e.clientY);
    hover = el && el !== document.documentElement && el !== document.body ? { el, r: el.getBoundingClientRect() } : null;
  }
  // The page never sees the press that picks, so a link or a router does not move on.
  function swallow(e: Event) {
    if (!picking || ours(e)) return;
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
    if (e.type === 'click' && hover && !target) pick(hover.el);
  }
  const PRESS = ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click'] as const;
  function startPicking() {
    addEventListener('pointermove', over, true);
    for (const t of PRESS) addEventListener(t, swallow, true);
    document.documentElement.style.cursor = 'crosshair';
  }
  function stopPicking() {
    removeEventListener('pointermove', over, true);
    for (const t of PRESS) removeEventListener(t, swallow, true);
    document.documentElement.style.cursor = '';
    hover = null;
  }
  $effect(() => {
    if (picking) startPicking();
    else {
      stopPicking();
      target = null;
    }
  });

  let draft = $state('');
  let box = $state<{ left: number; top: number } | null>(null);
  let saving = $state(false);
  let error = $state('');
  function pick(el: Element) {
    target = el;
    hover = { el, r: el.getBoundingClientRect() };
    draft = '';
    error = '';
    box = notePlace(hover.r, innerWidth, innerHeight);
  }
  function cancel() {
    target = null;
    box = null;
    picking = false;
  }

  async function save() {
    if (!target || !draft.trim() || saving) return;
    saving = true;
    error = '';
    const el = target;
    const info = describeElement(el);
    try {
      const res = await apiFetch('/api/annotations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: location.href, title: document.title, rid, comment: draft.trim(), selector: selectorFor(el), tag: info.tag, text: info.text, rect: info.rect, viewport: { x: scrollX, y: scrollY, w: innerWidth, h: innerHeight } })
      });
      if (!res.ok) throw new Error(await res.text());
      await load();
      cancel();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      saving = false;
    }
  }

  // Markers: a numbered pin at the top right of each open note's element,
  // kept on it as the page scrolls and reflows.
  let pins = $state<{ note: Note; n: number; x: number; y: number }[]>([]);
  let frame = 0;
  function place() {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(() => {
      // Several notes on one element line up beside each other.
      const on = new Map<Element, number>();
      pins = pageNotes.flatMap((note, i) => {
        let el: Element | null = null;
        try {
          el = document.querySelector(note.selector);
        } catch {}
        if (!el) return [];
        const r = el.getBoundingClientRect();
        if (!r.width && !r.height) return [];
        const k = on.get(el) ?? 0;
        on.set(el, k + 1);
        return [{ note, n: i + 1, x: Math.min(r.right, innerWidth - 14) - k * 26, y: Math.max(r.top, 14) }];
      });
      if (hover) hover = { el: hover.el, r: hover.el.getBoundingClientRect() };
    });
  }
  $effect(() => {
    void pageNotes;
    place();
  });

  // The open note: its comment, edited in place, or resolved or deleted.
  let openNote = $state<{ note: Note; left: number; top: number } | null>(null);
  let edit = $state('');
  function show(e: MouseEvent, note: Note) {
    e.stopPropagation();
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    edit = note.comment;
    openNote = { note, ...notePlace(r, innerWidth, innerHeight) };
  }

  // reveal brings a note's element into view and opens the note beside it,
  // or in the middle of the screen when the element is no longer on the page.
  export function reveal(note: Note) {
    let el: Element | null = null;
    try {
      el = document.querySelector(note.selector);
    } catch {}
    edit = note.comment;
    error = '';
    if (!el) {
      openNote = { note, left: Math.max(8, innerWidth / 2 - 160), top: Math.max(8, innerHeight / 2 - 100) };
      return;
    }
    el.scrollIntoView({ block: 'center', behavior: 'instant' as ScrollBehavior });
    openNote = { note, ...notePlace(el.getBoundingClientRect(), innerWidth, innerHeight) };
  }
  async function change(note: Note, init: RequestInit) {
    try {
      const res = await apiFetch(`/api/annotations/${note.id}`, init);
      if (!res.ok) throw new Error(await res.text());
      openNote = null;
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }
  // autofocus does not reach into a shadow root, so the box takes focus itself.
  const focus = (el: HTMLElement) => void setTimeout(() => el.focus());
  const post = (body: object): RequestInit => ({ method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  function keys(e: KeyboardEvent, submit: () => void, close: () => void) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submit();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      close();
    }
  }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && picking && !target && cancel()} onclick={() => (openNote = null)} />

{#if picking && hover}
  <div class="pick-box" class:locked={!!target} style="left:{hover.r.left}px;top:{hover.r.top}px;width:{hover.r.width}px;height:{hover.r.height}px">
    <span class="pick-tag">{hover.el.tagName.toLowerCase()}</span>
  </div>
{/if}

{#each pins as p (p.note.id)}
  <button class="pin" type="button" style="left:{p.x}px;top:{p.y}px" aria-label={m.debugbar_noteOpen({ n: p.n })} onclick={(e) => show(e, p.note)}>{p.n}</button>
{/each}

{#if target && box}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="note-box" style="left:{box.left}px;top:{box.top}px" onclick={(e) => e.stopPropagation()}>
    <h4>{m.debugbar_noteNew()}</h4>
    <textarea bind:value={draft} rows="4" placeholder={m.debugbar_notePlaceholder()} use:focus onkeydown={(e) => keys(e, save, cancel)}></textarea>
    {#if error}<p class="note-err">{error}</p>{/if}
    <div class="note-actions">
      <span class="note-hint">{m.debugbar_noteHint()}</span>
      <button type="button" class="note-btn" onclick={cancel}>{m.debugbar_noteCancel()}</button>
      <button type="button" class="note-btn primary" disabled={!draft.trim() || saving} onclick={save}>{saving ? m.debugbar_noteSaving() : m.debugbar_noteSave()}</button>
    </div>
  </div>
{/if}

{#if openNote}
  {@const note = openNote.note}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="note-box" style="left:{openNote.left}px;top:{openNote.top}px" onclick={(e) => e.stopPropagation()}>
    <h4>{note.tag ?? m.debugbar_noteTitle()}{#if note.text}<span class="note-text">&nbsp;· {note.text}</span>{/if}</h4>
    <textarea bind:value={edit} rows="4" use:focus onkeydown={(e) => keys(e, () => change(note, post({ comment: edit })), () => (openNote = null))}></textarea>
    {#if error}<p class="note-err">{error}</p>{/if}
    <div class="note-actions">
      <button type="button" class="note-btn danger" onclick={() => change(note, { method: 'DELETE' })}>{m.debugbar_noteDelete()}</button>
      <button type="button" class="note-btn" onclick={() => change(note, post({ status: 'resolved' }))}>{m.debugbar_noteResolve()}</button>
      <button type="button" class="note-btn primary" disabled={!edit.trim() || edit === note.comment} onclick={() => change(note, post({ comment: edit }))}>{m.debugbar_noteSave()}</button>
    </div>
  </div>
{/if}
