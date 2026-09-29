<script lang="ts">
  import ServiceCardShell from './ServiceCardShell.svelte';

  interface Props {
    compact?: boolean;
    suggested?: boolean;
  }
  let { compact = false, suggested = false }: Props = $props();
</script>

<ServiceCardShell {compact} {suggested}>
  <span>card body</span>
</ServiceCardShell>
