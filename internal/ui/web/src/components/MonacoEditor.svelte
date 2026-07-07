<script lang="ts">
  import { onMount } from 'svelte';
  import { theme } from '$stores/theme';
  import { loadMonaco, lerdThemeName, type MonacoModule } from '$lib/monaco';
  import type * as Monaco from 'monaco-editor';

  interface Props {
    value: string;
    /** Monaco language id, e.g. 'php', 'ini', 'plaintext'. */
    language?: string;
    onChange?: (next: string) => void;
    readOnly?: boolean;
    /** Extra classes on the editor container. */
    class?: string;
    /** Monaco editor options merged over the defaults. */
    options?: Monaco.editor.IStandaloneEditorConstructionOptions;
    /** 1-based line numbers to mark as inserted: a green gutter bar plus a
        faint line tint, styled like an SCM added-line decoration. */
    highlightLines?: number[];
    /** Fires once the editor and the monaco module are ready, for callers
        that need to add commands, attach a language client, etc. */
    onReady?: (ctx: { editor: Monaco.editor.IStandaloneCodeEditor; monaco: MonacoModule }) => void;
  }
  let {
    value = $bindable(''),
    language = 'plaintext',
    onChange,
    readOnly = false,
    class: extraClass = '',
    options = {},
    highlightLines = [],
    onReady
  }: Props = $props();

  let monacoRef: MonacoModule | undefined;
  let decorations: Monaco.editor.IEditorDecorationsCollection | undefined;

  let container: HTMLDivElement | undefined = $state();
  // $state so the value-mirroring $effect below re-runs once the async monaco
  // load assigns the editor, reconciling any value change that arrived while
  // the module was still loading.
  let editor: Monaco.editor.IStandaloneCodeEditor | undefined = $state();
  // Guards external value writes from looping back through onChange.
  let internalUpdate = false;
  let disposed = false;

  onMount(() => {
    let unsubTheme: (() => void) | undefined;
    let vvCleanup: (() => void) | undefined;
    void (async () => {
      const monaco = await loadMonaco();
      if (disposed || !container) return;
      monacoRef = monaco;

      // On touch the soft keyboard shrinks the visible area but Monaco still
      // lays out to the full container height, so with the caret near the top it
      // decides there's no room below and flips the suggest popup above,
      // off-screen. We constrain the editor to the visual viewport below so its
      // geometry matches what's actually visible and the popup lands by the
      // caret. Widgets stay fixed-positioned so they clear the overflow-hidden card.
      const coarse =
        typeof window !== 'undefined' &&
        typeof window.matchMedia === 'function' &&
        window.matchMedia('(pointer: coarse)').matches;

      const ed = monaco.editor.create(container, {
        value,
        language,
        readOnly,
        automaticLayout: true,
        minimap: { enabled: false },
        fontSize: 12,
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
        lineNumbersMinChars: 3,
        scrollBeyondLastLine: false,
        wordWrap: 'on',
        renderLineHighlightOnlyWhenFocus: true,
        // Render suggest/hover/parameter-hint widgets with fixed positioning
        // so they escape the `overflow-hidden` card wrapping the editor
        // instead of being clipped to its bounds.
        fixedOverflowWidgets: true,
        padding: { top: 8, bottom: 32 },
        tabSize: 4,
        ...options
      });
      editor = ed;

      // Keyboard-aware sizing on touch (see the note above the create call):
      // cap the editor at the gap between its top and the visual viewport
      // bottom (the keyboard), and refit as the keyboard shows or hides.
      // automaticLayout picks up the max-height change and Monaco then places
      // the suggest popup within the visible area.
      if (coarse && typeof window !== 'undefined' && window.visualViewport && container) {
        const host = container;
        const vv = window.visualViewport;
        const fit = () => {
          const top = host.getBoundingClientRect().top;
          const avail = vv.offsetTop + vv.height - top - 8;
          host.style.maxHeight = avail > 120 ? `${avail}px` : '';
        };
        fit();
        vv.addEventListener('resize', fit);
        vv.addEventListener('scroll', fit);
        vvCleanup = () => {
          vv.removeEventListener('resize', fit);
          vv.removeEventListener('scroll', fit);
          host.style.maxHeight = '';
        };
      }

      ed.onDidChangeModelContent(() => {
        if (internalUpdate) return;
        const next = ed.getValue();
        value = next;
        onChange?.(next);
      });

      // Self-contained theme decision so it stays correct regardless of
      // subscriber ordering against the theme store's own DOM toggle.
      unsubTheme = theme.subscribe((t) => monaco.editor.setTheme(lerdThemeName(t)));

      onReady?.({ editor: ed, monaco });
    })();

    return () => {
      disposed = true;
      unsubTheme?.();
      vvCleanup?.();
      editor?.dispose();
      editor = undefined;
    };
  });

  // Mirror external value mutations into the editor without re-entering the
  // change listener that would otherwise report them back as user edits.
  $effect(() => {
    const v = value;
    if (!editor) return;
    if (editor.getValue() !== v) {
      internalUpdate = true;
      try {
        editor.setValue(v);
      } finally {
        internalUpdate = false;
      }
      // A full setValue drops decorations inside the replaced range, so
      // re-apply the highlight after a programmatic content swap (user typing
      // goes through the change listener, not setValue, so its decorations are
      // tracked and moved by Monaco and don't need this).
      applyDecorations();
    }
  });

  $effect(() => {
    const ro = readOnly;
    editor?.updateOptions({ readOnly: ro });
  });

  // Paint highlightLines as SCM-style added-line decorations: a green gutter bar
  // plus a faint line tint. Monaco tracks these across user edits, so a line the
  // user changes keeps its bar and a line they delete loses it.
  function applyDecorations() {
    if (!editor || !monacoRef) return;
    // Nothing highlighted and nothing to clear: skip, so editors that never
    // highlight don't allocate an empty collection.
    if (highlightLines.length === 0 && !decorations) return;
    const monaco = monacoRef;
    const decos = highlightLines.map((ln) => ({
      range: new monaco.Range(ln, 1, ln, 1),
      options: {
        isWholeLine: true,
        className: 'lerd-added-line',
        linesDecorationsClassName: 'lerd-added-gutter'
      }
    }));
    if (!decorations) {
      decorations = editor.createDecorationsCollection(decos);
    } else {
      decorations.set(decos);
    }
  }

  // Re-apply when the set changes (e.g. staging a different proposal) or once the
  // async editor/monaco assignment lands.
  $effect(() => {
    void highlightLines;
    applyDecorations();
  });
</script>

<div bind:this={container} class="h-full w-full {extraClass}"></div>
