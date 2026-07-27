<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import SectionHeader from '$components/SectionHeader.svelte';
  import LoadingRow from '$components/LoadingRow.svelte';
  import LoadFailedRow from '$components/LoadFailedRow.svelte';
  import {
    entities,
    entityLoads,
    loadEntities,
    runEntityAction,
    kindLabel,
    type EntityKind
  } from '$stores/entities';
  import type { Service } from '$stores/services';
  import EntityCard from './EntityCard.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  // Reload whenever the selected service changes so switching between two
  // services doesn't show the previous one's contents.
  $effect(() => {
    void loadEntities(svc.name);
  });

  const loaded = $derived($entities[svc.name]);
  const kinds = $derived(loaded ?? []);
  const stopped = $derived(svc.status !== 'active');
  const load = $derived($entityLoads[svc.name]);
  const failed = $derived(!loaded && Boolean(load?.failed));

  let newName = $state<Record<string, string>>({});
  let creating = $state('');
  let createError = $state<Record<string, string>>({});

  function hasCreate(kind: EntityKind): boolean {
    return kind.actions.some((a) => a.name === 'create');
  }

  async function create(kind: EntityKind) {
    const name = (newName[kind.kind] ?? '').trim();
    if (!name) return;
    creating = kind.kind;
    createError = { ...createError, [kind.kind]: '' };
    const res = await runEntityAction(svc.name, kind.kind, 'create', name);
    creating = '';
    if (!res.ok) {
      createError = { ...createError, [kind.kind]: res.error || m.common_failed() };
      return;
    }
    newName = { ...newName, [kind.kind]: '' };
  }
</script>

<div class="p-3 sm:p-5 space-y-5 overflow-y-auto">
  {#if stopped}
    <p class="text-sm text-gray-400 dark:text-gray-500">{m.entities_startHint()}</p>
  {:else if failed}
    <LoadFailedRow
      message={m.entities_loadFailed()}
      retrying={Boolean(load?.pending)}
      onretry={() => loadEntities(svc.name)}
    />
  {:else if !loaded}
    <LoadingRow />
  {:else if kinds.length === 0}
    <p class="text-sm text-gray-400 dark:text-gray-500">{m.entities_empty()}</p>
  {:else}
    {#each kinds as kind (kind.kind)}
      <div class="space-y-4">
        {#if kinds.length > 1}
          <SectionHeader title={kindLabel(kind)} />
        {/if}

        {#if hasCreate(kind)}
          <div class="space-y-1.5">
            <div class="flex gap-2">
              <input
                bind:value={newName[kind.kind]}
                placeholder={m.entities_newPlaceholder()}
                onkeydown={(e) => e.key === 'Enter' && create(kind)}
                class="flex-1 min-w-0 rounded-lg border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-lerd-red/30"
              />
              <DetailButton
                tone="primary"
                onclick={() => create(kind)}
                loading={creating === kind.kind}
                disabled={creating === kind.kind || !(newName[kind.kind] ?? '').trim()}
              >
                <span class="flex items-center gap-1"><Icon name="plus" class="w-3.5 h-3.5" />{m.databases_create()}</span>
              </DetailButton>
            </div>
            {#if createError[kind.kind]}
              <p class="text-xs text-red-500">{createError[kind.kind]}</p>
            {/if}
          </div>
        {/if}

        {#if kind.error}
          <p class="text-xs text-red-500">{kind.error}</p>
        {:else if kind.rows.length === 0}
          <p class="text-sm text-gray-400 dark:text-gray-500">{m.entities_empty()}</p>
        {:else}
          <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {#each kind.rows as row (row.name)}
              <EntityCard service={svc.name} {kind} {row} />
            {/each}
          </div>
        {/if}
      </div>
    {/each}
  {/if}
</div>
