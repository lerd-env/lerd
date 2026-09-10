<script lang="ts">
  import WorkerControl from './WorkerControl.svelte';
  import type { WorkerOption } from '$stores/sites';

  interface Props {
    label?: string;
    running?: boolean;
    options?: WorkerOption[];
    onToggle?: () => void;
    onSaveOptions?: (values: Record<string, string>) => void;
    onLogs?: () => void;
  }
  let {
    label = 'Queue',
    running = false,
    options = [],
    onToggle = () => {},
    onSaveOptions = () => {},
    onLogs
  }: Props = $props();
</script>

<WorkerControl {label} {running} {options} {onToggle} {onSaveOptions} {onLogs} />
