<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import PlanReview from '$components/PlanReview.svelte';
  import PlanProgress from '$components/PlanProgress.svelte';
  import { closeModal } from '$stores/modals';
  import { pullPlan, streamPull, type BranchPlan } from '$stores/worktree';
  import { restoreSnapshot } from '$stores/databases';
  import { planRows, defaultSteps, type RowKey } from '$lib/planRows';
  import { m } from '../paraglide/messages.js';

  interface Props {
    domain: string;
    // '' is the main checkout; otherwise the worktree's branch as lerd lists it.
    branch: string;
    branchLabel: string;
    onDone?: () => void;
  }
  let { domain, branch, branchLabel, onDone = () => {} }: Props = $props();

  let plan = $state<BranchPlan | null>(null);
  let planning = $state(true);
  let on = $state<Record<RowKey, boolean>>({ snapshot: false, restore: false, composer: false, js: false, migrate: false });
  let pulling = $state(false);
  let finished = $state(false);
  let error = $state('');
  let logs = $state<string[]>([]);
  let failedSnapshot = $state('');
  let restoring = $state(false);
  let restored = $state('');

  const rows = $derived(plan ? planRows(plan, branchLabel) : []);
  const conflicts = $derived(plan?.conflicts ?? []);

  onMount(async () => {
    try {
      const p = await pullPlan(domain, branch);
      if (p.error) throw new Error(p.error);
      plan = p;
      on = defaultSteps(p);
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_failed();
    } finally {
      planning = false;
    }
  });

  async function doPull() {
    pulling = true;
    error = '';
    logs = [];
    let done: { ok?: boolean; error?: string; snapshot?: string } = { ok: false };
    try {
      await streamPull(domain, branch, { composer: on.composer, js: on.js, migrate: on.migrate, snapshot: on.snapshot }, (ev) => {
        if (ev.done) done = ev;
        else if (ev.line) logs = [...logs, ev.line];
      });
    } catch (e) {
      done = { ok: false, error: e instanceof Error ? e.message : m.common_failed() };
    }
    pulling = false;
    onDone();
    if (done.ok) {
      closeModal();
      return;
    }
    error = done.error || m.sites_gitPullFailed();
    failedSnapshot = done.snapshot ?? '';
    finished = true;
  }

  async function restoreFailed() {
    const db = plan?.db;
    if (!db || !failedSnapshot) return;
    restoring = true;
    const res = await restoreSnapshot(db.service, db.database, failedSnapshot);
    restoring = false;
    if (res.ok) restored = failedSnapshot;
    else error = res.error || m.common_failed();
  }
</script>

<!-- Closing mid-pull would drop its error and the snapshot restore it offers. -->
<Modal open title={m.gitSync_pullTitle({ branch: branchLabel })} onclose={() => (pulling ? undefined : closeModal())} size="lg">
  {#if !pulling && !finished}
    <div class="px-5 py-4 space-y-4">
      {#if planning}
        <div class="flex items-center justify-center gap-2 h-40 text-sm text-gray-500 dark:text-gray-400" role="status">
          <svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
          </svg>
          {m.pullPlan_comparing()}
        </div>
      {:else if plan && plan.behind === 0}
        <p class="text-sm text-gray-500 dark:text-gray-400">{m.sites_gitUpToDate()}</p>
      {:else if plan}
        <PlanReview
          {plan}
          {rows}
          bind:on
          conflictsText={m.pullPlan_conflicts()}
          thenText={m.pullPlan_then()}
          missingText={m.pullPlan_missingMigrations({ count: plan.migrations_missing })}
        />
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.gitSync_pullBody({ branch: branchLabel })}</p>
      {/if}
      {#if error}<p class="text-xs text-red-500 whitespace-pre-line">{error}</p>{/if}
    </div>
  {:else}
    <PlanProgress
      error={finished ? error : ''}
      note={finished && failedSnapshot
        ? restored ? m.branchSwitch_restored({ name: restored }) : m.pullPlan_failedWithSnapshot({ name: failedSnapshot })
        : ''}
      {logs}
    />
  {/if}

  {#snippet footer()}
    {#if finished}
      <DetailButton onclick={closeModal}>{m.common_close()}</DetailButton>
      {#if failedSnapshot && !restored}
        <DetailButton tone="primary" onclick={restoreFailed} loading={restoring} disabled={restoring}>{m.branchSwitch_restoreSnapshot()}</DetailButton>
      {/if}
    {:else if pulling}
      <DetailButton tone="primary" disabled loading={true}>{m.pullPlan_pulling()}</DetailButton>
    {:else}
      <DetailButton onclick={closeModal}>{m.common_cancel()}</DetailButton>
      <DetailButton tone="primary" onclick={doPull} disabled={planning || !plan || plan.behind === 0 || conflicts.length > 0}
        >{m.gitSync_pull()}</DetailButton
      >
    {/if}
  {/snippet}
</Modal>
