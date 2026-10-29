<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import PlanReview from '$components/PlanReview.svelte';
  import PlanProgress from '$components/PlanProgress.svelte';
  import StashRow from '$components/StashRow.svelte';
  import GitCommitList from '$components/GitCommitList.svelte';
  import GitDiffModal from './GitDiffModal.svelte';
  import type { GitCommit } from '$lib/gitDiff';
  import { closeModal } from '$stores/modals';
  import { pullPlan, streamPull, streamPullUndo, type BranchPlan, type StashKept } from '$stores/worktree';
  import { restoreSnapshot } from '$stores/databases';
  import { planRows, defaultSteps, type RowKey } from '$lib/planRows';
  import { m } from '../paraglide/messages.js';

  interface Props {
    domain: string;
    // '' is the main checkout; otherwise the worktree's branch as lerd lists it.
    branch: string;
    branchLabel: string;
    // The checkout's folder, for the incoming commits' diffs.
    path: string;
    onDone?: () => void;
  }
  let { domain, branch, branchLabel, path, onDone = () => {} }: Props = $props();

  let plan = $state<BranchPlan | null>(null);
  let planning = $state(true);
  let on = $state<Record<RowKey, boolean>>({ snapshot: false, isolate: false, restore: false, composer: false, js: false, migrate: false });
  let stash = $state(false);
  let pulling = $state(false);
  let finished = $state(false);
  let error = $state('');
  let logs = $state<string[]>([]);
  let failedSnapshot = $state('');
  let kept = $state<StashKept | null>(null);
  let restoring = $state(false);
  let restored = $state('');
  // The snapshot the pull took, which undoing it loads when migrations ran.
  let pullSnapshot = $state('');
  let moved = $state(false);
  let undoing = $state(false);
  let undoFailed = $state(false);
  // Opens over this dialog, which keeps its plan underneath.
  let viewing = $state<GitCommit | null>(null);
  let viewingFiles = $state(false);

  const rows = $derived(plan ? planRows(plan, branchLabel) : []);
  const conflicts = $derived(plan?.conflicts ?? []);
  // The fetch can show the branch diverged since the tab was last read; git
  // will not fast-forward it, so the pull is held back before any snapshot.
  const diverged = $derived(!!plan && plan.ahead > 0 && plan.behind > 0);

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
    let done: { ok?: boolean; error?: string; snapshot?: string; stash?: StashKept; moved?: boolean } = { ok: false };
    try {
      await streamPull(domain, branch, { branch: plan?.branch, head: plan?.head, target: plan?.target }, { composer: on.composer, js: on.js, migrate: on.migrate, snapshot: on.snapshot, isolate: on.isolate, stash: stash && conflicts.length > 0 }, (ev) => {
        if (ev.done) done = ev;
        else if (ev.line) logs = [...logs, ev.line];
      });
    } catch (e) {
      done = { ok: false, error: e instanceof Error ? e.message : m.common_failed() };
    }
    pulling = false;
    onDone();
    kept = done.stash ?? null;
    pullSnapshot = done.snapshot ?? '';
    moved = Boolean(done.moved);
    if (done.ok && !kept) {
      closeModal();
      return;
    }
    // Kept local changes are worth reading even when every step went through.
    if (!done.ok) {
      error = done.error || m.sites_gitPullFailed();
      failedSnapshot = done.snapshot ?? '';
    }
    finished = true;
  }

  // Back to the commit the pull started from, with the work the stash kept put
  // back on it; the database only goes back when migrations ran on a snapshot.
  async function undoPull() {
    undoing = true;
    error = '';
    logs = [];
    const restore = on.migrate && on.snapshot ? pullSnapshot : '';
    let done: { ok?: boolean; error?: string; stash?: StashKept; moved?: boolean } = { ok: false };
    try {
      await streamPullUndo(
        domain,
        branch,
        { branch: plan?.branch, head: plan?.target, target: plan?.head },
        { composer: on.composer, js: on.js, migrate: on.migrate && !on.snapshot, restore, db: restore ? plan?.db : undefined, stash: kept?.sha ?? '' },
        (ev) => {
          if (ev.done) done = ev;
          else if (ev.line) logs = [...logs, ev.line];
        }
      );
    } catch (e) {
      done = { ok: false, error: e instanceof Error ? e.message : m.common_failed() };
    }
    undoing = false;
    onDone();
    undoFailed = true;
    // Code that never moved back took nothing out of the stash or the database.
    if (done.ok || done.moved || done.stash) kept = done.stash ?? null;
    if (done.ok && !kept) {
      closeModal();
      return;
    }
    if (!done.ok) {
      error = done.error || m.common_failed();
      failedSnapshot = restore || failedSnapshot;
    }
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
<Modal open title={m.gitSync_pullTitle({ branch: branchLabel })} onclose={() => (pulling || undoing ? undefined : closeModal())} size="lg">
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
          thenText={m.pullPlan_then()}
          missingText={m.pullPlan_missingMigrations({ count: plan.migrations_missing })}
          onfiles={plan.target ? () => (viewingFiles = true) : undefined}
          filesTitle={m.gitDiff_incomingOpen()}
        />
        <!-- Read after the plan's fetch, so it lists what the pull will bring. -->
        <GitCommitList {domain} {path} direction="incoming" onopen={(c) => (viewing = c)} />
        {#if diverged}
          <p class="text-xs text-red-500">{m.gitSync_diverged({ ahead: plan.ahead, behind: plan.behind })}</p>
        {:else}
          <p class="text-xs text-gray-500 dark:text-gray-400">{m.gitSync_pullBody({ branch: branchLabel })}</p>
        {/if}
        <!-- Last before the buttons: it is what holds the pull back. -->
        {#if conflicts.length > 0}<StashRow files={conflicts} bind:on={stash} />{/if}
      {/if}
      {#if error}<p class="text-xs text-red-500 whitespace-pre-line">{error}</p>{/if}
    </div>
  {:else}
    <PlanProgress
      error={finished ? error : ''}
      note={finished && failedSnapshot
        ? restored ? m.branchSwitch_restored({ name: restored }) : m.pullPlan_failedWithSnapshot({ name: failedSnapshot })
        : ''}
      stash={finished ? kept : null}
      {logs}
    />
  {/if}

  {#snippet footer()}
    {#if undoing}
      <DetailButton tone="primary" disabled loading={true}>{m.pullPlan_undoing()}</DetailButton>
    {:else if finished && !error}
      <!-- The pull went through, only the stash was kept: closing is the expected way out. -->
      {#if moved && !undoFailed}
        <DetailButton onclick={undoPull}>{m.pullPlan_undo()}</DetailButton>
      {/if}
      <DetailButton tone="primary" onclick={closeModal}>{m.common_close()}</DetailButton>
    {:else if finished}
      <DetailButton onclick={closeModal}>{m.common_close()}</DetailButton>
      {#if failedSnapshot && !restored}
        <DetailButton onclick={restoreFailed} loading={restoring} disabled={restoring}>{m.branchSwitch_restoreSnapshot()}</DetailButton>
      {/if}
      {#if moved && !undoFailed && !restored}
        <DetailButton tone="primary" onclick={undoPull} disabled={restoring}>{m.pullPlan_undo()}</DetailButton>
      {/if}
    {:else if pulling}
      <DetailButton tone="primary" disabled loading={true}>{m.pullPlan_pulling()}</DetailButton>
    {:else if plan && plan.behind === 0}
      <DetailButton tone="primary" onclick={closeModal}>{m.common_ok()}</DetailButton>
    {:else}
      <DetailButton onclick={closeModal}>{m.common_cancel()}</DetailButton>
      <DetailButton tone="primary" onclick={doPull} disabled={planning || !plan || (conflicts.length > 0 && !stash) || diverged}
        >{m.gitSync_pull()}</DetailButton
      >
    {/if}
  {/snippet}
</Modal>
{#if viewing}
  <GitDiffModal {domain} {path} branch={branchLabel} commit={viewing} onclose={() => (viewing = null)} />
{:else if viewingFiles && plan?.target}
  <GitDiffModal {domain} {path} branch={branchLabel} upto={plan.target} onclose={() => (viewingFiles = false)} />
{/if}
