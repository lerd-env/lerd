<script lang="ts">
  import Modal from '$components/Modal.svelte';
  import DetailButton from '$components/DetailButton.svelte';
  import Icon from '$components/Icon.svelte';
  import { closeModal } from '$stores/modals';
  import { loadSites, sites, type Site } from '$stores/sites';
  import { status } from '$stores/status';
  import { addDomain, editDomain, removeDomain } from '$stores/domains';
  import { m } from '../paraglide/messages.js';

  interface Props {
    site: Site;
  }
  let { site }: Props = $props();

  // The global default ending, used to pre-fill the add picker.
  const defaultTld = $derived($status.dns.tld || 'test');

  // Reactively track the latest site record. Match by name first so we survive
  // primary-domain renames; fall back to the initial domain.
  const current = $derived(
    $sites.find((s) => site.name && s.name === site.name) ??
      $sites.find((s) => s.domain === site.domain) ??
      site
  );

  // Split each full domain into { label, tld } so different endings on the same
  // site (e.g. alice.test and alice.local) each render and edit under their own
  // ending instead of a single global suffix.
  interface DomainEntry {
    full: string;
    label: string;
    tld: string;
  }
  const entries = $derived<DomainEntry[]>(
    (current.domains || [current.domain]).map((d) => {
      const i = d.lastIndexOf('.');
      return i > 0
        ? { full: d, label: d.slice(0, i), tld: d.slice(i + 1) }
        : { full: d, label: d, tld: defaultTld };
    })
  );
  const conflicting = $derived(current.conflicting_domains || []);

  // Ending suggestions for the add picker: the global default plus endings
  // already used on this site, default first.
  const tldSuggestions = $derived(
    Array.from(new Set([defaultTld, ...entries.map((e) => e.tld)]))
  );

  let newDomain = $state('');
  let newTLD = $state('');
  let editIndex = $state(-1);
  let editValue = $state('');
  let loading = $state(false);
  let error = $state('');
  let flash = $state('');
  let notice = $state('');
  let flashTimer: ReturnType<typeof setTimeout> | null = null;

  function showFlash(msg: string) {
    flash = msg;
    if (flashTimer) clearTimeout(flashTimer);
    flashTimer = setTimeout(() => (flash = ''), 3000);
  }

  async function runAction(
    fn: () => Promise<{ ok: boolean; error?: string; warning?: string }>,
    successMsg: string
  ) {
    loading = true;
    error = '';
    try {
      const r = await fn();
      if (!r.ok) {
        error = r.error || m.common_failed();
        return;
      }
      await loadSites();
      // A backend warning (e.g. a new ending needs a one-time terminal step to
      // finish DNS setup) persists until the next action; otherwise flash OK.
      notice = r.warning || '';
      showFlash(successMsg);
    } finally {
      loading = false;
    }
  }

  function startEdit(i: number) {
    editIndex = i;
    editValue = entries[i].label;
  }
  function cancelEdit() {
    editIndex = -1;
    editValue = '';
  }
  async function saveEdit(i: number) {
    const e = entries[i];
    const newName = editValue.trim().toLowerCase();
    if (!newName || newName === e.label) {
      cancelEdit();
      return;
    }
    await runAction(() => editDomain(current, e.label, newName, e.tld), m.domains_flash_updated());
    if (!error) cancelEdit();
  }
  async function add() {
    const name = newDomain.trim().toLowerCase();
    if (!name) return;
    const tld = (newTLD || defaultTld).trim().toLowerCase().replace(/^\./, '');
    await runAction(() => addDomain(current, name, tld), m.domains_flash_added());
    if (!error) newDomain = '';
  }
  async function remove(e: DomainEntry) {
    if (entries.length <= 1) {
      error = m.domains_cannotRemoveLast();
      return;
    }
    await runAction(() => removeDomain(current, e.label, e.tld), m.domains_flash_removed());
  }
  async function removeConflict(fullDomain: string) {
    const i = fullDomain.lastIndexOf('.');
    const label = i > 0 ? fullDomain.slice(0, i) : fullDomain;
    const tld = i > 0 ? fullDomain.slice(i + 1) : defaultTld;
    await runAction(() => removeDomain(current, label, tld), m.domains_flash_removedYaml());
  }
</script>

<Modal open title={m.domains_title()} onclose={closeModal}>
  <div class="px-5 py-4 space-y-2 max-h-64 overflow-y-auto">
    {#each conflicting as c (c.domain)}
      <div class="flex items-center gap-2 group opacity-70">
        <svg class="w-4 h-4 text-amber-500 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M4.93 19h14.14a2 2 0 001.74-3l-7.07-12a2 2 0 00-3.48 0L3.19 16a2 2 0 001.74 3z"/>
        </svg>
        <div class="flex-1 min-w-0 flex items-center gap-1.5">
          <span class="text-sm font-mono text-gray-500 dark:text-gray-400 truncate line-through">{c.domain}</span>
          {#if c.owned_by}
            <span class="text-[10px] font-medium text-amber-700 dark:text-amber-400 bg-amber-50 dark:bg-amber-900/20 px-1.5 py-0.5 rounded-sm shrink-0">{m.domains_conflict_usedBy({ owner: c.owned_by })}</span>
          {/if}
        </div>
        <button
          onclick={() => removeConflict(c.domain)}
          disabled={loading}
          class="text-gray-400 hover:text-red-500 transition-colors disabled:opacity-50"
          title={m.domains_conflict_removeYaml()}
        >
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
          </svg>
        </button>
      </div>
    {/each}

    {#each entries as entry, i (entry.full + ':' + i)}
      <div class="flex items-center gap-2">
        {#if editIndex !== i}
          <div class="flex-1 min-w-0 flex items-center gap-1.5">
            <span class="text-sm font-mono text-gray-700 dark:text-gray-300 truncate">{entry.label}</span>
            <span class="text-sm text-gray-400 dark:text-gray-500 shrink-0">.{entry.tld}</span>
            {#if i === 0}
              <span class="text-[10px] font-medium text-lerd-red bg-red-50 dark:bg-red-900/20 px-1.5 py-0.5 rounded-sm shrink-0">{m.domains_primary()}</span>
            {/if}
          </div>
          <button
            onclick={() => startEdit(i)}
            class="text-gray-400 hover:text-lerd-red transition-colors"
            title={m.common_edit()}
          >
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z"/>
            </svg>
          </button>
          {#if entries.length > 1}
            <button
              onclick={() => remove(entry)}
              disabled={loading}
              class="text-gray-400 hover:text-red-500 transition-colors disabled:opacity-50"
              title={m.common_remove()}
            >
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
              </svg>
            </button>
          {/if}
        {:else}
          <input
            bind:value={editValue}
            onkeydown={(e) => {
              if (e.key === 'Enter') saveEdit(i);
              if (e.key === 'Escape') cancelEdit();
            }}
            class="flex-1 text-sm font-mono bg-transparent border border-lerd-red/50 rounded-sm px-2 py-1 text-gray-700 dark:text-gray-300 focus:outline-hidden focus:border-lerd-red"
            disabled={loading}
          />
          <span class="text-sm text-gray-400 shrink-0">.{entry.tld}</span>
          <button onclick={() => saveEdit(i)} disabled={loading} class="text-emerald-500 hover:text-emerald-600 disabled:opacity-50" title={m.common_save()}>
            <Icon name="check" class="w-4 h-4" />
          </button>
          <button onclick={cancelEdit} class="text-gray-400 hover:text-gray-600" title={m.common_cancel()}>
            <Icon name="close" class="w-4 h-4" />
          </button>
        {/if}
      </div>
    {/each}
  </div>

  <div class="px-5 py-3 border-t border-gray-100 dark:border-lerd-border">
    <div class="flex items-center gap-2">
      <input
        type="text"
        bind:value={newDomain}
        placeholder={m.domains_add()}
        onkeydown={(e) => e.key === 'Enter' && add()}
        disabled={loading}
        class="flex-1 text-sm font-mono bg-transparent border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 placeholder-gray-400 dark:placeholder-gray-600 focus:outline-hidden focus:border-lerd-red/50"
      />
      <span class="text-sm text-gray-400 shrink-0">.</span>
      <input
        type="text"
        list="domain-tld-options"
        bind:value={newTLD}
        placeholder={defaultTld}
        disabled={loading}
        onkeydown={(e) => e.key === 'Enter' && add()}
        class="w-24 text-sm font-mono bg-transparent border border-gray-200 dark:border-lerd-border rounded-sm px-2 py-1.5 text-gray-700 dark:text-gray-300 placeholder-gray-400 dark:placeholder-gray-600 focus:outline-hidden focus:border-lerd-red/50"
      />
      <datalist id="domain-tld-options">
        {#each tldSuggestions as t (t)}
          <option value={t}></option>
        {/each}
      </datalist>
      <DetailButton tone="primary" onclick={add} disabled={loading || !newDomain.trim()}>{m.common_add()}</DetailButton>
    </div>
  </div>

  {#if notice}
    <div class="px-5 py-2 border-t border-gray-100 dark:border-lerd-border">
      <p class="text-xs text-amber-700 dark:text-amber-400 bg-amber-50 dark:bg-amber-500/10 rounded-lg px-2 py-1.5">{notice}</p>
    </div>
  {/if}
  {#if flash}
    <div class="px-5 py-2 border-t border-gray-100 dark:border-lerd-border">
      <p class="text-xs text-emerald-700 dark:text-emerald-500 bg-emerald-50 dark:bg-emerald-500/10 rounded-lg px-2 py-1.5 text-center">{flash}</p>
    </div>
  {/if}
  {#if error}
    <div class="px-5 py-2 border-t border-gray-100 dark:border-lerd-border">
      <p class="text-xs text-red-500">{error}</p>
    </div>
  {/if}
</Modal>
