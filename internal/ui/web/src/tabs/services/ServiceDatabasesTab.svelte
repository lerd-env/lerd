<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { databases, loadEngine, createDatabase } from '$stores/databases';
  import type { Service } from '$stores/services';
  import { pairDatabases } from '$lib/databasePairs';
  import DatabaseCard from '../databases/DatabaseCard.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  // Reload whenever the selected service changes so switching between two
  // engines doesn't show the previous one's databases.
  $effect(() => {
    void loadEngine(svc.name);
  });

  const engine = $derived($databases.find((e) => e.service === svc.name));
  const active = $derived(engine?.status === 'active');
  const pairs = $derived(pairDatabases(engine?.databases ?? []));

  let newName = $state('');
  let creating = $state(false);
  let createError = $state('');

  async function create() {
    const name = newName.trim();
    if (!name) return;
    creating = true;
    createError = '';
    const res = await createDatabase(svc.name, name);
    creating = false;
    if (!res.ok) {
      createError = res.error || m.common_failed();
      return;
    }
    newName = '';
  }
</script>

<div class="p-3 sm:p-5 space-y-4 overflow-y-auto">
  {#if !active}
    <p class="text-sm text-gray-400 dark:text-gray-500">{m.databases_startHint()}</p>
  {:else}
    {#if engine?.supports_create}
      <div class="space-y-1.5">
        <div class="flex gap-2">
          <input
            bind:value={newName}
            placeholder={m.databases_newPlaceholder()}
            onkeydown={(e) => e.key === 'Enter' && create()}
            class="flex-1 min-w-0 rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-lerd-red/30"
          />
          <DetailButton tone="primary" onclick={create} loading={creating} disabled={creating || !newName.trim()}>
            <span class="flex items-center gap-1"><Icon name="plus" class="w-3.5 h-3.5" />{m.databases_create()}</span>
          </DetailButton>
        </div>
        {#if createError}
          <p class="text-xs text-red-500">{createError}</p>
        {/if}
      </div>
    {/if}

    {#if engine?.error}
      <p class="text-xs text-red-500">{engine.error}</p>
    {:else if !engine || engine.databases.length === 0}
      <p class="text-sm text-gray-400 dark:text-gray-500">{m.databases_noDatabases()}</p>
    {:else}
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {#each pairs as pair (pair.entry.name)}
          <DatabaseCard {engine} entry={pair.entry} testing={pair.testing} />
        {/each}
      </div>
    {/if}
  {/if}
</div>
