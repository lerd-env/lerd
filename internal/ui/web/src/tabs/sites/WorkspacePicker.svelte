<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import type { Site } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';
  import WorkspaceMenuItems from './WorkspaceMenuItems.svelte';

  interface Props {
    site: Site;
  }
  let { site }: Props = $props();

  const current = $derived(site.workspace ?? '');

  let open = $state(false);
  let creating = $state(false);
  let rootEl: HTMLDivElement | undefined = $state();
  let closeTimer: ReturnType<typeof setTimeout> | undefined;

  function close() {
    clearTimeout(closeTimer);
    open = false;
    creating = false;
  }

  function openNow() {
    clearTimeout(closeTimer);
    open = true;
  }

  // Same hover grace as the share menu; while the create input is up the
  // menu only closes explicitly (outside click, Esc, Cancel), not on
  // mouse-out mid-typing.
  function scheduleClose() {
    clearTimeout(closeTimer);
    closeTimer = setTimeout(() => {
      if (!creating) close();
    }, 250);
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

<div
  bind:this={rootEl}
  class="relative"
  role="none"
  onmouseenter={openNow}
  onmouseleave={scheduleClose}
  onfocusin={openNow}
  onfocusout={scheduleClose}
>
  <button
    type="button"
    onclick={openNow}
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
      <WorkspaceMenuItems {site} onDone={close} bind:creating />
    </div>
  {/if}
</div>
