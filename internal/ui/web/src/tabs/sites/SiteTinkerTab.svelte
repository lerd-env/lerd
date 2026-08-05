<script lang="ts">
  import { runTinker, fetchTinkerSnippets, saveTinkerSnippet, deleteTinkerSnippet, activeWorktreeDomain, type TinkerResponse, type TinkerSnippet, type Site } from '$stores/sites';
  import { modal, openErrorModal } from '$stores/modals';
  import { parseDump, looksLikeDump } from '$lib/dump-parser';
  import { parseBlock } from '$lib/tinker';
  import DumpView from '$components/DumpView.svelte';
  import Popover from '$components/Popover.svelte';
  import ConfirmModal from '$components/ConfirmModal.svelte';
  import TinkerSnippetSaveModal from './TinkerSnippetSaveModal.svelte';
  import MonacoEditor from '$components/MonacoEditor.svelte';
  import Icon from '$components/Icon.svelte';
  import { attachPhpLsp, type PhpLspHandle } from '$lib/lsp';
  import { tooltip } from '$lib/tooltip';
  import type { MonacoModule } from '$lib/monaco';
  import type * as Monaco from 'monaco-editor';
  import { onDestroy, onMount } from 'svelte';
  import { m } from '../../paraglide/messages.js';

  interface Props {
    site: Site;
    branch?: string;
  }
  let { site, branch = '' }: Props = $props();

  const draftKey = $derived(`tinker:${site.domain}${branch ? '@' + branch : ''}:draft`);

  // Seed the editor with the saved draft once at construction. We
  // deliberately read site/branch as initial values (not reactive) here
  // because the draft only needs to be loaded once; the persisting
  // $effect below keeps localStorage in sync on every edit thereafter.
  function loadInitialDraft(): string {
    if (typeof localStorage === 'undefined') return '';
    const key = `tinker:${site.domain}${branch ? '@' + branch : ''}:draft`;
    return localStorage.getItem(key) ?? '';
  }
  let code = $state(loadInitialDraft());
  let running = $state(false);
  let result = $state<TinkerResponse | null>(null);

  // Split direction is a single global preference (not per site), so the
  // choice follows the user across every tinker session. Full screen is
  // transient and never persisted.
  const SPLIT_KEY = 'tinker:splitDir';
  const SplitDir = { Horizontal: 'horizontal', Vertical: 'vertical' } as const;
  type SplitDir = typeof SplitDir[keyof typeof SplitDir];
  function loadSplitDir(): SplitDir {
    if (typeof localStorage === 'undefined') return SplitDir.Horizontal;
    return localStorage.getItem(SPLIT_KEY) === SplitDir.Vertical ? SplitDir.Vertical : SplitDir.Horizontal;
  }
  let splitDir = $state<SplitDir>(loadSplitDir());
  let fullscreen = $state(false);
  let fullscreenBtn: HTMLButtonElement | undefined;
  let editor: Monaco.editor.IStandaloneCodeEditor | null = null;

  // Backend frames each top-level statement's output as `\x1e<line>\x1f<out>`:
  // the record separator splits blocks, and `line` is the editor line that
  // produced the block (rendered as a "Line N" badge).
  type OutputBlock =
    | { kind: 'tree'; nodes: ReturnType<typeof parseDump>['nodes']; trailing: string; raw: string; line?: number }
    | { kind: 'error'; type: string; message: string; raw: string; line?: number }
    | { kind: 'query'; sql: string; line?: number }
    | { kind: 'text'; text: string; line?: number };

  // psysh emits runtime errors on stdout in the form
  //   `Error  Call to a member function get() on int.`
  //   `TypeError  Argument #1 ($x) must be of type int, string given`
  // even though `ok=true` and `exit_code=0`. Detect them so we can render
  // with the same red treatment as backend-level errors.
  const ERROR_RE = /^\s*([A-Z][A-Za-z]+(?:Error|Exception|Throwable))\s{2,}([\s\S]+)$/;

  const stdoutBlocks = $derived.by<OutputBlock[]>(() => {
    if (!result?.stdout) return [];
    const blocks: OutputBlock[] = [];
    for (const rawChunk of result.stdout.split('\x1e')) {
      // Peel the `<line>\x1f` marker before trimming so a block with only the
      // marker (a no-output statement) drops out as empty.
      const { line, kind, body } = parseBlock(rawChunk);
      const chunk = body.replace(/^\n+|\n+$/g, '');
      if (chunk.length === 0) continue;
      if (kind === 'query') {
        blocks.push({ kind: 'query', sql: chunk, line });
        continue;
      }
      const errMatch = chunk.match(ERROR_RE);
      if (errMatch) {
        blocks.push({ kind: 'error', type: errMatch[1], message: errMatch[2].trim(), raw: chunk, line });
        continue;
      }
      if (looksLikeDump(chunk)) {
        const parsed = parseDump(chunk);
        if (parsed.ok) {
          blocks.push({ kind: 'tree', nodes: parsed.nodes, trailing: parsed.trailing, raw: chunk, line });
          continue;
        }
      }
      blocks.push({ kind: 'text', text: chunk, line });
    }
    return blocks;
  });

  async function copyText(text: string) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // Fall back to a hidden textarea for non-secure contexts.
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.left = '-9999px';
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); } catch (_) { /* ignore */ }
      document.body.removeChild(ta);
    }
  }

  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(draftKey, code);
    }
  });

  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(SPLIT_KEY, splitDir);
    }
  });

  // Escape belongs to the topmost layer: a modal stacked on the tab owns it
  // and closes first, full screen only exits once nothing is stacked on top.
  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && fullscreen && !modalOnTop && !e.defaultPrevented) {
      e.preventDefault();
      fullscreen = false;
      requestAnimationFrame(() => fullscreenBtn?.focus());
    }
  }

  function toggleFullscreen() {
    fullscreen = !fullscreen;
    if (fullscreen) {
      requestAnimationFrame(() => editor?.focus());
    } else {
      requestAnimationFrame(() => fullscreenBtn?.focus());
    }
  }

  // Snippets are reusable .php files committed in the project
  // (.lerd/tinker/snippets, .tinkerwell/snippets) or kept globally in
  // ~/.config/lerd/tinker/snippets. Loading one only fills the editor; every
  // destructive step (replace a non-empty editor, overwrite, delete) confirms
  // first. The tinkerwell dir is read-only, so its rows carry no delete.
  let snippets = $state<TinkerSnippet[]>([]);
  const snippetSourceDirs: Record<TinkerSnippet['source'], string> = {
    project: '.lerd/tinker/snippets',
    tinkerwell: '.tinkerwell/snippets',
    global: '~/.config/lerd/tinker/snippets'
  };

  // The last loaded/saved snippet prefills the save dialog for update flows.
  let loadedSnippet = $state<{ name: string; source: 'project' | 'global' } | null>(null);
  let confirmLoad = $state<TinkerSnippet | null>(null);
  let confirmDelete = $state<TinkerSnippet | null>(null);
  let deleting = $state(false);
  let saveOpen = $state(false);
  let saving = $state(false);
  let saveError = $state('');
  const modalOnTop = $derived(
    saveOpen || confirmLoad !== null || confirmDelete !== null || $modal.kind !== null
  );

  function requestLoad(s: TinkerSnippet) {
    if (code.trim() && code !== s.content) confirmLoad = s;
    else applyLoad(s);
  }

  function applyLoad(s: TinkerSnippet) {
    code = s.content;
    loadedSnippet = s.source === 'tinkerwell' ? null : { name: s.name, source: s.source };
    confirmLoad = null;
  }

  function openSave() {
    saveError = '';
    saveOpen = true;
  }

  async function doSave(name: string, source: 'project' | 'global') {
    saving = true;
    try {
      const r = await saveTinkerSnippet(site.domain, { name, source, content: code }, branch);
      if (!r.ok) {
        saveError = r.error || m.common_requestFailed();
        return;
      }
      snippets = r.snippets ?? snippets;
      loadedSnippet = { name: name.endsWith('.php') ? name : name + '.php', source };
      saveOpen = false;
    } finally {
      saving = false;
    }
  }

  async function doDelete() {
    const target = confirmDelete;
    if (!target || deleting) return;
    deleting = true;
    try {
      const r = await deleteTinkerSnippet(site.domain, { name: target.name, source: target.source }, branch);
      confirmDelete = null;
      if (!r.ok) {
        openErrorModal(m.tinker_snippetDeleteFailed({ error: r.error || '' }));
        return;
      }
      snippets = r.snippets ?? [];
      if (loadedSnippet && loadedSnippet.name === target.name && loadedSnippet.source === target.source) {
        loadedSnippet = null;
      }
    } finally {
      deleting = false;
    }
  }

  onMount(() => {
    void fetchTinkerSnippets(site.domain, branch).then((list) => {
      snippets = list;
    });
    window.addEventListener('keydown', onKeydown);
    return () => window.removeEventListener('keydown', onKeydown);
  });

  async function run() {
    if (running || !code.trim()) return;
    running = true;
    result = null;
    try {
      result = await runTinker(site.domain, code, branch);
    } finally {
      running = false;
    }
  }

  function clearAll() {
    result = null;
    // MonacoEditor's $effect mirrors external value writes into the editor,
    // so assigning '' here clears the doc without us needing an editor ref.
    code = '';
  }

  // LSP status drives the small indicator next to the mode badge. phpantom
  // backs completion, diagnostics, and hover from the real project.
  let lspStatus = $state<'connecting' | 'ready' | 'unavailable'>('connecting');
  let lsp: PhpLspHandle | null = null;

  // Mod-Enter runs the buffer; the LSP attaches to the live editor. The
  // closure reads the current `code` state on each invocation.
  function onEditorReady({ editor: e, monaco }: { editor: Monaco.editor.IStandaloneCodeEditor; monaco: MonacoModule }) {
    editor = e;
    // Intercept at the keydown level rather than via addCommand: Ctrl/Cmd+Enter
    // must run even while the suggestion widget is open (which otherwise
    // captures Enter to accept the highlighted completion).
    e.onKeyDown((e2) => {
      if ((e2.ctrlKey || e2.metaKey) && e2.keyCode === monaco.KeyCode.Enter) {
        e2.preventDefault();
        e2.stopPropagation();
        void run();
      }
    });

    lsp?.dispose();
    lsp = attachPhpLsp({
      monaco,
      editor: e,
      domain: site.domain,
      branch,
      onStatus: (s) => { lspStatus = s; }
    });
  }

  onDestroy(() => lsp?.dispose());

  const placeholder = m.tinker_placeholder();
</script>

<!-- The flex column stays in both modes: full screen only swaps the box it
     lives in, so the panes keep filling the height they are given. -->
<div class="flex flex-col min-h-0 overflow-hidden gap-3 {fullscreen ? 'fixed inset-0 z-50 bg-white dark:bg-lerd-bg p-3' : 'flex-1 pt-4 px-3 sm:px-5 pb-3 sm:pb-5'}">
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2">
      <span
        class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded-sm border border-gray-200 dark:border-lerd-border text-gray-500 dark:text-gray-400"
        title={result?.mode === 'tinker' ? m.tinker_mode_tinkerTitle() : m.tinker_mode_phpTitle()}
      >
        {result?.mode ?? (site.is_laravel ? 'tinker' : 'php')}
      </span>
      <!-- Full screen hides the site header, so name the target here. -->
      {#if fullscreen}
        <span class="text-xs font-mono text-gray-600 dark:text-gray-300">{activeWorktreeDomain(site, branch)}</span>
        {#if branch}
          <span class="inline-flex items-center gap-1 text-[11px] font-mono text-violet-400">
            <Icon name="branch" class="w-3 h-3" />{branch}
          </span>
        {/if}
      {/if}
      {#if result}
        <span class="text-[10px] text-gray-400">{result.duration_ms} ms</span>
      {/if}
      {#if lspStatus !== 'ready'}
        <span
          class="text-[10px] {lspStatus === 'unavailable' ? 'text-amber-500' : 'text-gray-400'}"
          title={lspStatus === 'unavailable' ? m.tinker_lspUnavailable() : m.tinker_lspConnecting()}
        >{lspStatus === 'unavailable' ? m.tinker_lspUnavailable() : m.tinker_lspConnecting()}</span>
      {/if}
    </div>
    <div class="flex items-center gap-3">
      <Popover
        label={m.tinker_snippets()}
        align="right"
        width={280}
        onopen={() => {
          // Refresh on open so files added outside this tab (git pull, another
          // window) show up and the overwrite check compares against reality.
          void fetchTinkerSnippets(site.domain, branch).then((list) => {
            snippets = list;
          });
        }}
      >
        <!-- Bare trigger so the button reads exactly like its toolbar
             siblings: same classes, same default tooltip placement. -->
        {#snippet triggerButton(toggle)}
          <button
            type="button"
            onclick={toggle}
            class="block text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
            use:tooltip={m.tinker_snippets()}
            aria-label={m.tinker_snippets()}
          >
            <Icon name="docs" class="w-4 h-4" />
          </button>
        {/snippet}
        {#snippet children(close)}
          <div class="py-1" data-testid="snippets-menu">
            <!-- Only the rows scroll; the save action stays pinned below so it
                 never disappears under a long snippet list. -->
            <div class="max-h-60 overflow-y-auto">
            {#if snippets.length === 0}
              <p class="px-3 py-2 text-xs text-gray-400 dark:text-gray-500">{m.tinker_snippetsEmpty()}</p>
            {/if}
            {#each snippets as s (s.source + ':' + s.name)}
              <div class="group flex items-center pr-1">
                <button
                  type="button"
                  class="flex-1 min-w-0 px-3 py-1.5 text-left text-xs text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-white/5 transition-colors"
                  onclick={() => { close(); requestLoad(s); }}
                >
                  <span class="block truncate">{s.label}</span>
                  <span class="block text-[10px] text-gray-500 dark:text-gray-400 truncate">{snippetSourceDirs[s.source]}</span>
                </button>
                {#if s.source !== 'tinkerwell'}
                  <button
                    type="button"
                    class="shrink-0 p-1 rounded-sm opacity-0 group-hover:opacity-100 focus:opacity-100 text-gray-400 hover:text-red-600 dark:hover:text-red-400 transition-all"
                    aria-label={m.tinker_snippetDeleteTitle()}
                    use:tooltip={m.tinker_snippetDeleteTitle()}
                    onclick={() => { close(); confirmDelete = s; }}
                  >
                    <Icon name="trash" class="w-3.5 h-3.5" />
                  </button>
                {/if}
              </div>
            {/each}
            </div>
            <div class="border-t border-gray-100 dark:border-lerd-border mt-1 pt-1">
              <button
                type="button"
                class="w-full flex items-center gap-2 px-3 py-1.5 text-left text-xs text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-white/5 disabled:opacity-40 transition-colors"
                disabled={!code.trim()}
                onclick={() => { close(); openSave(); }}
              >
                <Icon name="plus" class="w-3.5 h-3.5" />{m.tinker_snippetSaveAction()}
              </button>
            </div>
          </div>
        {/snippet}
      </Popover>
      <button
        onclick={() => (splitDir = splitDir === SplitDir.Horizontal ? SplitDir.Vertical : SplitDir.Horizontal)}
        class="hidden md:block text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
        use:tooltip={splitDir === SplitDir.Horizontal ? m.tinker_splitVerticalTitle() : m.tinker_splitHorizontalTitle()}
        aria-label={splitDir === SplitDir.Horizontal ? m.tinker_splitVerticalTitle() : m.tinker_splitHorizontalTitle()}
      >
        <Icon name={splitDir === SplitDir.Horizontal ? 'splitVertical' : 'splitHorizontal'} class="w-4 h-4" />
      </button>
      <button
        bind:this={fullscreenBtn}
        onclick={toggleFullscreen}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
        use:tooltip={fullscreen ? m.tinker_exitFullscreenTitle() : m.tinker_fullscreenTitle()}
        aria-label={fullscreen ? m.tinker_exitFullscreenTitle() : m.tinker_fullscreenTitle()}
      >
        <Icon name={fullscreen ? 'minimize' : 'maximize'} class="w-4 h-4" />
      </button>
      <button
        onclick={clearAll}
        disabled={!code && !result}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 disabled:opacity-40 disabled:hover:text-gray-400 transition-colors"
        use:tooltip={m.tinker_clearTitle()}
        aria-label={m.common_clear()}
      >
        <Icon name="trash" class="w-4 h-4" />
      </button>
      <button
        onclick={run}
        disabled={running || !code.trim()}
        class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 disabled:opacity-40 disabled:hover:text-gray-400 transition-colors"
        use:tooltip={running ? m.tinker_running() : m.tinker_runTitle()}
        aria-label={running ? m.tinker_running() : m.tinker_run()}
      >
        <Icon name={running ? 'spinner' : 'play'} class="w-4 h-4 {running ? 'animate-spin' : ''}" />
      </button>
    </div>
  </div>

  <div
    class="flex-1 flex min-h-0 gap-3 {splitDir === SplitDir.Horizontal ? 'flex-col md:flex-row' : 'flex-col'}"
    data-splitdir={splitDir}
  >
    <div
      class="group flex-1 min-h-40 md:min-h-0 flex flex-col rounded-lg border border-gray-200 dark:border-lerd-border overflow-hidden bg-gray-50 dark:bg-black/40 relative"
    >
      <div class="flex-1 min-h-0 overflow-hidden">
        <MonacoEditor bind:value={code} language="php" onReady={onEditorReady} />
      </div>
      {#if code.trim()}
        <button
          onclick={() => copyText(code)}
          title={m.tinker_copyEditorTitle()}
          class="absolute top-2 right-2 z-10 opacity-0 group-hover:opacity-100 text-[10px] px-1.5 py-0.5 rounded-sm border border-gray-200 dark:border-lerd-border bg-white/90 dark:bg-lerd-card/90 text-gray-500 hover:text-gray-700 dark:hover:text-gray-200 transition-opacity"
        >{m.common_copy()}</button>
      {/if}
    </div>

    <div
      class="flex-1 min-h-30 md:min-h-0 flex flex-col overflow-y-auto rounded-lg border border-gray-200 dark:border-lerd-border bg-gray-50 dark:bg-black/40 tinker-output py-2"
    >
      {#if !result && running}
        <p class="text-xs text-gray-400">{m.tinker_running()}</p>
      {:else if !result}
        <p class="text-[11px] text-gray-400 dark:text-gray-500 font-mono whitespace-pre-line">{placeholder}</p>
      {:else}
        {#if result.error}
          <div class="output-row" data-line="!">
            <div class="output-content text-red-700 dark:text-red-300">
              <pre class="whitespace-pre-wrap">{result.error}</pre>
            </div>
          </div>
        {/if}
        {#each stdoutBlocks as block, i (i)}
          <div class="output-row group" data-line="">
            <div class="output-content">
              {#if block.kind === 'tree'}
                {#each block.nodes as node, j (j)}
                  <div class="mb-1 last:mb-0"><DumpView {node} /></div>
                {/each}
                {#if block.trailing.trim()}
                  <pre class="whitespace-pre-wrap text-gray-700 dark:text-gray-300">{block.trailing}</pre>
                {/if}
              {:else if block.kind === 'error'}
                <div class="flex items-start gap-2">
                  <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded-sm bg-red-100 dark:bg-red-900/40 text-red-700 dark:text-red-300 shrink-0">{block.type}</span>
                  <pre class="whitespace-pre-wrap text-red-700 dark:text-red-300">{block.message}</pre>
                </div>
              {:else if block.kind === 'query'}
                <div class="flex items-start gap-2 rounded-md border-l-2 border-sky-400 dark:border-sky-500 border-y border-r border-y-sky-200/60 border-r-sky-200/60 dark:border-y-sky-800/40 dark:border-r-sky-800/40 bg-sky-50/80 dark:bg-sky-950/30 px-2 py-1">
                  <Icon name="database" class="w-4 h-4 mt-0.5 text-sky-500 dark:text-sky-400 shrink-0" />
                  <pre class="whitespace-pre-wrap text-[11px] leading-relaxed text-sky-800 dark:text-sky-300">{block.sql}</pre>
                </div>
              {:else}
                <pre class="whitespace-pre-wrap">{block.text}</pre>
              {/if}
            </div>
            {#if block.line !== undefined && block.kind !== 'query'}
              <span
                class="output-line shrink-0 select-none text-[10px] text-gray-400 dark:text-gray-500"
                title={m.tinker_lineTitle({ n: block.line })}
              >{m.tinker_lineLabel({ n: block.line })}</span>
            {/if}
            <button
              onclick={() =>
                copyText(
                  block.kind === 'tree' ? block.raw :
                  block.kind === 'error' ? block.raw :
                  block.kind === 'query' ? block.sql : block.text
                )}
              title={m.tinker_copyOutputTitle()}
              class="output-copy opacity-0 group-hover:opacity-100 text-[10px] px-1.5 py-0.5 rounded-sm border border-gray-200 dark:border-lerd-border text-gray-500 hover:text-gray-700 dark:hover:text-gray-200 transition-opacity shrink-0 {block.kind === 'query' ? 'output-copy--abs' : ''}"
            >{m.common_copy()}</button>
          </div>
        {/each}
        {#if result.stderr}
          <div class="output-row" data-line="e">
            <div class="output-content text-amber-700 dark:text-amber-300">
              <pre class="whitespace-pre-wrap">{result.stderr}</pre>
            </div>
          </div>
        {/if}
        {#if stdoutBlocks.length === 0 && !result.stderr && !result.error}
          <div class="output-row" data-line="·">
            <div class="output-content text-gray-400">{m.tinker_noOutput()}</div>
          </div>
        {/if}
      {/if}
    </div>
  </div>
</div>

<TinkerSnippetSaveModal
  open={saveOpen}
  {snippets}
  initialName={loadedSnippet ? loadedSnippet.name.replace(/\.php$/, '') : ''}
  initialSource={loadedSnippet?.source ?? 'project'}
  {saving}
  error={saveError}
  onsave={doSave}
  onclose={() => {
    if (!saving) saveOpen = false;
  }}
/>

<ConfirmModal
  open={confirmLoad !== null}
  title={m.tinker_snippetLoadTitle()}
  body={m.tinker_snippetLoadBody({ name: confirmLoad?.label ?? '' })}
  confirmLabel={m.tinker_snippetLoadConfirm()}
  onconfirm={() => confirmLoad && applyLoad(confirmLoad)}
  onclose={() => (confirmLoad = null)}
/>

<ConfirmModal
  open={confirmDelete !== null}
  title={m.tinker_snippetDeleteTitle()}
  body={m.tinker_snippetDeleteBody({
    name: confirmDelete?.name ?? '',
    dir: confirmDelete ? snippetSourceDirs[confirmDelete.source] : ''
  })}
  confirmLabel={m.tinker_snippetDeleteConfirm()}
  danger
  loading={deleting}
  onconfirm={doDelete}
  onclose={() => {
    if (!deleting) confirmDelete = null;
  }}
/>

<style>
  /* Output panel, visually mirrors the editor on the left: bordered box,
     monospace, line-number gutter that the user can't mouse-select or copy.
     Numbers come from `data-line` via `::before`, so they're CSS-generated
     content (excluded from text selection in all modern browsers). */
  .tinker-output {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 12px;
    line-height: 1.5;
  }
  .tinker-output :global(.output-row) {
    display: flex;
    align-items: flex-start;
    padding: 2px 8px 2px 0;
    position: relative;
  }
  /* Gutter only renders for rows that carry a marker (error/stderr/no-output).
     Result and query rows use data-line="", so it collapses, their info is on
     the right ("Line N" badge), leaving no dead column on the left. */
  .tinker-output :global(.output-row:not([data-line=''])::before) {
    content: attr(data-line);
    flex-shrink: 0;
    width: 32px;
    padding-right: 8px;
    text-align: right;
    color: #9ca3af;
    font-size: 11px;
    user-select: none;
    -webkit-user-select: none;
    pointer-events: none;
  }
  :global(html.dark) .tinker-output :global(.output-row:not([data-line=''])::before) {
    color: #4b5563;
  }
  .tinker-output :global(.output-content) {
    flex: 1;
    min-width: 0;
    padding-left: 8px;
  }
  .tinker-output :global(.output-copy) {
    margin-left: 8px;
  }
  .tinker-output :global(.output-line) {
    margin-left: 8px;
    padding-top: 1px;
    white-space: nowrap;
  }
  /* Query rows keep the result gutter/left padding but float the copy button
     so the card can span the full width to the right edge. */
  .tinker-output :global(.output-copy--abs) {
    position: absolute;
    top: 4px;
    right: 6px;
    margin-left: 0;
  }
  @media (prefers-reduced-motion: reduce) {
    .tinker-output, .tinker-output * {
      animation-duration: 0.01ms !important;
      transition-duration: 0.01ms !important;
    }
  }
</style>
