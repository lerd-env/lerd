<script lang="ts">
  import SiteControls from './SiteControls.svelte';
  import SiteServiceCard from './SiteServiceCard.svelte';
  import SiteActionCards from './SiteActionCards.svelte';
  import type { Site } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
    activeWorktreeBranch?: string;
  }
  let { site, activeWorktreeBranch = '' }: Props = $props();

  const svcNames = $derived(site.services || []);
</script>

{#snippet sectionTitle(title: string)}
  <h3 class="mb-2.5 text-[11px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500">
    {title}
  </h3>
{/snippet}

<div class="flex-1 min-h-0 overflow-y-auto p-4 space-y-4">
  <section>
    {@render sectionTitle(m.sites_overview_runtimeWorkers())}
    <SiteControls {site} {activeWorktreeBranch} />
  </section>

  {#if svcNames.length > 0}
    <section>
      {@render sectionTitle(m.services_title())}
      <div class="grid grid-cols-2 sm:grid-cols-3 xl:grid-cols-4 gap-3">
        {#each svcNames as name (name)}
          <SiteServiceCard {name} />
        {/each}
      </div>
    </section>
  {/if}

  <section>
    {@render sectionTitle(m.palette_group_actions())}
    <SiteActionCards {site} branch={activeWorktreeBranch} />
  </section>
</div>
