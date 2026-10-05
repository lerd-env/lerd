<script lang="ts">
  import { traceRows, traceRuns, funcLabel, traceText, traceCodePref, saveTraceCodePref, type TraceFrame } from '$lib/traceFrames';
  import { loadSource, type SourceLine } from '$lib/sourceCode';
  import SourcePath from './SourcePath.svelte';
  import CopyButton from './CopyButton.svelte';
  import Icon from './Icon.svelte';
  import { openInEditor } from '$lib/editor';
  import { tooltip } from '$lib/tooltip';
  import { editorTitle } from '$stores/editors';
  import { m } from '../paraglide/messages.js';

  // A stack trace as the path from the line that triggered an event back to
  // the entry point: app frames are stops, vendor runs fold into one, and the
  // code around the picked stop shows beside it.
  interface Props {
    trace: TraceFrame[];
    // start is the frame it opens on, the line that triggered the event.
    start?: number;
    showCode?: boolean;
  }
  let { trace, start = 0, showCode = $bindable(traceCodePref()) }: Props = $props();

  function toggleCode() {
    showCode = !showCode;
    saveTraceCodePref(showCode);
  }

  const rows = $derived(traceRows(trace));
  const runs = $derived(traceRuns(rows));
  // svelte-ignore state_referenced_locally
  let picked = $state(start);
  // The run holding the frame it opens on starts unfolded, so that frame shows
  // when the trace has no app frame to open on.
  // svelte-ignore state_referenced_locally
  let unfolded = $state<Record<number, boolean>>(Object.fromEntries(traceRuns(traceRows(trace)).flatMap((r, k) => (!r.app && r.rows.some((x) => x.i === start) ? [[k, true]] : []))));
  let pathEl: HTMLElement | null = $state(null);

  const row = $derived(rows[picked]);
  let code = $state<SourceLine[] | null>(null);
  let missing = $state(false);
  $effect(() => {
    if (!showCode || !row?.file) return;
    const want = row;
    code = null;
    missing = false;
    loadSource(want.file, want.line).then(
      (lines) => rows[picked] === want && (code = lines),
      () => rows[picked] === want && (missing = true)
    );
  });

  // Up and down step through the stops that show, a folded run's included
  // once it is open.
  function step(e: KeyboardEvent) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    e.preventDefault();
    const shown = runs.flatMap((r, k) => (r.app || unfolded[k] ? r.rows.map((x) => x.i) : []));
    const at = shown.indexOf(picked);
    const next = shown[Math.max(0, Math.min(shown.length - 1, at + (e.key === 'ArrowDown' ? 1 : -1)))];
    if (next === undefined) return;
    picked = next;
    pathEl?.querySelector(`[data-i="${next}"]`)?.scrollIntoView?.({ block: 'nearest' });
  }
</script>

{#snippet stop(r: (typeof rows)[number], small: boolean)}
  <button
    type="button"
    role="option"
    aria-selected={picked === r.i}
    data-i={r.i}
    onclick={() => (picked = r.i)}
    class="block w-full min-w-0 text-left rounded-md px-2 {small ? 'py-0.5' : 'py-1 mb-1'} {picked === r.i ? 'bg-lerd-red/10' : 'hover:bg-gray-100 dark:hover:bg-white/5'}"
  >
    <span class="block truncate font-mono {small ? 'text-gray-500 dark:text-gray-400' : 'font-semibold text-gray-800 dark:text-gray-100'}" title={r.inside}>{funcLabel(r.inside)}</span>
    <span class="block truncate text-[11px]"><SourcePath file={r.file} line={r.line} muted={!r.app} short={!r.app} /></span>
  </button>
{/snippet}

<div class="flex flex-col min-h-0 max-h-[inherit] text-[11px]">
  <div class="flex items-center gap-2 px-3 py-2 border-b border-gray-100 dark:border-lerd-border/60 text-[10px] uppercase tracking-wide text-gray-400">
    <span class="flex-1">{m.trace_title({ count: trace.length })}</span>
    <button type="button" aria-pressed={showCode} title={m.trace_codeHint()} onclick={toggleCode} class="normal-case tracking-normal inline-flex items-center gap-1 rounded-md border border-gray-200 dark:border-lerd-border px-1.5 py-0.5 text-[11px] {showCode ? 'text-gray-800 dark:text-gray-100 bg-gray-100 dark:bg-white/10' : 'text-gray-500 dark:text-gray-400'}">
      <Icon name="code" class="w-3 h-3" />{m.trace_code()}
    </button>
    <CopyButton text={() => traceText(trace)} label={m.common_copy()} tone="faint" />
  </div>
  <div class="grid min-h-0 flex-1 {showCode ? 'grid-cols-[minmax(0,44%)_minmax(0,1fr)]' : 'grid-cols-1'}">
    <div bind:this={pathEl} role="listbox" tabindex="0" aria-label={m.trace_show()} onkeydown={step} class="min-h-0 overflow-y-auto overscroll-contain py-2 pr-2 focus:outline-none {showCode ? 'border-r border-gray-100 dark:border-lerd-border/60' : ''}">
      {#each runs as run, k (k)}
        <div class="relative grid grid-cols-[18px_minmax(0,1fr)] gap-1 pl-2">
          <span class="relative" aria-hidden="true">
            <span class="absolute left-[8px] w-0.5 bg-gray-200 dark:bg-lerd-border {k === 0 ? 'top-3' : 'top-0'} {k === runs.length - 1 ? 'h-3' : 'bottom-0'}"></span>
            {#if run.app}
              <span class="absolute left-[3px] top-[9px] w-3 h-3 rounded-full border-2 border-lerd-red {picked === run.rows[0].i ? 'bg-lerd-red' : 'bg-white dark:bg-lerd-card'}"></span>
            {:else}
              <span class="absolute left-[6px] top-[11px] w-1.5 h-1.5 rounded-full bg-gray-300 dark:bg-gray-600"></span>
            {/if}
          </span>
          <div class="min-w-0">
            {#if run.app}
              {@render stop(run.rows[0], false)}
            {:else}
              <button type="button" aria-expanded={!!unfolded[k]} onclick={() => (unfolded = { ...unfolded, [k]: !unfolded[k] })} class="inline-flex items-center gap-1 rounded-md px-2 py-1 mb-1 text-gray-500 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-100 hover:bg-gray-100 dark:hover:bg-white/5">
                {run.rows.length === 1 ? m.trace_vendorFrame() : m.trace_vendorFrames({ count: run.rows.length })}
                <Icon name="chevron" class="w-3 h-3 transition-transform {unfolded[k] ? '' : '-rotate-90'}" />
              </button>
              {#if unfolded[k]}
                <div class="mb-1">
                  {#each run.rows as r (r.i)}{@render stop(r, true)}{/each}
                </div>
              {/if}
            {/if}
          </div>
        </div>
      {/each}
    </div>
    {#if showCode && row}
      <div class="min-w-0 min-h-0 flex flex-col bg-gray-50 dark:bg-white/[0.03]">
        <div class="grid grid-cols-[minmax(0,1fr)_auto] gap-x-2 gap-y-0.5 items-center px-3 py-2 border-b border-gray-100 dark:border-lerd-border/60">
          <span class="truncate font-mono font-semibold text-gray-800 dark:text-gray-100" title={row.inside}>{funcLabel(row.inside)}</span>
          <span class="justify-self-end whitespace-nowrap rounded border border-gray-200 dark:border-lerd-border px-1 text-[10px] uppercase tracking-wide text-gray-400">{row.pkg || 'app'}</span>
          <span class="col-span-2 min-w-0 truncate [direction:rtl] text-left"><bdi class="[direction:ltr]"><SourcePath file={row.file} line={row.line} muted={!row.app} /></bdi></span>
        </div>
        <div class="min-h-0 flex-1 overflow-auto">
          {#if code}
            <pre class="w-max min-w-full py-2 font-mono text-[12px] leading-[1.7] [font-variant-ligatures:none]">{#each code as l (l.n)}<div class="px-3 {l.n === row.line ? 'bg-lerd-red/10 shadow-[inset_3px_0_0_var(--color-lerd-red)]' : ''}"><button type="button" onclick={() => openInEditor(row.file, l.n)} use:tooltip={$editorTitle(row.file)} class="inline-block w-[3ch] mr-3.5 text-right text-gray-400 select-none hover:text-lerd-red hover:underline">{l.n}</button>{#if l.html}{@html l.html}{:else}{l.text}{/if}</div>{/each}</pre>
          {:else if missing}
            <p class="px-3 py-4 text-gray-500 dark:text-gray-400">{m.trace_noSource()}</p>
          {:else}
            <p class="px-3 py-4 text-gray-400">…</p>
          {/if}
        </div>
      </div>
    {/if}
  </div>
</div>
