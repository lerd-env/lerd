<script lang="ts">
  import type { Snippet } from 'svelte';
  import Popover from '$components/Popover.svelte';
  import PopoverRow from '$components/PopoverRow.svelte';
  import StatusDot from '$components/StatusDot.svelte';
  import FrameworkMark from '$components/FrameworkMark.svelte';
  import { brokenSites, siteIssue, type SiteIssue } from '$stores/sites';
  import { failedServices, serviceLabel } from '$stores/services';
  import { goToTab } from '$stores/route';
  import { m } from '../../paraglide/messages.js';

  // The yellow status pill, opening onto what is not running; each row leads to
  // the page that already holds the fix for it.
  interface Props {
    box: string;
    pill: Snippet;
  }
  let { box, pill }: Props = $props();

  const siteReason: Record<SiteIssue, () => string> = {
    build_failed: () => m.sites_controls_buildFailedBadge(),
    dev_server_down: () => m.dashboard_issue_devServerDown(),
    container_down: () => m.dashboard_issue_containerDown(),
    php_down: () => m.dashboard_issue_phpDown()
  };

  const rows = $derived([
    ...$brokenSites.map((s) => {
      const issue = siteIssue(s) as SiteIssue;
      return {
        key: 'site:' + s.domain,
        label: s.domain,
        reason: siteReason[issue](),
        failed: issue === 'build_failed',
        framework: s.framework,
        open: () => goToTab('sites', s.domain)
      };
    }),
    ...$failedServices.map((svc) => ({
      key: 'service:' + svc.name,
      label: serviceLabel(svc.name),
      reason: svc.status === 'offline' ? m.externalDb_offline() : m.dashboard_issue_failed(),
      failed: svc.status === 'failed',
      framework: undefined,
      open: () => goToTab('services', svc.name)
    }))
  ]);
  const label = $derived(m.dashboard_hero_showIssues());
</script>

<Popover {label} align="right" width={300}>
  {#snippet triggerButton(toggle: () => void, isOpen: boolean)}
    <button
      type="button"
      onclick={toggle}
      title={label}
      aria-expanded={isOpen}
      class="inline-flex items-center gap-2 px-2.5 py-1 rounded-full border text-xs font-medium hover:brightness-95 dark:hover:brightness-125 transition {box}"
    >
      {@render pill()}
      <span class="tabular-nums rounded-full px-1.5 bg-yellow-200/70 dark:bg-yellow-500/20">{rows.length}</span>
      <svg class="w-3 h-3 transition-transform {isOpen ? 'rotate-180' : ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polyline points="6 9 12 15 18 9" />
      </svg>
    </button>
  {/snippet}
  {#snippet children(close: () => void)}
    <ul class="py-1 max-h-72 overflow-y-auto">
      {#each rows as row (row.key)}
        <PopoverRow label={row.label} onclick={() => { close(); row.open(); }}>
          {#snippet lead()}<StatusDot color={row.failed ? 'red' : 'gray'} size="xs" />{/snippet}
          {#snippet trail()}
            <span class="shrink-0 {row.failed ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-gray-400'}">{row.reason}</span>
            <FrameworkMark name={row.framework} />
          {/snippet}
        </PopoverRow>
      {/each}
    </ul>
  {/snippet}
</Popover>
