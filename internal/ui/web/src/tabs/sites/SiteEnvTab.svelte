<script lang="ts">
  import { loadSiteEnv, saveSiteEnv, type Site } from '$stores/sites';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
    branch: string;
  }
  let { site, branch }: Props = $props();

  // text reflects the editor buffer; loaded is the last-saved baseline,
  // used to drive the dirty-state badge and to discard unsaved changes
  // when the user picks "Descartar".
  let text = $state<string>('');
  let loaded = $state<string>('');
  let loading = $state(true);
  let error = $state<string>('');
  let saving = $state(false);
  let saveSuccess = $state(false);

  const dirty = $derived(text !== loaded);

  const envPath = $derived.by(() => {
    if (branch) {
      const wt = (site.worktrees || []).find((w) => w.branch === branch);
      if (wt?.path) return wt.path + '/.env';
    }
    return (site.path || '') + '/.env';
  });

  // Reload whenever the user navigates between sites or worktree branches.
  $effect(() => {
    const domain = site.domain;
    const b = branch;
    loading = true;
    error = '';
    text = '';
    loaded = '';
    saveSuccess = false;
    loadSiteEnv(domain, b)
      .then((t) => {
        text = t;
        loaded = t;
      })
      .catch((e: unknown) => {
        error = e instanceof Error ? e.message : String(e);
      })
      .finally(() => {
        loading = false;
      });
  });

  // Warn before navigating away with unsaved changes — same UX as the
  // PHP install page so the user doesn't lose edits to a refresh or
  // tab close.
  $effect(() => {
    if (!dirty) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });

  async function onSave() {
    saving = true;
    error = '';
    saveSuccess = false;
    try {
      await saveSiteEnv(site.domain, branch, text);
      loaded = text;
      saveSuccess = true;
      setTimeout(() => (saveSuccess = false), 2500);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }

  function onDiscard() {
    if (!dirty) return;
    if (!confirm('Descartar todas as alterações não salvas no .env?')) return;
    text = loaded;
    error = '';
  }

  // Ctrl/Cmd+S to save without leaving the textarea.
  function onKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault();
      if (dirty && !saving) onSave();
    }
  }
</script>

<div class="flex flex-col h-full overflow-hidden">
  <!-- Toolbar -->
  <div class="flex items-center justify-between gap-2 px-3 py-2 border-b border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-white/3 shrink-0">
    <div class="flex items-center gap-2 min-w-0">
      <span class="text-[10px] font-semibold text-gray-400 uppercase tracking-wider shrink-0">.env</span>
      <span class="text-[10px] font-mono text-gray-400 truncate" title={envPath}>{envPath}</span>
      {#if dirty}
        <span class="text-[10px] font-medium text-amber-600 dark:text-amber-400 shrink-0">● não salvo</span>
      {/if}
    </div>
    <div class="flex items-center gap-2 shrink-0">
      {#if saveSuccess}
        <span class="text-[10px] text-emerald-600 dark:text-emerald-400">✓ salvo</span>
      {/if}
      <button
        type="button"
        onclick={onDiscard}
        disabled={!dirty || saving}
        class="text-[11px] px-2 py-1 rounded text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-200 dark:hover:bg-white/5 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
      >Descartar</button>
      <button
        type="button"
        onclick={onSave}
        disabled={!dirty || saving || loading}
        class="text-[11px] px-2.5 py-1 rounded bg-emerald-600 text-white hover:bg-emerald-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
        title="Ctrl+S / Cmd+S"
      >{saving ? 'Salvando…' : 'Salvar'}</button>
    </div>
  </div>

  <!-- Editor body -->
  {#if loading}
    <p class="text-xs text-gray-400 p-3">{m.common_loading()}</p>
  {:else}
    <textarea
      bind:value={text}
      onkeydown={onKeydown}
      spellcheck="false"
      autocomplete="off"
      autocorrect="off"
      autocapitalize="off"
      placeholder={text === '' ? `${envPath} ainda não existe — escreva o .env aqui e clique Salvar` : ''}
      class="flex-1 w-full px-3 py-2.5 bg-white dark:bg-black/40 text-gray-800 dark:text-gray-200 font-mono text-[11px] leading-relaxed resize-none focus:outline-none focus:ring-1 focus:ring-inset focus:ring-emerald-500/50 border-0"
    ></textarea>
    {#if error}
      <div class="text-[11px] text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-500/10 px-3 py-2 border-t border-red-200 dark:border-red-500/20 break-words shrink-0">
        {error}
      </div>
    {/if}
  {/if}

  <!-- Status footer -->
  <div class="text-[10px] text-gray-400 px-3 py-1.5 border-t border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-white/3 shrink-0 leading-relaxed">
    O backend cria <span class="font-mono">.env.before_lerd</span> automaticamente na primeira edição. Restaurável com <span class="font-mono">lerd env:restore</span>.
  </div>
</div>
