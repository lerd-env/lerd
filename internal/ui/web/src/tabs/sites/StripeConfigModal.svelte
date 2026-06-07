<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import { m } from '../../paraglide/messages.js';

  // Edits the per-project Stripe webhook route (persisted to .lerd.yaml). Lets
  // non-Laravel projects whose route is not /stripe/webhook point the listener
  // at the right path. Prefilled with the current value each time it opens.
  interface Props {
    open: boolean;
    path: string;
    onclose: () => void;
    onsave: (path: string) => void;
  }
  let { open, path, onclose, onsave }: Props = $props();

  let value = $state('');

  $effect(() => {
    if (open) value = path || '/stripe/webhook';
  });

  function save() {
    const trimmed = value.trim();
    if (trimmed) onsave(trimmed.startsWith('/') ? trimmed : '/' + trimmed);
    onclose();
  }
</script>

<Modal {open} {onclose} title={m.sites_controls_stripeConfigTitle()} size="sm">
  <div class="px-5 py-4 space-y-3">
    <label class="block text-sm text-gray-600 dark:text-gray-400" for="stripe-path">
      {m.sites_controls_stripePathLabel()}
    </label>
    <input
      id="stripe-path"
      type="text"
      bind:value
      spellcheck="false"
      autocomplete="off"
      onkeydown={(e) => e.key === 'Enter' && save()}
      class="w-full text-sm font-mono px-2.5 py-1.5 rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card text-gray-800 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-lerd-red"
    />
    <p class="text-[11px] text-gray-400 dark:text-gray-500">
      {m.sites_controls_stripePathHint()}
    </p>
  </div>
  {#snippet footer()}
    <button
      type="button"
      onclick={onclose}
      class="text-xs px-3 py-1.5 rounded-sm border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5 transition-colors"
    >{m.common_cancel()}</button>
    <button
      type="button"
      onclick={save}
      class="text-xs px-3 py-1.5 rounded-sm bg-lerd-red hover:bg-lerd-redhov text-white transition-colors"
    >{m.common_save()}</button>
  {/snippet}
</Modal>
