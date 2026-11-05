<script lang="ts">
  import DetailButton from '$components/DetailButton.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import { apiFetch } from '$lib/api';
  import type { Service } from '$stores/services';
  import { loadSites, sites, useSiteDatabase } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  let site = $state('');
  let using = $state(false);
  let changing = $state(false);
  let password = $state('');
  let saving = $state(false);
  let error = $state('');
  let note = $state('');

  const choices = $derived(
    $sites
      .filter((s) => s.external_db !== svc.name && !s.custom_container)
      .map((s) => ({ value: s.domain, label: s.domain }))
  );

  async function use() {
    using = true;
    error = '';
    note = '';
    const res = await useSiteDatabase(site, svc.name);
    using = false;
    if (!res.ok) {
      error = res.error || m.common_requestFailed();
      return;
    }
    note = m.externalDb_siteUsing({ site, name: svc.name });
    site = '';
    await loadSites();
  }

  async function savePassword() {
    saving = true;
    error = '';
    note = '';
    const res = await apiFetch('/api/services/external/password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: svc.name, password })
    });
    saving = false;
    if (!res.ok) {
      error = (await res.text()).trim() || m.common_requestFailed();
      return;
    }
    password = '';
    changing = false;
    note = m.externalDb_passwordChanged();
  }

  const fieldClass =
    'flex-1 min-w-0 text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-lg px-3 py-1.5';
</script>

<div class="space-y-2">
  <div class="flex flex-wrap items-center gap-2">
    <p class="text-xs text-gray-500 dark:text-gray-400 flex-1 min-w-0">{m.externalDb_server({ address: svc.external ?? '' })}</p>
    {#if !changing}
      <DetailButton onclick={() => (changing = true)}>{m.externalDb_changePassword()}</DetailButton>
    {/if}
  </div>
  {#if changing}
    <form class="flex gap-2" onsubmit={(e) => { e.preventDefault(); savePassword(); }}>
      <input type="password" bind:value={password} aria-label={m.externalDb_password()} placeholder={m.externalDb_password()} class={fieldClass} disabled={saving} autocomplete="new-password" />
      <DetailButton onclick={() => (changing = false)} disabled={saving}>{m.common_cancel()}</DetailButton>
      <DetailButton tone="primary" onclick={savePassword} loading={saving} disabled={saving}>{m.common_save()}</DetailButton>
    </form>
  {/if}
  {#if choices.length > 0}
    <div class="flex items-center gap-2">
      <div class="flex-1 min-w-0">
        <Dropdown value={site} options={[{ value: '', label: m.externalDb_useOnSite() }, ...choices]} onchange={(v) => (site = v)} width="full" disabled={using} />
      </div>
      <DetailButton tone="primary" onclick={use} loading={using} disabled={using || !site}>{m.externalDb_use()}</DetailButton>
    </div>
    {#if site}
      <p class="text-[11px] text-gray-500 dark:text-gray-400">{m.externalDb_useHint()}</p>
    {/if}
  {/if}
  {#if note}
    <p class="text-xs text-emerald-600 dark:text-emerald-400">{note}</p>
  {/if}
  {#if error}
    <p class="text-xs text-red-500 break-words">{error}</p>
  {/if}
</div>
