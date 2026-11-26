<script lang="ts">
  import Popover from '$components/Popover.svelte';
  import PopoverRow from '$components/PopoverRow.svelte';
  import { buttonMenuBaseClass, buttonMenuToneClass } from '$components/ButtonMenu.svelte';
  import Icon from '$components/Icon.svelte';
  import FrameworkMark from '$components/FrameworkMark.svelte';
  import StatusDot from '$components/StatusDot.svelte';
  import CoffeeMark from '$components/CoffeeMark.svelte';
  import { sites, siteDotColor } from '$stores/sites';
  import { goToTab } from '$stores/route';
  import { m } from '../paraglide/messages.js';

  interface Props {
    domains: string[];
    // Mark pinned sites, for a service they keep awake while it could sleep.
    showPinned?: boolean;
    // Joins a + to the group, which shows even with no sites yet.
    onadd?: () => void;
    addLabel?: string;
  }
  let { domains, showPinned = false, onadd, addLabel }: Props = $props();

  // The state comes from the sites store, so a row says what the site is
  // actually doing instead of carrying a decorative dot.
  const rows = $derived(
    domains.map((d) => {
      const site = $sites.find((s) => s.domain === d);
      return { domain: d, color: siteDotColor(site), framework: site?.framework, pinned: showPinned && !!site?.pinned };
    })
  );
  const anyPinned = $derived(rows.some((r) => r.pinned));
  const label = $derived(m.sites_count({ count: domains.length }));

  function open(close: () => void, domain: string) {
    close();
    goToTab('sites', domain);
  }
</script>

{#snippet addButton(corners: string)}
  <button type="button" onclick={onadd} title={addLabel} aria-label={addLabel} class="{buttonMenuBaseClass} {corners} px-1.5 {buttonMenuToneClass.secondary}">
    <Icon name="plus" class="w-3.5 h-3.5" />
  </button>
{/snippet}

{#if domains.length > 0}
  <div class="inline-flex">
  <Popover {label} align="right" width={260}>
    {#snippet triggerButton(toggle: () => void, isOpen: boolean)}
      <button
        type="button"
        onclick={toggle}
        title={label}
        aria-label={label}
        aria-expanded={isOpen}
        class="{buttonMenuBaseClass} {onadd ? 'rounded-l-lg' : 'rounded-lg'} {buttonMenuToneClass.secondary}"
      >
        {#if anyPinned}<span data-testid="sites-coffee" class="inline-flex"><CoffeeMark class="w-3.5 h-3.5" /></span>{/if}
        <Icon name="sites" class="w-3.5 h-3.5" />
        <span class="tabular-nums">{domains.length}</span>
        <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="6 9 12 15 18 9"/>
        </svg>
      </button>
    {/snippet}
    {#snippet children(close: () => void)}
      <ul class="py-1 max-h-72 overflow-y-auto">
        {#each rows as row (row.domain)}
          <PopoverRow label={row.domain} onclick={() => open(close, row.domain)}>
            {#snippet lead()}<StatusDot color={row.color} size="xs" />{/snippet}
            {#snippet trail()}
              {#if row.pinned}<span class="inline-flex shrink-0 text-gray-400" title={m.sites_pinned()}><CoffeeMark class="w-3 h-3" /></span>{/if}
              <FrameworkMark name={row.framework} />
            {/snippet}
          </PopoverRow>
        {/each}
      </ul>
    {/snippet}
  </Popover>
  {#if onadd}{@render addButton('rounded-r-lg border-l border-l-black/10 dark:border-l-white/10')}{/if}
  </div>
{:else if onadd}
  {@render addButton('rounded-lg')}
{/if}
