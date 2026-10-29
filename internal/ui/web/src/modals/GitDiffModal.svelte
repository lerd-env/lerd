<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Toggle from '$components/Toggle.svelte';
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import MonacoDiffEditor from '$components/MonacoDiffEditor.svelte';
  import GitCommitBox from '$components/GitCommitBox.svelte';
  import { closeModal } from '$stores/modals';
  import {
    SECTIONS,
    languageFor,
    loadCommitFiles,
    loadCommitVersions,
    loadGitChanges,
    loadGitVersions,
    loadOutgoingFiles,
    loadOutgoingVersions,
    loadUptoFiles,
    loadUptoVersions,
    stageFile,
    stageHunk,
    unstageFile,
    type GitChangedFile,
    type GitCommit,
    type GitDiffSection,
    type GitVersions,
    type Hunk
  } from '$lib/gitDiff';
  import { fileTreeRows } from '$lib/fileTree';
  import { m } from '../paraglide/messages.js';

  interface Props {
    domain: string;
    path: string;
    branch: string;
    // Shows what this commit changed instead of the uncommitted work.
    commit?: GitCommit;
    // Or what HEAD to this commit changes: everything a pull would bring in.
    upto?: string;
    // Or everything a push would send.
    outgoing?: boolean;
    // Replaces the title for a span the dialog has no wording of its own for.
    title?: string;
    onclose?: () => void;
  }
  let { domain, path, branch, commit, upto, outgoing = false, title, onclose = closeModal }: Props = $props();

  // Uncommitted work lists one group per section; a commit, a pull or a push is one unnamed group.
  type Group = { key: GitDiffSection | 'commit' | 'upto' | 'outgoing'; label: string; files: GitChangedFile[] };
  let groups = $state<Group[] | null>(null);
  let selected = $state<{ group: Group['key']; file: string } | null>(null);
  let versions = $state<GitVersions | null>(null);
  let stats = $state<{ added: number; removed: number } | null>(null);
  let error = $state('');
  // Outlives the commit box, which goes once the last change is committed.
  let notice = $state('');
  let filesHidden = $state(false);
  // Keyed by group and folder, so a folder collapsed under Staged stays open under Unstaged.
  let collapsed = $state<Record<string, boolean>>({});

  // Full screen, the view and hidden whitespace are remembered, so the next diff opens the way this one was left.
  const PREFS_KEY = 'lerd:gitDiff';
  type Prefs = { fullscreen: boolean; unified: boolean; ignoreSpace: boolean };
  function readPrefs(): Prefs {
    try {
      const v = JSON.parse(localStorage.getItem(PREFS_KEY) ?? '{}');
      return { fullscreen: v.fullscreen === true, unified: v.unified === true, ignoreSpace: v.ignoreSpace === true };
    } catch {
      return { fullscreen: false, unified: false, ignoreSpace: false };
    }
  }
  const prefs = readPrefs();
  let fullscreen = $state(prefs.fullscreen);
  let unified = $state(prefs.unified);
  let ignoreSpace = $state(prefs.ignoreSpace);
  $effect(() => {
    try {
      localStorage.setItem(PREFS_KEY, JSON.stringify({ fullscreen, unified, ignoreSpace }));
    } catch {
      /* localStorage may be unavailable (private mode, quota), non-fatal */
    }
  });

  // Close never just restores the size: full screen is a remembered choice, not a step to back out of.
  const close = () => onclose();

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

  const empty = $derived(!!groups && groups.every((g) => g.files.length === 0));
  // Only uncommitted work can be staged and committed; a commit, a pull or a push is history.
  const uncommitted = $derived(!commit && !upto && !outgoing);
  const stagedCount = $derived(groups?.find((g) => g.key === 'staged')?.files.length ?? 0);

  async function loadUncommitted() {
    const changes = await loadGitChanges(domain, path);
    groups = SECTIONS.map((s) => ({ key: s, label: sectionLabel[s](), files: changes[s] }));
  }

  // After staging or committing, the open file stays in its section while it has
  // changes there, and otherwise follows to the section it moved to.
  async function refresh(keep?: string, inGroup?: Group['key']) {
    try {
      await loadUncommitted();
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_failed();
      return;
    }
    const all = (groups ?? []).flatMap((g) => g.files.map((f) => ({ group: g.key, file: f.path })));
    const next = all.find((x) => x.file === keep && x.group === inGroup) ?? all.find((x) => x.file === keep) ?? all[0];
    if (next) select(next.group, next.file);
    else selected = versions = null;
  }

  // Blocks of a modified file can move on their own; an added or deleted one moves whole.
  const hunkAction = $derived.by(() => {
    const sel = selected;
    if (!uncommitted || !sel || (sel.group !== 'staged' && sel.group !== 'unstaged')) return undefined;
    const f = groups?.find((g) => g.key === sel.group)?.files.find((f) => f.path === sel.file);
    if (f?.status !== 'M') return undefined;
    const unstage = sel.group === 'staged';
    return {
      label: unstage ? m.gitDiff_unstageHunk() : m.gitDiff_stageHunk(),
      unstage,
      run: async (h: Hunk) => {
        const res = await stageHunk(domain, path, sel.file, h, unstage);
        if (res.error) error = res.error;
        else await refresh(sel.file, sel.group);
      }
    };
  });

  async function toggleStage(group: Group['key'], file: string) {
    const res = await (group === 'staged' ? unstageFile : stageFile)(domain, path, file);
    if (res.error) error = res.error;
    else await refresh(file);
  }

  onMount(async () => {
    try {
      if (commit) {
        groups = [{ key: 'commit', label: '', files: await loadCommitFiles(domain, path, commit.hash) }];
      } else if (upto) {
        groups = [{ key: 'upto', label: '', files: await loadUptoFiles(domain, path, upto) }];
      } else if (outgoing) {
        groups = [{ key: 'outgoing', label: '', files: await loadOutgoingFiles(domain, path) }];
      } else {
        await loadUncommitted();
      }
      const first = groups?.find((g) => g.files.length > 0);
      if (first) select(first.key, first.files[0].path);
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_failed();
    }
  });

  async function select(group: Group['key'], file: string) {
    selected = { group, file };
    versions = null;
    stats = null;
    error = '';
    try {
      const v =
        group === 'commit'
          ? await loadCommitVersions(domain, path, commit!.hash, file)
          : group === 'upto'
            ? await loadUptoVersions(domain, path, upto!, file)
            : group === 'outgoing'
              ? await loadOutgoingVersions(domain, path, file)
              : await loadGitVersions(domain, path, group, file);
      // A slower answer for an earlier click must not replace this one.
      if (selected?.group === group && selected.file === file) versions = v;
    } catch (e) {
      if (selected?.group === group && selected.file === file) error = e instanceof Error ? e.message : m.common_failed();
    }
  }
</script>

<Modal open size="2xl" tall {fullscreen} title={title ?? (commit ? `${commit.hash.slice(0, 7)} ${commit.subject}` : upto ? m.gitDiff_incomingTitle({ branch }) : outgoing ? m.gitDiff_outgoingTitle({ branch }) : m.gitDiff_title({ branch }))} onclose={close}>
  {#snippet actions()}
    <!-- Same gap and icon size as the modal's own close button beside them. -->
    <div class="flex items-center gap-3">
      <button
        type="button"
        onclick={() => (unified = !unified)}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
        use:tooltip={unified ? m.gitDiff_sideBySide() : m.gitDiff_unified()}
        aria-label={unified ? m.gitDiff_sideBySide() : m.gitDiff_unified()}
      >
        <Icon name={unified ? 'splitHorizontal' : 'splitVertical'} class="w-5 h-5" />
      </button>
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
        use:tooltip={fullscreen ? m.gitDiff_exitFullscreen() : m.tinker_fullscreenTitle()}
        aria-label={fullscreen ? m.gitDiff_exitFullscreen() : m.tinker_fullscreenTitle()}
      >
        <Icon name={fullscreen ? 'minimize' : 'maximize'} class="w-5 h-5" />
      </button>
    </div>
  {/snippet}
  {#if notice}<p class="px-5 pt-3 text-xs text-amber-600 dark:text-amber-400">{notice}</p>{/if}
  {#if empty}
    <p class="px-5 py-6 text-sm text-gray-500 dark:text-gray-400">{commit ? m.gitDiff_commitEmpty() : m.gitDiff_empty()}</p>
  {:else}
    <div class="flex flex-col md:flex-row min-h-0 flex-1">
      <!-- Width animates like the dashboard's own side panel; the list keeps its
           width inside so rows slide out instead of squeezing. -->
      <div
        data-testid="file-list"
        inert={filesHidden}
        class="shrink-0 overflow-hidden md:flex md:transition-[width] md:duration-200 md:ease-out {filesHidden ? 'hidden md:w-0' : 'md:w-72'}"
      >
      <div class="md:w-72 shrink-0 flex flex-col md:h-full border-b md:border-b-0 md:border-r border-gray-100 dark:border-lerd-border">
      <!-- Rows keep their full width and the list scrolls sideways, so deep paths stay readable. -->
      <nav class="max-h-48 md:max-h-none md:flex-1 min-h-0 overflow-auto py-2">
        <div class="w-max min-w-full">
        {#each groups ?? [] as g (g.key)}
          {#if g.files.length > 0}
            {#if g.label}
              <h4 class="px-4 pt-2 pb-1 text-[11px] font-semibold uppercase tracking-wide text-gray-400">
                {g.label} · {g.files.length}
              </h4>
            {/if}
            {#each fileTreeRows(g.files, (dir) => !!collapsed[`${g.key}:${dir}`]) as row (row.kind + row.path)}
              {@const indent = `padding-left: ${1 + row.depth * 0.75}rem`}
              {#if row.kind === 'dir'}
                {@const key = `${g.key}:${row.path}`}
                <button
                  type="button"
                  onclick={() => (collapsed[key] = !collapsed[key])}
                  aria-expanded={!collapsed[key]}
                  style={indent}
                  class="w-full flex items-center gap-1 pr-4 py-1 text-left font-mono text-xs whitespace-nowrap text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-white/5"
                >
                  <Icon name="chevron" class="w-3 h-3 shrink-0 transition-transform {collapsed[key] ? '-rotate-90' : ''}" />
                  {row.label}
                </button>
              {:else}
              {@const f = row.file}
              {@const active = selected?.group === g.key && selected.file === f.path}
              {@const staging = g.key === 'staged' ? m.gitDiff_unstage() : m.gitDiff_stage()}
              <div class="group flex items-center {active
                ? 'bg-gray-100 dark:bg-white/10 text-gray-900 dark:text-white'
                : 'text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5'}">
                <button
                  type="button"
                  onclick={() => select(g.key, f.path)}
                  aria-current={active ? 'true' : undefined}
                  style={indent}
                  class="flex-1 flex items-center gap-2 pr-1 py-1 text-left font-mono text-xs whitespace-nowrap"
                >
                  <span class="w-3 shrink-0 font-semibold {statusClass[f.status] ?? 'text-amber-600 dark:text-amber-400'}">{f.status}</span>
                  <span title={f.path}>{row.label}</span>
                </button>
                {#if uncommitted}
                  <!-- Pinned to the pane's visible edge while a long path scrolls; the opaque
                       base hides text under it and the inner layer repeats the row's tint. -->
                  <span class="sticky right-0 shrink-0 self-stretch flex bg-white dark:bg-lerd-card">
                    <span class="flex items-center pl-1 pr-2 {active ? 'bg-gray-100 dark:bg-white/10' : 'group-hover:bg-gray-50 dark:group-hover:bg-white/5'}">
                      <button
                        type="button"
                        onclick={() => toggleStage(g.key, f.path)}
                        class="p-0.5 rounded-sm text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                        use:tooltip={staging}
                        aria-label="{staging} {f.path}"
                      >
                        <Icon name={g.key === 'staged' ? 'minus' : 'plus'} class="w-3.5 h-3.5" />
                      </button>
                    </span>
                  </span>
                {/if}
              </div>
              {/if}
            {/each}
          {/if}
        {/each}
        </div>
      </nav>
      {#if uncommitted}
        <GitCommitBox
          {domain}
          {path}
          staged={stagedCount}
          oncommitted={(warning) => {
            notice = warning ?? '';
            refresh(selected?.file);
          }}
        />
      {/if}
      </div>
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
          {#if versions.binary || versions.too_large || versions.unreadable}
            <p class="px-5 py-4 text-sm text-gray-500 dark:text-gray-400">
              {versions.binary ? m.gitDiff_binary() : versions.too_large ? m.gitDiff_tooLarge() : m.gitDiff_unreadable()}
            </p>
          {:else}
            <div class="flex-1 min-h-0">
              <MonacoDiffEditor
                original={versions.original}
                modified={versions.modified}
                language={languageFor(selected.file)}
                {ignoreSpace}
                sideBySide={!unified}
                {hunkAction}
                onStats={(s) => (stats = s)}
              />
            </div>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</Modal>
