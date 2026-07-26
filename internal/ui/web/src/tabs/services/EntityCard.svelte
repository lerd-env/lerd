<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import Modal from '$components/Modal.svelte';
  import ActionCard from '$components/ActionCard.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { tooltip } from '$lib/tooltip';
  import { formatBytes } from '$lib/bytes';
  import {
    entityExportUrl,
    importEntity,
    runEntityAction,
    type EntityKind,
    type EntityRow
  } from '$stores/entities';
  import { goToTab } from '$stores/route';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    service: string;
    kind: EntityKind;
    row: EntityRow;
  }
  let { service, kind, row }: Props = $props();

  // The destructive action awaiting its confirmation, or null.
  let confirm = $state<string | null>(null);
  let busy = $state(false);
  let error = $state('');
  let fileInput = $state<HTMLInputElement | null>(null);
  // The archive being imported; null whenever no import has run on this card.
  let importOp = $state<{
    tone: 'busy' | 'done' | 'error';
    file: string;
    percent: number | null;
    error?: string;
  } | null>(null);
  const importBusy = $derived(importOp?.tone === 'busy');
  let clearDone: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(clearDone));

  const hasExport = $derived(kind.actions.some((a) => a.name === 'export'));
  const hasImport = $derived(kind.actions.some((a) => a.name === 'import'));
  // Create lives in the tab's own row and export/import render as streaming
  // affordances, so the generic buttons are everything else.
  const rowActions = $derived(
    kind.actions.filter((a) => !['create', 'export', 'import'].includes(a.name))
  );

  const importMessage = $derived.by(() => {
    if (!importOp) return '';
    if (importOp.tone === 'error') return m.databases_importFailed({ error: importOp.error ?? '' });
    if (importOp.tone === 'done') return m.databases_imported({ file: importOp.file });
    return importOp.percent === null
      ? m.databases_importing({ file: importOp.file })
      : m.databases_importingPercent({
          file: importOp.file,
          percent: Math.round(importOp.percent * 100)
        });
  });

  function formatValue(value: string, format?: string): string {
    if (format === 'bytes') {
      const n = Number(value);
      return Number.isFinite(n) ? formatBytes(n) : value;
    }
    return value;
  }

  async function run(action: string) {
    busy = true;
    error = '';
    const res = await runEntityAction(service, kind.kind, action, row.name);
    busy = false;
    if (!res.ok) {
      error = res.error || m.common_failed();
      return;
    }
    confirm = null;
  }

  async function onFilePicked(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0] ?? null;
    input.value = '';
    if (!file) return;
    importOp = { tone: 'busy', file: file.name, percent: 0 };
    const res = await importEntity(service, kind.kind, row.name, file, (p) => {
      if (importOp?.tone !== 'busy') return;
      // Once the last byte is out the command is still loading it, and there is
      // nothing left to measure, so the bar gives way to a spinner.
      importOp = { ...importOp, percent: p.uploaded ? null : p.percent };
    });
    if (!res.ok) {
      importOp = { tone: 'error', file: file.name, percent: null, error: res.error || m.common_failed() };
      return;
    }
    importOp = { tone: 'done', file: file.name, percent: null };
    clearTimeout(clearDone);
    clearDone = setTimeout(() => {
      if (importOp?.tone === 'done') importOp = null;
    }, 4000);
  }
</script>

<ActionCard>
  {#snippet header()}
    <Icon name="cube" class="w-4 h-4 mt-0.5 shrink-0 text-gray-300 dark:text-gray-600" />
    <div class="min-w-0 flex-1">
      <p class="truncate text-sm font-semibold text-gray-800 dark:text-gray-100" title={row.name}>{row.name}</p>
      <p class="flex flex-wrap items-center gap-x-1.5 text-xs text-gray-400 dark:text-gray-500">
        {#each kind.columns as col, i (col.key)}
          {#if i > 0}<span class="shrink-0" aria-hidden="true">·</span>{/if}
          <span class="shrink-0 tabular-nums" use:tooltip={col.label || col.key}>
            {formatValue(row.values?.[i] ?? '', col.format)}
          </span>
        {/each}
        {#if row.site}
          <!-- The separator travels with the domain so it never strands at the
               end of the line when the two wrap onto separate rows. -->
          <span class="inline-flex min-w-0 max-w-full items-center gap-1.5">
            <span class="shrink-0" aria-hidden="true">·</span>
            <button
              type="button"
              onclick={() => goToTab('sites', row.site ?? '')}
              class="min-w-0 truncate text-sky-600 dark:text-sky-400 hover:underline"
              title={row.site}
            >{row.site}</button>
          </span>
        {/if}
      </p>
    </div>
  {/snippet}

  {#snippet actions()}
    {#if hasExport}
      <a
        href={entityExportUrl(service, kind.kind, row.name)}
        use:tooltip={m.databases_export()}
        aria-label={m.databases_export()}
        class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
      >
        <Icon name="download" class="w-3.5 h-3.5" />
      </a>
    {/if}

    {#if hasImport}
      <button
        type="button"
        use:tooltip={m.databases_import()}
        aria-label={m.databases_import()}
        onclick={() => fileInput?.click()}
        disabled={importBusy}
        class="flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors"
      >
        <Icon name={importBusy ? 'spinner' : 'upload'} class="w-3.5 h-3.5 {importBusy ? 'animate-spin' : ''}" />
      </button>
      <input bind:this={fileInput} type="file" class="hidden" onchange={onFilePicked} />
    {/if}

    {#each rowActions as action (action.name)}
      <button
        type="button"
        use:tooltip={action.name}
        aria-label={action.name}
        onclick={() => (action.destructive ? (confirm = action.name) : run(action.name))}
        disabled={busy}
        class="flex items-center justify-center w-7 h-7 rounded-md transition-colors {action.destructive
          ? 'ml-auto text-gray-400 dark:text-gray-500 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-500/10'
          : 'text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5'}"
      >
        <Icon name={action.destructive ? 'trash' : 'play'} class="w-3.5 h-3.5" />
      </button>
    {/each}
  {/snippet}

  {#if importOp}
    <p
      class="mt-2 text-xs {importOp.tone === 'error'
        ? 'text-red-500'
        : importOp.tone === 'done'
          ? 'text-emerald-600 dark:text-emerald-400'
          : 'text-gray-500 dark:text-gray-400'}"
    >{importMessage}</p>
  {/if}
  {#if error && !confirm}
    <p class="mt-2 text-xs text-red-500">{error}</p>
  {/if}
</ActionCard>

{#if confirm}
  <Modal
    open
    title={m.entities_actionTitle({ action: confirm, name: row.name })}
    onclose={() => !busy && (confirm = null)}
    size="sm"
  >
    <div class="px-5 py-4 space-y-2">
      <p class="text-sm text-gray-700 dark:text-gray-300">{m.entities_actionBody()}</p>
      {#if error}
        <p class="text-xs text-red-500">{error}</p>
      {/if}
    </div>
    {#snippet footer()}
      <DetailButton onclick={() => (confirm = null)} disabled={busy}>{m.common_cancel()}</DetailButton>
      <DetailButton tone="danger" onclick={() => confirm && run(confirm)} loading={busy} disabled={busy}>
        {confirm}
      </DetailButton>
    {/snippet}
  </Modal>
{/if}
