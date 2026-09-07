<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    // A stored token is never read back, so the field starts empty and the
    // state is shown as a sentence rather than as a masked value.
    tokenSet?: boolean;
    // The extra flags are not a credential, so unlike the token they are read
    // back and the field starts on the stored value.
    ngrokArgs?: string;
    error?: string;
    busy?: boolean;
    onclose: () => void;
    // An absent field means "leave the stored value alone", so the token
    // survives an args-only save and the other way round.
    onsubmit: (p: { token?: string; args?: string }) => void;
  }
  let {
    open,
    tokenSet = false,
    ngrokArgs = '',
    error = '',
    busy = false,
    onclose,
    onsubmit
  }: Props = $props();

  let token = $state('');
  let args = $state('');

  $effect(() => {
    if (open) {
      token = '';
      args = ngrokArgs;
    }
  });

  const clean = $derived(token.trim());
  const argsChanged = $derived(args.trim() !== ngrokArgs.trim());

  function save() {
    onsubmit({
      ...(clean ? { token: clean } : {}),
      ...(argsChanged ? { args: args.trim() } : {})
    });
  }
  const btnBase =
    'px-3 py-1.5 text-xs font-medium rounded-md transition-colors disabled:opacity-50';
</script>

<Modal {open} title={m.shareToken_title()} {onclose} size="md">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-600 dark:text-gray-300">{m.shareToken_body()}</p>

    <div>
      <label
        for="share-ngrok-token"
        class="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1"
      >
        {m.shareToken_label()}
      </label>
      <input
        id="share-ngrok-token"
        type="password"
        bind:value={token}
        onkeydown={(e) => e.key === 'Enter' && (clean || argsChanged) && !busy && save()}
        placeholder={tokenSet ? m.shareToken_placeholderSet() : m.shareToken_placeholder()}
        autocomplete="off"
        spellcheck="false"
        data-testid="share-token-input"
        class="w-full px-2.5 py-1.5 text-sm font-mono rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-bg text-gray-800 dark:text-gray-200 focus:outline-none focus:border-lerd-red"
      />
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {tokenSet ? m.shareToken_stateSet() : m.shareToken_stateUnset()}
      </p>
    </div>

    <div>
      <label
        for="share-ngrok-args"
        class="block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1"
      >
        {m.shareToken_argsLabel()}
      </label>
      <input
        id="share-ngrok-args"
        type="text"
        bind:value={args}
        onkeydown={(e) => e.key === 'Enter' && (clean || argsChanged) && !busy && save()}
        placeholder={m.shareToken_argsPlaceholder()}
        autocomplete="off"
        spellcheck="false"
        data-testid="share-ngrok-args-input"
        class="w-full px-2.5 py-1.5 text-sm font-mono rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-bg text-gray-800 dark:text-gray-200 focus:outline-none focus:border-lerd-red"
      />
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{m.shareToken_argsHelp()}</p>
    </div>

    <p class="text-xs text-gray-500 dark:text-gray-400">
      <a
        href="https://dashboard.ngrok.com/get-started/your-authtoken"
        target="_blank"
        rel="noopener noreferrer"
        class="text-violet-600 dark:text-violet-400 hover:underline">{m.shareToken_getOne()}</a
      >
    </p>

    {#if error}
      <p class="text-xs text-red-600 dark:text-red-400 break-words">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    {#if tokenSet}
      <button
        type="button"
        disabled={busy}
        onclick={() => onsubmit({ token: '' })}
        data-testid="share-token-clear"
        class="{btnBase} border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5"
        >{m.shareToken_clear()}</button
      >
    {:else}
      <button
        type="button"
        disabled={busy}
        onclick={onclose}
        class="{btnBase} border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5"
        >{m.common_cancel()}</button
      >
    {/if}
    <button
      type="button"
      disabled={busy || (!clean && !argsChanged)}
      onclick={save}
      class="{btnBase} bg-lerd-red hover:bg-lerd-redhov text-white">{m.common_save()}</button
    >
  {/snippet}
</Modal>
