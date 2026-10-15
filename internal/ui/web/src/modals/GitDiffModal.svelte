<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Toggle from '$components/Toggle.svelte';
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import MonacoDiffEditor from '$components/MonacoDiffEditor.svelte';
  import { closeModal } from '$stores/modals';
  import {
    SECTIONS,
    languageFor,
    loadGitChanges,
    loadGitVersions,
    type GitChanges,
    type GitDiffSection,
    type GitVersions
  } from '$lib/gitDiff';
  import { m } from '../paraglide/messages.js';

  interface Props {
    domain: string;
    path: string;
    branch: string;
  }
  let { domain, path, branch }: Props = $props();

  let changes = $state<GitChanges | null>(null);
  let selected = $state<{ section: GitDiffSection; file: string } | null>(null);
  let versions = $state<GitVersions | null>(null);
  let stats = $state<{ added: number; removed: number } | null>(null);
  let error = $state('');
  let ignoreSpace = $state(false);
  let fullscreen = $state(false);
  let filesHidden = $state(false);

  // Escape and the close button leave full screen first, as Tinker's does.
  function close() {
    if (fullscreen) fullscreen = false;
    else closeModal();
  }

  const sectionLabel: Record<GitDiffSection, () => string> = {
    staged: m.gitDiff_staged,
    unstaged: m.gitDiff_unstaged,
    untracked: m.gitDiff_untracked
  };
  const statusClass: Record<string, string> = {
    A: 'text-green-600 dark:text-green-400',
    '?': 'text-green-600 dark:text-green-400',
    D: 'text-red-600 dark:text-red-400',
    U: 'text-red-600 dark:text-red-400'
  };

  const empty = $derived(!!changes && SECTIONS.every((s) => changes![s].length === 0));

  onMount(async () => {
    try {
      changes = await loadGitChanges(domain, path);
      const first = SECTIONS.find((s) => changes![s].length > 0);
      if (first) select(first, changes[first][0].path);
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_failed();
    }
  });

  async function select(section: GitDiffSection, file: string) {
    selected = { section, file };
    versions = null;
    stats = null;
    error = '';
    try {
      const v = await loadGitVersions(domain, path, section, file);
      // A slower answer for an earlier click must not replace this one.
      if (selected?.section === section && selected.file === file) versions = v;
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_failed();
    }
  }
</script>

<Modal open size="2xl" tall {fullscreen} title={m.gitDiff_title({ branch })} onclose={close}>
  {#snippet actions()}
    <!-- Same gap and icon size as the modal's own close button beside them. -->
    <div class="flex items-center gap-3">
      <button
        type="button"
        onclick={() => (filesHidden = !filesHidden)}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
        use:tooltip={filesHidden ? m.gitDiff_showFiles() : m.gitDiff_hideFiles()}
        aria-label={filesHidden ? m.gitDiff_showFiles() : m.gitDiff_hideFiles()}
        aria-pressed={!filesHidden}
      >
        <Icon name="sidebar" class="w-5 h-5" />
      </button>
      <button
        type="button"
        onclick={() => (fullscreen = !fullscreen)}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
        use:tooltip={fullscreen ? m.tinker_exitFullscreenTitle() : m.tinker_fullscreenTitle()}
        aria-label={fullscreen ? m.tinker_exitFullscreenTitle() : m.tinker_fullscreenTitle()}
      >
        <Icon name={fullscreen ? 'minimize' : 'maximize'} class="w-5 h-5" />
      </button>
    </div>
  {/snippet}
  {#if empty}
    <p class="px-5 py-6 text-sm text-gray-500 dark:text-gray-400">{m.gitDiff_empty()}</p>
  {:else}
    <div class="flex flex-col md:flex-row min-h-0 flex-1">
      <!-- Width animates like the dashboard's own side panel; the list keeps its
           width inside so rows slide out instead of squeezing. -->
      <div
        inert={filesHidden}
        class="shrink-0 overflow-hidden md:flex md:transition-[width] md:duration-200 md:ease-out {filesHidden ? 'hidden md:w-0' : 'md:w-72'}"
      >
      <nav class="md:w-72 shrink-0 max-h-48 md:max-h-none md:h-full overflow-y-auto border-b md:border-b-0 md:border-r border-gray-100 dark:border-lerd-border py-2">
        {#each SECTIONS as section (section)}
          {#if changes && changes[section].length > 0}
            <h4 class="px-4 pt-2 pb-1 text-[11px] font-semibold uppercase tracking-wide text-gray-400">
              {sectionLabel[section]()} · {changes[section].length}
            </h4>
            {#each changes[section] as f (f.path)}
              {@const active = selected?.section === section && selected.file === f.path}
              <button
                type="button"
                onclick={() => select(section, f.path)}
                aria-current={active ? 'true' : undefined}
                class="w-full flex items-center gap-2 px-4 py-1 text-left font-mono text-xs {active
                  ? 'bg-gray-100 dark:bg-white/10 text-gray-900 dark:text-white'
                  : 'text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5'}"
              >
                <span class="w-3 shrink-0 font-semibold {statusClass[f.status] ?? 'text-amber-600 dark:text-amber-400'}">{f.status}</span>
                <span class="truncate" title={f.path}>{f.path}</span>
              </button>
            {/each}
          {/if}
        {/each}
      </nav>
      </div>
      <div class="flex-1 min-w-0 min-h-0 flex flex-col">
        {#if error}
          <p class="px-5 py-4 text-sm text-red-600 dark:text-red-400">{error}</p>
        {:else if versions && selected}
          <div class="sticky top-0 z-10 flex items-center gap-3 px-4 py-2 border-b border-gray-100 dark:border-lerd-border bg-white dark:bg-lerd-card font-mono text-xs">
            <span class="truncate text-gray-800 dark:text-gray-100" title={selected.file}>{selected.file}</span>
            {#if stats}
              <span class="ml-auto shrink-0 tabular-nums text-green-600 dark:text-green-400">+{stats.added}</span>
              <span class="shrink-0 tabular-nums text-red-600 dark:text-red-400">−{stats.removed}</span>
            {/if}
            <span class="{stats ? '' : 'ml-auto'} shrink-0 flex items-center gap-2 pl-3 border-l border-gray-200 dark:border-lerd-border font-sans text-gray-500 dark:text-gray-400">
              {m.gitDiff_ignoreSpace()}
              <Toggle on={ignoreSpace} title={m.gitDiff_ignoreSpace()} onclick={() => (ignoreSpace = !ignoreSpace)} />
            </span>
          </div>
          {#if versions.binary || versions.too_large}
            <p class="px-5 py-4 text-sm text-gray-500 dark:text-gray-400">
              {versions.binary ? m.gitDiff_binary() : m.gitDiff_tooLarge()}
            </p>
          {:else}
            <div class="flex-1 min-h-0">
              <MonacoDiffEditor
                original={versions.original}
                modified={versions.modified}
                language={languageFor(selected.file)}
                {ignoreSpace}
                onStats={(s) => (stats = s)}
              />
            </div>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</Modal>
