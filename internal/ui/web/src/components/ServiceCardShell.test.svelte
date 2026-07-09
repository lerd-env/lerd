<script lang="ts">
  import ServiceCardShell from './ServiceCardShell.svelte';

  interface Props {
    compact?: boolean;
  }
  let { compact = false }: Props = $props();
</script>

<ServiceCardShell {compact}>
  <span>card body</span>
</ServiceCardShell>
