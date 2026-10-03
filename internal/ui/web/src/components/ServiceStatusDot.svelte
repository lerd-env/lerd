<script lang="ts">
  import StatusDot from '$components/StatusDot.svelte';
  import MoonMark from '$components/MoonMark.svelte';
  import type { Service } from '$stores/services';
  import { m } from '../paraglide/messages.js';

  interface Props {
    svc: Pick<Service, 'status' | 'idle_suspended'> | undefined;
    size?: 'xs' | 'sm';
  }
  let { svc, size = 'sm' }: Props = $props();
</script>

{#if svc?.idle_suspended && svc.status !== 'active'}
  <span class="inline-flex shrink-0 text-sky-500 dark:text-sky-400" title={m.services_sleeping()} data-testid="service-moon">
    <MoonMark class={size === 'xs' ? 'w-2.5 h-2.5' : 'w-3 h-3'} />
  </span>
{:else}
  <StatusDot color={svc?.status === 'active' ? 'green' : 'gray'} {size} />
{/if}
