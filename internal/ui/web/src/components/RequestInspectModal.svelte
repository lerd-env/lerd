<script lang="ts">
  import Modal from './Modal.svelte';
  import SiteDebugTab from '$tabs/sites/SiteDebugTab.svelte';
  import ProfilePrompt from './ProfilePrompt.svelte';
  import type { RecentRequest } from '$stores/analytics';
  import type { Site } from '$stores/sites';
  import { m } from '../paraglide/messages.js';

  // One recent request, through the same lenses as the Debug tab, pinned to
  // what it ran and opening on its timeline.

  interface Props {
    site: Site;
    request: RecentRequest | null;
    // origin is where the request's route is profiled, empty where SPX cannot run.
    origin: string;
    onclose: () => void;
  }
  let { site, request, origin, onclose }: Props = $props();

  // The access log records a request as it ends, so it started that long before.
  const served = $derived(request ? { label: `${request.method} ${request.uri}`, start: request.at_millis - request.millis, millis: request.millis } : undefined);
</script>

<Modal open={request !== null} title={served?.label ?? ''} size="2xl" {onclose}>
  {#if request?.rid}
    <div class="h-[75vh] flex flex-col">
      <div class="flex-1 min-h-0">
        {#key request.rid}
          <SiteDebugTab siteName={site.name} framework={site.framework} domain={site.domain} phpLenses={Boolean(site.uses_php)} rid={request.rid} {served} {origin} onleave={onclose} />
        {/key}
      </div>
    </div>
  {:else if request}
    <!-- Nothing captured, or no longer buffered: the request can still be profiled. -->
    <div class="p-5 space-y-3">
      <p class="text-sm text-gray-600 dark:text-gray-300">{m.sites_timing_nothingCaptured()}</p>
      <ProfilePrompt {origin} request={served?.label ?? ''} profileKey={request?.profile_key ?? ''} onleave={onclose} />
    </div>
  {/if}
</Modal>
