<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import BuildLog from '$components/BuildLog.svelte';
  import Toggle from '$components/Toggle.svelte';
  import Icon, { type IconName } from '$components/Icon.svelte';
  import { closeModal } from '$stores/modals';
  import { loadSites, type Site } from '$stores/sites';
  import {
    worktreeOptions,
    branchPlan,
    streamBranchSwitch,
    type BranchPlan,
    type PlanStep
  } from '$stores/worktree';
  import { restoreSnapshot } from '$stores/databases';
  import { branchOptions } from '$lib/branchOptions';
  import { relativeTime } from '$lib/relativeTime';
  import { m } from '../paraglide/messages.js';

  let { site }: { site: Site } = $props();

  let local = $state<string[]>([]);
  let remote = $state<string[]>([]);
  let branch = $state('');
  let loading = $state(true);
  let plan = $state<BranchPlan | null>(null);
  let planning = $state(false);
  type RowKey = 'snapshot' | 'restore' | 'composer' | 'js' | 'migrate';
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
  const schemaDiffers = $derived(!!plan && plan.migrations_added + plan.migrations_missing > 0);

  type Row = { key: RowKey; icon: IconName; label: string; mono: boolean; reason: string; needed: boolean };
  // Listed in the order they run: snapshot, switch, restore, installs, migrate.
  const rows = $derived.by<Row[]>(() => {
    if (!plan) return [];
    const out: Row[] = [];
    const db = plan.db;
    if (db) {
      out.push({ key: 'snapshot', icon: 'camera', label: m.branchSwitch_snapshot(), mono: false, needed: schemaDiffers,
        reason: m.branchSwitch_snapshotWhy({ database: db.database, branch: site.branch || '' }) });
      // A branch being created has no copy of its own to go back to.
      if (db.restore && !creating) {
        const when = relativeTime(Date.parse(db.restore.created), Date.now());
        out.push({ key: 'restore', icon: 'clock', label: m.branchSwitch_restore({ branch }), mono: false, needed: false,
          reason: m.branchSwitch_restoreWhen({ when }) });
      }
    }
    const cmd = (key: RowKey, icon: IconName, s?: PlanStep) => s && out.push({ key, icon, label: s.label, mono: true, needed: s.needed, reason: reason(key, s) });
    cmd('composer', 'cube', plan.composer);
    cmd('js', 'download', plan.js);
    cmd('migrate', 'database', plan.migrate);
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
      // A copy of the database is worth taking whenever the schema will move.
      const moves = p.migrations_added + p.migrations_missing > 0;
      on = { snapshot: !!p.db && moves, restore: false, composer: !!p.composer?.needed, js: !!p.js?.needed, migrate: !!p.migrate?.needed };
    } catch (e) {
      if (seq !== planSeq) return;
      plan = null;
      error = e instanceof Error ? e.message : m.common_failed();
    } finally {
      if (seq === planSeq) planning = false;
    }
  }

  function reason(key: RowKey, s: PlanStep): string {
    if (key === 'migrate') return plan?.migrations_added ? m.branchSwitch_newMigrations({ count: plan.migrations_added }) : m.branchSwitch_upToDate();
    if (s.changed) return m.branchSwitch_changed({ file: s.changed });
    if (s.missing) return m.branchSwitch_notInstalled();
    return m.branchSwitch_upToDate();
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

<Modal open title={m.sites_switchBranch()} onclose={closeModal} size="lg">
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
          {#if conflicts.length > 0}
            <div class="flex items-start gap-2.5 rounded-lg border border-red-200 dark:border-red-500/30 bg-red-50 dark:bg-red-500/10 px-3 py-2.5 text-xs leading-relaxed text-red-700 dark:text-red-300" role="alert">
              <Icon name="alert" class="w-4 h-4 shrink-0 mt-px" />
              <div class="min-w-0">
                <p>{m.branchSwitch_conflicts({ branch: creating ? base || site.branch || 'HEAD' : branch })}</p>
                <ul class="mt-1.5 space-y-0.5 font-mono">
                  {#each conflicts.slice(0, 6) as f (f)}<li class="truncate">{f}</li>{/each}
                </ul>
                {#if conflicts.length > 6}<p class="mt-1">{m.branchSwitch_conflictsMore({ count: conflicts.length - 6 })}</p>{/if}
              </div>
            </div>
          {/if}
          {#if plan.behind + plan.ahead + plan.files > 0}
          <div class="grid grid-cols-3 gap-2">
            {#each [[m.branchSwitch_statBehind(), plan.behind], [m.branchSwitch_statAhead(), plan.ahead], [m.branchSwitch_statFiles(), plan.files]] as [label, n] (label)}
              <div class="rounded-lg bg-gray-50 dark:bg-white/[0.03] border border-gray-100 dark:border-lerd-border px-3 py-2">
                <div class="text-lg font-semibold tabular-nums text-gray-800 dark:text-gray-100">{n}</div>
                <div class="text-[11px] text-gray-500 dark:text-gray-400">{label}</div>
              </div>
            {/each}
          </div>
          {/if}

          {#if rows.length > 0}
            <div class="space-y-1.5">
              <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{m.branchSwitch_then()}</div>
              <div class="rounded-lg border border-gray-200 dark:border-lerd-border divide-y divide-gray-100 dark:divide-lerd-border">
                {#each rows as r (r.key)}
                  <div class="flex items-center gap-3 px-3 py-2.5">
                    <span class="w-8 h-8 shrink-0 rounded-md flex items-center justify-center {r.needed
                      ? 'bg-lerd-red/10 text-lerd-red'
                      : 'bg-gray-100 dark:bg-white/5 text-gray-400 dark:text-gray-500'}">
                      <Icon name={r.icon} class="w-4 h-4" />
                    </span>
                    <div class="flex-1 min-w-0">
                      <div class="{r.mono ? 'font-mono' : ''} text-sm text-gray-800 dark:text-gray-100 truncate">{r.label}</div>
                      <div class="text-xs {r.needed ? 'text-lerd-red' : 'text-gray-400 dark:text-gray-500'}">{r.reason}</div>
                    </div>
                    <Toggle on={on[r.key]} title={r.label} onclick={() => (on[r.key] = !on[r.key])} />
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          {#if plan.migrations_missing > 0}
            <div class="flex items-start gap-2.5 rounded-lg border border-amber-200 dark:border-amber-500/30 bg-amber-50 dark:bg-amber-500/10 px-3 py-2.5 text-xs leading-relaxed text-amber-700 dark:text-amber-300">
              <Icon name="alert" class="w-4 h-4 shrink-0 mt-px" />
              <span>{m.branchSwitch_missingMigrations({ count: plan.migrations_missing })}</span>
            </div>
          {/if}
        {/if}
        <p class="text-xs text-gray-500 dark:text-gray-400">{m.branchSwitch_hint()}</p>
      {/if}
      {#if error}<p class="text-xs text-red-500 whitespace-pre-line">{error}</p>{/if}
    </div>
  {:else}
    <div class="px-5 py-3 space-y-2">
      {#if finished && error}
        <div class="rounded-lg border border-red-200 dark:border-red-500/30 bg-red-50 dark:bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-300 whitespace-pre-line">
          {error}
        </div>
      {/if}
      {#if finished && failedSnapshot}
        <div class="rounded-lg border border-gray-200 dark:border-lerd-border px-3 py-2.5 text-xs text-gray-700 dark:text-gray-300">
          {restored ? m.branchSwitch_restored({ name: restored }) : m.branchSwitch_failedWithSnapshot({ name: failedSnapshot })}
        </div>
      {/if}
      {#if logs.length > 0}<BuildLog {logs} />{/if}
    </div>
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
