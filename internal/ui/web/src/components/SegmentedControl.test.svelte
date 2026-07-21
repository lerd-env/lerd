<script lang="ts">
  import SegmentedControl, { type SegmentOption } from './SegmentedControl.svelte';

  interface Props {
    options: SegmentOption[];
    value: string;
    label?: string;
    onchange?: (value: string) => void;
  }
  let { options, value, label = '', onchange = () => {} }: Props = $props();
</script>

<SegmentedControl {options} {value} {label} {onchange} />
