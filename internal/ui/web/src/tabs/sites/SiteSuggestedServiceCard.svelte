<script lang="ts">
  import ServiceCardShell from '$components/ServiceCardShell.svelte';
  import ServiceIcon from '$components/ServiceIcon.svelte';
  import Icon from '$components/Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { apiFetch, decodeJSONResult } from '$lib/api';
  import { notifyLocalFailure } from '$lib/notify';
  import { serviceLabel } from '$stores/services';
  import type { ServiceSuggestion } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    suggestion: ServiceSuggestion;
    domain: string;
  }
  let { suggestion, domain }: Props = $props();

  const name = $derived(suggestion.name);
  const why = $derived(
    suggestion.package
      ? m.sites_suggestedService_why({ package: suggestion.package, reason: suggestion.reason || '' })
      : suggestion.reason || ''
  );

  let busy = $state(false);

  // Either action changes the site, and the dashboard's site event redraws the
  // overview without this card, so there is nothing to reset on success.
  async function act(action: 'service:add' | 'service:dismiss') {
    busy = true;
    try {
      const res = await apiFetch(
        `/api/sites/${encodeURIComponent(domain)}/${action}?name=${encodeURIComponent(name)}`,
        { method: 'POST' }
      );
      const out = await decodeJSONResult<{ ok?: boolean; error?: string }>(res);
      if (!out.ok) throw new Error(out.error || m.common_requestFailed());
    } catch (e) {
      notifyLocalFailure(
        'site_service',
        m.sites_suggestedService_failed({ name: serviceLabel(name) }),
        e instanceof Error ? e.message : String(e)
      );
    } finally {
      busy = false;
    }
  }
</script>

<ServiceCardShell compact suggested>
  <!-- Faded until hovered, so a suggestion never reads as a service the site
       already has; the actions stay at full strength to read as clickable. -->
  <span
    class="flex min-w-0 flex-1 items-center gap-2.5 opacity-60 group-hover:opacity-100 transition-opacity"
    data-testid="suggested-identity"
    use:tooltip={why}
  >
    <ServiceIcon {name} compact />
    <span class="min-w-0 flex-1">
      <span class="block text-xs font-semibold text-gray-800 dark:text-gray-100 truncate">{serviceLabel(name)}</span>
      <span class="block text-[10px] text-gray-500 dark:text-gray-400 truncate">
        {m.sites_suggestedService_status()}{#if suggestion.reason}: {suggestion.reason}{/if}
      </span>
    </span>
  </span>
  <button
    type="button"
    disabled={busy}
    use:tooltip={m.sites_suggestedService_add({ name: serviceLabel(name) })}
    aria-label={m.sites_suggestedService_add({ name: serviceLabel(name) })}
    onclick={() => act('service:add')}
    class="shrink-0 flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5 transition-colors disabled:opacity-50"
  >
    <Icon name={busy ? 'spinner' : 'plus'} class="w-3.5 h-3.5 {busy ? 'animate-spin' : ''}" />
  </button>
  <button
    type="button"
    disabled={busy}
    use:tooltip={m.sites_suggestedService_dismiss({ name: serviceLabel(name) })}
    aria-label={m.sites_suggestedService_dismiss({ name: serviceLabel(name) })}
    onclick={() => act('service:dismiss')}
    class="shrink-0 flex items-center justify-center w-7 h-7 rounded-md text-gray-400 dark:text-gray-500 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-white/5 transition-colors disabled:opacity-50"
  >
    <Icon name="close" class="w-3.5 h-3.5" />
  </button>
</ServiceCardShell>
