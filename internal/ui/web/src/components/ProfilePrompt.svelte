<script lang="ts">
  import ProfilerIcon from './ProfilerIcon.svelte';
  import { profileRoute } from '$lib/profileRoute';
  import { normalizeRoute } from '$lib/route';
  import { openProfiler, openProfilerReport } from '$stores/dashboard';
  import { m } from '../paraglide/messages.js';

  // Under a request's timeline, for when its rows are not enough: profile the
  // same route with SPX and land on the flame graph.

  interface Props {
    // origin is the site's scheme and host, empty where SPX cannot run.
    origin: string;
    // request is the "METHOD /uri" the timeline is about.
    request: string;
    // profileKey names the SPX capture of this very request, when there is one.
    profileKey?: string;
    // onleave runs before the dashboard moves to the Profiler, so a dialog
    // holding the prompt can close itself.
    onleave?: () => void;
  }
  let { origin, request, profileKey = '', onleave }: Props = $props();

  // Only a GET can be reopened in a tab without replaying a form.
  const uri = $derived(/^GET (\/\S*)$/.exec(request)?.[1] ?? '');
  let busy = $state(false);
  let status = $state('');

  // The key is read before onleave, which may tear down whatever passed it in.
  function openGraph() {
    const key = profileKey;
    onleave?.();
    openProfilerReport(key);
  }

  async function profile() {
    if (busy) return;
    busy = true;
    const caught = await profileRoute(new URL(origin).host, normalizeRoute('GET', uri), origin + uri, (p) => {
      status = p === 'arming' ? m.sites_reqstats_profileArming() : m.sites_reqstats_profileWaiting();
    });
    status = caught ? '' : m.sites_reqstats_profileMissed();
    busy = false;
    if (caught) {
      onleave?.();
      openProfiler();
    }
  }
</script>

{#if profileKey}
  <div class="flex items-center gap-3 rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-3 py-2 text-xs">
    <ProfilerIcon class="w-3.5 h-3.5 shrink-0 text-gray-400" />
    <span class="flex-1 min-w-0 text-gray-600 dark:text-gray-300">{m.debug_profiled()}</span>
    <button type="button" onclick={openGraph} class="shrink-0 rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5">{m.timeline_flameGraph()}</button>
  </div>
{:else if origin && uri}
  <div class="flex items-center gap-3 rounded-sm border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-3 py-2 text-xs">
    <ProfilerIcon class="w-3.5 h-3.5 shrink-0 text-gray-400" />
    <span class="flex-1 min-w-0 text-gray-600 dark:text-gray-300">{status || m.debug_profilePrompt()}</span>
    <button type="button" onclick={profile} disabled={busy} class="shrink-0 rounded-sm border border-gray-300 dark:border-lerd-border px-2 py-1 hover:bg-gray-50 dark:hover:bg-white/5 disabled:opacity-50">{m.sites_reqstats_profile()}</button>
  </div>
{/if}
