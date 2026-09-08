<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import { importPalette } from '$stores/palettes';
  import { palette } from '$stores/theme';
  import { m } from '../../paraglide/messages.js';

  // Imports a theme file into ~/.config/lerd/themes. The file's own base name
  // becomes the theme id, which is what a hand-written file in that directory
  // gets too, so importing and copying land on the same thing.
  interface Props {
    open: boolean;
    onclose: () => void;
  }
  let { open, onclose }: Props = $props();

  let id = $state('');
  let content = $state('');
  let saving = $state(false);
  let error = $state('');

  async function pick(e: Event) {
    const file = (e.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    content = await file.text();
    if (!id) id = file.name.replace(/\.ya?ml$/i, '').toLowerCase().replace(/[^a-z0-9-]/g, '-');
    error = '';
  }

  async function save() {
    if (saving || !id.trim() || !content.trim()) return;
    saving = true;
    error = await importPalette(id.trim(), content);
    saving = false;
    if (error) return;
    palette.set(id.trim());
    onclose();
  }
</script>

<Modal {open} title={m.system_theme_importTitle()} onclose={() => { if (!saving) onclose(); }} size="sm">
  <div class="px-5 py-4 space-y-3">
    <p class="text-xs text-gray-500 dark:text-gray-400">{m.system_theme_importBody()}</p>
    <input
      type="file"
      accept=".yaml,.yml"
      onchange={pick}
      class="block w-full text-xs text-gray-600 dark:text-gray-400 file:mr-3 file:rounded-sm file:border-0 file:bg-gray-100 dark:file:bg-white/10 file:px-2.5 file:py-1.5 file:text-xs file:text-gray-700 dark:file:text-gray-200"
    />
    <label class="block text-sm text-gray-600 dark:text-gray-400" for="theme-id">
      {m.system_theme_importNameLabel()}
    </label>
    <input
      id="theme-id"
      type="text"
      bind:value={id}
      spellcheck="false"
      autocomplete="off"
      placeholder="ocean"
      class="w-full text-sm font-mono px-2.5 py-1.5 rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card text-gray-800 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-lerd-red"
    />
    <textarea
      bind:value={content}
      spellcheck="false"
      rows="7"
      placeholder={'name: Ocean\naccent: "#3b7ea1"'}
      class="w-full text-xs font-mono px-2.5 py-1.5 rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card text-gray-800 dark:text-gray-100 focus:outline-none focus:ring-1 focus:ring-lerd-red"
    ></textarea>
    {#if error}
      <p class="text-xs text-red-600 dark:text-red-400">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    <DetailButton onclick={onclose} disabled={saving}>{m.common_cancel()}</DetailButton>
    <DetailButton
      tone="primary"
      onclick={save}
      loading={saving}
      disabled={saving || !id.trim() || !content.trim()}
    >
      {m.system_theme_importAction()}
    </DetailButton>
  {/snippet}
</Modal>
