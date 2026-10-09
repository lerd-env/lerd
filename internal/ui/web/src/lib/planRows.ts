import type { IconName } from '$components/Icon.svelte';
import type { BranchPlan, PlanStep } from '$stores/worktree';
import { m } from '../paraglide/messages.js';

export type RowKey = 'snapshot' | 'isolate' | 'restore' | 'composer' | 'js' | 'migrate';
export type PlanRow = { key: RowKey; icon: IconName; label: string; mono: boolean; reason: string; needed: boolean };

export const schemaMoves = (p: BranchPlan) => p.migrations_added + p.migrations_missing > 0;

// planRows lists what to run around a switch or pull, in the order it runs:
// snapshot, worktree copies, then restore when given, installs, migrate.
// branch is the one being left.
export function planRows(p: BranchPlan, branch: string, restore?: PlanRow): PlanRow[] {
  const out: PlanRow[] = [];
  if (p.db) {
    out.push({ key: 'snapshot', icon: 'camera', label: m.branchSwitch_snapshot(), mono: false, needed: schemaMoves(p),
      reason: m.branchSwitch_snapshotWhy({ database: p.db.database, branch }) });
    const shared = p.shared_worktrees ?? [];
    if (shared.length > 0 && p.migrate) {
      out.push({ key: 'isolate', icon: 'branch', label: m.branchSwitch_isolate(), mono: false, needed: p.migrations_added > 0,
        reason: m.branchSwitch_isolateWhy({ branches: shared.join(', '), database: p.db.database }) });
    }
    if (restore) out.push(restore);
  }
  const cmd = (key: RowKey, icon: IconName, s?: PlanStep) => s && out.push({ key, icon, label: s.label, mono: true, needed: s.needed, reason: reason(p, key, s) });
  cmd('composer', 'cube', p.composer);
  cmd('js', 'download', p.js);
  cmd('migrate', 'database', p.migrate);
  return out;
}

// defaultSteps ticks what the diff calls for; a copy of the database is worth
// taking whenever the schema will move.
export function defaultSteps(p: BranchPlan): Record<RowKey, boolean> {
  const isolate = !!p.db && !!p.migrate && (p.shared_worktrees ?? []).length > 0 && p.migrations_added > 0;
  return { snapshot: !!p.db && schemaMoves(p), isolate, restore: false, composer: !!p.composer?.needed, js: !!p.js?.needed, migrate: !!p.migrate?.needed };
}

function reason(p: BranchPlan, key: RowKey, s: PlanStep): string {
  if (key === 'migrate') return p.migrations_added ? m.branchSwitch_newMigrations({ count: p.migrations_added }) : m.branchSwitch_upToDate();
  if (s.changed) return m.branchSwitch_changed({ file: s.changed });
  if (s.missing) return m.branchSwitch_notInstalled();
  return m.branchSwitch_upToDate();
}
