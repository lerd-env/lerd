<script lang="ts">
  import ConfigConfirmModal from '$components/ConfigConfirmModal.svelte';
  import BackupCheckbox from '$components/BackupCheckbox.svelte';
  import { modal } from '$stores/modals';
  import { saveSiteNginx } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.nginxSave);
  const web = $derived(target?.scope === 'web');
  let backup = $state(false);
  let validationOutput = $state('');

  async function run(t: NonNullable<typeof target>) {
    validationOutput = '';
    const res = await saveSiteNginx(t.domain, t.scope, t.content, backup);
    if (res.ok) return;
    // nginx's line-and-directive diagnostic; the modal stays open so the
    // user can fix the text in the editor and retry.
    validationOutput = res.validationOutput || '';
    return res.error || m.nginxEditor_saveFailed();
  }
</script>

<ConfigConfirmModal
  title={web ? m.webEditor_confirmTitle() : m.nginxEditor_confirmTitle()}
  size="md"
  {target}
  acceptLabel={m.common_save()}
  busyLabel={m.nginxEditor_saving()}
  {run}
>
  {#snippet children(t, busy)}
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {web ? m.webEditor_confirmBody({ domain: t.domain }) : m.nginxEditor_confirmBody({ domain: t.domain })}
    </p>
    {#if t.exists}
      <BackupCheckbox bind:checked={backup} disabled={busy} label={m.nginxEditor_backupLabel()} file={web ? `${t.domain}.yaml` : `${t.domain}.conf`} />
    {/if}
  {/snippet}
  {#snippet notice()}
    {#if validationOutput}
      <pre class="text-[11px] leading-relaxed font-mono p-3 rounded-sm border border-red-200 dark:border-red-900/40 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 overflow-auto max-h-48 whitespace-pre-wrap">{validationOutput}</pre>
    {/if}
  {/snippet}
</ConfigConfirmModal>
