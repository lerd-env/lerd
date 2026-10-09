import { m } from '../paraglide/messages.js';
import { apiFetch, apiJson } from '$lib/api';
import { readSSE } from '$lib/sse';

export interface LabeledOption {
  value: string;
  label: string;
}

export interface WorktreeOptions {
  local_branches: string[];
  remote_branches: string[];
  default_branch_label: string;
  branch_dates?: Record<string, number>;
  // Every branch, for a new branch's base: includes ones open in a worktree.
  base_local_branches?: string[];
  base_remote_branches?: string[];
  build_options: LabeledOption[];
  build_default: string;
  db_options: LabeledOption[];
  can_migrate: boolean;
  error?: string;
}

export async function worktreeOptions(domain: string, branch = ''): Promise<WorktreeOptions> {
  const qs = new URLSearchParams({ domain });
  if (branch) qs.set('branch', branch);
  return apiJson<WorktreeOptions>('/api/sites/worktree-options?' + qs.toString());
}

export interface RemoveWorktreeOpts {
  force?: boolean;
  dropDB?: boolean;
}

export async function removeWorktree(
  domain: string,
  branch: string,
  opts: RemoveWorktreeOpts = {}
): Promise<{ ok: boolean; error?: string }> {
  const qs = new URLSearchParams({ branch });
  if (opts.force) qs.set('force', '1');
  if (opts.dropDB) qs.set('drop_db', '1');
  try {
    const res = await apiFetch(
      `/api/sites/${encodeURIComponent(domain)}/worktree:remove?` + qs.toString(),
      { method: 'POST' }
    );
    const data = (await res.json()) as { ok?: boolean; error?: string };
    return { ok: Boolean(data.ok), error: data.error };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : m.common_requestFailed() };
  }
}

export interface AddWorktreeParams {
  newBranch?: string;
  existingBranch?: string;
  baseRef?: string;
  db?: string;
  migrate?: boolean;
  build?: string;
}

export interface WorktreeAddEvent {
  line?: string;
  done?: boolean;
  ok?: boolean;
  branch?: string;
  domain?: string;
  error?: string;
  warnings?: string[];
}

// streamWorktreeAdd POSTs to the SSE endpoint and invokes onEvent for each
// progress line and the final done payload. Mirrors streamLinkSite.
export async function streamWorktreeAdd(
  domain: string,
  params: AddWorktreeParams,
  onEvent: (e: WorktreeAddEvent) => void
): Promise<void> {
  const qs = new URLSearchParams({ domain });
  if (params.newBranch) qs.set('new_branch', params.newBranch);
  if (params.existingBranch) qs.set('existing_branch', params.existingBranch);
  if (params.baseRef) qs.set('base_ref', params.baseRef);
  if (params.db) qs.set('db', params.db);
  if (params.migrate) qs.set('migrate', '1');
  if (params.build) qs.set('build', params.build);

  const res = await apiFetch('/api/sites/worktree-add?' + qs.toString(), { method: 'POST' });
  await readSSE(res, (event, data) => {
    if (event === 'done') {
      try {
        const r = JSON.parse(data) as {
          ok?: boolean;
          branch?: string;
          domain?: string;
          error?: string;
          warnings?: string[];
        };
        onEvent({
          done: true,
          ok: Boolean(r.ok),
          branch: r.branch,
          domain: r.domain,
          error: r.error,
          warnings: Array.isArray(r.warnings) ? r.warnings : []
        });
      } catch {
        onEvent({ done: true, ok: false, error: 'bad done payload' });
      }
    } else {
      onEvent({ line: data });
    }
  });
}

export interface PlanStep {
  label: string;
  needed: boolean;
  changed?: string;
  missing?: boolean;
}

export interface BranchPlan {
  ahead: number;
  behind: number;
  files: number;
  composer?: PlanStep;
  js?: PlanStep;
  migrate?: PlanStep;
  migrations_added: number;
  migrations_missing: number;
  conflicts?: string[];
  db?: { service: string; database: string; restore?: { name: string; created: string } };
  error?: string;
}

export async function branchPlan(domain: string, branch: string): Promise<BranchPlan> {
  return apiJson<BranchPlan>('/api/sites/branch-plan?' + new URLSearchParams({ domain, branch }).toString());
}

export interface BranchSteps {
  composer: boolean;
  js: boolean;
  migrate: boolean;
  snapshot: boolean;
  // A snapshot to load once the branch is checked out, or '' for none.
  restore: string;
  // Create makes the branch new, starting at base ('' for the current commit).
  create?: boolean;
  base?: string;
}

export interface BranchSwitchEvent {
  line?: string;
  done?: boolean;
  ok?: boolean;
  error?: string;
  // The snapshot taken before switching, offered back if a later step failed.
  snapshot?: string;
}

// streamBranchSwitch checks branch out in the site's main checkout and runs
// the chosen steps, streaming their output.
export async function streamBranchSwitch(
  domain: string,
  branch: string,
  steps: BranchSteps,
  onEvent: (e: BranchSwitchEvent) => void
): Promise<void> {
  const qs = stepsQuery(domain, branch, steps);
  if (steps.restore) qs.set('restore', steps.restore);
  if (steps.create) qs.set('create', '1');
  if (steps.base) qs.set('base', steps.base);
  await streamSteps('/api/sites/branch-switch?' + qs.toString(), onEvent);
}

// pullPlan fetches and compares a checkout ('' for the main one, else the
// worktree's branch) with its upstream, the same way a switch is planned.
export async function pullPlan(domain: string, branch: string): Promise<BranchPlan> {
  return apiJson<BranchPlan>('/api/sites/pull-plan?' + new URLSearchParams({ domain, branch }).toString());
}

// streamPull fast-forwards the checkout and runs the chosen steps, streaming their output.
export async function streamPull(
  domain: string,
  branch: string,
  steps: Omit<BranchSteps, 'restore'>,
  onEvent: (e: BranchSwitchEvent) => void
): Promise<void> {
  await streamSteps('/api/sites/pull?' + stepsQuery(domain, branch, steps).toString(), onEvent);
}

function stepsQuery(domain: string, branch: string, steps: Omit<BranchSteps, 'restore'>): URLSearchParams {
  const qs = new URLSearchParams({ domain, branch });
  for (const k of ['composer', 'js', 'migrate', 'snapshot'] as const) if (steps[k]) qs.set(k, '1');
  return qs;
}

async function streamSteps(url: string, onEvent: (e: BranchSwitchEvent) => void): Promise<void> {
  const res = await apiFetch(url, { method: 'POST' });
  await readSSE(res, (event, data) => {
    if (event !== 'done') {
      onEvent({ line: data });
      return;
    }
    try {
      const r = JSON.parse(data) as { ok?: boolean; error?: string; snapshot?: string };
      onEvent({ done: true, ok: Boolean(r.ok), error: r.error, snapshot: r.snapshot || undefined });
    } catch {
      onEvent({ done: true, ok: false, error: 'bad done payload' });
    }
  });
}
