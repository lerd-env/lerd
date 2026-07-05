<script lang="ts">
  import { m } from '../paraglide/messages.js';

  interface Props {
    ports: string[];
    disabled?: boolean;
    onadd: (spec: string) => void;
    onremove: (spec: string) => void;
    empty?: string;
  }
  let { ports, disabled = false, onadd, onremove, empty }: Props = $props();

  const inputCls =
    'w-20 text-sm tabular-nums bg-transparent border border-gray-200 dark:border-lerd-border rounded-md px-2.5 py-2 text-gray-700 dark:text-gray-300 placeholder-gray-400 dark:placeholder-gray-600 focus:outline-hidden focus:border-lerd-red/50 transition-colors disabled:opacity-50';

  let newHost = $state<number | null>(null);
  let newContainer = $state<number | null>(null);
  let error = $state('');

  function validPort(n: number | null): n is number {
    return n != null && Number.isInteger(n) && n >= 1 && n <= 65535;
  }

  // Split "host:container" or "ip:host:container" (with optional /proto) into the
  // two ports for display; a bare port shows the same value on both sides.
  function split(spec: string): { host: string; container: string } {
    const segs = spec.split('/')[0].split(':');
    if (segs.length >= 2) return { host: segs[segs.length - 2], container: segs[segs.length - 1] };
    return { host: segs[0], container: segs[0] };
  }

  function add() {
    if (!validPort(newHost) || !validPort(newContainer)) {
      error = m.services_ports_invalidPort();
      return;
    }
    error = '';
    const spec = newHost + ':' + newContainer;
    if (!ports.includes(spec)) onadd(spec);
    newHost = null;
    newContainer = null;
  }
</script>

<div class="flex flex-wrap gap-3">
  {#each ports as spec (spec)}
    {@const p = split(spec)}
    <div
      class="relative group rounded-xl border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card pl-4 pr-10 py-3 min-w-[132px]"
    >
      <div class="text-lg font-mono font-semibold text-gray-800 dark:text-gray-100 leading-tight">
        {p.host}
      </div>
      <div class="text-xs font-mono text-gray-500 dark:text-gray-400 leading-tight mt-0.5">
        &rarr; {p.container}
      </div>
      <button
        type="button"
        onclick={() => onremove(spec)}
        {disabled}
        title={m.common_remove()}
        aria-label={m.common_remove()}
        class="absolute top-1.5 right-1.5 p-1 rounded-md text-gray-300 hover:text-red-500 hover:bg-red-50 dark:text-gray-600 dark:hover:text-red-400 dark:hover:bg-red-500/10 opacity-0 group-hover:opacity-100 focus:opacity-100 transition disabled:opacity-30"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  {/each}

  <div
    class="flex items-center gap-2 rounded-xl border border-dashed border-gray-300 dark:border-lerd-border px-3 py-3"
  >
    <input
      type="number"
      min="1"
      max="65535"
      bind:value={newHost}
      placeholder={m.services_ports_extraHostPlaceholder()}
      aria-label={m.services_ports_extraHostPlaceholder()}
      onkeydown={(e) => e.key === 'Enter' && add()}
      {disabled}
      class={inputCls}
    />
    <span class="text-xs text-gray-400 shrink-0">:</span>
    <input
      type="number"
      min="1"
      max="65535"
      bind:value={newContainer}
      placeholder={m.services_ports_extraContainerPlaceholder()}
      aria-label={m.services_ports_extraContainerPlaceholder()}
      onkeydown={(e) => e.key === 'Enter' && add()}
      {disabled}
      class={inputCls}
    />
    <button
      type="button"
      onclick={add}
      disabled={disabled || newHost == null || newContainer == null}
      title={m.common_add()}
      aria-label={m.common_add()}
      class="p-1.5 rounded-md bg-lerd-red hover:bg-lerd-redhov text-white transition-colors disabled:opacity-40"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
      </svg>
    </button>
  </div>
</div>

{#if ports.length === 0 && empty}
  <p class="text-xs text-gray-400 dark:text-gray-500 italic mt-2">{empty}</p>
{/if}
{#if error}
  <p class="text-xs text-red-500 mt-2">{error}</p>
{/if}
