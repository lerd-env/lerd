<script lang="ts">
  import { onMount } from 'svelte';
  import Popover from './Popover.svelte';
  import Toggle from './Toggle.svelte';
  import Icon from './Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { notifyLocalFailure } from '$lib/notify';
  import { loadPagePlugins, setPagePlugin, type PagePlugin } from '$stores/plugins';
  import { m } from '../paraglide/messages.js';

  // Switches plugin page scripts on or off for one site. Draws nothing until an
  // installed plugin ships a page script, so the header stays as it was.

  interface Props {
    site: string;
    class?: string;
  }
  let { site, class: cls = '' }: Props = $props();

  let list = $state<PagePlugin[]>([]);
  let busy = $state('');
  const anyOn = $derived(list.some((p) => p.on));

  onMount(() => {
    loadPagePlugins(site)
      .then((l) => (list = l))
      .catch(() => {});
  });

  async function flip(p: PagePlugin) {
    busy = p.name;
    try {
      list = await setPagePlugin(site, p.name, !p.on);
    } catch (err) {
      notifyLocalFailure('page_plugin', m.sites_pagePlugins_label(), err instanceof Error ? err.message : String(err));
    } finally {
      busy = '';
    }
  }
</script>

{#if list.length > 0}
  <div class="{cls} items-center">
    <Popover label={m.sites_pagePlugins_label()} align="right" width={280}>
      {#snippet triggerButton(toggle, open)}
        <button
          type="button"
          onclick={toggle}
          aria-label={m.sites_pagePlugins_label()}
          aria-expanded={open}
          use:tooltip={m.sites_pagePlugins_label()}
          class="w-8 h-8 flex items-center justify-center rounded-md transition-colors {anyOn
            ? 'text-emerald-500 dark:text-emerald-400 hover:bg-emerald-50 dark:hover:bg-emerald-900/20'
            : 'text-gray-500 dark:text-gray-400 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5'}"
        >
          <Icon name="cube" class="w-4 h-4" />
        </button>
      {/snippet}
      {#snippet children()}
        <div class="p-3 space-y-2">
          <p class="text-xs text-gray-500 dark:text-gray-400">{m.sites_pagePlugins_hint()}</p>
          {#each list as p (p.name)}
            <div class="flex items-center gap-3">
              <div class="flex-1 min-w-0">
                <div class="text-sm text-gray-900 dark:text-white">{p.name}</div>
                {#if p.description}<div class="text-xs text-gray-500 dark:text-gray-400 truncate">{p.description}</div>{/if}
              </div>
              <Toggle on={p.on} loading={busy === p.name} disabled={busy !== ''} title={p.name} onclick={() => flip(p)} />
            </div>
          {/each}
        </div>
      {/snippet}
    </Popover>
  </div>
{/if}
