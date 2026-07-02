<script lang="ts">
  import EnvEditor from '$components/EnvEditor.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import {
    loadSiteEnv,
    loadSiteEnvFiles,
    loadSiteEnvBackups,
    loadSiteEnvBackupContent,
    proposeSiteEnv,
    type Site,
    type SiteEnvBackup
  } from '$stores/sites';
  import { openEnvSaveModal, openEnvRestoreModal } from '$stores/modals';
  import { addedLineNumbers } from '$lib/diff';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
    branch: string;
  }
  let { site, branch }: Props = $props();

  let files = $state<string[]>(['.env']);
  let file = $state<string>('.env');
  let original = $state<string>('');
  let text = $state<string>('');
  let loading = $state(true);
  let error = $state<string>('');
  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | null = null;
  let backups = $state<SiteEnvBackup[]>([]);
  let restoring = $state(false);
  // Missing-key proposal state. The proposal is framework-scoped (its own env
  // file), so it's tracked separately from the file dropdown and only surfaced
  // when that file is the one selected.
  let missingCount = $state(0);
  let optionalCount = $state(0);
  let proposeFile = $state('.env');
  let inserting = $state(false);
  let insertError = $state('');

  const envPath = $derived.by(() => {
    if (branch) {
      const wt = (site.worktrees || []).find((w) => w.branch === branch);
      if (wt?.path) return wt.path + '/' + file;
    }
    return (site.path || '') + '/' + file;
  });

  const dirty = $derived(text !== original);
  const latestBackup = $derived(backups[0]);
  // Which editor lines are unsaved insertions, recomputed live from the diff
  // against the on-disk content so the green markers track edits (a line the
  // user deletes simply stops being reported, no stale/hopping decoration).
  const highlightLines = $derived(addedLineNumbers(original, text));
  // Offer the insert banner only for the file the proposal targets, once its
  // content has loaded and there are no unsaved edits (staging would clobber).
  const canPropose = $derived(missingCount > 0 && file === proposeFile && !loading && !error && !dirty);

  function refreshProposal() {
    const domain = site.domain;
    const b = branch;
    proposeSiteEnv(domain, b, false)
      .then((p) => {
        if (site.domain !== domain || branch !== b) return;
        missingCount = p.added.length;
        optionalCount = p.optional.length;
        proposeFile = p.file;
      })
      .catch(() => {
        if (site.domain !== domain || branch !== b) return;
        missingCount = 0;
        optionalCount = 0;
      });
  }

  // Stage the proposed merge into the editable buffer: the user then fills in
  // values, drops keys they don't want, and saves with the normal Save button.
  // The green bars track their edits, so this is an ordinary unsaved change, not
  // a separate mode.
  async function insertProposed(includeOptional: boolean) {
    const domain = site.domain;
    const b = branch;
    inserting = true;
    insertError = '';
    try {
      const p = await proposeSiteEnv(domain, b, includeOptional);
      if (site.domain !== domain || branch !== b || file !== p.file) return;
      // Staging the merge makes the buffer differ from disk, so highlightLines
      // (derived from that diff) lights up the inserted lines on its own.
      text = p.merged;
    } catch (e: unknown) {
      if (site.domain !== domain || branch !== b) return;
      insertError = e instanceof Error ? e.message : m.envEditor_proposeFailed();
    } finally {
      inserting = false;
    }
  }

  // Refresh the file list whenever the site or branch changes. When the
  // selected file disappears we only snap back to .env if there are no
  // unsaved edits; a dirty buffer for a file that vanished on disk stays
  // open so the user can copy out or save to recreate.
  $effect(() => {
    const domain = site.domain;
    const b = branch;
    loadSiteEnvFiles(domain, b).then((list) => {
      if (site.domain !== domain || branch !== b) return;
      files = list;
      if (!list.includes(file) && !dirty) file = '.env';
    });
  });

  // Rescan for missing example keys when the site or branch changes. The
  // proposal targets the framework env file, so it's independent of the file
  // dropdown and doesn't need to re-run when only `file` changes.
  $effect(() => {
    void site.domain;
    void branch;
    refreshProposal();
  });

  // Reload content + backups whenever the chosen file (or site/branch) changes.
  $effect(() => {
    const domain = site.domain;
    const b = branch;
    const f = file;
    loading = true;
    error = '';
    insertError = '';
    original = '';
    text = '';
    backups = [];
    Promise.all([loadSiteEnv(domain, b, f), loadSiteEnvBackups(domain, b, f)])
      .then(([t, list]) => {
        if (site.domain !== domain || branch !== b || file !== f) return;
        original = t;
        text = t;
        backups = list;
      })
      .catch((e: unknown) => {
        // Guard the error setter the same way the success branch does, so
        // a stale rejection from a previous site cannot blow away the
        // current view's error state.
        if (site.domain !== domain || branch !== b || file !== f) return;
        error = e instanceof Error ? e.message : String(e);
      })
      .finally(() => {
        if (site.domain === domain && branch === b && file === f) loading = false;
      });
  });

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      copied = true;
      if (copyTimer) clearTimeout(copyTimer);
      copyTimer = setTimeout(() => (copied = false), 1500);
    } catch {
      /* no-op */
    }
  }

  // Drop unsaved edits (including a staged Add) back to what's on disk. This is
  // the plain "undo my changes" the Discard button offers; it never touches
  // backups, which is what the old single Revert button conflated.
  function discardChanges() {
    text = original;
  }

  async function restoreBackup() {
    if (latestBackup) {
      // The diff modal's "current" baseline is the on-screen text (which
      // includes any unsaved edits), so accepting the restore visibly
      // discards everything the user could still see — no silent loss.
      // The backup content is loaded into the modal so the modal does not
      // need its own loader.
      restoring = true;
      try {
        // Snapshot the file the user is on now; if it changes during the
        // network round-trip the success callback should still apply to
        // the file we restored, not whatever is current at completion.
        const restoredFile = file;
        const restoredBranch = branch;
        const restoredDomain = site.domain;
        const backupContent = await loadSiteEnvBackupContent(
          restoredDomain,
          latestBackup.name,
          restoredBranch,
          restoredFile
        );
        openEnvRestoreModal(
          {
            domain: restoredDomain,
            branch: restoredBranch,
            file: restoredFile,
            current: text,
            backupName: latestBackup.name,
            backup: backupContent
          },
          async () => {
            // Only refresh local state if the user is still looking at
            // the file we restored; if they navigated away, the next
            // load effect for the new context will populate fresh state.
            if (
              site.domain !== restoredDomain ||
              branch !== restoredBranch ||
              file !== restoredFile
            ) {
              return;
            }
            // The restore endpoint already returns the new content, so
            // we use the backupContent we loaded for the diff instead of
            // re-fetching via loadSiteEnv. We do refetch the backups list
            // because it shrank by one.
            original = backupContent;
            text = backupContent;
            backups = await loadSiteEnvBackups(restoredDomain, restoredBranch, restoredFile);
          }
        );
      } catch (e: unknown) {
        error = e instanceof Error ? e.message : String(e);
      } finally {
        restoring = false;
      }
    }
  }

  function save() {
    // Snapshot the file we are saving so a concurrent file-list refresh
    // (or any other reactive change) cannot redirect the post-save reload
    // at the wrong file.
    const savedDomain = site.domain;
    const savedBranch = branch;
    const savedFile = file;
    openEnvSaveModal(
      { domain: savedDomain, branch: savedBranch, file: savedFile, content: text, original },
      async () => {
        const [t, list] = await Promise.all([
          loadSiteEnv(savedDomain, savedBranch, savedFile),
          loadSiteEnvBackups(savedDomain, savedBranch, savedFile)
        ]);
        // Only apply if the user is still on the file we saved; otherwise
        // the load effect for the new file will populate its own state.
        if (
          site.domain !== savedDomain ||
          branch !== savedBranch ||
          file !== savedFile
        ) {
          return;
        }
        original = t;
        text = t;
        backups = list;
        // The keys we just inserted are now on disk, so rescan to clear the
        // banner (or surface whatever's still missing).
        refreshProposal();
      }
    );
  }
</script>

<div class="flex-1 flex flex-col min-h-0 overflow-hidden">
  <div class="sticky top-0 z-10">
    <div class="flex items-center justify-between bg-gray-50 dark:bg-white/3 px-3 py-1.5 border-b border-gray-200 dark:border-lerd-border">
      <div class="flex items-center gap-2">
        <Dropdown
          value={file}
          options={files}
          disabled={loading || files.length <= 1}
          onchange={(v) => (file = v)}
        />
        {#if dirty && !loading && !error}
          <span class="text-[10px] font-medium text-amber-600 dark:text-amber-400">{m.envEditor_unsaved()}</span>
        {/if}
        {#if backups.length > 0 && !loading}
          <span
            class="text-[10px] font-medium text-gray-500 dark:text-gray-400"
            title={latestBackup?.name}
          >{m.envEditor_backupAvailable({ n: backups.length })}</span>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        <button
          type="button"
          onclick={copy}
          disabled={loading || !!error}
          class="text-xs px-2 py-1 rounded-sm border border-gray-300 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5 disabled:opacity-40"
        >
          {copied ? m.common_copied() : m.common_copy()}
        </button>
        {#if backups.length > 0}
          <button
            type="button"
            onclick={restoreBackup}
            disabled={loading || !!error || restoring}
            class="text-xs px-2 py-1 rounded-sm border border-gray-300 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5 disabled:opacity-40"
          >
            {m.envEditor_restoreBtn()}
          </button>
        {/if}
        <button
          type="button"
          onclick={discardChanges}
          disabled={!dirty || loading || restoring}
          class="text-xs px-2 py-1 rounded-sm border border-gray-300 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5 disabled:opacity-40"
        >
          {m.envEditor_discard()}
        </button>
        <button
          type="button"
          onclick={save}
          disabled={!dirty || loading}
          class="text-xs px-3 py-1 rounded-sm bg-lerd-red hover:bg-lerd-redhov text-white disabled:opacity-40 transition-colors"
        >
          {m.common_save()}
        </button>
      </div>
    </div>
  </div>

  {#if canPropose}
    <div class="flex items-center justify-between gap-3 px-3 py-1.5 bg-amber-50 dark:bg-amber-900/15 border-b border-amber-200 dark:border-amber-900/40">
      <span class="text-xs text-amber-700 dark:text-amber-300">
        {m.envEditor_proposeBanner({ n: missingCount, file: proposeFile })}
      </span>
      <div class="flex items-center gap-2 shrink-0">
        {#if optionalCount > 0}
          <button
            type="button"
            onclick={() => insertProposed(true)}
            disabled={inserting}
            class="text-xs px-2 py-1 rounded-sm text-amber-700 dark:text-amber-300 hover:bg-amber-100 dark:hover:bg-amber-900/25 disabled:opacity-40 whitespace-nowrap"
          >
            {m.envEditor_proposeAddOptional({ n: optionalCount })}
          </button>
        {/if}
        <button
          type="button"
          onclick={() => insertProposed(false)}
          disabled={inserting}
          class="text-xs px-2 py-1 rounded-sm border border-amber-300 dark:border-amber-900/50 text-amber-800 dark:text-amber-200 hover:bg-amber-100 dark:hover:bg-amber-900/25 disabled:opacity-40 whitespace-nowrap"
        >
          {m.envEditor_proposeAdd({ n: missingCount })}
        </button>
      </div>
    </div>
  {/if}

  {#if insertError}
    <p class="text-xs text-red-500 dark:text-red-400 px-3 py-1.5 border-b border-red-200 dark:border-red-900/40">{insertError}</p>
  {/if}

  <div class="flex-1 min-h-0 overflow-hidden bg-gray-50 dark:bg-black/40">
    {#if loading}
      <p class="text-xs text-gray-400 px-3 py-2.5">{m.common_loading()}</p>
    {:else if error}
      <p class="text-xs text-red-500 dark:text-red-400 px-3 py-2.5">{error}</p>
    {:else}
      {#if !original && highlightLines.length === 0}
        <p class="text-xs text-gray-400 px-3 py-2.5">
          {m.sites_env_missing({ path: envPath })}
        </p>
      {/if}
      <div class="h-full min-h-64">
        <EnvEditor bind:value={text} {highlightLines} />
      </div>
    {/if}
  </div>
</div>
