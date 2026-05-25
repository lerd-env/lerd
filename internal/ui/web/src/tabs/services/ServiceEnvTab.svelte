<script lang="ts">
  import EnvBlock from '$components/EnvBlock.svelte';
  import type { Service } from '$stores/services';
  import { loadServiceEnv, saveServiceEnv, type ServiceEnvPayload } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  // Container env (Environment= block) is editable. We keep two parallel
  // arrays for key/value pairs so the user can rename a key without React-
  // ish reconciliation glitches.
  let entries = $state<Array<{ key: string; value: string }>>([]);
  let loadedSnapshot = $state<string>(''); // JSON.stringify of last-saved entries, for dirty check
  let loading = $state(true);
  let error = $state<string>('');
  let saving = $state(false);
  let saveSuccess = $state(false);
  let source = $state<'preset' | 'custom'>('preset');

  const dirty = $derived(JSON.stringify(entries) !== loadedSnapshot);

  function payloadFromEntries(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const e of entries) {
      const k = e.key.trim();
      if (!k) continue;
      out[k] = e.value;
    }
    return out;
  }

  function entriesFromPayload(p: ServiceEnvPayload): Array<{ key: string; value: string }> {
    return Object.entries(p.environment)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([key, value]) => ({ key, value }));
  }

  $effect(() => {
    const name = svc.name;
    loading = true;
    error = '';
    entries = [];
    saveSuccess = false;
    loadServiceEnv(name)
      .then((p) => {
        const fresh = entriesFromPayload(p);
        entries = fresh;
        loadedSnapshot = JSON.stringify(fresh);
        source = p.source;
      })
      .catch((e: unknown) => {
        error = e instanceof Error ? e.message : String(e);
      })
      .finally(() => {
        loading = false;
      });
  });

  $effect(() => {
    if (!dirty) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  function addEntry() {
    entries = [...entries, { key: '', value: '' }];
  }
  function removeEntry(idx: number) {
    entries = entries.filter((_, i) => i !== idx);
  }

  async function onSave() {
    saving = true;
    error = '';
    saveSuccess = false;
    try {
      const env = payloadFromEntries();
      await saveServiceEnv(svc.name, env);
      // Re-pull from server so we surface any normalisation it applied.
      const p = await loadServiceEnv(svc.name);
      const fresh = entriesFromPayload(p);
      entries = fresh;
      loadedSnapshot = JSON.stringify(fresh);
      source = p.source;
      saveSuccess = true;
      setTimeout(() => (saveSuccess = false), 2500);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }

  function onDiscard() {
    if (!dirty) return;
    if (!confirm('Descartar alterações não salvas?')) return;
    entries = JSON.parse(loadedSnapshot);
    error = '';
  }
</script>

<div class="flex flex-col h-full overflow-hidden">
  <!-- Connection URL (read-only, unchanged) -->
  {#if svc.connection_url}
    <div class="p-3 sm:p-5 pb-3 shrink-0">
      <div class="rounded-lg border border-gray-200 dark:border-lerd-border overflow-hidden">
        <div class="flex items-center justify-between bg-gray-50 dark:bg-white/3 px-3 py-1.5 border-b border-gray-200 dark:border-lerd-border">
          <span class="text-[10px] font-semibold text-gray-400 uppercase tracking-wider">{m.services_env_connect()}</span>
        </div>
        <div class="bg-gray-50 dark:bg-black/40 px-3 py-2.5">
          <a href={svc.connection_url} class="font-mono text-[10px] text-sky-600 dark:text-sky-400 hover:underline break-all">{svc.connection_url}</a>
          <p class="text-[10px] text-gray-400 dark:text-gray-600 mt-1.5">
            {@html m.services_env_connectHint({ loopback4: '<code class="text-gray-500 dark:text-gray-400">127.0.0.1</code>', loopback6: '<code class="text-gray-500 dark:text-gray-400">localhost</code>' })}
          </p>
        </div>
      </div>
    </div>
  {/if}

  <!-- Container environment editor -->
  <div class="flex flex-col flex-1 min-h-0">
    <div class="flex items-center justify-between gap-2 px-3 sm:px-5 py-2 border-t border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-white/3 shrink-0">
      <div class="flex items-center gap-2 min-w-0">
        <span class="text-[10px] font-semibold text-gray-400 uppercase tracking-wider shrink-0">Container env</span>
        <span class="text-[10px] {source === 'custom' ? 'text-amber-600 dark:text-amber-400' : 'text-gray-400'}">
          {source === 'custom' ? '· override do usuário' : '· defaults do preset'}
        </span>
        {#if dirty}
          <span class="text-[10px] font-medium text-amber-600 dark:text-amber-400">● não salvo</span>
        {/if}
      </div>
      <div class="flex items-center gap-2 shrink-0">
        {#if saveSuccess}
          <span class="text-[10px] text-emerald-600 dark:text-emerald-400">✓ salvo · reinicie p/ aplicar</span>
        {/if}
        <button
          type="button"
          onclick={onDiscard}
          disabled={!dirty || saving}
          class="text-[11px] px-2 py-1 rounded text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-200 dark:hover:bg-white/5 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
        >Descartar</button>
        <button
          type="button"
          onclick={onSave}
          disabled={!dirty || saving || loading}
          class="text-[11px] px-2.5 py-1 rounded bg-emerald-600 text-white hover:bg-emerald-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        >{saving ? 'Salvando…' : 'Salvar'}</button>
      </div>
    </div>

    {#if loading}
      <p class="text-xs text-gray-400 p-3">{m.common_loading()}</p>
    {:else}
      <div class="flex-1 overflow-y-auto p-3 sm:p-5 pt-3 space-y-1.5">
        {#if entries.length === 0}
          <p class="text-[11px] text-gray-400 italic">Nenhuma variável de ambiente no container. Clique "+ adicionar" pra começar.</p>
        {/if}
        {#each entries as entry, idx (idx)}
          <div class="grid grid-cols-[1fr_2fr_auto] gap-1.5 items-center">
            <input
              type="text"
              placeholder="KEY"
              bind:value={entry.key}
              spellcheck="false"
              class="font-mono text-[11px] px-2 py-1 bg-white dark:bg-lerd-dark-2 border border-gray-200 dark:border-lerd-border rounded text-gray-900 dark:text-gray-100 placeholder:text-gray-400 focus:outline-none focus:ring-1 focus:ring-emerald-500"
            />
            <input
              type="text"
              placeholder="value"
              bind:value={entry.value}
              spellcheck="false"
              class="font-mono text-[11px] px-2 py-1 bg-white dark:bg-lerd-dark-2 border border-gray-200 dark:border-lerd-border rounded text-gray-900 dark:text-gray-100 placeholder:text-gray-400 focus:outline-none focus:ring-1 focus:ring-emerald-500"
            />
            <button
              type="button"
              onclick={() => removeEntry(idx)}
              class="text-[11px] px-2 py-1 text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-500/10 rounded transition-colors"
              title="Remover esta variável"
            >×</button>
          </div>
        {/each}
        <button
          type="button"
          onclick={addEntry}
          class="text-[11px] mt-2 px-2.5 py-1 rounded border border-dashed border-gray-300 dark:border-lerd-border text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-400 transition-colors"
        >+ adicionar variável</button>
      </div>
      {#if error}
        <div class="text-[11px] text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-500/10 px-3 py-2 border-t border-red-200 dark:border-red-500/20 break-words shrink-0">
          {error}
        </div>
      {/if}
      <p class="text-[10px] text-gray-400 px-3 sm:px-5 py-1.5 border-t border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-white/3 shrink-0 leading-relaxed shrink-0">
        Edita o bloco <span class="font-mono">Environment=</span> do quadlet em <span class="font-mono">~/.config/lerd/services/{svc.name}.yaml</span>. Após salvar, reinicie o serviço (botão Restart no header) pra aplicar.
      </p>
    {/if}
  </div>

  <!-- Read-only: env_vars (Laravel hints) -->
  {#if svc.env_vars && Object.keys(svc.env_vars).length > 0}
    <div class="p-3 sm:p-5 pt-0 shrink-0">
      <EnvBlock vars={svc.env_vars} label="dicas .env para o projeto (read-only)" />
    </div>
  {/if}
</div>
