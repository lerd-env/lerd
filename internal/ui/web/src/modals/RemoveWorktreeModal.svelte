<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal } from '$stores/modals';
  import { sites, loadSites, type Site } from '$stores/sites';
  import { removeWorktree } from '$stores/worktree';
  import { m } from '../paraglide/messages.js';

  interface Props {
    site: Site;
    branch: string;
  }
  let { site, branch }: Props = $props();

  const cur = $derived($sites.find((s) => s.domain === site.domain) ?? site);
  const wt = $derived((cur.worktrees ?? []).find((w) => w.branch === branch));

  let force = $state(false);
  let dropDB = $state(false);
  let busy = $state(false);
  let error = $state('');

  async function doRemove() {
    if (!wt) return;
    busy = true;
    error = '';
    try {
      const res = await removeWorktree(cur.domain, branch, { force, dropDB });
      if (!res.ok) {
        error = res.error || m.common_failed();
        return;
      }
      await loadSites();
      closeModal();
    } finally {
      busy = false;
    }
  }
</script>

<Modal open title={m.worktreeMgr_removeTitle({ branch })} onclose={closeModal} size="md">
  <div class="px-5 py-4 space-y-3">
    {#if !wt}
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.worktreeMgr_listEmpty()}</p>
    {:else}
      <div class="flex items-center gap-2">
        <svg class="w-3.5 h-3.5 shrink-0 text-violet-400" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24">
          <path d="M6 3v12M15 6a3 3 0 1 0 6 0a3 3 0 1 0-6 0M3 18a3 3 0 1 0 6 0a3 3 0 1 0-6 0M18 9a9 9 0 0 1-9 9" />
        </svg>
        <span class="font-mono text-sm text-gray-800 dark:text-gray-200 truncate">{wt.branch}</span>
        {#if wt.db_isolated}
          <span class="shrink-0 text-[10px] uppercase tracking-wider rounded-sm px-1.5 py-0.5 bg-violet-100 dark:bg-violet-500/15 text-violet-600 dark:text-violet-300">{m.worktreeMgr_isolatedDbBadge()}</span>
        {/if}
        <span class="ml-auto font-mono text-[11px] text-gray-400 truncate" title={wt.domain}>{wt.domain}</span>
      </div>

      <label class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
        <input type="checkbox" bind:checked={force} disabled={busy} class="rounded-sm border-gray-300 dark:border-lerd-border" />
        {m.worktreeMgr_force()}
      </label>

      {#if wt.db_isolated}
        <label class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
          <input type="checkbox" bind:checked={dropDB} disabled={busy} class="rounded-sm border-gray-300 dark:border-lerd-border" />
          {m.worktreeMgr_dropDb({ db: wt.db_database ?? '' })}
        </label>
      {/if}

      {#if error}
        <p class="text-xs text-red-500">{error}</p>
      {/if}
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={closeModal} disabled={busy}>{m.common_cancel()}</DetailButton>
    {#if wt}
      <DetailButton tone="danger" onclick={doRemove} loading={busy} disabled={busy}>
        {busy ? m.worktreeMgr_removing() : m.common_remove()}
      </DetailButton>
    {/if}
  {/snippet}
</Modal>
