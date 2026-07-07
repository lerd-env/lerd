<script lang="ts">
  import { dashboardIconSvg } from '$lib/dashboardIcons';
  import { categoryOf, type CategoryKey } from '$lib/presetCategories';
  import { services, serviceLabel } from '$stores/services';
  import { goToTab } from '$stores/route';
  import { m } from '../../paraglide/messages.js';

  // Per-category icon tints, mirrored from the services PresetCard so a service
  // reads the same colour wherever it appears. Full static strings for Tailwind.
  const ICON_TINT: Record<CategoryKey, string> = {
    databases: 'bg-indigo-50 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400',
    cache: 'bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-400',
    messaging: 'bg-violet-50 text-violet-600 dark:bg-violet-500/10 dark:text-violet-400',
    search: 'bg-sky-50 text-sky-600 dark:bg-sky-500/10 dark:text-sky-400',
    mail: 'bg-rose-50 text-rose-600 dark:bg-rose-500/10 dark:text-rose-400',
    admin: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400',
    storage: 'bg-cyan-50 text-cyan-600 dark:bg-cyan-500/10 dark:text-cyan-400',
    testing: 'bg-fuchsia-50 text-fuchsia-600 dark:bg-fuchsia-500/10 dark:text-fuchsia-400',
    other: 'bg-gray-100 text-gray-500 dark:bg-white/5 dark:text-gray-400'
  };

  interface Props {
    name: string;
  }
  let { name }: Props = $props();

  const tint = $derived(ICON_TINT[categoryOf(name)]);
  const active = $derived($services.find((s) => s.name === name)?.status === 'active');
</script>

<button
  type="button"
  onclick={() => goToTab('services', name)}
  title={'Open ' + serviceLabel(name)}
  class="group flex items-center gap-2.5 rounded-lg border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card p-2.5 text-left transition duration-150 hover:border-gray-300 dark:hover:border-white/15 hover:shadow-sm"
>
  <span class="shrink-0 inline-flex items-center justify-center w-8 h-8 rounded-lg {tint}">
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">{@html dashboardIconSvg(name)}</svg>
  </span>
  <span class="min-w-0 flex-1">
    <span class="block text-xs font-semibold text-gray-800 dark:text-gray-100 truncate">{serviceLabel(name)}</span>
    <span class="flex items-center gap-1 text-[10px] text-gray-500 dark:text-gray-400">
      <span class="w-1.5 h-1.5 rounded-full {active ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-gray-600'}"></span>
      {active ? m.common_running() : m.common_stopped()}
    </span>
  </span>
</button>
