<script lang="ts">
  import ConfigConfirmModal from '$components/ConfigConfirmModal.svelte';
  import { modal } from '$stores/modals';
  import { resetSiteNginx } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.nginxReset);
  const web = $derived(target?.scope === 'web');

  async function run(t: NonNullable<typeof target>) {
    const res = await resetSiteNginx(t.domain, t.scope);
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
      {web ? m.webEditor_resetBody({ domain: t.domain }) : m.nginxEditor_resetBody({ domain: t.domain })}
    </p>
    <p class="text-[10px] text-gray-500 dark:text-gray-400 font-mono break-all">{t.path}</p>
  {/snippet}
</ConfigConfirmModal>
