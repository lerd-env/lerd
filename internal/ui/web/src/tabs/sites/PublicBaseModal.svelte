<script lang="ts">
  import { untrack } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    base?: string;
    error?: string;
    busy?: boolean;
    onclose: () => void;
    onsubmit: (base: string) => void;
  }
  let { open, base = '', error = '', busy = false, onclose, onsubmit }: Props = $props();

  let value = $state('');
  // Seed only on the open transition (the base read untracked), so a
  // background share-tools refresh never overwrites what the user is typing.
  $effect(() => {
    if (open) value = untrack(() => base);
  });

  const clean = $derived(value.trim().replace(/^\.+|\.+$/g, '').toLowerCase());
  // A domain you control, at least two labels; a bare TLD is refused.
  const valid = $derived(clean.split('.').filter(Boolean).length >= 2);
  const preview = $derived(clean && valid ? `site.${clean}` : '');
  const btn = 'px-3 py-1.5 text-xs font-medium rounded-md transition-colors disabled:opacity-50';
</script>

<Modal {open} title={m.publicBase_title()} {onclose} size="md">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-600 dark:text-gray-300">{m.publicBase_body()}</p>
    <div>
      <label for="public-base" class="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">
        {m.publicBase_label()}
      </label>
      <input
        id="public-base"
        bind:value
        onkeydown={(e) => e.key === 'Enter' && clean && valid && !busy && onsubmit(clean)}
        placeholder="dev.example.com"
        autocomplete="off"
        spellcheck="false"
        class="w-full px-2.5 py-1.5 text-sm font-mono rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-bg text-gray-800 dark:text-gray-200 focus:outline-none focus:border-lerd-red"
      />
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {#if preview}
          {m.publicBase_preview()}
          <span class="font-mono text-sky-600 dark:text-sky-400">{preview}</span>
        {:else if clean && !valid}
          <span class="text-amber-600 dark:text-amber-400">{m.publicBase_notValid()}</span>
        {:else}
          {m.publicBase_previewEmpty()}
        {/if}
      </p>
    </div>
    {#if error}
      <p class="text-xs text-red-600 dark:text-red-400 break-words">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    <button
      type="button"
      disabled={busy || !base}
      onclick={() => onsubmit('')}
      class="{btn} border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5"
      >{m.publicBase_clear()}</button
    >
    <button
      type="button"
      disabled={busy || !clean || !valid}
      onclick={() => onsubmit(clean)}
      class="{btn} bg-lerd-red hover:bg-lerd-redhov text-white">{m.common_save()}</button
    >
  {/snippet}
</Modal>
