<script lang="ts">
  import UpdateBatchModal, { type BatchResult } from './UpdateBatchModal.svelte';
  import { updateTools, type ToolStatus } from '$stores/status';
  import { notifyLocalFailure } from '$lib/notify';
  import { m } from '../paraglide/messages.js';

  interface Props {
    tools: ToolStatus[];
    onclose: () => void;
  }
  let { tools, onclose }: Props = $props();

  const items = $derived(
    tools.map((t) => ({ name: t.name, label: t.name, target: t.installed + ' → ' + t.pinned }))
  );

  // The tool endpoint streams nothing, so the row just says which one is running.
  let running = $state('');
  const progress = $derived(running ? { [running]: { message: m.system_tools_updating() } } : {});

  function run(names: string[], onresult: (r: BatchResult) => void) {
    return updateTools(
      names,
      (r) => {
        running = '';
        onresult(r);
      },
      (name) => (running = name)
    );
  }

  // The dialog closes when the batch ends, so each failure outlives it as a
  // notification that names the tool.
  function reportFailures(results: BatchResult[]) {
    for (const r of results.filter((x) => !x.ok)) notifyLocalFailure('tool_update', m.system_tools_title(), `${r.name}: ${r.error}`);
  }
</script>

<UpdateBatchModal
  {items}
  title={m.services_updateAll()}
  body={m.system_tools_updateAll_body()}
  confirmLabel={m.services_update()}
  {progress}
  {run}
  {onclose}
  ondone={reportFailures}
/>
