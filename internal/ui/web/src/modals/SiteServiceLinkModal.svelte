<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { serviceLabel } from '$stores/services';
  import { linkableServices, linkSiteService, loadSites } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.siteServiceLink);

  let names = $state<string[] | null>(null);
  let picked = $state('');
  let busy = $state(false);
  let error = $state('');

  // The name tells two versions of one service apart.
  const options = $derived(
    (names ?? []).map((name) => {
      const label = serviceLabel(name);
      return { value: name, label, description: label.toLowerCase() === name ? undefined : name };
    })
  );

  onMount(async () => {
    if (!target) return;
    try {
      names = await linkableServices(target.domain);
      picked = names[0] ?? '';
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_requestFailed();
    }
  });

  function safeClose() {
    if (!busy) closeModal();
  }

  async function add() {
    if (busy || !target || !picked) return;
    busy = true;
    error = '';
    const res = await linkSiteService(target.domain, picked);
    busy = false;
    if (!res.ok) {
      error = res.error || m.common_requestFailed();
      return;
    }
    await loadSites();
    closeModal();
  }
</script>

<Modal open title={target ? m.sites_linkService_title({ domain: target.domain }) : ''} onclose={safeClose} size="sm">
  <div class="px-5 py-4 space-y-3">
    {#if names?.length}
      <Dropdown value={picked} {options} onchange={(v) => (picked = v)} width="full" disabled={busy}>
        {#snippet optionIcon(name)}<ServiceIcon {name} bare inline />{/snippet}
      </Dropdown>
      <p class="text-xs text-gray-500 dark:text-gray-400">{m.sites_linkService_body()}</p>
    {:else if names}
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.sites_linkService_empty()}</p>
    {:else if !error}
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.common_loading()}</p>
    {/if}
    {#if error}
      <p class="text-xs text-red-500 break-words">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={busy}>{m.common_cancel()}</DetailButton>
    <DetailButton tone="primary" onclick={add} loading={busy} disabled={busy || !picked}>{m.common_add()}</DetailButton>
  {/snippet}
</Modal>
