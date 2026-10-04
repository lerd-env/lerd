<script lang="ts">
  import { onMount } from 'svelte';
  import Dropdown from './Dropdown.svelte';
  import Modal from './Modal.svelte';
  import { editors, loadEditors } from '$stores/editors';
  import { m } from '../paraglide/messages.js';

  // Which editor files open in: for one site, where empty follows the global
  // choice, or globally, where empty finds an installed editor by itself. An
  // editor not on the list is set as a command or URL template.

  interface Props {
    value: string;
    onchange: (choice: string) => void;
    scope: 'site' | 'global';
    disabled?: boolean;
  }
  let { value, onchange, scope, disabled = false }: Props = $props();
  onMount(loadEditors);

  const CUSTOM = '__custom';
  let editing = $state(false);
  let template = $state('');

  const known = $derived(new Set($editors.editors.map((e) => e.id)));
  const shown = $derived(value && !known.has(value) ? CUSTOM : value);
  const globalLabel = $derived(
    $editors.global === 'custom' ? m.editor_custom() : ($editors.editors.find((e) => e.id === $editors.global)?.label ?? m.editor_auto())
  );
  const options = $derived([
    // A site following a global choice names it, still drawn as inherited.
    scope === 'site' && $editors.global
      ? { value: '', label: globalLabel, description: m.editor_default(), group: m.editor_label() }
      : { value: '', label: m.editor_default(), description: scope === 'site' ? globalLabel : m.editor_auto(), group: m.editor_label() },
    // What was found on this machine first, the rest after it still pickable.
    ...$editors.editors.filter((e) => e.installed).map((e) => ({ value: e.id, label: e.label, group: m.editor_groupFound() })),
    ...$editors.editors.filter((e) => !e.installed).map((e) => ({ value: e.id, label: e.label, group: m.editor_groupOther() })),
    { value: CUSTOM, label: m.editor_customOption(), group: m.editor_groupOther() }
  ]);

  function pick(choice: string) {
    if (choice !== CUSTOM) return onchange(choice);
    template = value && !known.has(value) ? value : '';
    editing = true;
  }
  function save() {
    if (!template.includes('{file}')) return;
    editing = false;
    onchange(template.trim());
  }
</script>

<Dropdown title={m.editor_label()} value={shown} {options} {disabled} inherited={scope === 'site' && !value} onchange={pick} minMenuWidth={220} />

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
