<script lang="ts">
  import { apiFetch, apiJson } from '$lib/api';
  import { executePackageInstall } from '$stores/commands';
  import { accessMode } from '$stores/accessMode';
  import { openErrorModal } from '$stores/modals';
  import { goToDocsPage } from '$stores/docs';
  import Icon from './Icon.svelte';
  import { m } from '../paraglide/messages.js';

  // Composer packages the site's framework suggests and the project does not
  // have installed, each with a button to install it and one to stop offering it.

  interface Suggestion {
    name: string;
    dev?: boolean;
    reason?: string;
    docs?: string;
  }
  interface Props {
    domain: string;
  }
  let { domain }: Props = $props();

  let packages = $state<Suggestion[]>([]);
  let busy = $state('');
  const base = $derived(`/api/sites/${encodeURIComponent(domain)}/packages`);

  async function load() {
    try {
      packages = (await apiJson<{ packages: Suggestion[] }>(base)).packages ?? [];
    } catch {
      packages = [];
    }
  }
  $effect(() => {
    void base;
    load();
  });

  // A page on lerd.sh opens in the built-in docs, the version this lerd ships.
  function openDocs(e: MouseEvent, url: string) {
    const match = /^https:\/\/lerd\.sh\/([^#]*?)\/?(?:#(.*))?$/.exec(url);
    if (!match) return;
    e.preventDefault();
    goToDocsPage(match[1], match[2] ?? '');
  }

  async function install(p: Suggestion) {
    busy = p.name;
    await executePackageInstall(domain, p.name, `composer require${p.dev ? ' --dev' : ''} ${p.name}`);
    busy = '';
    await load();
  }
  async function dismiss(p: Suggestion) {
    const res = await apiFetch(`${base}/dismiss?name=${encodeURIComponent(p.name)}`, { method: 'POST' });
    const data = (await res.json().catch(() => ({}))) as { error?: string };
    if (data.error) openErrorModal(data.error);
    await load();
  }
</script>

{#if $accessMode.localControl}
  {#each packages as p (p.name)}
    <div class="flex items-center gap-3 px-3 py-2 border-b border-gray-200 dark:border-lerd-border bg-sky-50/60 dark:bg-sky-500/5 text-xs">
      <Icon name="download" class="w-3.5 h-3.5 shrink-0 text-sky-600 dark:text-sky-400" />
      <span class="min-w-0 flex-1 text-gray-600 dark:text-gray-300">
        <span class="font-mono font-medium text-gray-800 dark:text-gray-100">{p.name}</span>
        {#if p.reason}<span class="ml-1.5">{p.reason}</span>{/if}
        {#if p.docs}<a class="ml-1.5 text-sky-700 dark:text-sky-400 hover:underline" href={p.docs} target="_blank" rel="noreferrer" onclick={(e) => openDocs(e, p.docs ?? '')}>{m.packages_docs()}</a>{/if}
      </span>
      <button
        type="button"
        class="shrink-0 px-2.5 py-1 rounded-md bg-sky-600 text-white hover:bg-sky-700 disabled:opacity-50"
        disabled={busy !== ''}
        onclick={() => install(p)}
      >{busy === p.name ? m.packages_installing() : m.packages_install()}</button>
      <button
        type="button"
        class="shrink-0 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
        aria-label={m.packages_dismiss()}
        title={m.packages_dismiss()}
        onclick={() => dismiss(p)}
      ><Icon name="close" class="w-3.5 h-3.5" /></button>
    </div>
  {/each}
{/if}
