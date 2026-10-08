<script lang="ts">
  import { dashboardIconSvg } from '$lib/dashboardIcons';

  // The Profiler's flame, for every control that profiles or opens a flame graph.
  let { class: klass = 'w-3.5 h-3.5' }: { class?: string } = $props();
</script>

<svg class={klass} fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
  {@html dashboardIconSvg('profiler')}
</svg>
