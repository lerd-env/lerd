<script lang="ts">
  import { m } from '../../paraglide/messages.js';

  interface Props {
    value: number | null;
    defaultPort?: number;
    disabled?: boolean;
    onenter?: () => void;
  }
  let { value = $bindable(), defaultPort, disabled = false, onenter }: Props = $props();

  // Matches the domains modal's add inputs so every port field reads identically.
  const inputCls =
    'text-sm tabular-nums bg-transparent border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 placeholder-gray-400 dark:placeholder-gray-600 focus:outline-hidden focus:border-lerd-red/50 transition-colors disabled:opacity-50';
</script>

<div class="flex items-center gap-3">
  <input
    type="number"
    min="0"
    max="65535"
    bind:value
    placeholder={defaultPort ? String(defaultPort) : ''}
    onkeydown={(e) => e.key === 'Enter' && onenter?.()}
    {disabled}
    class="w-32 {inputCls}"
  />
  {#if defaultPort}
    <span class="text-xs text-gray-500 dark:text-gray-400">
      {m.services_ports_defaultHint({ port: defaultPort })}
    </span>
    <button
      type="button"
      onclick={() => (value = defaultPort ?? null)}
      disabled={value === defaultPort}
      class="ml-auto text-xs text-gray-500 dark:text-gray-400 hover:text-lerd-red transition-colors disabled:opacity-40 disabled:hover:text-gray-500 dark:disabled:hover:text-gray-400"
    >{m.services_ports_resetDefault()}</button>
  {/if}
</div>
