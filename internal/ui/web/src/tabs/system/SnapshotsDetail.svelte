<script lang="ts">
  import DetailPanel from '$components/DetailPanel.svelte';
  import DetailHeader from '$components/DetailHeader.svelte';
  import SettingsCard from '$components/SettingsCard.svelte';
  import StatusPill from '$components/StatusPill.svelte';
  import SnapshotScheduleFields from '$components/SnapshotScheduleFields.svelte';
  import { autoSnapshot, loadAutoSnapshot } from '$stores/autoSnapshot';
  import { onMount } from 'svelte';
  import { m } from '../../paraglide/messages.js';

  onMount(loadAutoSnapshot);
</script>

{#snippet pill()}
  <StatusPill
    tone={$autoSnapshot.enabled ? 'ok' : 'muted'}
    label={$autoSnapshot.enabled ? m.common_enabled() : m.common_disabled()}
  />
{/snippet}

<DetailPanel>
  <DetailHeader title={m.snapshots_title()} trailing={pill} />
  <div class="p-3 sm:p-5 space-y-4 overflow-y-auto">
    <SettingsCard>
      <SnapshotScheduleFields />
    </SettingsCard>
    <p class="text-xs text-gray-400 dark:text-gray-500">{m.snapshots_auto_perDatabaseHint()}</p>
  </div>
</DetailPanel>
