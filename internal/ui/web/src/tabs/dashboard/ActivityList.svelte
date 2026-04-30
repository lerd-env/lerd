<script lang="ts">
  import { onMount } from 'svelte';
  import { slide } from 'svelte/transition';
  import StatusDot from '$components/StatusDot.svelte';
  import { activity, now, type ActivityEvent, type ActivityKind } from '$stores/activity';
  import { m } from '../../paraglide/messages.js';


  // Suppress entry animations during the first paint so existing events
  // don't all slide in when the dashboard mounts. Flip to true after mount
  // so subsequent prepends animate.
  let mounted = $state(false);
  onMount(() => {
    requestAnimationFrame(() => (mounted = true));
  });

  type DotColor = 'green' | 'red' | 'gray' | 'amber' | 'sky' | 'violet';

  const dotColor: Record<ActivityKind, DotColor> = {
    site_linked: 'green',
    site_removed: 'gray',
    site_paused: 'amber',
    site_resumed: 'sky',
    site_running: 'green',
    site_stopped: 'red',
    service_added: 'green',
    service_removed: 'gray',
    service_active: 'green',
    service_inactive: 'red',
    service_update: 'amber',
    service_version: 'violet',
    worker_failed: 'red',
    worker_healed: 'green'
  };

  function label(e: ActivityEvent): string {
    const v = e.meta?.version ?? '';
    const w = e.meta?.worker ?? '';
    switch (e.kind) {
      case 'site_linked': return m.activity_site_linked({ subject: e.subject });
      case 'site_removed': return m.activity_site_removed({ subject: e.subject });
      case 'site_paused': return m.activity_site_paused({ subject: e.subject });
      case 'site_resumed': return m.activity_site_resumed({ subject: e.subject });
      case 'site_running': return m.activity_site_running({ subject: e.subject });
      case 'site_stopped': return m.activity_site_stopped({ subject: e.subject });
      case 'service_added': return m.activity_service_added({ subject: e.subject });
      case 'service_removed': return m.activity_service_removed({ subject: e.subject });
      case 'service_active': return m.activity_service_active({ subject: e.subject });
      case 'service_inactive': return m.activity_service_inactive({ subject: e.subject });
      case 'service_update': return m.activity_service_update({ subject: e.subject, version: v || '?' });
      case 'service_version': return m.activity_service_version({ subject: e.subject, version: v });
      case 'worker_failed': return m.activity_worker_failed({ worker: w, subject: e.subject });
      case 'worker_healed': return m.activity_worker_healed({ subject: e.subject });
    }
  }

  function relative(at: number, ref: number): string {
    const diff = Math.max(0, ref - at);
    if (diff < 30_000) return m.time_now();
    const mins = Math.floor(diff / 60_000);
    if (mins < 60) return m.time_min_ago({ n: Math.max(1, mins) });
    const hours = Math.floor(mins / 60);
    if (hours < 24) return m.time_hour_ago({ n: hours });
    const days = Math.floor(hours / 24);
    return m.time_day_ago({ n: days });
  }
</script>

{#if $activity.length === 0}
  <p class="text-xs text-gray-400 dark:text-gray-500">{m.dashboard_activity_empty()}</p>
{:else}
  <div class="space-y-1">
    {#each $activity as e (e.id)}
      <div
        class="flex items-start gap-2 text-xs py-0.5"
        in:slide|global={{ duration: mounted ? 220 : 0 }}
      >
        <span class="mt-1.5 shrink-0"><StatusDot color={dotColor[e.kind]} size="xs" /></span>
        <span class="flex-1 text-gray-700 dark:text-gray-300 leading-snug truncate">{label(e)}</span>
        <span class="shrink-0 text-[10px] font-mono text-gray-400 dark:text-gray-500 mt-0.5">{relative(e.at, $now)}</span>
      </div>
    {/each}
  </div>
{/if}
