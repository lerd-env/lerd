<script lang="ts">
  // Generic stand-in for ServiceDetail's heavy children (log SSE, database
  // fetches) so the tab-visibility tests mount without their IO.
  const {} = $props();
</script>

<div data-testid="child-stub"></div>
