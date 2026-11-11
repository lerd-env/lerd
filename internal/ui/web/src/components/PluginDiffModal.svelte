<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import Toggle from '$components/Toggle.svelte';
  import MonacoDiffEditor from '$components/MonacoDiffEditor.svelte';
  import MonacoEditor from '$components/MonacoEditor.svelte';
  import DiffViewActions from '$components/DiffViewActions.svelte';
  import { readDiffPrefs, writeDiffPrefs } from '$lib/diffPrefs';
  import { languageFor } from '$lib/gitDiff';
  import type { PluginDiff } from '$lib/pluginBridge';
  import { m } from '../paraglide/messages.js';

  interface Props {
    diff: PluginDiff;
    onclose: () => void;
    // Makes a single file editable; Save hands the text back.
    onsave?: (text: string) => void;
  }
  let { diff, onclose, onsave }: Props = $props();
  let text = $state('');
  $effect(() => {
    text = diff.modified;
  });

  let stats = $state<{ added: number; removed: number } | null>(null);
  const prefs = readDiffPrefs();
  let fullscreen = $state(prefs.fullscreen);
  let unified = $state(prefs.unified);
  let ignoreSpace = $state(prefs.ignoreSpace);
  $effect(() => writeDiffPrefs({ fullscreen, unified, ignoreSpace }));
</script>

<!-- A plugin's two versions of one file, in the same editor the git diff uses,
     or a single file read only when the page sent no original. -->
{#snippet saveBar()}
  <div class="flex justify-end gap-2">
    <button type="button" onclick={onclose} class="text-xs px-2 py-1 rounded-sm border border-gray-300 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5">{m.common_cancel()}</button>
    <button type="button" onclick={() => onsave?.(text)} class="text-xs px-3 py-1 rounded-sm bg-lerd-red hover:bg-lerd-redhov text-lerd-onred transition-colors">{m.common_save()}</button>
  </div>
{/snippet}

<!-- Only an editable file gets a footer; a diff or a view has nothing to save. -->
<Modal open title={diff.path} {onclose} size="2xl" tall {fullscreen} footer={onsave ? saveBar : undefined}>
  {#snippet actions()}
    <DiffViewActions bind:fullscreen bind:unified split={diff.original !== undefined} />
  {/snippet}
  <div class="flex flex-col h-full min-h-0">
    <div class="flex items-center gap-3 px-4 py-2 border-b border-gray-100 dark:border-lerd-border font-mono text-xs">
      <!-- Monaco puts the original on the left, so the names read in the same order. -->
      {#if diff.original === undefined}
        <span class="min-w-0 truncate font-sans text-gray-500 dark:text-gray-400">{diff.after ?? ''}</span>
      {:else if diff.before || diff.after}
        <span class="min-w-0 truncate font-sans text-gray-500 dark:text-gray-400">
          <span>{diff.before ?? ''}</span> → <span>{diff.after ?? ''}</span>
        </span>
      {/if}
      {#if stats && diff.original !== undefined}
        <span class="ml-auto shrink-0 tabular-nums text-green-600 dark:text-green-400">+{stats.added}</span>
        <span class="shrink-0 tabular-nums text-red-600 dark:text-red-400">−{stats.removed}</span>
      {/if}
      {#if diff.original !== undefined}
      <span class="{stats ? '' : 'ml-auto'} shrink-0 flex items-center gap-2 pl-3 border-l border-gray-200 dark:border-lerd-border font-sans text-gray-500 dark:text-gray-400">
        {m.gitDiff_ignoreSpace()}
        <Toggle on={ignoreSpace} title={m.gitDiff_ignoreSpace()} onclick={() => (ignoreSpace = !ignoreSpace)} />
      </span>
      {/if}
    </div>
    <div class="flex-1 min-h-0">
      <!-- Monaco takes the language only when it is created, so a new file gets a new editor. -->
      {#key diff}
      {#if diff.original === undefined}
        {#if onsave}
          <MonacoEditor bind:value={text} language={languageFor(diff.path)} onSave={() => onsave(text)} class="h-full" />
        {:else}
          <MonacoEditor value={diff.modified} language={languageFor(diff.path)} readOnly class="h-full" />
        {/if}
      {:else}
      <MonacoDiffEditor
        original={diff.original}
        modified={diff.modified}
        language={languageFor(diff.path)}
        {ignoreSpace}
        sideBySide={!unified}
        onStats={(s) => (stats = s)}
      />
      {/if}
      {/key}
    </div>
  </div>
</Modal>
