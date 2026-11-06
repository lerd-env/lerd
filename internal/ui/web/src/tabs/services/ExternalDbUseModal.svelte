<script lang="ts">
  import DetailButton from '$components/DetailButton.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import Modal from '$components/Modal.svelte';
  import type { Service } from '$stores/services';
  import { loadSites, sites, useSiteDatabase } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    svc: Service;
    onclose: () => void;
  }
  let { open, svc, onclose }: Props = $props();

  let site = $state('');
  let using = $state(false);
  let error = $state('');

  const choices = $derived(
    $sites
      .filter((s) => s.external_db !== svc.name && !s.custom_container)
      .map((s) => ({ value: s.domain, label: s.domain }))
  );

  $effect(() => {
    if (open) {
      site = '';
      error = '';
    }
  });

  async function use() {
    using = true;
    error = '';
    const res = await useSiteDatabase(site, svc.name);
    using = false;
    if (!res.ok) {
      error = res.error || m.common_requestFailed();
      return;
    }
    await loadSites();
    onclose();
  }

  function safeClose() {
    if (!using) onclose();
  }
</script>

<Modal {open} title={m.externalDb_useTitle()} onclose={safeClose} size="sm">
  <div class="px-5 py-4 space-y-3">
    <Dropdown value={site} options={[{ value: '', label: m.externalDb_useOnSite() }, ...choices]} onchange={(v) => (site = v)} width="full" disabled={using} />
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.externalDb_useHint()}</p>
    {#if error}
      <p class="text-xs text-red-500 break-words">{error}</p>
    {/if}
  </div>
  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={using}>{m.common_cancel()}</DetailButton>
    <DetailButton tone="primary" onclick={use} loading={using} disabled={using || !site}>{m.externalDb_use()}</DetailButton>
  {/snippet}
</Modal>
