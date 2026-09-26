<script lang="ts">
  import Icon from '$components/Icon.svelte';
  import StatusDot from '$components/StatusDot.svelte';
  import MoonMark from '$components/MoonMark.svelte';
  import { runningWorkerColors, idleWorkerColors, siteWorkerFailing, siteHasWorkers, type Site } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  interface Props {
    site: Site;
  }
  let { site }: Props = $props();

  const dots = $derived(runningWorkerColors(site));
  // While asleep, keep showing the suspended workers' dots (dimmed) so the site
  // doesn't look like it lost its workers.
  const idleDots = $derived(idleWorkerColors(site));

  // A share on a branch counts for the row: the list shows one row per site,
  // and a shared worktree is still that site reachable from outside.
  const worktrees = $derived(site.worktrees ?? []);
  const tunnelUrl = $derived(site.tunnel_url || worktrees.find((w) => w.tunnel_url)?.tunnel_url || '');
  const lanUrl = $derived(
    site.lan_share_url || worktrees.find((w) => w.lan_share_url)?.lan_share_url || ''
  );
</script>

{#if tunnelUrl}
  <span title={m.sites_sharedPublicly({ url: tunnelUrl })} class="inline-flex shrink-0 text-violet-500 dark:text-violet-400">
    <Icon name="globe" class="w-3 h-3" />
  </span>
{/if}
{#if lanUrl}
  <span title={m.sites_sharedOnLan({ url: lanUrl })} class="inline-flex shrink-0 text-teal-500 dark:text-teal-400">
    <Icon name="wifi" class="w-3 h-3" />
  </span>
{/if}

{#if site.worktrees && site.worktrees.length > 0}
  <span title={m.sites_gitWorktrees()} class="inline-flex shrink-0">
    <svg class="w-3 h-3 text-violet-400" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 24 24">
      <path d="M6 3v12M15 6a3 3 0 1 0 6 0a3 3 0 1 0-6 0M3 18a3 3 0 1 0 6 0a3 3 0 1 0-6 0M18 9a9 9 0 0 1-9 9"/>
    </svg>
  </span>
{/if}
{#if siteWorkerFailing(site)}
  <span title={m.sites_workerFailing()} class="shrink-0"><StatusDot color="red" size="xs" pulse /></span>
{/if}
{#if site.idle_suspended && siteHasWorkers(site)}
  <!-- Asleep: keep the worker dots (dimmed) and float a moon above them. A site
       with no workers never sleeps, so it gets no moon at all. We key off
       idle_suspended (the engine's actual stopped-workers state, cleared the
       instant resume publishes) rather than idle (the timeout prediction read
       from the watcher's activity file, only re-saved every tick) so the moon
       drops in real time on resume instead of lingering until the next poll. -->
  {#if idleDots.length > 0}
    <span class="relative inline-flex items-center shrink-0" title={m.sites_idleHint()}>
      <span class="absolute -top-2.5 left-1/2 -translate-x-1/2 text-sky-500 dark:text-sky-400">
        <MoonMark class="w-3.5 h-3.5 drop-shadow-[0_1px_2px_rgba(0,0,0,0.45)]" />
      </span>
      <span class="inline-flex items-center gap-1 opacity-50">
        {#each idleDots as c, i (i + ':' + c)}
          <StatusDot color={c} size="xs" />
        {/each}
      </span>
    </span>
  {:else}
    <span class="inline-flex shrink-0 text-sky-500 dark:text-sky-400" title={m.sites_idleHint()}>
      <MoonMark class="w-4 h-4" />
    </span>
  {/if}
{:else}
  {#each dots as c, i (i + ':' + c)}
    <StatusDot color={c} size="xs" />
  {/each}
{/if}
