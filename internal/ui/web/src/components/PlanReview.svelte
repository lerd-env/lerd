<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import Toggle from '$components/Toggle.svelte';
  import SyncStats from '$components/SyncStats.svelte';
  import type { BranchPlan } from '$stores/worktree';
  import type { PlanRow, RowKey } from '$lib/planRows';

  // What a switch or pull would change and the steps to run after it. The
  // wording that differs between the two comes in from the dialog.
  interface Props {
    plan: BranchPlan;
    rows: PlanRow[];
    on: Record<RowKey, boolean>;
    thenText: string;
    missingText: string;
    // Makes the files card open the full list of changed files.
    onfiles?: () => void;
    filesTitle?: string;
  }
  let { plan, rows, on = $bindable(), thenText, missingText, onfiles, filesTitle }: Props = $props();
</script>

{#if plan.behind + plan.ahead + plan.files > 0}
  <SyncStats behind={plan.behind} ahead={plan.ahead} files={plan.files} {onfiles} {filesTitle} />
{/if}

{#if rows.length > 0}
  <div class="space-y-1.5">
    <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{thenText}</div>
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
    <span>{missingText}</span>
  </div>
{/if}
