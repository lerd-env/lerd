<script lang="ts">
  import SiteHeader from './SiteHeader.svelte';
  import type { Site } from '$stores/sites';

  interface Props {
    site: Site;
    withTabs?: boolean;
    activeWorktreeBranch?: string;
  }
  let { site, withTabs = true, activeWorktreeBranch = '' }: Props = $props();
</script>

{#snippet tabs()}
  <button type="button">Overview</button>
  <button type="button">Env</button>
{/snippet}

<SiteHeader {site} {activeWorktreeBranch} tabs={withTabs ? tabs : undefined} />
