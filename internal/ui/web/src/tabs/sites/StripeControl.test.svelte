<script lang="ts">
  import StripeControl from './StripeControl.svelte';

  interface Props {
    running?: boolean;
    loading?: boolean;
    webhookPath?: string;
    onToggle?: () => void;
    onSaveConfig?: (path: string) => void;
  }
  let {
    running = false,
    loading = false,
    webhookPath = '',
    onToggle = () => {},
    onSaveConfig = () => {}
  }: Props = $props();
</script>

<StripeControl {running} {loading} {webhookPath} {onToggle} {onSaveConfig} />
