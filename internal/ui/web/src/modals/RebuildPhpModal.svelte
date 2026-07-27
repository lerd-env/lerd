<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import BuildLog from '$components/BuildLog.svelte';
  import { closeModal } from '$stores/modals';
  import { streamPhpRebuild } from '$stores/phpVersions';
  import { loadStatus } from '$stores/status';
  import { m } from '../paraglide/messages.js';

  interface Props {
    version: string;
  }
  let { version }: Props = $props();

  let finished = $state(false);
  let error = $state('');
  let logs = $state<string[]>([]);

  // Cap retained log lines so a long rebuild doesn't grow the array (and
  // re-render cost) without bound; the tail is what matters.
  const MAX_LOG_LINES = 1000;

  // The rebuild keeps running server-side after the modal closes; a push
  // notification reports the result instead.
  let alive = true;
  let controller: AbortController | null = null;
  onDestroy(() => {
    alive = false;
    controller?.abort();
  });

  async function rebuild() {
    controller = new AbortController();
    try {
      const box: { done: boolean; ok: boolean; error?: string } = { done: false, ok: false };
      await streamPhpRebuild(
        version,
        (ev) => {
          if (!alive) return;
          if (ev.done) {
            box.done = true;
            box.ok = Boolean(ev.ok);
            box.error = ev.error;
            return;
          }
          if (ev.line !== undefined) {
            logs = [...logs, ev.line].slice(-MAX_LOG_LINES);
          }
        },
        controller.signal
      );
      if (!alive) return;
      await loadStatus();
      if (box.done && !box.ok) {
        error = box.error || m.system_php_rebuildFailed();
      }
      finished = true;
    } catch (e) {
      if (!alive) return;
      error = e instanceof Error ? e.message : m.system_php_rebuildFailed();
      finished = true;
    }
  }

  onMount(() => {
    void rebuild();
  });
</script>

<Modal open title={m.system_php_rebuildTitleFor({ version })} onclose={closeModal} size="lg">
  <div class="px-5 py-3 space-y-2">
    {#if finished && error}
      <div class="rounded-lg border border-red-200 dark:border-red-500/30 bg-red-50 dark:bg-red-500/10 px-3 py-2 text-xs text-red-700 dark:text-red-300">
        {error}
      </div>
    {/if}
    <BuildLog {logs} />
  </div>

  {#snippet footer()}
    {#if finished}
      <DetailButton tone="primary" onclick={closeModal}>{m.common_close()}</DetailButton>
    {:else}
      <DetailButton tone="primary" disabled loading={true}>{m.system_php_rebuilding()}</DetailButton>
    {/if}
  {/snippet}
</Modal>
