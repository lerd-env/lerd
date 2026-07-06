<script lang="ts">
  import Toggle from '$components/Toggle.svelte';
  import { setServiceShim, type Service } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    svc: Service;
  }
  let { svc }: Props = $props();

  const shims = $derived(svc.client_shims ?? []);

  let pending = $state<Record<string, boolean>>({});
  let failed = $state<Record<string, boolean>>({});

  async function toggle(tool: string, enabled: boolean) {
    pending = { ...pending, [tool]: true };
    failed = { ...failed, [tool]: false };
    const res = await setServiceShim(svc.name, { tool, enabled });
    pending = { ...pending, [tool]: false };
    if (!res.ok) failed = { ...failed, [tool]: true };
  }
</script>

<div class="p-3 sm:p-5 space-y-3 overflow-y-auto">
  <p class="text-[11px] text-gray-500 dark:text-gray-400">{m.services_tools_hint()}</p>

  {#if shims.length === 0}
    <p class="text-xs text-gray-400">{m.services_tools_none()}</p>
  {:else}
    <div class="flex flex-wrap gap-3">
      {#each shims as shim (shim.tool)}
        {@const managedElsewhere = Boolean(shim.owner && shim.owner !== svc.name)}
        <div
          class="rounded-xl border border-gray-200 dark:border-lerd-border bg-white dark:bg-lerd-card px-4 py-3 min-w-[150px] flex flex-col gap-2"
        >
          <div class="min-w-0">
            <code class="block text-sm font-mono font-semibold text-gray-800 dark:text-gray-100 leading-tight truncate">{shim.tool}</code>
            {#if managedElsewhere}
              <p class="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">{m.services_tools_providedBy({ service: shim.owner ?? '' })}</p>
            {:else if shim.host_has}
              <p class="text-[10px] text-amber-600 dark:text-amber-400 mt-0.5">{m.services_tools_shadow()}</p>
            {/if}
          </div>
          <div class="flex justify-end">
            <Toggle
              on={shim.enabled}
              tone={shim.host_has ? 'amber' : 'emerald'}
              loading={pending[shim.tool]}
              failing={failed[shim.tool]}
              disabled={managedElsewhere}
              title={managedElsewhere ? m.services_tools_providedBy({ service: shim.owner ?? '' }) : shim.tool}
              onclick={() => toggle(shim.tool, !shim.enabled)}
            />
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
