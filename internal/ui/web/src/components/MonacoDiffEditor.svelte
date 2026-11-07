<script lang="ts">
  import { get } from 'svelte/store';
  import { onMount, untrack } from 'svelte';
  import { editorFontSize } from '$stores/editorFont';
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
    /** Share of the width the original side opens with; read once, at creation. */
    splitRatio?: number;
    /** Fires with the original side's share whenever the side-by-side layout changes, a drag included. */
    onSplit?: (ratio: number) => void;
  }
  let { original, modified, language = 'plaintext', ignoreSpace = false, sideBySide = true, onStats, hunkAction, splitRatio, onSplit }: Props = $props();
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
    let removeDragListener: (() => void) | undefined;
    void (async () => {
      const monaco = await loadMonaco();
      if (disposed || !container) return;
      monacoRef = monaco;
      const ed = monaco.editor.createDiffEditor(container, {
        readOnly: true,
        automaticLayout: true,
        minimap: { enabled: false },
        fontSize: $editorFontSize,
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
        scrollBeyondLastLine: false,
        renderOverviewRuler: false,
        // Long unchanged stretches fold away, as on a code review page.
        hideUnchangedRegions: { enabled: true },
        ignoreTrimWhitespace: ignoreSpace,
        renderSideBySide: sideBySide,
        splitViewDefaultRatio: splitRatio,
        // Otherwise a narrow window silently goes unified and the toggle says the opposite.
        useInlineViewWhenSpaceIsLimited: false
      });
      ed.onDidUpdateDiff(() => {
        requestAnimationFrame(() => (ready = true));
        const model = ed.getModel();
        changes = ed.getLineChanges() ?? [];
        if (model) onStats?.(diffStats(changes, model.original.getValue(), model.modified.getValue()));
      });
      // Monaco has no event for its sash, but a drag relays out both sides. Only a
      // drag counts: a new editor passes through in-between layouts as it opens.
      let dragging = false;
      container.addEventListener('pointerdown', (e) => (dragging = !!(e.target as Element).closest?.('.monaco-sash.vertical')), true);
      window.addEventListener('pointerup', stopDrag, true);
      // The modified side is laid out after the original one, so its width is current here.
      ed.getModifiedEditor().onDidLayoutChange(() => {
        // Monaco places the sash where the modified side starts, as a share of the full width.
        const full = container?.clientWidth ?? 0;
        const right = ed.getModifiedEditor().getLayoutInfo().width;
        if (dragging && sideBySide && full > 0 && right > 0) onSplit?.(1 - right / full);
      });
      function stopDrag() {
        dragging = false;
      }
      removeDragListener = () => window.removeEventListener('pointerup', stopDrag, true);
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
      removeDragListener?.();
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
    // Monaco resets the split to half on any update that leaves its ratio out.
    editor?.updateOptions({ ignoreTrimWhitespace: ignoreSpace, renderSideBySide: sideBySide, fontSize: $editorFontSize, splitViewDefaultRatio: untrack(() => splitRatio) });
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
