<script lang="ts">
  import { routeRest } from '$stores/route';
  import { services } from '$stores/services';
  import ServiceDetail from './services/ServiceDetail.svelte';
  import ServicesDashboard from './services/ServicesDashboard.svelte';

  // routeRest may carry a sub-tab (e.g. "<service>/snapshots/<site>"); match the service segment.
  const selected = $derived($routeRest.split('/')[0]);
  const svc = $derived($services.find((s) => s.name === selected));
</script>

{#if svc}
  <ServiceDetail {svc} />
{:else}
  <ServicesDashboard />
{/if}
