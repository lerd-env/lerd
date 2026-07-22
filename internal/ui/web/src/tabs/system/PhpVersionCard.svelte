<script lang="ts">
  import StatusPill from '$components/StatusPill.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    version: string;
    patch?: string;
    running: boolean;
    isDefault: boolean;
    selected: boolean;
    onselect: () => void;
  }
  let { version, patch, running, isDefault, selected, onselect }: Props = $props();

  // Show the full build when known ("8.5.8"), with the patch tail dimmed so it
  // reads as one number; fall back to the minor until the probe lands.
  const display = $derived(patch || version);
  const minor = $derived(display.split('.').slice(0, 2).join('.'));
  const tail = $derived(
    display.split('.').length > 2 ? '.' + display.split('.').slice(2).join('.') : ''
  );
</script>

<button
  type="button"
  onclick={onselect}
  title={'PHP ' + display + ' — ' + (running ? m.common_running() : m.common_stopped()) + (isDefault ? ' · ' + m.common_default() : '')}
  class="shrink-0 w-[9.5rem] snap-start text-left flex flex-col gap-2.5 rounded-2xl border p-3 transition-colors {selected
    ? 'border-lerd-red bg-white dark:bg-lerd-card ring-1 ring-lerd-red'
    : 'border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card hover:border-gray-300 dark:hover:border-gray-600'}"
>
  <div class="flex items-center justify-between gap-2">
    <span
      class="text-[8px] font-extrabold tracking-wide text-white rounded-md px-1.5 py-1 leading-none"
      style="background: linear-gradient(150deg, #8892bf, #6b74a3);">PHP</span>
    <StatusPill
      tone={running ? 'ok' : 'muted'}
      label={running ? m.common_running() : m.common_stopped()}
    />
  </div>
  <div class="flex items-center gap-2">
    <span class="font-mono text-2xl font-semibold tabular-nums tracking-tight leading-none text-gray-900 dark:text-gray-100">
      {minor}<span class="text-[0.8em] font-medium text-gray-400 dark:text-gray-500">{tail}</span>
    </span>
    {#if isDefault}
      <svg
        class="w-3.5 h-3.5 shrink-0 text-lerd-red"
        fill="currentColor"
        viewBox="0 0 20 20"
        aria-label={m.common_default()}
      >
        <path d="M10 1.5l2.6 5.27 5.82.85-4.21 4.1.99 5.78L10 14.77l-5.2 2.73.99-5.78L1.58 7.62l5.82-.85L10 1.5z" />
      </svg>
    {/if}
  </div>
</button>
