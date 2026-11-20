<script lang="ts">
  import ConfigConfirmModal from '$components/ConfigConfirmModal.svelte';
  import { modal } from '$stores/modals';
  import { resetNginxConfig } from '$stores/nginx';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.nginxGlobalReset);
  const web = $derived(target?.target === 'web');

  async function run(t: NonNullable<typeof target>) {
    const res = await resetNginxConfig(t.target);
    if (!res.ok) return res.error || m.nginxEditor_resetFailed();
  }
</script>

<ConfigConfirmModal
  title={web ? m.webEditor_resetTitle() : m.nginxEditor_resetTitle()}
  size="md"
  {target}
  acceptLabel={m.nginxEditor_resetAccept()}
  busyLabel={m.nginxEditor_resetting()}
  {run}
>
  {#snippet children(t)}
    <p class="text-sm text-gray-700 dark:text-gray-300">
      {web ? m.webGlobalEditor_resetBody() : m.nginxGlobalEditor_resetBody()}
    </p>
    <p class="text-[10px] text-gray-500 dark:text-gray-400 font-mono break-all">{t.path}</p>
  {/snippet}
</ConfigConfirmModal>
