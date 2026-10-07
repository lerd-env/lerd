<script lang="ts">
  import { onMount } from 'svelte';
  import Dropdown from './Dropdown.svelte';
  import Modal from './Modal.svelte';
  import { editors, loadEditors } from '$stores/editors';
  import { m } from '../paraglide/messages.js';

  // Which editor sites open in. Empty is Disabled: the site header offers no
  // editor, while file links still find an installed one as they always have.
  // An editor not on the list is set as a command or URL template.

  interface Props {
    value: string;
    onchange: (choice: string) => void;
    disabled?: boolean;
  }
  let { value, onchange, disabled = false }: Props = $props();
  onMount(loadEditors);

  const CUSTOM = '__custom';
  let editing = $state(false);
  let template = $state('');

  const known = $derived(new Set($editors.editors.map((e) => e.id)));
  // A saved template is an option of its own, so picking Custom… always opens
  // the dialog to edit it rather than reselecting what is already chosen.
  const saved = $derived(value && !known.has(value) ? value : '');
  const options = $derived([
    { value: '', label: m.editor_disabled(), group: m.editor_label() },
    // Only what this machine can open, plus a past choice since uninstalled so
    // it still reads as chosen; anything else goes in as a template.
    ...$editors.editors.filter((e) => e.installed || e.id === value).map((e) => ({ value: e.id, label: e.label, group: m.editor_groupFound() })),
    // The template itself is the label, so the button shows what was entered.
    ...(saved ? [{ value: saved, label: saved, description: m.editor_custom(), group: m.editor_groupOther() }] : []),
    { value: CUSTOM, label: m.editor_customOption(), group: m.editor_groupOther() }
  ]);

  function pick(choice: string) {
    if (choice !== CUSTOM) return onchange(choice);
    template = saved;
    editing = true;
  }
  function save() {
    if (!template.includes('{file}')) return;
    editing = false;
    onchange(template.trim());
  }
</script>

<!-- Capped so a long custom template truncates instead of crowding the row. -->
<div class="max-w-64 min-w-0">
  <Dropdown title={m.editor_label()} {value} {options} {disabled} onchange={pick} width="full" minMenuWidth={220} />
</div>

<Modal open={editing} title={m.editor_customTitle()} onclose={() => (editing = false)}>
  <div class="px-5 py-4 space-y-2 text-sm">
    <input class="w-full font-mono text-xs px-2 py-1.5 rounded-md border border-gray-300 dark:border-lerd-border bg-white dark:bg-lerd-card" placeholder={'myeditor --line {line} {file}'} bind:value={template} onkeydown={(e) => e.key === 'Enter' && save()} />
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.editor_customHint({ file: '{file}', line: '{line}' })}</p>
  </div>
  {#snippet footer()}
    <button type="button" class="text-xs px-3 py-1.5 rounded-md border border-gray-300 dark:border-lerd-border" onclick={() => (editing = false)}>{m.common_cancel()}</button>
    <button type="button" class="text-xs px-3 py-1.5 rounded-md bg-lerd-red text-white disabled:opacity-50" disabled={!template.includes('{file}')} onclick={save}>{m.common_save()}</button>
  {/snippet}
</Modal>
