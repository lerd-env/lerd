<script lang="ts">
  import ConfigConfirmModal from '$components/ConfigConfirmModal.svelte';
  import BackupDiff from '$components/BackupDiff.svelte';
  import { modal } from '$stores/modals';
  import { restoreSiteNginx } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.nginxRestore);
  const web = $derived(target?.scope === 'web');

  async function run(t: NonNullable<typeof target>) {
    // Pass the exact backup previewed in the diff, so a save landing a newer
    // backup since the modal opened cannot swap a different file into place.
    const res = await restoreSiteNginx(t.domain, t.scope, t.backupName);
    if (!res.ok) return res.error || m.nginxEditor_restoreFailed();
  }
</script>

<ConfigConfirmModal
  title={web ? m.webEditor_restoreTitle() : m.nginxEditor_restoreTitle()}
  size="lg"
  {target}
  acceptLabel={m.nginxEditor_restoreAccept()}
  busyLabel={m.nginxEditor_restoring()}
  {run}
>
  {#snippet children(t)}
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {web
        ? m.webEditor_restoreBody({ domain: t.domain, name: t.backupName })
        : m.nginxEditor_restoreBody({ domain: t.domain, name: t.backupName })}
    </p>
    <BackupDiff current={t.current} backup={t.backup} emptyText={m.nginxEditor_restoreNoDiff()} />
  {/snippet}
</ConfigConfirmModal>
