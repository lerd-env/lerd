<script lang="ts">
  import DetailButton from './DetailButton.svelte';
  import { m } from '../paraglide/messages.js';

  interface Props {
    message: string;
    retrying?: boolean;
    onretry: () => void;
  }
  let { message, retrying = false, onretry }: Props = $props();
</script>

<div class="space-y-2">
  <p class="text-sm text-red-500 dark:text-red-400">{message}</p>
  <DetailButton onclick={onretry} loading={retrying} disabled={retrying}>
    {m.common_retry()}
  </DetailButton>
</div>
