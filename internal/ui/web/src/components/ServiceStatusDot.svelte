<script lang="ts">
  import StatusDot from '$components/StatusDot.svelte';
  import MoonMark from '$components/MoonMark.svelte';
  import CoffeeMark from '$components/CoffeeMark.svelte';
  import type { Service } from '$stores/services';
  import { idleEnabled, idleServices } from '$stores/idle';
  import { m } from '../paraglide/messages.js';

  interface Props {
    svc: Pick<Service, 'status' | 'idle_suspended' | 'pinned' | 'kept_awake_by'> | undefined;
    size?: 'xs' | 'sm';
  }
  let { svc, size = 'sm' }: Props = $props();
  const keptBy = $derived(svc?.kept_awake_by ?? []);
  // The moon is wider than the dot, so both sit in its box and a sleeping
  // row's name starts where every other row's does.
  const box = $derived(`inline-flex shrink-0 items-center justify-center ${size === 'xs' ? 'w-2.5 h-2.5' : 'w-3 h-3'}`);
</script>

{#if svc?.idle_suspended && svc.status !== 'active'}
  <span class="{box} text-sky-500 dark:text-sky-400" title={m.services_sleeping()} data-testid="service-moon">
    <MoonMark class="w-full h-full" />
  </span>
{:else if (svc?.pinned || keptBy.length > 0) && $idleEnabled && $idleServices}
  <span class="{box} {svc?.status === 'active' ? 'text-emerald-500' : 'text-gray-400'}" title={svc?.pinned ? m.services_pinned() : m.services_keptAwakeBy({ sites: keptBy.join(', ') })} data-testid="service-coffee">
    <CoffeeMark class="w-full h-full" />
  </span>
{:else}
  <span class={box}><StatusDot color={svc?.status === 'active' ? 'green' : 'gray'} {size} /></span>
{/if}
