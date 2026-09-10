<script lang="ts">
  import ToggleButton from '$components/ToggleButton.svelte';
  import LogsButton from '$components/LogsButton.svelte';
  import WorkerOptionsModal from './WorkerOptionsModal.svelte';
  import { m } from '../../paraglide/messages.js';
  import type { WorkerOption } from '$stores/sites';

  // A worker toggle plus the segments its definition earns: a gear with one
  // field per tune_command placeholder, and a shortcut to its journal. A worker
  // that earns neither renders as the plain toggle, so every worker row can go
  // through this one component.
  interface Props {
    label: string;
    running: boolean;
    asleep?: boolean;
    failing?: boolean;
    unreachable?: boolean;
    loading?: boolean;
    disabled?: boolean;
    title?: string;
    options?: WorkerOption[];
    onToggle: () => void;
    onSaveOptions?: (values: Record<string, string>) => void;
    onLogs?: () => void;
  }
  let {
    label,
    running,
    asleep = false,
    failing = false,
    unreachable = false,
    loading = false,
    disabled = false,
    title = '',
    options = [],
    onToggle,
    onSaveOptions = () => {},
    onLogs
  }: Props = $props();

  let modalOpen = $state(false);

  const hasGear = $derived(options.length > 0);
  const segmented = $derived(hasGear || Boolean(onLogs));
</script>

<div class="inline-flex items-center">
  <ToggleButton
    {label}
    on={running}
    {asleep}
    {failing}
    {unreachable}
    {loading}
    {disabled}
    {title}
    rounding={segmented ? 'rounded-l-md border-r-0' : 'rounded-md'}
    onclick={onToggle}
  />
  {#if hasGear}
    <button
      type="button"
      title={m.sites_controls_workerOptions()}
      aria-label={m.sites_controls_workerOptions()}
      onclick={() => (modalOpen = true)}
      class="inline-flex items-center justify-center h-7 w-8 {onLogs
        ? 'border-r-0'
        : 'rounded-r-md'} border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card hover:bg-gray-50 dark:hover:bg-white/5 transition-colors text-gray-400 dark:text-gray-500"
    >
      <svg
        class="w-3.5 h-3.5"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="12" cy="12" r="3" />
        <path
          d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
        />
      </svg>
    </button>
  {/if}
  {#if onLogs}
    <LogsButton {label} onclick={onLogs} />
  {/if}
</div>

{#if hasGear}
  <WorkerOptionsModal
    open={modalOpen}
    {label}
    {options}
    onclose={() => (modalOpen = false)}
    onsave={onSaveOptions}
  />
{/if}
