<script lang="ts">
  import PulseToggle from './PulseToggle.svelte';

  interface Props {
    enabled: boolean;
    busy?: boolean;
    title?: string;
    onclick?: (e: MouseEvent) => void;
  }
  let { enabled, busy, title, onclick }: Props = $props();
</script>

<PulseToggle {enabled} {busy} {title} {onclick}>
  <svg data-testid="icon"></svg>
</PulseToggle>
