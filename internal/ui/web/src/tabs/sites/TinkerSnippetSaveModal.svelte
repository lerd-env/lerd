<script lang="ts">
  import { untrack } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import Dropdown from '$components/Dropdown.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import type { TinkerSnippet } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  // Saves the Tinker editor contents as a snippet file. When the chosen name
  // already exists at the destination, saving turns into an explicit
  // overwrite confirmation before anything is written.
  interface Props {
    open: boolean;
    snippets: TinkerSnippet[];
    initialName: string;
    initialSource: 'project' | 'global';
    saving?: boolean;
    error?: string;
    onsave: (name: string, source: 'project' | 'global') => void;
    onclose: () => void;
  }
  let { open, snippets, initialName, initialSource, saving = false, error = '', onsave, onclose }: Props = $props();

  let name = $state('');
  let source = $state<'project' | 'global'>('project');
  let confirmOverwrite = $state(false);

  // Seed only on the open transition (initial values read untracked), so a
  // background snippets refresh never overwrites what the user is typing.
  $effect(() => {
    if (open) {
      name = untrack(() => initialName);
      source = untrack(() => initialSource);
      confirmOverwrite = false;
    }
  });

  const fileName = $derived.by(() => {
    const n = name.trim();
    return n && !n.endsWith('.php') ? n + '.php' : n;
  });
  // Case-insensitive so macOS's default case-insensitive filesystem cannot
  // overwrite count-users.php via "Count-Users" without the confirmation.
  const exists = $derived(
    snippets.some((s) => s.source === source && s.name.toLowerCase() === fileName.toLowerCase())
  );

  const destinations = [
    { value: 'project', label: m.tinker_snippetDestProject(), description: '.lerd/tinker/snippets' },
    { value: 'global', label: m.tinker_snippetDestGlobal(), description: '~/.config/lerd/tinker/snippets' }
  ];

  function save() {
    if (saving || !fileName) return;
    if (exists && !confirmOverwrite) {
      confirmOverwrite = true;
      return;
    }
    onsave(name.trim(), source);
  }
</script>

<Modal {open} title={m.tinker_snippetSaveTitle()} onclose={() => { if (!saving) onclose(); }} size="sm">
  <div class="px-5 py-4 space-y-3">
    <label class="block text-sm text-gray-600 dark:text-gray-400" for="snippet-name">
      {m.tinker_snippetNameLabel()}
    </label>
    <input
      id="snippet-name"
      type="text"
      bind:value={name}
      oninput={() => (confirmOverwrite = false)}
      spellcheck="false"
      autocomplete="off"
      onkeydown={(e) => e.key === 'Enter' && !e.repeat && save()}
      class="w-full text-sm font-mono px-2.5 py-1.5 rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card text-gray-800 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-lerd-red"
    />
    <label class="block text-sm text-gray-600 dark:text-gray-400" for="snippet-dest">
      {m.tinker_snippetLocationLabel()}
    </label>
    <Dropdown
      value={source}
      options={destinations}
      onchange={(v) => { source = v as 'project' | 'global'; confirmOverwrite = false; }}
      width="full"
    />
    {#if confirmOverwrite}
      <p class="text-xs text-amber-600 dark:text-amber-400">{m.tinker_snippetOverwriteBody({ name: fileName })}</p>
    {/if}
    {#if error}
      <p class="text-xs text-red-600 dark:text-red-400">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={onclose} disabled={saving}>{m.common_cancel()}</DetailButton>
    <DetailButton
      tone={confirmOverwrite ? 'danger' : 'primary'}
      onclick={save}
      loading={saving}
      disabled={saving || !fileName}
    >
      {confirmOverwrite ? m.tinker_snippetOverwriteConfirm() : m.common_save()}
    </DetailButton>
  {/snippet}
</Modal>
