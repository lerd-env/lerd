<script lang="ts">
  import SiteEnvTab from './SiteEnvTab.svelte';
  import type { Site } from '$stores/sites';

  // Only the site is a prop: branch is pinned here so a rerender moves the one
  // thing the sites store actually replaces, the site object.
  let { site }: { site: Site } = $props();
</script>

<SiteEnvTab {site} branch="" />
