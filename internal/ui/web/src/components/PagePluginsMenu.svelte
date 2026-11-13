<script lang="ts">
  import { onMount } from 'svelte';
  import Popover from './Popover.svelte';
  import Icon from './Icon.svelte';
  import { tooltip } from '$lib/tooltip';
  import { notifyLocalFailure } from '$lib/notify';
  import { loadPagePlugins, setPagePlugin, siteSwitches, type PagePlugin, type PagePluginKind } from '$stores/plugins';
  import { m } from '../paraglide/messages.js';

  // Switches what plugins put on one site: scripts on its pages, cards on its
  // Overview and tabs beside it. Draws nothing until an installed plugin has
  // one of those, so the header stays as it was.

  interface Props {
    site: string;
    class?: string;
  }
  let { site, class: cls = '' }: Props = $props();

  let busy = $state('');
  const list = $derived($siteSwitches[site] ?? []);
  // Tabs are on until hidden, so only an opt-in switch lights the icon.
  const anyOn = $derived(list.some((p) => p.on && p.kind !== 'site_tab'));
  // One row per plugin and a column per kind any of them has, so a plugin's
  // name and description show once however many switches it brings.
  const columns = $derived(
    [
      { kind: 'page_script' as PagePluginKind, title: m.sites_pagePlugins_scripts(), hint: m.sites_pagePlugins_scriptsHint() },
      { kind: 'overview_card' as PagePluginKind, title: m.sites_tabs_overview(), hint: '' },
      { kind: 'site_tab' as PagePluginKind, title: m.sites_pagePlugins_tabs(), hint: '' }
    ].filter((c) => list.some((p) => p.kind === c.kind))
  );
  const rows = $derived(
    [...new Set(list.map((p) => p.name))].map((name) => ({
      name,
      description: list.find((p) => p.name === name)?.description,
      switches: columns.map((c) => list.find((p) => p.name === name && p.kind === c.kind))
    }))
  );

  onMount(() => {
    loadPagePlugins(site).catch(() => {});
  });

  async function flip(p: PagePlugin, box: HTMLInputElement) {
    busy = p.kind + ':' + p.name;
    try {
      await setPagePlugin(site, p.name, p.kind, !p.on);
    } catch (err) {
      box.checked = p.on;
      notifyLocalFailure('page_plugin', m.sites_pagePlugins_label(), err instanceof Error ? err.message : String(err));
    } finally {
      busy = '';
    }
  }
</script>

{#if list.length > 0}
  <div class="{cls} items-center">
    <Popover label={m.sites_pagePlugins_label()} align="right" width={320}>
      {#snippet triggerButton(toggle, open)}
        <button
          type="button"
          onclick={() => {
            // Read again on every open, so a switch made in another dashboard shows.
            if (!open) loadPagePlugins(site).catch(() => {});
            toggle();
          }}
          aria-label={m.sites_pagePlugins_label()}
          aria-expanded={open}
          use:tooltip={m.sites_pagePlugins_label()}
          class="w-8 h-8 flex items-center justify-center rounded-md transition-colors {anyOn
            ? 'text-emerald-500 dark:text-emerald-400 hover:bg-emerald-50 dark:hover:bg-emerald-900/20'
            : 'text-gray-500 dark:text-gray-400 hover:text-lerd-red hover:bg-gray-100 dark:hover:bg-white/5'}"
        >
          <Icon name="puzzle" class="w-4 h-4" />
        </button>
      {/snippet}
      {#snippet children()}
        <table class="w-full my-1.5">
          <thead>
            <tr class="text-[10px] font-semibold uppercase tracking-wider text-gray-400">
              <th class="px-3 py-1.5"></th>
              {#each columns as c (c.kind)}
                <th class="px-2 py-1.5 text-center font-semibold" use:tooltip={c.hint || undefined}>{c.title}</th>
              {/each}
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-lerd-border">
            {#each rows as r (r.name)}
              <tr>
                <td class="px-3 py-2">
                  <span class="text-sm font-medium text-gray-900 dark:text-white" use:tooltip={{ label: r.description ?? '', maxWidth: 280 }}>{r.name}</span>
                </td>
                {#each r.switches as p, i (columns[i].kind)}
                  <td class="px-2 py-2 text-center">
                    {#if p}
                      <input
                        type="checkbox"
                        class="align-middle accent-lerd-red disabled:opacity-50"
                        aria-label="{p.name}: {columns[i].title}"
                        checked={p.on}
                        disabled={busy !== ''}
                        onchange={(e) => flip(p, e.currentTarget)}
                      />
                    {/if}
                  </td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      {/snippet}
    </Popover>
  </div>
{/if}
