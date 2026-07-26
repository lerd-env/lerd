<script lang="ts">
  import TuningEditor from '$components/TuningEditor.svelte';
  import ConfigToolbar from '$components/ConfigToolbar.svelte';
  import {
    getPhpIni,
    loadPhpIniBackups,
    loadPhpIniBackupContent
  } from '$stores/phpVersions';
  import type { SiteNginxBackup } from '$stores/sites';
  import {
    openPhpIniSaveModal,
    openPhpIniRestoreModal,
    openPhpIniResetModal
  } from '$stores/modals';
  import { onMount } from 'svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    // The API scope token: a bare PHP version, or "shared" for the file every
    // version loads. One pane owns exactly one file, so its whole editing
    // state is local and saving here cannot disturb the sibling pane.
    scope: string;
    // Pane heading and the human-readable scope the confirm modals name.
    title: string;
    hint: string;
    label: string;
  }
  let { scope, title, hint, label }: Props = $props();

  let original = $state<string>('');
  let text = $state<string>('');
  let path = $state<string>('');
  let exists = $state<boolean>(false);
  let loading = $state(true);
  let error = $state<string>('');
  let actionError = $state<string>('');
  let backupsError = $state<string>('');
  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | null = null;
  let backups = $state<SiteNginxBackup[]>([]);
  let restoring = $state(false);
  // True from the moment an action's request goes out until the pane has reloaded
  // the file, so the editor cannot take a second write against state it has not
  // caught up with yet.
  let applying = $state(false);

  const dirty = $derived(text !== original);
  const latestBackup = $derived(backups[0]);
  const hasBackup = $derived(backups.length > 0 && !loading && !error);
  const canRevert = $derived(dirty && !loading && !error && !applying);
  const canReset = $derived(exists && !loading && !error && !applying);
  const canSave = $derived(dirty && !loading && !error && !applying);

  // Pin the loader's reactive input to the pane's scope so an unrelated store
  // push can't clobber unsaved edits, and a version switch reloads the file.
  const currentScope = $derived(scope);

  $effect(() => {
    const v = currentScope;
    loading = true;
    error = '';
    actionError = '';
    backupsError = '';
    original = '';
    text = '';
    path = '';
    backups = [];
    // allSettled so a transient failure on one half doesn't discard the
    // other half's data, e.g. backups loaded fine but getPhpIni 500s and we
    // still want the user able to Restore.
    Promise.allSettled([getPhpIni(v), loadPhpIniBackups(v)])
      .then(([cfgRes, listRes]) => {
        if (currentScope !== v) return;
        if (cfgRes.status === 'fulfilled') {
          original = cfgRes.value.content;
          text = cfgRes.value.content;
          path = cfgRes.value.path;
          exists = cfgRes.value.exists;
        } else {
          error = cfgRes.reason instanceof Error ? cfgRes.reason.message : String(cfgRes.reason);
        }
        if (listRes.status === 'fulfilled') {
          if (listRes.value.ok) {
            backups = listRes.value.list;
          } else {
            backupsError = listRes.value.error || 'Could not load backups';
          }
        } else {
          backupsError = listRes.reason instanceof Error ? listRes.reason.message : String(listRes.reason);
        }
      })
      .finally(() => {
        if (currentScope === v) loading = false;
      });
  });

  // Run cleanup on unmount. Plain onMount is cheaper than a $effect with no
  // reactive reads and makes the intent obvious.
  onMount(() => () => {
    if (copyTimer) clearTimeout(copyTimer);
  });

  async function refreshAfterAction(v: string) {
    applying = true;
    try {
      const [cfgRes, listRes] = await Promise.allSettled([getPhpIni(v), loadPhpIniBackups(v)]);
      if (currentScope !== v) return;
      if (cfgRes.status === 'fulfilled') {
        original = cfgRes.value.content;
        text = cfgRes.value.content;
        path = cfgRes.value.path;
        exists = cfgRes.value.exists;
      } else {
        actionError = cfgRes.reason instanceof Error ? cfgRes.reason.message : String(cfgRes.reason);
      }
      if (listRes.status === 'fulfilled') {
        if (listRes.value.ok) {
          backups = listRes.value.list;
          backupsError = '';
        } else {
          backupsError = listRes.value.error || 'Could not load backups';
        }
      } else {
        backupsError = listRes.reason instanceof Error ? listRes.reason.message : String(listRes.reason);
      }
    } catch (e: unknown) {
      if (currentScope !== v) return;
      actionError = e instanceof Error ? e.message : String(e);
    } finally {
      applying = false;
    }
  }

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

  async function restore() {
    if (!latestBackup) return;
    restoring = true;
    actionError = '';
    try {
      const v = currentScope;
      const name = latestBackup.name;
      const backupContent = await loadPhpIniBackupContent(v, name);
      openPhpIniRestoreModal(
        { version: v, label, current: original, backupName: name, backup: backupContent },
        async () => {
          if (currentScope !== v) return;
          original = backupContent;
          text = backupContent;
          exists = true;
          try {
            const listRes = await loadPhpIniBackups(v);
            if (currentScope !== v) return;
            if (listRes.ok) {
              backups = listRes.list;
              backupsError = '';
            } else {
              backupsError = listRes.error || 'Could not load backups';
            }
          } catch (e: unknown) {
            if (currentScope !== v) return;
            actionError = e instanceof Error ? e.message : String(e);
          }
        }
      );
    } catch (e: unknown) {
      actionError = e instanceof Error ? e.message : String(e);
    } finally {
      restoring = false;
    }
  }

  function revert() {
    text = original;
  }

  function reset() {
    const v = currentScope;
    openPhpIniResetModal({ version: v, label, path }, () => refreshAfterAction(v));
  }

  function save() {
    const v = currentScope;
    openPhpIniSaveModal({ version: v, label, content: text, original, exists }, () =>
      refreshAfterAction(v)
    );
  }
</script>

<section class="flex flex-col flex-1 min-w-0 min-h-64">
  <div
    class="flex items-baseline gap-2 px-3 sm:px-5 py-2 border-b border-gray-100 dark:border-lerd-border shrink-0"
  >
    <span class="text-[11px] uppercase tracking-wide font-semibold text-gray-500 dark:text-gray-400"
      >{title}</span
    >
    <span class="text-[11px] text-gray-400 dark:text-gray-500 truncate">{hint}</span>
  </div>
  <ConfigToolbar
    {path}
    {dirty}
    {loading}
    {error}
    backupCount={backups.length}
    latestBackupName={latestBackup?.name}
    {backupsError}
    {actionError}
    {canRevert}
    {canReset}
    {canSave}
    {hasBackup}
    restoring={restoring || applying}
    {copied}
    onCopy={copy}
    onRevert={revert}
    onReset={reset}
    onRestore={restore}
    onSave={save}
  />

  <div class="flex-1 min-h-0 overflow-hidden bg-gray-50 dark:bg-black/40">
    {#if loading}
      <p class="text-xs text-gray-400 px-3 py-2.5">{m.common_loading()}</p>
    {:else if error}
      <p class="text-xs text-red-500 dark:text-red-400 px-3 py-2.5">{error}</p>
    {:else}
      <div class="h-full min-h-64">
        <TuningEditor bind:value={text} />
      </div>
    {/if}
  </div>
</section>
