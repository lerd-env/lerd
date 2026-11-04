<script lang="ts">
  import UpdateBatchModal, { type BatchResult } from './UpdateBatchModal.svelte';
  import { updateNodeVersions, type NodeVersionStatus } from '$stores/nodeVersions';
  import { notifyLocalFailure } from '$lib/notify';
  import { m } from '../paraglide/messages.js';

  interface Props {
    versions: NodeVersionStatus[];
    onclose: () => void;
  }
  let { versions, onclose }: Props = $props();

  const items = $derived(
    versions.map((v) => ({ name: v.major, label: 'Node ' + v.major, target: v.installed + ' → ' + v.latest }))
  );

  // The endpoint streams nothing, so the row just says which one is running.
  let running = $state('');
  const progress = $derived(running ? { [running]: { message: m.system_tools_updating() } } : {});

  function run(names: string[], onresult: (r: BatchResult) => void) {
    return updateNodeVersions(
      names,
      (r) => {
        running = '';
        onresult(r);
      },
      (name) => (running = name)
    );
  }

  // The dialog closes when the batch ends, so each failure outlives it as a
  // notification that names the version.
  function reportFailures(results: BatchResult[]) {
    for (const r of results.filter((x) => !x.ok)) notifyLocalFailure('node_update', m.system_nodeJs(), `${r.name}: ${r.error}`);
  }
</script>

<UpdateBatchModal
  {items}
  title={m.services_updateAll()}
  body={m.system_node_updateAll_body()}
  confirmLabel={m.services_update()}
  {progress}
  {run}
  {onclose}
  ondone={reportFailures}
/>
