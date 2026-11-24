<script lang="ts">
  import { scrollbarInPadding } from '$lib/scrollbarInPadding';
  import SiteControls from './SiteControls.svelte';
  import SiteServiceCard from './SiteServiceCard.svelte';
  import SiteSuggestedServiceCard from './SiteSuggestedServiceCard.svelte';
  import SiteRequestTiming from './SiteRequestTiming.svelte';
  import PluginFrame from '$components/PluginFrame.svelte';
  import Icon from '$components/Icon.svelte';
  import { openSiteServiceLinkModal } from '$stores/modals';
  import { coreServices } from '$stores/services';
  import { plugins } from '$stores/plugins';
  import { activeWorktreeDomain, type Site } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
    activeWorktreeBranch?: string;
  }
  let { site, activeWorktreeBranch = '' }: Props = $props();

  const svcNames = $derived(site.services || []);
  const suggested = $derived(site.suggested_services || []);
  const declared = $derived(site.declared_services || []);

  const unused = $derived($coreServices.some((s) => !svcNames.includes(s.name)));

  // The cards plugins draw here, the ones switched on for this site. They come
  // with the site, so a switch made in another dashboard lands here too.
  const cards = $derived(
    (site.overview_cards ?? []).flatMap((name) => $plugins.filter((p) => p.name === name && p.overview_card))
  );
</script>

{#snippet sectionTitle(title: string)}
  <h3 class="mb-2.5 text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
    {title}
  </h3>
{/snippet}

<div class="flex-1 min-h-0 overflow-y-auto scrollbar-autohide p-4 space-y-4 @container" use:scrollbarInPadding={'y'}>
  <section>
    {@render sectionTitle(m.sites_overview_runtimeWorkers())}
    <SiteControls {site} {activeWorktreeBranch} />
  </section>

  {#if svcNames.length > 0 || suggested.length > 0 || unused}
    <section>
      <div class="mb-2.5 flex items-center justify-between gap-3">
        <h3 class="text-[11px] font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
          {m.services_title()}
        </h3>
        {#if unused}
          <button
            type="button"
            onclick={() => openSiteServiceLinkModal({ domain: site.domain })}
            class="flex items-center gap-1 text-[11px] text-gray-500 dark:text-gray-400 hover:text-lerd-red transition-colors"
          >
            <Icon name="plus" class="w-3 h-3" />{m.sites_linkService_button()}
          </button>
        {/if}
      </div>
      <!-- The pane's width, not the window's: a tiled window with the sidebar open is narrow. -->
      <div class="grid grid-cols-2 @3xl:grid-cols-3 @5xl:grid-cols-4 gap-3">
        {#each svcNames as name (name)}
          <SiteServiceCard
            {name}
            database={site.db_database}
            domain={site.domain}
            site={site.name}
            declared={declared.includes(name)}
          />
        {/each}
        {#each suggested as suggestion (suggestion.name)}
          <SiteSuggestedServiceCard {suggestion} domain={site.domain} />
        {/each}
      </div>
    </section>
  {/if}

  {#each cards as p (p.name)}
    <section>
      {@render sectionTitle(p.overview_card!.title)}
      <div class="rounded-xl border border-gray-200/80 dark:border-lerd-border bg-white dark:bg-lerd-card overflow-hidden">
        <PluginFrame
          plugin={p.name}
          url={p.overview_card!.url}
          title={p.overview_card!.title}
          context={{ site: site.name ?? '', domain: activeWorktreeDomain(site, activeWorktreeBranch), branch: activeWorktreeBranch }}
          autoHeight
        />
      </div>
    </section>
  {/each}

  <SiteRequestTiming {site} {activeWorktreeBranch} />
</div>
