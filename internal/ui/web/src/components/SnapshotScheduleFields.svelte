<script lang="ts">
  import Toggle from '$components/Toggle.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import SegmentedControl from '$components/SegmentedControl.svelte';
  import { autoSnapshot, saveAutoSnapshot, nextSnapshotAt } from '$stores/autoSnapshot';
  import { intervalParts, intervalDuration, type SnapshotUnit } from '$lib/snapshots';
  import { m } from '../paraglide/messages.js';

  // The schedule and its retention, shared by the System panel and the settings
  // modal so the one policy is edited through one set of controls.
  const { amount, unit } = $derived(intervalParts($autoSnapshot.every));

  const unitOptions = $derived(
    [
      { value: 'hour', label: amount === 1 ? m.snapshots_unitHour() : m.snapshots_unitHours() },
      { value: 'day', label: amount === 1 ? m.snapshots_unitDay() : m.snapshots_unitDays() },
      { value: 'week', label: amount === 1 ? m.snapshots_unitWeek() : m.snapshots_unitWeeks() },
      { value: 'month', label: amount === 1 ? m.snapshots_unitMonth() : m.snapshots_unitMonths() }
    ]
  );

  function applyInterval(count: number, nextUnit: SnapshotUnit) {
    apply({ every: intervalDuration(count, nextUnit) });
  }
  const keepForOptions = [
    { value: '', label: m.snapshots_auto_keepForNever() },
    { value: '168h', label: m.snapshots_auto_keepForDays({ count: 7 }) },
    { value: '720h', label: m.snapshots_auto_keepForDays({ count: 30 }) }
  ];
  const keepOptions = ['3', '5', '7', '10', '20'];

  // Go prints a whole-hour duration as "24h0m0s"; the pickers speak "24h".
  function normalizeDuration(v: string): string {
    return v.replace(/^(\d+)h0m0s$/, '$1h');
  }
  const every = $derived(normalizeDuration($autoSnapshot.every));
  const keepFor = $derived($autoSnapshot.keep_for ? normalizeDuration($autoSnapshot.keep_for) : '');

  const nextRun = $derived(nextSnapshotAt($autoSnapshot));
  const covers = $derived($autoSnapshot.sites.some((s) => s.covered));

  let error = $state('');

  async function apply(
    patch: Partial<{
      enabled: boolean;
      every: string;
      keep: number;
      keep_for: string;
      selection: 'opt_in' | 'opt_out';
    }>
  ) {
    error = '';
    const res = await saveAutoSnapshot({
      enabled: $autoSnapshot.enabled,
      every,
      keep: $autoSnapshot.keep,
      keep_for: keepFor,
      selection: $autoSnapshot.selection,
      ...patch
    });
    if (!res.ok) error = res.error || m.common_failed();
  }
</script>

<div class="flex items-center justify-between gap-3 mb-2">
  <span class="text-sm font-semibold text-gray-700 dark:text-gray-300">{m.snapshots_auto_title()}</span>
  <Toggle
    on={$autoSnapshot.enabled}
    title={m.snapshots_auto_title()}
    onclick={() => apply({ enabled: !$autoSnapshot.enabled })}
  />
</div>
<p class="text-xs text-gray-500 dark:text-gray-400">{m.snapshots_auto_description()}</p>

{#if $autoSnapshot.enabled && covers}
  <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
    {m.snapshots_auto_nextRun()}:
    <span class="text-gray-700 dark:text-gray-300">
      {nextRun
        ? nextRun.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
        : m.snapshots_auto_nextCheck()}
    </span>
  </p>
{/if}

<div class="mt-3 flex flex-wrap items-center justify-between gap-2">
  <div class="min-w-0">
    <p class="text-xs font-medium text-gray-600 dark:text-gray-400">{m.snapshots_auto_selection()}</p>
    <p class="text-[11px] text-gray-400 dark:text-gray-500">
      {$autoSnapshot.selection === 'opt_in'
        ? m.snapshots_auto_selectionOptInHint()
        : m.snapshots_auto_selectionOptOutHint()}
    </p>
  </div>
  <SegmentedControl
    label={m.snapshots_auto_selection()}
    value={$autoSnapshot.selection}
    options={[
      { value: 'opt_in', label: m.snapshots_auto_selectionOptIn() },
      { value: 'opt_out', label: m.snapshots_auto_selectionOptOut() }
    ]}
    onchange={(v) => apply({ selection: v })}
  />
</div>

<div class="mt-3 grid gap-3 sm:grid-cols-3">
  <div class="space-y-1 text-xs text-gray-500 dark:text-gray-400">
    <span>{m.snapshots_auto_every()}</span>
    <!-- One control group: the count and its unit share a border, so they read
         as "every 3 days" rather than as two unrelated fields. -->
    <div class="flex items-stretch">
      <input
        type="number"
        min="1"
        max="999"
        value={amount}
        aria-label={m.snapshots_auto_every()}
        onchange={(e) => applyInterval(Number(e.currentTarget.value), unit)}
        class="h-7 w-14 shrink-0 rounded-md rounded-r-none border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-2 text-xs font-medium tabular-nums text-gray-700 dark:text-gray-200 focus:outline-none focus:border-lerd-red"
      />
      <Dropdown
        value={unit}
        options={unitOptions}
        width="full"
        joined
        onchange={(v) => applyInterval(amount, v as SnapshotUnit)}
      />
    </div>
  </div>
  <div class="space-y-1 text-xs text-gray-500 dark:text-gray-400">
    <span>{m.snapshots_auto_keep()}</span>
    <Dropdown
      value={String($autoSnapshot.keep)}
      options={keepOptions}
      width="full"
      onchange={(v) => apply({ keep: Number(v) })}
    />
  </div>
  <div class="space-y-1 text-xs text-gray-500 dark:text-gray-400">
    <span>{m.snapshots_auto_keepFor()}</span>
    <Dropdown value={keepFor} options={keepForOptions} width="full" onchange={(v) => apply({ keep_for: v })} />
  </div>
</div>

{#if error}
  <p class="text-xs text-red-500 mt-2">{error}</p>
{/if}
