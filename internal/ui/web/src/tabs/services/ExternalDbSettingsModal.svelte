<script lang="ts">
  import DetailButton from '$components/DetailButton.svelte';
  import Modal from '$components/Modal.svelte';
  import { apiFetch } from '$lib/api';
  import type { Service } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    svc: Service;
    onclose: () => void;
  }
  let { open, svc, onclose }: Props = $props();

  let password = $state('');
  let tools = $state('');
  let saving = $state(false);
  let saved = $state(false);
  let error = $state('');

  $effect(() => {
    if (open) {
      password = '';
      tools = svc.external_tools ?? '';
      saved = false;
      error = '';
    }
  });

  const dirty = $derived(password !== '' || tools.trim() !== (svc.external_tools ?? ''));

  async function save() {
    if (saving || !dirty) return;
    saving = true;
    error = '';
    const res = await apiFetch('/api/services/external/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: svc.name, password, tools: tools.trim() })
    });
    saving = false;
    if (!res.ok) {
      error = (await res.text()).trim() || m.common_requestFailed();
      return;
    }
    password = '';
    saved = true;
  }

  function safeClose() {
    if (!saving) onclose();
  }

  const labelClass = 'block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1';
  const fieldClass =
    'w-full text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-3 py-1.5';
</script>

<Modal {open} title={m.externalDb_settings()} onclose={safeClose} size="sm">
  <div class="px-5 py-4 space-y-3">
    {#if saved}
      <p class="text-xs text-emerald-600 dark:text-emerald-400">{m.externalDb_saved()}</p>
    {:else}
      <form class="space-y-3" onsubmit={(e) => { e.preventDefault(); save(); }}>
        <div>
          <label class={labelClass} for="ext-new-password">{m.externalDb_password()}</label>
          <input id="ext-new-password" type="password" bind:value={password} class={fieldClass} disabled={saving} autocomplete="new-password" placeholder={m.externalDb_keepPassword()} />
        </div>
        <div>
          <label class={labelClass} for="ext-new-tools">{m.externalDb_tools()}</label>
          <input id="ext-new-tools" bind:value={tools} class={fieldClass} disabled={saving} autocomplete="off" spellcheck="false" />
        </div>
      </form>
    {/if}
    {#if error}
      <p class="text-xs text-red-500 break-words">{error}</p>
    {/if}
  </div>
  {#snippet footer()}
    {#if saved}
      <DetailButton onclick={onclose}>{m.common_close()}</DetailButton>
    {:else}
      <DetailButton onclick={safeClose} disabled={saving}>{m.common_cancel()}</DetailButton>
      <DetailButton tone="primary" onclick={save} loading={saving} disabled={saving || !dirty}>{m.common_save()}</DetailButton>
    {/if}
  {/snippet}
</Modal>
