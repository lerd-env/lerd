<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { relativeTime } from '$lib/relativeTime';
  import { loadGitCommits, type GitCommit, type GitCommitDirection, type GitCommitList } from '$lib/gitDiff';
  import { m } from '../paraglide/messages.js';

  // The commits a pull or push confirmation is about; picking one opens its diff.
  interface Props {
    domain: string;
    path: string;
    direction: GitCommitDirection;
    onopen: (c: GitCommit) => void;
  }
  let { domain, path, direction, onopen }: Props = $props();

  let list = $state<GitCommitList | null>(null);
  let error = $state('');
  let open = $state(false);
  const now = Date.now();

  onMount(async () => {
    try {
      list = await loadGitCommits(domain, path, direction);
    } catch (e) {
      error = e instanceof Error ? e.message : m.common_failed();
    }
  });
</script>

{#if error}
  <p class="text-xs text-red-500">{error}</p>
{:else if list && list.commits.length > 0}
  <div class="space-y-1.5">
    <button
      type="button"
      onclick={() => (open = !open)}
      aria-expanded={open}
      class="flex items-center gap-1 text-xs font-medium text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
    >
      <Icon name="chevron" class="w-3 h-3 shrink-0 transition-transform {open ? '' : '-rotate-90'}" />
      {m.gitCommits_title()} · {list.commits.length}{list.more ? '+' : ''}
    </button>
    <ul hidden={!open} class="max-h-48 overflow-y-auto rounded-lg border border-gray-200 dark:border-lerd-border divide-y divide-gray-100 dark:divide-lerd-border">
      {#each list.commits as c (c.hash)}
        <li>
          <button
            type="button"
            onclick={() => onopen(c)}
            title={m.gitCommits_open()}
            class="w-full flex items-center gap-3 px-3 py-1.5 text-left text-xs hover:bg-gray-50 dark:hover:bg-white/5"
          >
            <span class="shrink-0 font-mono text-lerd-red">{c.hash.slice(0, 7)}</span>
            <span class="truncate text-gray-700 dark:text-gray-200">{c.subject}</span>
            <span class="ml-auto shrink-0 tabular-nums text-gray-400" title={new Date(c.time * 1000).toLocaleString()}>{relativeTime(c.time * 1000, now)}</span>
          </button>
        </li>
      {/each}
    </ul>
    {#if open && list.more}
      <p class="text-[11px] text-gray-400">{m.gitCommits_more({ count: list.commits.length })}</p>
    {/if}
  </div>
{/if}
