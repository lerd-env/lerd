<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import { closeModal, modal } from '$stores/modals';
  import { presets, presetsLoaded, loadPresets, presetsFillingRole, type Preset } from '$stores/presets';
  import { services, serviceLabel } from '$stores/services';
  import { confirmDownload } from '$stores/downloadConfirm';
  import { addSuggestedService, loadSites, useSiteDatabase } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  const target = $derived($modal.siteServiceAdd);
  const choices = $derived(target ? presetsFillingRole($presets, target.name) : []);
  // External servers of the same engine fill the role as well as a preset does,
  // and need no download.
  const externals = $derived(target ? $services.filter((s) => s.external && fillsRole(s.family, target.name)) : []);
  const pickedExternal = $derived(externals.find((s) => s.name === preset));

  function fillsRole(family: string | undefined, role: string): boolean {
    return family === role || (role === 'mysql' && family === 'mariadb');
  }

  let preset = $state('');
  let version = $state('');
  let busy = $state(false);
  let error = $state('');

  const picked = $derived(choices.find((p) => p.name === preset));

  onMount(() => {
    loadPresets();
  });

  // Start on what this machine already runs, since that adds without a
  // download; otherwise on the suggested service at its default version.
  $effect(() => {
    if (preset || choices.length === 0 || !target) return;
    const running = choices.find((p) => (p.installed_tags || []).length > 0 || (p.installed && !(p.versions || []).length));
    const start = running || choices.find((p) => p.name === target.name) || choices[0];
    pick(start.name, running ? (running.installed_tags || [])[0] : undefined);
  });

  function pick(name: string, tag?: string) {
    preset = name;
    const p = choices.find((x) => x.name === name);
    version = tag ?? ((p?.installed_tags || [])[0] || p?.default_version || (p?.versions || [])[0]?.tag || '');
  }

  // Installed versions stay on the list: picking one adds it with no download.
  function versionOptions(p: Preset) {
    const installed = new Set(p.installed_tags || []);
    return (p.versions || []).map((v) => ({
      value: v.tag,
      label: v.label || v.tag,
      description: installed.has(v.tag) ? m.common_installed() : undefined
    }));
  }

  function presetOptions() {
    return [
      ...choices.map((p) => ({ value: p.name, label: serviceLabel(p.name), description: p.description })),
      ...externals.map((s) => ({ value: s.name, label: s.name, description: m.externalDb_server({ address: s.external ?? '' }) }))
    ];
  }

  function safeClose() {
    if (busy) return;
    closeModal();
  }

  async function useExternal(name: string) {
    if (!target) return;
    busy = true;
    error = '';
    const res = await useSiteDatabase(target.domain, name);
    busy = false;
    if (!res.ok) {
      error = res.error || m.common_requestFailed();
      return;
    }
    await loadSites();
    closeModal();
  }

  async function add() {
    if (pickedExternal) return useExternal(pickedExternal.name);
    if (busy || !target || !picked) return;
    // Busy from the first await, and the pick held, so what gets added is what
    // the download prompt described.
    busy = true;
    error = '';
    const { domain, name } = target;
    const chosen = picked.name;
    const tag = version;
    if (!(await confirmDownload(serviceLabel(chosen), { preset: chosen, version: tag }))) {
      busy = false;
      return;
    }
    const res = await addSuggestedService(domain, name, chosen, tag);
    busy = false;
    if (!res.ok) {
      error = res.error || m.common_requestFailed();
      return;
    }
    await loadSites();
    closeModal();
  }
</script>

<Modal
  open
  title={target ? m.sites_suggestedService_add({ name: serviceLabel(target.name) }) : ''}
  onclose={safeClose}
  size="sm"
>
  <div class="px-5 py-4 space-y-3">
    {#if !$presetsLoaded}
      <p class="text-sm text-gray-500 dark:text-gray-400">{m.common_loading()}</p>
    {:else if target}
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.sites_suggestedService_pick()}</p>
      {#if choices.length + externals.length > 1}
        <Dropdown value={preset} options={presetOptions()} onchange={(v) => pick(v)} width="full" disabled={busy}>
          {#snippet optionIcon(name)}<ServiceIcon {name} bare inline />{/snippet}
        </Dropdown>
      {/if}
      {#if picked && (picked.versions || []).length > 0}
        <Dropdown value={version} options={versionOptions(picked)} onchange={(v) => (version = v)} width="full" disabled={busy} />
      {/if}
      {#if error}
        <p class="text-xs text-red-500">{error}</p>
      {/if}
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={safeClose} disabled={busy}>{m.common_cancel()}</DetailButton>
    <DetailButton tone="primary" onclick={add} loading={busy} disabled={busy || (!picked && !pickedExternal)}>{m.common_add()}</DetailButton>
  {/snippet}
</Modal>
