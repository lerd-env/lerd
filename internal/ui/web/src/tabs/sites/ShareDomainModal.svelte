<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    // "start" asks on the way to a tunnel and offers to skip; "configure" is
    // the cog, which only records the answer.
    mode: 'start' | 'configure';
    label: string;
    baseDomain?: string;
    remembered?: boolean;
    error?: string;
    busy?: boolean;
    onclose: () => void;
    onsubmit: (domain: string, remember: boolean) => void;
  }
  let {
    open,
    mode,
    label,
    baseDomain = '',
    remembered = false,
    error = '',
    busy = false,
    onclose,
    onsubmit
  }: Props = $props();

  let domain = $state('');
  let remember = $state(false);

  // Reopening starts from what is stored, so the cog shows the current answer
  // and the ask starts empty rather than from a half-typed previous attempt.
  $effect(() => {
    if (open) {
      domain = baseDomain;
      remember = remembered || mode === 'configure';
    }
  });

  const clean = $derived(domain.trim().replace(/^\.+|\.+$/g, '').toLowerCase());
  const preview = $derived(clean ? `${label}.${clean}` : '');
  const btnBase =
    'px-3 py-1.5 text-xs font-medium rounded-md transition-colors disabled:opacity-50';
</script>

<Modal {open} title={m.shareDomain_title()} {onclose} size="md">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-600 dark:text-gray-300">{m.shareDomain_body()}</p>

    <div>
      <label for="share-base-domain" class="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">
        {m.shareDomain_label()}
      </label>
      <input
        id="share-base-domain"
        bind:value={domain}
        onkeydown={(e) => e.key === 'Enter' && clean && !busy && onsubmit(clean, remember)}
        placeholder="example.com"
        autocomplete="off"
        spellcheck="false"
        class="w-full px-2.5 py-1.5 text-sm font-mono rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-bg text-gray-800 dark:text-gray-200 focus:outline-none focus:border-lerd-red"
      />
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {#if preview}
          {m.shareDomain_preview()}
          <span class="font-mono text-violet-600 dark:text-violet-400">{preview}</span>
        {:else}
          {m.shareDomain_previewEmpty()}
        {/if}
      </p>
    </div>

    <div class="flex items-start gap-2 text-xs">
      <input id="share-remember" type="checkbox" bind:checked={remember} class="mt-0.5 accent-lerd-red" />
      <span>
        <label for="share-remember" class="text-gray-600 dark:text-gray-300 cursor-pointer"
          >{m.shareDomain_remember()}</label
        >
        <span class="block text-gray-500 dark:text-gray-400">{m.shareDomain_rememberHint()}</span>
      </span>
    </div>

    {#if error}
      <p class="text-xs text-red-600 dark:text-red-400 break-words">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    {#if mode === 'start'}
      <button
        type="button"
        disabled={busy}
        onclick={() => onsubmit('', remember)}
        class="{btnBase} border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5"
        >{m.shareDomain_skip()}</button
      >
      <button
        type="button"
        disabled={busy || !clean}
        onclick={() => onsubmit(clean, remember)}
        class="{btnBase} bg-lerd-red hover:bg-lerd-redhov text-white">{m.shareDomain_use()}</button
      >
    {:else}
      <button
        type="button"
        disabled={busy}
        onclick={onclose}
        class="{btnBase} border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5"
        >{m.common_cancel()}</button
      >
      <button
        type="button"
        disabled={busy}
        onclick={() => onsubmit(clean, remember)}
        class="{btnBase} bg-lerd-red hover:bg-lerd-redhov text-white">{m.common_save()}</button
      >
    {/if}
  {/snippet}
</Modal>
