<script lang="ts">
  import AppLogsTab from './AppLogsTab.svelte';
  import type { Site } from '$stores/sites';

  // Only the site is a prop: branch is pinned here so a rerender moves the one
  // thing the sites store actually replaces, the site object.
  let { site }: { site: Site } = $props();
</script>

<AppLogsTab {site} branch="" />
