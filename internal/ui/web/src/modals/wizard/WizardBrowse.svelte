<script lang="ts">
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { browseDir } from '$stores/browse';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    dir: string;
    // Directory path to the domain already serving it, so a project that is
    // linked is recognisable before it is picked.
    linked?: Record<string, string>;
    onnavigate: (dir: string) => void;
    onerror?: (message: string) => void;
  }
  let { dir, linked = {}, onnavigate, onerror }: Props = $props();

  let current = $state('');
  let dirs = $state<Array<{ name: string; path: string }>>([]);
  let loading = $state(false);

  async function browse(target: string) {
    loading = true;
    try {
      const res = await browseDir(target);
      if (res.error) {
        onerror?.(res.error);
        return;
      }
      dirs = res.dirs;
      current = res.current;
      onnavigate(res.current);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void browse(dir);
  });
</script>

<div class="px-5 py-2 max-h-[50vh] min-h-64 overflow-y-auto">
  {#if loading}
    <div class="py-4 text-center text-xs text-gray-400">{m.common_loading()}</div>
  {:else}
    <!-- Keyed on the folder, so walking the tree fades the next set of names in
         rather than swapping the list out from under the pointer. -->
    {#key current}
      <div in:fade={{ duration: 140 }}>
        {#each dirs as d (d.path)}
          <button
            onclick={() => browse(d.path)}
            class="w-full flex items-center gap-2 px-2 py-1.5 text-left text-sm rounded-sm hover:bg-gray-50 dark:hover:bg-white/5 transition-colors {d.name ===
            '..'
              ? 'text-gray-400'
              : 'text-gray-700 dark:text-gray-300'}"
          >
            <svg class="w-4 h-4 shrink-0 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="1.5"
                d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"
              />
            </svg>
            <span class="truncate">{d.name}</span>
            {#if linked[d.path]}
              <span
                class="ml-auto shrink-0 text-[11px] text-gray-500 dark:text-gray-400"
                title={m.siteWizard_alreadyLinked()}
              >
                {linked[d.path]}
              </span>
            {/if}
          </button>
        {/each}
        {#if dirs.length === 0}
          <div class="py-4 text-center text-xs text-gray-400">{m.link_noSubdirs()}</div>
        {/if}
      </div>
    {/key}
  {/if}
</div>
