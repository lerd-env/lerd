<script lang="ts">
  import { get } from 'svelte/store';
  import { onMount } from 'svelte';
  import { palette, theme } from '$stores/theme';
  import { diffStats, type Hunk } from '$lib/gitDiff';
  import { applyEditorAccent, loadMonaco, lerdThemeName, type MonacoModule } from '$lib/monaco';
  import type * as Monaco from 'monaco-editor';

  interface Props {
    original: string;
    modified: string;
    /** Monaco language id, e.g. 'php', 'ini', 'plaintext'. */
    language?: string;
    ignoreSpace?: boolean;
    sideBySide?: boolean;
    /** Fires each time Monaco finishes computing the diff. */
    onStats?: (s: { added: number; removed: number }) => void;
    /** A gutter button on each change block; unstage draws a minus instead of a plus. */
    hunkAction?: { label: string; unstage: boolean; run: (h: Hunk) => void };
  }
  let { original, modified, language = 'plaintext', ignoreSpace = false, sideBySide = true, onStats, hunkAction }: Props = $props();
  let changes = $state<Monaco.editor.ILineChange[]>([]);
  // Monaco paints both files whole before its diff folds the unchanged lines
  // away, a frame after the diff lands, so a new file stays hidden until then.
  let ready = $state(false);

  // A block with no lines on the working side sits after its start line.
  const anchor = (c: Monaco.editor.ILineChange) => (c.modifiedEndLineNumber > 0 ? c.modifiedStartLineNumber : Math.max(c.modifiedStartLineNumber, 1));

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
        ignoreTrimWhitespace: ignoreSpace,
        renderSideBySide: sideBySide,
        // Otherwise a narrow window silently goes unified and the toggle says the opposite.
        useInlineViewWhenSpaceIsLimited: false
      });
      ed.onDidUpdateDiff(() => {
        requestAnimationFrame(() => (ready = true));
        const model = ed.getModel();
        changes = ed.getLineChanges() ?? [];
        if (model) onStats?.(diffStats(changes, model.original.getValue(), model.modified.getValue()));
      });
      ed.getModifiedEditor().onMouseDown((e) => {
        const model = ed.getModel();
        if (!hunkAction || !model || e.target.type !== monaco.editor.MouseTargetType.GUTTER_GLYPH_MARGIN) return;
        const c = changes.find((c) => anchor(c) === e.target.position?.lineNumber);
        if (!c) return;
        hunkAction.run({
          orig_start: c.originalStartLineNumber,
          orig_end: c.originalEndLineNumber,
          mod_start: c.modifiedStartLineNumber,
          mod_end: c.modifiedEndLineNumber,
          original: model.original.getValue(),
          modified: model.modified.getValue()
        });
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
    ready = false;
    ed.setModel({
      original: monaco.editor.createModel(original, language),
      modified: monaco.editor.createModel(modified, language)
    });
    old?.original.dispose();
    old?.modified.dispose();
  });

  $effect(() => {
    editor?.updateOptions({ ignoreTrimWhitespace: ignoreSpace, renderSideBySide: sideBySide });
  });

  let glyphs: Monaco.editor.IEditorDecorationsCollection | undefined;
  $effect(() => {
    const side = editor?.getModifiedEditor();
    const monaco = monacoRef;
    if (!side || !monaco) return;
    side.updateOptions({ glyphMargin: !!hunkAction });
    glyphs ??= side.createDecorationsCollection();
    glyphs.set(
      hunkAction
        ? changes.map((c) => ({
            range: new monaco.Range(anchor(c), 1, anchor(c), 1),
            options: {
              glyphMarginClassName: `lerd-hunk ${hunkAction.unstage ? 'lerd-hunk-unstage' : 'lerd-hunk-stage'}`,
              glyphMarginHoverMessage: { value: hunkAction.label }
            }
          }))
        : []
    );
  });
</script>

<!-- Opacity, not visibility: Monaco sets visibility on its own layers, which shows them through a hidden parent. -->
<div bind:this={container} class="h-full w-full {ready ? '' : 'opacity-0'}"></div>

<style>
  :global(.lerd-hunk) {
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    font: 600 13px/1 ui-monospace, monospace;
    color: var(--color-gray-400, #9ca3af);
  }
  :global(.lerd-hunk:hover) {
    color: var(--color-lerd-red, currentColor);
  }
  :global(.lerd-hunk-stage::before) {
    content: '+';
  }
  :global(.lerd-hunk-unstage::before) {
    content: '\2212';
  }
</style>
