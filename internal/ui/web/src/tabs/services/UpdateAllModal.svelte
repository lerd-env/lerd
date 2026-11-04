<script lang="ts">
  import UpdateBatchModal from '$components/UpdateBatchModal.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import {
    serviceLabel,
    updateServices,
    updateProgress,
    type Service,
    type ServiceUpdateResult
  } from '$stores/services';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    services: Service[];
    onclose: () => void;
    ondone: (results: ServiceUpdateResult[]) => void;
  }
  let { services, onclose, ondone }: Props = $props();

  function target(s: Service): string {
    if (!s.latest_version || s.latest_version === s.version) return s.version ?? '';
    return s.version ? s.version + ' → ' + s.latest_version : '→ ' + s.latest_version;
  }

  const items = $derived(services.map((s) => ({ name: s.name, label: serviceLabel(s.name), target: target(s) })));
  const byName = $derived(new Map(services.map((s) => [s.name, s])));
</script>

<UpdateBatchModal
  {items}
  title={m.services_updateAll()}
  body={m.services_updateAll_body()}
  confirmLabel={m.services_update()}
  progress={$updateProgress}
  run={updateServices}
  {onclose}
  {ondone}
>
  {#snippet icon(item)}
    {@const s = byName.get(item.name)}
    {#if s}<ServiceIcon name={s.name} category={s.category} icon={s.icon} color={s.color} preset={s.preset} compact />{/if}
  {/snippet}
</UpdateBatchModal>
