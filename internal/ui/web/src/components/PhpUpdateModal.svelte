<script lang="ts">
  import UpdateBatchModal, { type BatchProgress, type BatchResult } from './UpdateBatchModal.svelte';
  import { updatePhpVersions } from '$stores/phpVersions';
  import { loadStatus } from '$stores/status';
  import { phpRuntime } from '$stores/phpRuntime';
  import { notifyLocalFailure } from '$lib/notify';
  import { m } from '../paraglide/messages.js';

  interface Props {
    versions: string[];
    onclose: () => void;
  }
  let { versions, onclose }: Props = $props();

  // The native runtime downloads a published build rather than building an
  // image, so the same endpoint is an update there and has to say so.
  const native = $derived($phpRuntime === 'native');
  const label = (v: string) => 'PHP ' + v;
  const items = $derived(
    versions.map((v) => ({
      name: v,
      label: label(v),
      target: native ? m.system_php_updateBuild() : m.system_php_baseUpdate()
    }))
  );

  let progress = $state<Record<string, BatchProgress | undefined>>({});

  async function run(names: string[], onresult: (r: BatchResult) => void) {
    const results = await updatePhpVersions(
      names,
      !native,
      (r) => {
        progress[r.name] = undefined;
        onresult(r);
      },
      (v, line) => (progress[v] = { message: line })
    );
    await loadStatus();
    return results;
  }

  // The dialog closes when the batch ends, so each failure outlives it as a
  // notification that names the version.
  function reportFailures(results: BatchResult[]) {
    for (const r of results.filter((x) => !x.ok)) notifyLocalFailure('php_update', 'PHP', `${label(r.name)}: ${r.error}`);
  }
</script>

<UpdateBatchModal
  {items}
  title={m.services_updateAll()}
  body={native ? m.system_php_updateAll_bodyNative() : m.system_php_updateAll_body()}
  confirmLabel={m.services_update()}
  {progress}
  {run}
  {onclose}
  ondone={reportFailures}
/>
