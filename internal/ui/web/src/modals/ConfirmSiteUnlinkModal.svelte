<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { unlinkSite, loadSites, sites, previewSiteDirDelete, type SiteDirReport } from '$stores/sites';
  import { accessMode } from '$stores/accessMode';
  import { routeRest, goToTab } from '$stores/route';
  import { get } from 'svelte/store';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.siteUnlink);
  const siteName = $derived($sites.find((s) => s.domain === target?.domain)?.name ?? '');

  let busy = $state(false);
  let error = $state('');
  // Deleting the directory is offered on the lerd host only, and waits for the
  // site's name to be typed back once the preview says it may go.
  let deleteDir = $state(false);
  let report = $state<SiteDirReport | null>(null);
  let typed = $state('');
  const canConfirm = $derived(!deleteDir || (!!report && !report.refused && typed === siteName));

  async function toggleDelete(on: boolean) {
    deleteDir = on;
    typed = '';
    if (!on || report || !target) return;
    try {
      report = await previewSiteDirDelete(target.domain);
    } catch {
      error = m.common_failed();
    }
  }

  function safeClose() {
    if (busy) return;
    closeModal();
  }

  async function confirm() {
    if (!target || !canConfirm) return;
    busy = true;
    error = '';
    const domain = target.domain;
    const res = deleteDir ? await unlinkSite(domain, typed) : await unlinkSite(domain);
    busy = false;
    if (!res.ok) {
      error = res.error || m.common_failed();
      return;
    }
    await loadSites();
    closeModal();
    // Leave the route of a site that no longer exists, so the URL and tab title stop naming it.
    if (get(routeRest).split('/')[0] === domain) goToTab('sites');
  }
</script>

<Modal open title={target ? m.sites_unlinkTitle({ domain: target.domain }) : ''} onclose={safeClose} size="sm">
  <div class="px-5 py-4 space-y-3">
    {#if target}
      <p class="text-sm text-gray-700 dark:text-gray-300">{deleteDir ? m.sites_unlinkDeleteBody() : m.sites_unlinkBody()}</p>
      {#if $accessMode.local}
        <label class="flex items-center gap-2 text-xs text-gray-700 dark:text-gray-300">
          <input
            type="checkbox"
            checked={deleteDir}
            disabled={busy}
            onchange={(e) => toggleDelete(e.currentTarget.checked)}
            class="rounded-sm border-gray-300 dark:border-lerd-border"
          />
          {m.sites_unlinkDeleteDir()}
        </label>
      {/if}
      {#if deleteDir && report}
        <div class="space-y-1 text-xs">
          <p class="font-mono text-gray-600 dark:text-gray-400 break-all">{report.path}</p>
          {#if report.refused}
            <p class="text-red-500">{report.refused}</p>
          {:else}
            {#each report.inside_worktrees ?? [] as wt (wt)}
              <p class="text-gray-600 dark:text-gray-400">{m.sites_unlinkDeleteWorktree({ path: wt })}</p>
            {/each}
            {#each report.outside_worktrees ?? [] as wt (wt)}
              <p class="text-amber-600 dark:text-amber-400">{m.sites_unlinkDeleteOutsideWorktree({ path: wt })}</p>
            {/each}
            {#each report.uncommitted ?? [] as dir (dir)}
              <p class="text-amber-600 dark:text-amber-400">{m.sites_unlinkDeleteUncommitted({ path: dir })}</p>
            {/each}
            {#if report.unpushed}
              <p class="text-amber-600 dark:text-amber-400">{m.sites_unlinkDeleteUnpushed({ count: report.unpushed })}</p>
            {/if}
            {#each report.unchecked ?? [] as dir (dir)}
              <p class="text-amber-600 dark:text-amber-400">{m.sites_unlinkDeleteUnchecked({ path: dir })}</p>
            {/each}
            <label class="block pt-1 space-y-1 text-gray-700 dark:text-gray-300">
              <span>{m.sites_unlinkDeleteType({ name: siteName })}</span>
              <input
                type="text"
                bind:value={typed}
                disabled={busy}
                autocomplete="off"
                spellcheck="false"
                class="w-full text-sm font-mono bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
              />
            </label>
          {/if}
        </div>
      {/if}
      {#if error}
        <p class="text-xs text-red-500">{error}</p>
      {/if}
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={busy}>{m.common_cancel()}</DetailButton>
    {#if target}
      <DetailButton tone="danger" onclick={confirm} loading={busy} disabled={busy || !canConfirm}>
        {deleteDir ? m.sites_unlinkAndDelete() : m.sites_unlink()}
      </DetailButton>
    {/if}
  {/snippet}
</Modal>
