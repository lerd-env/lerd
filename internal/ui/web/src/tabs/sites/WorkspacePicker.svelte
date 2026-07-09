<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { status } from '$stores/status';
  import { assignSiteWorkspace } from '$stores/workspaces';
  import type { Site } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
  }
  let { site }: Props = $props();

  const workspaces = $derived($status.workspaces ?? []);
  const current = $derived(site.workspace ?? '');

  let open = $state(false);
  let creating = $state(false);
  let newName = $state('');
  let rootEl: HTMLDivElement | undefined = $state();

  function close() {
    open = false;
    creating = false;
    newName = '';
  }

  async function assign(workspace: string, create = false) {
    close();
    if (!site.name) return;
    const res = await assignSiteWorkspace([site.name], workspace, create);
    if (!res.ok) console.error('assign workspace failed:', res.error);
  }

  function submitNew() {
    const name = newName.trim();
    if (!name) return;
    assign(name, true); // one round trip: create it, then move the site in
  }

  function onDocClick(e: MouseEvent) {
    if (rootEl && !rootEl.contains(e.target as Node)) close();
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }
  $effect(() => {
    if (!open) return;
    document.addEventListener('mousedown', onDocClick);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDocClick);
      document.removeEventListener('keydown', onKey);
    };
  });
</script>

<div bind:this={rootEl} class="relative">
  <button
    type="button"
    onclick={() => (open = !open)}
    aria-haspopup="menu"
    aria-expanded={open}
    aria-label={m.workspaces_pickerLabel()}
    use:tooltip={current ? m.workspaces_pickerLabel() + ': ' + current : m.workspaces_pickerLabel()}
    class="w-8 h-8 flex items-center justify-center rounded-md transition-colors hover:bg-gray-100 dark:hover:bg-white/5 {current
      ? 'text-lerd-red'
      : 'text-gray-500 dark:text-gray-400 hover:text-lerd-red'}"
  >
    <Icon name="workspace" class="w-4 h-4" />
  </button>

  {#if open}
    <div
      role="menu"
      tabindex="-1"
      class="absolute right-0 top-full mt-1 z-50 min-w-52 rounded-xl border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card shadow-xl py-1"
    >
      {#if creating}
        <div class="p-2">
          <!-- svelte-ignore a11y_autofocus -->
          <input
            autofocus
            bind:value={newName}
            onkeydown={(e) => (e.key === 'Enter' ? submitNew() : null)}
            placeholder={m.workspaces_namePlaceholder()}
            class="w-full px-2 py-1.5 text-xs rounded-md border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-bg text-gray-800 dark:text-gray-200 focus:outline-none focus:border-lerd-red"
          />
          <div class="flex justify-end gap-1.5 mt-2">
            <button
              type="button"
              onclick={close}
              class="px-2 py-1 text-xs text-gray-500 hover:text-gray-700 dark:hover:text-gray-300">{m.common_cancel()}</button
            >
            <button
              type="button"
              onclick={submitNew}
              class="px-2 py-1 text-xs font-medium rounded-md bg-lerd-red hover:bg-lerd-redhov text-white">{m.common_add()}</button
            >
          </div>
        </div>
      {:else}
        {#each workspaces as name (name)}
          <button
            type="button"
            role="menuitemradio"
            aria-checked={current === name}
            onclick={() => assign(name)}
            class="w-full flex items-center gap-2 px-3 py-1.5 text-left text-xs transition-colors hover:bg-gray-50 dark:hover:bg-white/5 {current ===
            name
              ? 'text-lerd-red font-semibold'
              : 'text-gray-700 dark:text-gray-200'}"
          >
            <span class="w-3 h-3 shrink-0">
              {#if current === name}<Icon name="check" class="w-3 h-3" />{/if}
            </span>
            <span class="flex-1 truncate">{name}</span>
          </button>
        {/each}
        <button
          type="button"
          role="menuitemradio"
          aria-checked={current === ''}
          onclick={() => assign('')}
          class="w-full flex items-center gap-2 px-3 py-1.5 text-left text-xs transition-colors hover:bg-gray-50 dark:hover:bg-white/5 {current ===
          ''
            ? 'text-lerd-red font-semibold'
            : 'text-gray-700 dark:text-gray-200'}"
        >
          <span class="w-3 h-3 shrink-0">
            {#if current === ''}<Icon name="check" class="w-3 h-3" />{/if}
          </span>
          <span class="flex-1">{m.workspaces_none()}</span>
        </button>
        <div class="my-1 border-t border-gray-100 dark:border-lerd-border"></div>
        <button
          type="button"
          role="menuitem"
          onclick={() => (creating = true)}
          class="w-full flex items-center gap-2 px-3 py-1.5 text-left text-xs text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-white/5"
        >
          <Icon name="plus" class="w-3 h-3 shrink-0" />
          <span class="flex-1">{m.workspaces_new()}</span>
        </button>
      {/if}
    </div>
  {/if}
</div>
