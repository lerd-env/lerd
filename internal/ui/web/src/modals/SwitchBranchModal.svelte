<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import PlanReview from '$components/PlanReview.svelte';
  import PlanProgress from '$components/PlanProgress.svelte';
  import { closeModal } from '$stores/modals';
  import { loadSites, type Site } from '$stores/sites';
  import {
    worktreeOptions,
    branchPlan,
    streamBranchSwitch,
    type BranchPlan
  } from '$stores/worktree';
  import { restoreSnapshot } from '$stores/databases';
  import { branchOptions } from '$lib/branchOptions';
  import { relativeTime } from '$lib/relativeTime';
  import { planRows, defaultSteps, type PlanRow, type RowKey } from '$lib/planRows';
  import { m } from '../paraglide/messages.js';

  let { site }: { site: Site } = $props();

  let local = $state<string[]>([]);
  let remote = $state<string[]>([]);
  let branch = $state('');
  let loading = $state(true);
  let plan = $state<BranchPlan | null>(null);
  let planning = $state(false);
  let on = $state<Record<RowKey, boolean>>({ snapshot: false, restore: false, composer: false, js: false, migrate: false });
  let mode = $state<'existing' | 'new'>('existing');
  let newName = $state('');
  // Where a new branch starts; '' is the commit checked out now.
  let base = $state('');
  let failedSnapshot = $state('');
  let restoring = $state(false);
  let restored = $state('');
  let switching = $state(false);
  let finished = $state(false);
  let error = $state('');
  let logs = $state<string[]>([]);
  let dates = $state<Record<string, number>>({});
  let baseLocal = $state<string[]>([]);
  let baseRemote = $state<string[]>([]);
  let planSeq = 0;

  const options = $derived(branchOptions(local, remote, dates));
  const baseOptions = $derived([
    { value: '', label: m.worktreeMgr_currentBranch({ branch: site.branch || 'HEAD' }), description: '' },
    ...branchOptions(baseLocal.filter((b) => b !== site.branch), baseRemote, dates)
  ]);
  const creating = $derived(mode === 'new');
  const target = $derived(creating ? newName.trim() : branch);
  const nameTaken = $derived(creating && local.includes(newName.trim()));
  const conflicts = $derived(plan?.conflicts ?? []);

  const rows = $derived.by<PlanRow[]>(() => {
    if (!plan) return [];
    // A branch being created has no copy of its own to go back to.
    const snap = !creating ? plan.db?.restore : undefined;
    const restore: PlanRow | undefined = snap && {
      key: 'restore', icon: 'clock', label: m.branchSwitch_restore({ branch }), mono: false, needed: false,
      reason: m.branchSwitch_restoreWhen({ when: relativeTime(Date.parse(snap.created), Date.now()) })
    };
    const out = planRows(plan, site.branch || '', restore);
    // Branching off the current commit changes nothing, so only what is
    // genuinely due (an install never done here) is worth a row.
    return creating && !base ? out.filter((r) => r.needed) : out;
  });

  function pick(b: string) {
    branch = b;
    void loadPlan(b);
  }

  // A new branch starts where its base is, so that is what it is compared with.
  function pickBase(b: string) {
    base = b;
    void loadPlan(b || 'HEAD');
  }

  function setMode(next: 'existing' | 'new') {
    mode = next;
    if (next === 'new') void loadPlan(base || 'HEAD');
    else if (branch) void loadPlan(branch);
  }

  // Each pick re-plans; a slower answer for an earlier pick is dropped.
  async function loadPlan(ref: string) {
    const seq = ++planSeq;
    planning = true;
    error = '';
    try {
      const p = await branchPlan(site.domain, ref);
      if (seq !== planSeq) return;
      if (p.error) throw new Error(p.error);
      plan = p;
      on = defaultSteps(p);
    } catch (e) {
      if (seq !== planSeq) return;
      plan = null;
      error = e instanceof Error ? e.message : m.common_failed();
    } finally {
      if (seq === planSeq) planning = false;
    }
  }

  onMount(async () => {
    try {
      const o = await worktreeOptions(site.domain);
      local = o.local_branches ?? [];
      remote = o.remote_branches ?? [];
      dates = o.branch_dates ?? {};
      baseLocal = o.base_local_branches ?? [];
      baseRemote = o.base_remote_branches ?? [];
      const first = branchOptions(local, remote, dates)[0]?.value;
      if (first) void pick(first);
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_failed();
    } finally {
      loading = false;
    }
  });

  async function doSwitch() {
    switching = true;
    error = '';
    logs = [];
    let done: { ok?: boolean; error?: string; snapshot?: string } = { ok: false };
    const steps = {
      composer: on.composer,
      js: on.js,
      migrate: on.migrate,
      snapshot: on.snapshot,
      restore: on.restore && !creating ? plan?.db?.restore?.name ?? '' : '',
      create: creating,
      base: creating ? base : ''
    };
    try {
      await streamBranchSwitch(site.domain, target, steps, (ev) => {
        if (ev.done) done = ev;
        else if (ev.line) logs = [...logs, ev.line];
      });
    } catch (e) {
      done = { ok: false, error: e instanceof Error ? e.message : m.common_failed() };
    }
    switching = false;
    await loadSites();
    if (done.ok) {
      closeModal();
      return;
    }
    error = done.error || m.branchSwitch_failed();
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

<!-- Closing mid-switch would drop its error and the snapshot restore it offers. -->
<Modal open title={m.sites_switchBranch()} onclose={() => (switching ? undefined : closeModal())} size="lg">
  {#if !switching && !finished}
    <div class="px-5 py-4 space-y-4">
      <div class="inline-flex rounded-md bg-gray-100 dark:bg-white/5 p-0.5 text-xs" role="tablist">
        {#each [['existing', m.worktreeMgr_existingBranchOpt()], ['new', m.worktreeMgr_newBranchOpt()]] as [v, label] (v)}
          <button
            type="button"
            role="tab"
            aria-selected={mode === v}
            onclick={() => setMode(v as 'existing' | 'new')}
            class="px-3 py-1 rounded-sm transition-colors {mode === v
              ? 'bg-white dark:bg-white/10 text-gray-800 dark:text-gray-100 shadow-sm dark:shadow-none'
              : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'}">{label}</button
          >
        {/each}
      </div>
      {#if !creating && !loading && options.length === 0}
        <p class="text-sm text-gray-500 dark:text-gray-400">{m.branchSwitch_noBranches()}</p>
      {:else}
        {#if creating}
          <div class="space-y-2">
            <input
              type="text"
              bind:value={newName}
              placeholder={m.worktreeMgr_branchNamePlaceholder()}
              aria-label={m.worktreeMgr_newBranchOpt()}
              class="w-full text-sm font-mono bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-md px-2.5 py-1.5 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red/50"
            />
            {#if nameTaken}<p class="text-xs text-red-500">{m.branchSwitch_nameTaken({ name: newName.trim() })}</p>{/if}
            <div class="text-xs text-gray-500 dark:text-gray-400">{m.worktreeMgr_basedOn()}</div>
            <Dropdown value={base} width="full" options={baseOptions} onchange={pickBase} searchable />
          </div>
        {:else}
          <Dropdown value={branch} width="full" {options} onchange={pick} searchable />
        {/if}

        {#if planning}
          <div class="flex items-center justify-center gap-2 h-40 text-sm text-gray-500 dark:text-gray-400" role="status">
            <svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
            </svg>
            {m.branchSwitch_comparing()}
          </div>
        {:else if plan}
          <PlanReview
            {plan}
            {rows}
            bind:on
            conflictsText={m.branchSwitch_conflicts({ branch: creating ? base || site.branch || 'HEAD' : branch })}
            thenText={m.branchSwitch_then()}
            missingText={m.branchSwitch_missingMigrations({ count: plan.migrations_missing })}
          />
        {/if}
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.branchSwitch_hint()}</p>
      {/if}
      {#if error}<p class="text-xs text-red-500 whitespace-pre-line">{error}</p>{/if}
    </div>
  {:else}
    <PlanProgress
      error={finished ? error : ''}
      note={finished && failedSnapshot
        ? restored ? m.branchSwitch_restored({ name: restored }) : m.branchSwitch_failedWithSnapshot({ name: failedSnapshot })
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
    {:else if switching}
      <DetailButton tone="primary" disabled loading={true}>{m.branchSwitch_switching()}</DetailButton>
    {:else}
      <DetailButton onclick={closeModal}>{m.common_cancel()}</DetailButton>
      <DetailButton
        tone="primary"
        onclick={doSwitch}
        disabled={loading || planning || !plan || conflicts.length > 0 || !target || nameTaken}
        >{creating ? m.branchSwitch_create() : m.branchSwitch_switch()}</DetailButton
      >
    {/if}
  {/snippet}
</Modal>
