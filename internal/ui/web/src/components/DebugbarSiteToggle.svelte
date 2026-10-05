<script lang="ts">
  import Toggle from './Toggle.svelte';
  import { loadDebugbarSite, setDebugbarSite } from '$stores/debugbar';
  import { m } from '../paraglide/messages.js';

  // Whether this site's pages carry the debug bar.
  let { site }: { site: string } = $props();
  let on = $state(false);
  let busy = $state(false);
  let source = $state('');

  $effect(() => {
    loadDebugbarSite(site).then((s) => ((on = s.enabled), (source = s.source)), () => {});
  });

  async function toggle() {
    if (busy) return;
    busy = true;
    try {
      const s = await setDebugbarSite(site, !on);
      on = s.enabled;
      source = s.source;
    } finally {
      busy = false;
    }
  }
</script>

<span class="inline-flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
  <Toggle {on} loading={busy} onclick={toggle} title={source === 'lerd.yaml' ? m.debugbar_site_inProject() : m.debugbar_site_title()} />
  {m.debugbar_site_label()}
</span>
