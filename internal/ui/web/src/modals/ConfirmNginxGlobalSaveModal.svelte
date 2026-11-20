<script lang="ts">
  import ConfigConfirmModal from '$components/ConfigConfirmModal.svelte';
  import BackupCheckbox from '$components/BackupCheckbox.svelte';
  import { modal } from '$stores/modals';
  import { saveNginxConfig } from '$stores/nginx';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.nginxGlobalSave);
  const web = $derived(target?.target === 'web');
  let backup = $state(false);
  let validationOutput = $state('');

  async function run(t: NonNullable<typeof target>) {
    validationOutput = '';
    const res = await saveNginxConfig(t.content, backup, t.target);
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
      {web ? m.webGlobalEditor_confirmBody() : m.nginxGlobalEditor_confirmBody()}
    </p>
    {#if t.exists}
      <BackupCheckbox bind:checked={backup} disabled={busy} label={m.nginxEditor_backupLabel()} file={web ? "global.yaml" : "zz-lerd-user.conf"} />
    {/if}
  {/snippet}
  {#snippet notice()}
    {#if validationOutput}
      <pre class="text-[11px] leading-relaxed font-mono p-3 rounded-sm border border-red-200 dark:border-red-900/40 bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 overflow-auto max-h-48 whitespace-pre-wrap">{validationOutput}</pre>
    {/if}
  {/snippet}
</ConfigConfirmModal>
