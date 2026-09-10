<script lang="ts">
  import { m } from '../paraglide/messages.js';

  // The logs half of a worker's segmented control: one click lands on that
  // worker's journal instead of the Logs tab plus a hunt through its sources.
  interface Props {
    label: string;
    rounding?: string;
    onclick: () => void;
  }
  let { label, rounding = 'rounded-r-md', onclick }: Props = $props();

  const title = $derived(m.sites_controls_workerLogs({ label }));
</script>

<button
  type="button"
  {title}
  aria-label={title}
  {onclick}
  class="inline-flex items-center justify-center h-7 w-8 {rounding} border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card hover:bg-gray-50 dark:hover:bg-white/5 transition-colors text-gray-400 dark:text-gray-500"
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
    <line x1="4" y1="6" x2="20" y2="6" />
    <line x1="4" y1="10" x2="14" y2="10" />
    <line x1="4" y1="14" x2="18" y2="14" />
    <line x1="4" y1="18" x2="11" y2="18" />
  </svg>
</button>
