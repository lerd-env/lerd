<script lang="ts">
  import { get } from 'svelte/store';
  import { onMount } from 'svelte';
  import { palette, theme } from '$stores/theme';
  import { diffStats } from '$lib/gitDiff';
  import { applyEditorAccent, loadMonaco, lerdThemeName, type MonacoModule } from '$lib/monaco';
  import type * as Monaco from 'monaco-editor';

  interface Props {
    original: string;
    modified: string;
    /** Monaco language id, e.g. 'php', 'ini', 'plaintext'. */
    language?: string;
    ignoreSpace?: boolean;
    /** Fires each time Monaco finishes computing the diff. */
    onStats?: (s: { added: number; removed: number }) => void;
  }
  let { original, modified, language = 'plaintext', ignoreSpace = false, onStats }: Props = $props();

  let container: HTMLDivElement | undefined = $state();
  let editor: Monaco.editor.IStandaloneDiffEditor | undefined = $state();
  let monacoRef: MonacoModule | undefined;

  onMount(() => {
    let disposed = false;
    let unsubTheme: (() => void) | undefined;
    let unsubPalette: (() => void) | undefined;
    void (async () => {
      const monaco = await loadMonaco();
      if (disposed || !container) return;
      monacoRef = monaco;
      const ed = monaco.editor.createDiffEditor(container, {
        readOnly: true,
        automaticLayout: true,
        minimap: { enabled: false },
        fontSize: 12,
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
        scrollBeyondLastLine: false,
        renderOverviewRuler: false,
        // Long unchanged stretches fold away, as on a code review page.
        hideUnchangedRegions: { enabled: true },
        ignoreTrimWhitespace: ignoreSpace
      });
      ed.onDidUpdateDiff(() => {
        const model = ed.getModel();
        if (model) onStats?.(diffStats(ed.getLineChanges() ?? [], model.original.getValue(), model.modified.getValue()));
      });
      const retheme = () => {
        applyEditorAccent();
        monaco.editor.setTheme(lerdThemeName(get(theme)));
      };
      unsubTheme = theme.subscribe(retheme);
      unsubPalette = palette.subscribe(retheme);
      editor = ed;
    })();
    return () => {
      disposed = true;
      unsubTheme?.();
      unsubPalette?.();
      const model = editor?.getModel();
      editor?.dispose();
      model?.original.dispose();
      model?.modified.dispose();
    };
  });

  // A new file swaps in fresh models; the old pair is disposed once replaced.
  $effect(() => {
    const ed = editor;
    const monaco = monacoRef;
    if (!ed || !monaco) return;
    const old = ed.getModel();
    ed.setModel({
      original: monaco.editor.createModel(original, language),
      modified: monaco.editor.createModel(modified, language)
    });
    old?.original.dispose();
    old?.modified.dispose();
  });

  $effect(() => {
    editor?.updateOptions({ ignoreTrimWhitespace: ignoreSpace });
  });
</script>

<div bind:this={container} class="h-full w-full"></div>
