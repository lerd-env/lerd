<script lang="ts">
  import { tick, untrack } from 'svelte';
  import type { Site } from '$stores/sites';
  import {
    listAppLogFiles,
    loadAppLogEntries,
    clearAppLogs,
    mergeNewest,
    type AppLogFile,
    type AppLogEntry
  } from '$stores/appLogs';
  import Dropdown from '$components/Dropdown.svelte';
  import ActionButton from '$components/ActionButton.svelte';
  import LoadingRow from '$components/LoadingRow.svelte';
  import ClearAppLogsModal from './ClearAppLogsModal.svelte';
  import { openErrorModal } from '$stores/modals';
  import StructuredValue from '$components/StructuredValue.svelte';
  import { splitLogContext } from '$lib/structured';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
    branch?: string;
  }
  let { site, branch = '' }: Props = $props();

  // The sites store replaces every site object on each snapshot, so an effect
  // reading site.domain re-runs on a payload that changed nothing here and
  // reloads the list under the user. A derived stops at the value.
  const siteDomain = $derived(site.domain);
  const suspended = $derived(site.idle_suspended === true);

  let files = $state<AppLogFile[]>([]);
  let selectedFile = $state('');
  let entries = $state<AppLogEntry[]>([]);
  let loading = $state(false);
  // Older pages load as the reader scrolls up, so a large log never arrives
  // in one piece; more says whether the file has any left.
  let more = $state(false);
  let loadingOlder = $state(false);
  let search = $state('');
  let expandedIdx = $state(-1);
  // What an expanded entry shows below its context: the lines after the
  // message (a stack trace), or the whole entry when no context was split off.
  function restOf(entry: { message?: string; detail?: string }, split: boolean): string {
    return split ? (entry.detail ?? '').slice((entry.message ?? '').length).trim() : entry.detail || entry.message || '';
  }
  let scrollEl: HTMLDivElement | null = $state(null);
  // Clearing deletes real log files, so it goes through a confirmation modal
  // rather than an inline button: a deliberate confirm matches how lerd's other
  // destructive actions work and guards against wiping a lot of history with a
  // stray double click. The active log is recreated on the app's next write.
  let confirmOpen = $state(false);
  let clearing = $state(false);

  const totalBytes = $derived(files.reduce((sum, f) => sum + (f.size ?? 0), 0));

  function fmtBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  }

  async function doClear() {
    if (clearing) return;
    clearing = true;
    try {
      const r = await clearAppLogs(siteDomain, branch);
      if (!r.ok) {
        // The confirmation closes first, or the failure stacks on top of it.
        confirmOpen = false;
        openErrorModal(m.sites_appLogs_clearFailed({ error: r.error || '' }));
        return;
      }
      await loadFiles();
      confirmOpen = false;
    } finally {
      clearing = false;
    }
  }

  async function loadFiles() {
    loading = true;
    try {
      const list = await listAppLogFiles(siteDomain, branch);
      files = list;
      if (list.length > 0) {
        selectedFile = list[0].name;
        await loadEntries();
      } else {
        selectedFile = '';
        entries = [];
      }
    } finally {
      loading = false;
    }
  }

  async function loadEntries() {
    if (!selectedFile) return;
    loading = true;
    try {
      ({ entries, more } = await loadAppLogEntries(siteDomain, selectedFile, 0, branch));
    } finally {
      loading = false;
    }
    await tick();
    if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
  }

  // The older page lands above what the reader is looking at, so the scroll
  // moves down by its height to keep the same lines in view.
  async function loadOlder() {
    if (!more || loadingOlder || loading || !selectedFile) return;
    loadingOlder = true;
    // loadAppLogEntries never throws; a failed page comes back empty.
    const page = await loadAppLogEntries(siteDomain, selectedFile, entries.length, branch);
    const before = scrollEl?.scrollHeight ?? 0;
    entries = [...entries, ...page.entries];
    more = page.more;
    if (expandedIdx >= 0) expandedIdx += page.entries.length;
    loadingOlder = false;
    await tick();
    if (scrollEl) scrollEl.scrollTop += scrollEl.scrollHeight - before;
  }

  function onScroll() {
    if (scrollEl && scrollEl.scrollTop < 200) loadOlder();
  }

  // The tab has no log stream of its own, so without a timer it only ever showed
  // what was on disk when it opened. It is mounted only while it is the selected
  // source, so the poll lives and dies with the tab; a suspended site writes
  // nothing worth asking for.
  $effect(() => {
    if (suspended) return;
    const poll = setInterval(refreshEntries, 5000);
    return () => clearInterval(poll);
  });

  // The poll must not flicker the spinner or move the reader's scroll, so it
  // swaps the entries in and leaves the view exactly where it was.
  async function refreshEntries() {
    if (!selectedFile || loading || loadingOlder) return;
    const page = await loadAppLogEntries(siteDomain, selectedFile, 0, branch);
    const merged = mergeNewest(entries, page.entries);
    entries = merged.entries;
    if (merged.replaced) {
      more = page.more;
      expandedIdx = -1;
    }
  }

  // Re-fetch the file list whenever the active site or branch changes.
  // Without this the dropdown sticks on the first mount's branch, so
  // switching from parent to a worktree (or between worktrees) leaves a
  // stale "No log entries found." state — the API was scoped to the
  // wrong path, not actually empty.
  $effect(() => {
    siteDomain;
    branch;
    untrack(() => loadFiles());
  });

  function toggleEntry(i: number) {
    expandedIdx = expandedIdx === i ? -1 : i;
  }

  const filtered = $derived(
    entries.filter((e) => {
      if (!search.trim()) return true;
      const q = search.toLowerCase();
      return (
        (e.message && e.message.toLowerCase().includes(q)) ||
        (e.level && e.level.toLowerCase().includes(q)) ||
        (e.detail && e.detail.toLowerCase().includes(q)) ||
        (e.date && e.date.includes(search))
      );
    })
  );

  const reversed = $derived(filtered.slice().reverse());

  function levelClass(level: string | undefined): string {
    const l = (level || '').toUpperCase();
    if (['ERROR', 'CRITICAL', 'EMERGENCY', 'ALERT'].includes(l))
      return 'bg-red-100 dark:bg-red-500/10 text-red-600 dark:text-red-400';
    if (l === 'WARNING') return 'bg-yellow-100 dark:bg-yellow-500/10 text-yellow-700 dark:text-yellow-400';
    if (['INFO', 'NOTICE'].includes(l)) return 'bg-blue-100 dark:bg-blue-500/10 text-blue-600 dark:text-blue-400';
    return 'bg-gray-100 dark:bg-white/5 text-gray-500 dark:text-gray-400';
  }
</script>

<div class="flex-1 flex flex-col overflow-hidden min-h-0">
  <div class="flex flex-wrap items-center gap-2 px-3 py-2 shrink-0 border-b border-gray-100 dark:border-lerd-border">
    {#if files.length > 0}
      <Dropdown
        value={selectedFile}
        options={files.map((f) => ({ value: f.name, label: f.name }))}
        onchange={(v) => { selectedFile = v; loadEntries(); }}
      />
    {/if}

    {#if loading}
      <svg class="animate-spin w-3.5 h-3.5 text-gray-400 shrink-0" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"/>
      </svg>
    {/if}

    <div class="relative flex-1 min-w-[7rem]">
      <svg class="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>
      </svg>
      <input
        type="text"
        bind:value={search}
        placeholder={m.sites_appLogs_search()}
        class="w-full text-xs bg-transparent border border-gray-200 dark:border-lerd-border rounded-sm pl-7 pr-2 py-1 text-gray-700 dark:text-gray-300 placeholder-gray-400 dark:placeholder-gray-600 hover:border-gray-300 dark:hover:border-lerd-muted focus:outline-hidden focus:border-lerd-red/50 transition-colors"
      />
    </div>

    <ActionButton title={m.common_refresh()} onclick={loadEntries}>
      <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
      </svg>
    </ActionButton>

    {#if files.length > 0}
      <ActionButton
        title={`${m.sites_appLogs_clear()} · ${fmtBytes(totalBytes)}`}
        loading={clearing}
        disabled={clearing}
        onclick={() => (confirmOpen = true)}
      >
        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
        </svg>
      </ActionButton>
    {/if}
  </div>

  <div bind:this={scrollEl} onscroll={onScroll} class="flex-1 overflow-y-auto">
    {#if files.length === 0 && !loading}
      <div class="text-gray-400 dark:text-gray-600 italic text-xs p-4">
        {branch ? m.sites_appLogs_noFilesWorktree() : m.sites_appLogs_noFiles()}
      </div>
    {:else if reversed.length === 0 && !loading}
      <div class="text-gray-400 dark:text-gray-600 italic text-xs p-4">{m.sites_appLogs_empty()}</div>
    {/if}
    {#if loadingOlder}
      <LoadingRow />
    {/if}
    {#each reversed as entry, i (i + ':' + (entry.date ?? '') + ':' + (entry.message ?? '').slice(0, 40))}
      <div class="border-b border-gray-100 dark:border-lerd-border/50">
        <button
          onclick={() => toggleEntry(i)}
          class="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-white/3 transition-colors"
        >
          <span class="shrink-0 text-[10px] font-bold uppercase px-1.5 py-0.5 rounded-sm leading-tight {levelClass(entry.level)}">
            {entry.level || 'LOG'}
          </span>
          <span class="shrink-0 text-[11px] text-gray-400 font-mono w-[135px]">{entry.date ?? ''}</span>
          <span class="text-xs text-gray-700 dark:text-gray-300 truncate flex-1">{entry.message ?? ''}</span>
          <svg
            class="w-3 h-3 shrink-0 ml-auto text-gray-400 transition-transform duration-150 {expandedIdx === i ? 'rotate-180' : ''}"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/>
          </svg>
        </button>
        {#if expandedIdx === i}
          {@const split = splitLogContext(entry.message ?? '')}
          <div class="px-3 py-3 bg-gray-50 dark:bg-lerd-bg border-t border-gray-100 dark:border-lerd-border/30 max-h-80 overflow-y-auto space-y-2">
            {#if split.context}
              <div class="font-mono text-[11px] text-gray-700 dark:text-gray-300 break-all">{split.text}</div>
              <StructuredValue value={split.context} open class="text-gray-600 dark:text-gray-400" />
            {/if}
            {#if restOf(entry, !!split.context)}
              <div class="font-mono text-[11px] text-gray-600 dark:text-gray-400 whitespace-pre-wrap break-all leading-relaxed">{restOf(entry, !!split.context)}</div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>
</div>

<ClearAppLogsModal
  open={confirmOpen}
  sizeLabel={fmtBytes(totalBytes)}
  loading={clearing}
  onconfirm={doClear}
  onclose={() => {
    if (!clearing) confirmOpen = false;
  }}
/>
