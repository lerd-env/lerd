<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { closeModal } from '$stores/modals';
  import { apiFetch } from '$lib/api';
  import { loadServices } from '$stores/services';
  import { m } from '../paraglide/messages.js';

  const DEFAULT_PORT: Record<string, number> = { mysql: 3306, mariadb: 3306, postgres: 5432 };
  const engines = [
    { value: 'mysql', label: 'MySQL' },
    { value: 'mariadb', label: 'MariaDB' },
    { value: 'postgres', label: 'PostgreSQL' }
  ];

  let engine = $state('mysql');
  let name = $state('');
  let host = $state('127.0.0.1');
  let port = $state(3306);
  let user = $state('root');
  let password = $state('');
  let tools = $state('');
  let busy = $state(false);
  let error = $state('');
  let connected = $state('');
  let foundTools = $state('');

  function pickEngine(value: string) {
    if (port === DEFAULT_PORT[engine]) port = DEFAULT_PORT[value];
    if (value === 'postgres' && user === 'root') user = 'postgres';
    if (value !== 'postgres' && user === 'postgres') user = 'root';
    engine = value;
  }

  async function send(path: string): Promise<Response | null> {
    busy = true;
    error = '';
    connected = '';
    try {
      const res = await apiFetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim(), engine, host, port: Number(port), user, password, tools })
      });
      if (res.ok) return res;
      error = (await res.text()).trim() || m.common_requestFailed();
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_requestFailed();
    } finally {
      busy = false;
    }
    return null;
  }

  async function test() {
    const res = await send('/api/services/external/test');
    if (!res) return;
    const found = await res.json();
    connected = found.version;
    foundTools = found.tools;
  }

  async function add() {
    if (await send('/api/services/external')) {
      await loadServices();
      closeModal();
    }
  }

  function safeClose() {
    if (!busy) closeModal();
  }

  const fieldClass =
    'w-full text-sm bg-white dark:bg-lerd-bg border border-gray-200 dark:border-lerd-border rounded-sm px-3 py-1.5';
  const labelClass = 'block text-xs font-medium text-gray-500 dark:text-gray-400 mb-1';
</script>

<Modal open title={m.externalDb_add()} onclose={safeClose} size="sm">
  <form class="px-5 py-4 space-y-3" onsubmit={(e) => { e.preventDefault(); add(); }}>
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.externalDb_hint()}</p>
    <Dropdown value={engine} options={engines} onchange={pickEngine} width="full" disabled={busy} />
    <div>
      <label class={labelClass} for="ext-name">{m.externalDb_name()}</label>
      <input id="ext-name" bind:value={name} placeholder="dbngin" class={fieldClass} disabled={busy} />
    </div>
    <div class="grid grid-cols-3 gap-2">
      <div class="col-span-2">
        <label class={labelClass} for="ext-host">{m.externalDb_host()}</label>
        <input id="ext-host" bind:value={host} class={fieldClass} disabled={busy} />
      </div>
      <div>
        <label class={labelClass} for="ext-port">{m.externalDb_port()}</label>
        <input id="ext-port" type="number" min="1" max="65535" bind:value={port} class={fieldClass} disabled={busy} />
      </div>
    </div>
    <div>
      <label class={labelClass} for="ext-user">{m.externalDb_user()}</label>
      <input id="ext-user" bind:value={user} class={fieldClass} disabled={busy} autocomplete="off" />
    </div>
    <div>
      <label class={labelClass} for="ext-password">{m.externalDb_password()}</label>
      <input id="ext-password" type="password" bind:value={password} class={fieldClass} disabled={busy} autocomplete="new-password" />
    </div>
    <div>
      <label class={labelClass} for="ext-tools">{m.externalDb_tools()}</label>
      <input id="ext-tools" bind:value={tools} class={fieldClass} disabled={busy} autocomplete="off" spellcheck="false" />
      <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">{m.externalDb_toolsHint()}</p>
    </div>
    {#if connected}
      <p class="text-xs text-emerald-600 dark:text-emerald-400">{m.externalDb_connected({ version: connected })}</p>
      <p class="text-[11px] text-gray-500 dark:text-gray-400 break-all">{m.externalDb_toolsFound({ tools: foundTools })}</p>
    {/if}
    {#if error}
      <p class="text-xs text-red-500 break-words">{error}</p>
    {/if}
  </form>
  {#snippet footer()}
    <DetailButton onclick={test} disabled={busy}>{m.externalDb_test()}</DetailButton>
    <DetailButton tone="primary" onclick={add} loading={busy} disabled={busy || !name.trim()}>{m.externalDb_save()}</DetailButton>
  {/snippet}
</Modal>
