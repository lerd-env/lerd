<script lang="ts">
  import DetailPanel from '$components/DetailPanel.svelte';
  import PhpDetail from './PhpDetail.svelte';
  import PhpVersionCard from './PhpVersionCard.svelte';
  import { phpVersions } from '$stores/phpVersions';
  import { status } from '$stores/status';
  import { sitesByPhp } from '$stores/sites';
  import { routeRest, goToTab } from '$stores/route';
  import { openPhpAddModal } from '$stores/modals';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    initialVersion?: string;
  }
  let { initialVersion = '' }: Props = $props();

  const phpDefault = $derived($status.php_default || '');

  // Default version first, then most sites (what you reach for most), then the
  // newest version. Keeps the busiest, most-relevant versions at the front.
  const ordered = $derived.by(() => {
    const counts = $sitesByPhp;
    return [...$phpVersions].sort((a, b) => {
      const ad = a === phpDefault ? 1 : 0;
      const bd = b === phpDefault ? 1 : 0;
      if (ad !== bd) return bd - ad;
      const ca = counts.get(a) ?? 0;
      const cb = counts.get(b) ?? 0;
      if (cb !== ca) return cb - ca;
      return parseFloat(b) - parseFloat(a);
    });
  });

  function pickInitial(): string {
    if (initialVersion && $phpVersions.includes(initialVersion)) return initialVersion;
    if (phpDefault && $phpVersions.includes(phpDefault)) return phpDefault;
    return $phpVersions[0] ?? '';
  }

  let active = $state<string>(pickInitial());

  // Honour deep links like #system/php-8.3 and react when the available
  // versions change (e.g. after removing the currently active one).
  $effect(() => {
    const rest = $routeRest;
    if (rest.startsWith('php-')) {
      const v = rest.slice(4);
      if ($phpVersions.includes(v) && v !== active) active = v;
    }
  });

  // When the active version disappears (Remove action, manual rm, etc.) fall
  // back AND realign the URL — otherwise the hash keeps pointing at the
  // removed version while the page shows the fallback.
  $effect(() => {
    if (!active || !$phpVersions.includes(active)) {
      const next = pickInitial();
      if (next && next !== active) {
        active = next;
        goToTab('system', 'php-' + next);
      } else if (!next) {
        active = '';
        if ($routeRest.startsWith('php-')) goToTab('system', '');
      }
    }
  });

  function pickVersion(v: string) {
    if (v === active) return;
    active = v;
    goToTab('system', 'php-' + v);
  }

  const fpmFor = (v: string) => $status.php_fpms.find((f) => f.version === v);
</script>

<DetailPanel>
  <div class="bg-gray-50/60 dark:bg-white/[0.02] border-b border-gray-100 dark:border-lerd-border shrink-0">
    <div class="flex items-stretch gap-3 px-3 py-3 overflow-x-auto snap-x">
      {#each ordered as v (v)}
        <PhpVersionCard
          version={v}
          patch={fpmFor(v)?.patch}
          running={fpmFor(v)?.running ?? false}
          isDefault={v === phpDefault}
          selected={v === active}
          onselect={() => pickVersion(v)}
        />
      {/each}
      <button
        type="button"
        onclick={() => openPhpAddModal()}
        class="shrink-0 w-24 snap-start flex flex-col items-center justify-center gap-1.5 rounded-2xl border border-dashed border-gray-200 dark:border-lerd-border text-gray-400 hover:text-lerd-red hover:border-lerd-red hover:bg-lerd-red/5 transition-colors"
        title={m.system_php_add()}
        aria-label={m.system_php_add()}
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        <span class="text-xs font-medium">{m.system_php_add()}</span>
      </button>
    </div>
  </div>

  {#if active}
    <PhpDetail version={active} />
  {/if}
</DetailPanel>
