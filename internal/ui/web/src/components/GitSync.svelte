<script lang="ts">
  import { gitRemote, type GitRemoteOp } from '$stores/sites';
  import { openErrorModal } from '$stores/modals';
  import type { GitStatus } from '$lib/gitStatus';
  import { tooltip } from '$lib/tooltip';
  import ConfirmModal from './ConfirmModal.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    domain: string;
    // '' is the main checkout; otherwise the worktree's branch as lerd lists it.
    branch: string;
    // The branch name as shown on the tab, for the confirmation copy.
    branchLabel: string;
    status: GitStatus;
    onDone?: () => void;
  }
  let { domain, branch, branchLabel, status, onDone = () => {} }: Props = $props();

  const ops: { op: GitRemoteOp; label: () => string; failed: () => string }[] = [
    { op: 'fetch', label: m.sites_gitFetch, failed: m.sites_gitFetchFailed },
    { op: 'pull', label: m.sites_gitPull, failed: m.sites_gitPullFailed },
    { op: 'push', label: m.sites_gitPush, failed: m.sites_gitPushFailed }
  ];

  // Results are keyed by checkout so switching tabs never shows another one's ✓.
  let busy = $state<{ key: string; op: GitRemoteOp } | null>(null);
  let done = $state<{ key: string; op: GitRemoteOp; message: string } | null>(null);
  let doneTimer: ReturnType<typeof setTimeout> | undefined;
  const key = $derived(domain + '\n' + branch);
  $effect(() => () => clearTimeout(doneTimer));

  const diverged = $derived(status.ahead > 0 && status.behind > 0);

  function count(op: GitRemoteOp): number {
    return op === 'pull' ? status.behind : op === 'push' ? status.ahead : 0;
  }

  // Pull and push change the branch or the remote, so they ask first; fetch
  // only refreshes what is known about the remote.
  let confirming = $state<(typeof ops)[number] | null>(null);
  function ask(o: (typeof ops)[number]) {
    if (o.op !== 'fetch' && diverged) return;
    if (o.op === 'fetch') void run(o.op, o.failed());
    else confirming = o;
  }
  function confirmed() {
    const o = confirming;
    confirming = null;
    if (o) void run(o.op, o.failed());
  }

  async function run(op: GitRemoteOp, failed: string) {
    const k = key;
    busy = { key: k, op };
    const res = await gitRemote(domain, op, branch);
    busy = null;
    if (!res.ok) {
      openErrorModal(res.error || m.common_requestFailed(), failed);
    } else {
      clearTimeout(doneTimer);
      done = { key: k, op, message: res.message || m.sites_gitUpToDate() };
      doneTimer = setTimeout(() => (done = null), 4000);
    }
    onDone();
  }
</script>

<div class="flex items-center gap-0.5">
  {#each ops as o (o.op)}
    {@const n = count(o.op)}
    {@const running = busy?.key === key && busy.op === o.op}
    {@const ok = done?.key === key && done.op === o.op}
    {@const held = diverged && o.op !== 'fetch'}
    {@const label = ok && done ? done.message : held ? m.gitSync_diverged({ ahead: status.ahead, behind: status.behind }) : o.label()}
    <button
      type="button"
      onclick={() => ask(o)}
      disabled={busy?.key === key || (o.op === 'push' && n === 0)}
      aria-disabled={held}
      use:tooltip={label}
      aria-label={label}
      class="h-8 min-w-8 px-1.5 flex items-center justify-center gap-0.5 rounded-md transition-colors hover:bg-gray-100 dark:hover:bg-white/5 disabled:opacity-40 disabled:hover:bg-transparent {held
        ? 'opacity-40 cursor-not-allowed text-gray-500 dark:text-gray-400'
        : n > 0
          ? 'text-lerd-red'
          : 'text-gray-500 dark:text-gray-400 hover:text-lerd-red'}"
    >
      <svg
        class="w-4 h-4 shrink-0 {ok ? 'text-emerald-500' : ''} {running && o.op === 'fetch' ? 'animate-spin' : ''}"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        viewBox="0 0 24 24"
      >
        {#if ok}
          <path d="M5 13l4 4L19 7" />
        {:else if o.op === 'fetch'}
          <path d="M20 11a8 8 0 0 0-14.9-3M4 4v4h4M4 13a8 8 0 0 0 14.9 3M20 20v-4h-4" />
        {:else if o.op === 'pull'}
          <path d="M12 4v12m-5-5l5 5 5-5M5 20h14" class={running ? 'animate-pulse' : ''} />
        {:else}
          <path d="M12 20V8m-5 5l5-5 5 5M5 4h14" class={running ? 'animate-pulse' : ''} />
        {/if}
      </svg>
      {#if n > 0}
        <span class="text-[11px] font-medium leading-none tabular-nums">{n}</span>
      {/if}
    </button>
  {/each}
</div>

<ConfirmModal
  open={confirming !== null}
  title={confirming?.op === 'push' ? m.gitSync_pushTitle({ branch: branchLabel }) : m.gitSync_pullTitle({ branch: branchLabel })}
  body={confirming?.op === 'push'
    ? m.gitSync_pushBody({ branch: branchLabel, count: status.ahead })
    : m.gitSync_pullBody({ branch: branchLabel })}
  confirmLabel={confirming?.op === 'push' ? m.gitSync_push() : m.gitSync_pull()}
  onconfirm={confirmed}
  onclose={() => (confirming = null)}
/>
