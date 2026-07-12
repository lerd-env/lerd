<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    open: boolean;
    branch: string;
    database?: string;
    onclose: () => void;
    onconfirm: () => void;
  }
  let { open, branch, database = '', onclose, onconfirm }: Props = $props();

  function confirm() {
    onconfirm();
    onclose();
  }
</script>

<Modal {open} {onclose} title={m.worktreeDb_dropTitle()} size="sm">
  <div class="px-5 py-4 space-y-3">
    <p class="text-sm text-gray-700 dark:text-gray-300">{m.worktreeDb_dropBody({ branch })}</p>
    {#if database}
      <p class="text-[11px] font-mono text-gray-400 dark:text-gray-500">{database}</p>
    {/if}
  </div>
  {#snippet footer()}
    <button
      type="button"
      onclick={onclose}
      class="text-xs px-3 py-1.5 rounded-sm border border-gray-200 dark:border-lerd-border text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-white/5 transition-colors"
    >{m.common_cancel()}</button>
    <button
      type="button"
      onclick={confirm}
      class="text-xs px-3 py-1.5 rounded-sm bg-lerd-red hover:bg-lerd-redhov text-white transition-colors"
    >{m.worktreeDb_dropAction()}</button>
  {/snippet}
</Modal>
