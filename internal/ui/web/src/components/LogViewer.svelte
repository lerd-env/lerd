<script lang="ts">
  import { onDestroy, tick } from 'svelte';
  import { writable } from 'svelte/store';
  import { createLogStream, type LogStream } from '$lib/logStream';
  import { ansiToHtml } from '$lib/ansi';
  import { apiFetch, decodeJSONResult } from '$lib/api';
  import { tooltip } from '$lib/tooltip';
  import { m } from '../paraglide/messages.js';

  interface Props {
    path: string;
    emptyLabel?: string;
    maxLines?: number;
    highlight?: (line: string) => string | null;
  }
  let { path, emptyLabel, maxLines = 500, highlight }: Props = $props();
  const resolvedEmpty = $derived(emptyLabel ?? m.sites_appLogs_waiting());

  let current: LogStream | null = null;
  const lines = writable<string[]>([]);
  const connected = writable<boolean>(false);
  let lineUnsub: (() => void) | null = null;
  let connUnsub: (() => void) | null = null;
  let scrollEl: HTMLDivElement | null = $state(null);

  function bind(stream: LogStream) {
    lineUnsub?.();
    connUnsub?.();
    lineUnsub = stream.lines.subscribe((v) => {
      lines.set(v);
      tick().then(() => {
        if (scrollEl) scrollEl.scrollTop = scrollEl.scrollHeight;
      });
    });
    connUnsub = stream.connected.subscribe((v) => connected.set(v));
  }

  $effect(() => {
    const p = path;
    const max = maxLines;
    current?.close();
    const s = createLogStream(p, max);
    current = s;
    bind(s);
    s.connect();
    return () => {
      s.close();
    };
  });

  onDestroy(() => {
    lineUnsub?.();
    connUnsub?.();
    current?.close();
  });

  function reconnect() {
    current?.connect();
  }
  function clearLines() {
    current?.clear();
  }

  let terminalError: string | null = $state(null);

  // Hand the stream path to the daemon, which resolves it to the unit and
  // tails it in the user's terminal emulator.
  async function followInTerminal() {
    terminalError = null;
    try {
      const res = await apiFetch('/api/logs/terminal', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path })
      });
      const out = await decodeJSONResult<{ ok?: boolean; error?: string }>(res);
      if (!out.ok) terminalError = out.error ?? m.common_failed();
    } catch (e) {
      terminalError = e instanceof Error ? e.message : String(e);
    }
  }

  function lineClass(line: string): string {
    if (highlight) {
      const out = highlight(line);
      if (out) return out;
    }
    return 'text-gray-600 dark:text-gray-400';
  }
</script>

<div class="flex-1 flex flex-col overflow-hidden min-h-0">
  <div class="flex items-center justify-between px-3 sm:px-5 py-2 shrink-0">
    <span
      class="flex items-center gap-1.5 text-[10px] {$connected
        ? 'text-emerald-600 dark:text-emerald-500'
        : 'text-gray-400 dark:text-gray-600'}"
    >
      <span
        class="w-1.5 h-1.5 rounded-full {$connected
          ? 'bg-emerald-500 animate-pulse'
          : 'bg-gray-400 dark:bg-gray-600'}"
      ></span>
      {$connected ? m.common_live() : m.common_disconnected()}
    </span>
    <div class="flex items-center gap-2">
      {#if terminalError}
        <span class="text-[10px] text-red-500 truncate max-w-[16rem]" use:tooltip={terminalError}
          >{terminalError}</span
        >
      {/if}
      <button
        onclick={clearLines}
        use:tooltip={m.common_clear()}
        aria-label={m.common_clear()}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
          stroke-linecap="round" stroke-linejoin="round">
          <path d="m7 21-4.3-4.3a2 2 0 0 1 0-2.8l9.6-9.6a2 2 0 0 1 2.8 0l5.6 5.6a2 2 0 0 1 0 2.8L13 21" />
          <path d="M22 21H7" />
          <path d="m5 11 9 9" />
        </svg>
      </button>
      <button
        onclick={reconnect}
        use:tooltip={m.common_reconnect()}
        aria-label={m.common_reconnect()}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
      >
        <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
          stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" />
          <path d="M21 3v5h-5" />
          <path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" />
          <path d="M3 21v-5h5" />
        </svg>
      </button>
      <button
        onclick={followInTerminal}
        use:tooltip={m.common_followInTerminal()}
        aria-label={m.common_followInTerminal()}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
      >
        <svg class="w-[18px] h-[18px]" viewBox="0 0 24 24" fill="none" stroke="currentColor"
          stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="4" width="18" height="16" rx="2" />
          <path d="M7 9l3 3-3 3M13 15h4" />
        </svg>
      </button>
    </div>
  </div>
  <div
    bind:this={scrollEl}
    class="flex-1 overflow-y-auto bg-gray-50 dark:bg-lerd-bg px-3 py-3 font-mono text-[11px] leading-relaxed space-y-0.5"
  >
    {#if $lines.length === 0}
      <div class="text-gray-400 dark:text-gray-700 italic">{resolvedEmpty}</div>
    {:else}
      {#each $lines as line, i (i + ':' + line.slice(0, 20))}
        <div class="whitespace-pre-wrap break-all {lineClass(line)}">{@html ansiToHtml(line)}</div>
      {/each}
    {/if}
  </div>
</div>
