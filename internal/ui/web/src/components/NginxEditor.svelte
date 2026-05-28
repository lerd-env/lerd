<script lang="ts">
  import {
    EditorView,
    keymap,
    lineNumbers,
    highlightActiveLine,
    Decoration,
    ViewPlugin
  } from '@codemirror/view';
  import type { DecorationSet, ViewUpdate } from '@codemirror/view';
  import { RangeSetBuilder } from '@codemirror/state';
  import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
  import CodeEditor from './CodeEditor.svelte';

  interface Props {
    value: string;
    readOnly?: boolean;
    onChange?: (next: string) => void;
  }
  let { value = $bindable(''), readOnly = false, onChange }: Props = $props();

  // Lightweight nginx highlighter. We only colour what is clearly identifiable
  // line by line (comments, directive names, string literals, variables, and
  // block braces) so the decorations stay correct even for unfamiliar
  // directives. Anything we don't recognise is left as plain text rather than
  // risk mislabeling it.
  const COMMENT_RE = /#.*$/;
  const STRING_RE = /"[^"\\]*(?:\\.[^"\\]*)*"|'[^'\\]*(?:\\.[^'\\]*)*'/g;
  const VAR_RE = /\$[A-Za-z_][A-Za-z0-9_]*/g;
  const DIRECTIVE_RE = /^(\s*)([A-Za-z_][A-Za-z0-9_]*)\b/;
  const BLOCK_KEYWORDS = new Set(['server', 'location', 'http', 'events', 'upstream', 'if', 'map', 'types']);

  const nginxHighlighter = ViewPlugin.fromClass(
    class {
      decorations: DecorationSet;
      constructor(v: EditorView) {
        this.decorations = this.build(v);
      }
      update(u: ViewUpdate) {
        if (u.docChanged || u.viewportChanged) this.decorations = this.build(u.view);
      }
      build(v: EditorView): DecorationSet {
        type Range = { from: number; to: number; cls: string };
        const ranges: Range[] = [];
        for (const { from, to } of v.visibleRanges) {
          let pos = from;
          while (pos <= to) {
            const line = v.state.doc.lineAt(pos);
            const text = line.text;
            let codeEnd = text.length;
            const cm = COMMENT_RE.exec(text);
            if (cm) {
              const start = line.from + cm.index;
              ranges.push({ from: start, to: start + cm[0].length, cls: 'cm-nginx-comment' });
              codeEnd = cm.index;
            }
            const code = text.slice(0, codeEnd);
            const dm = DIRECTIVE_RE.exec(code);
            if (dm) {
              const [, lead, dir] = dm;
              const start = line.from + lead.length;
              const end = start + dir.length;
              const cls = BLOCK_KEYWORDS.has(dir) ? 'cm-nginx-block' : 'cm-nginx-directive';
              ranges.push({ from: start, to: end, cls });
            }
            STRING_RE.lastIndex = 0;
            let sm: RegExpExecArray | null;
            while ((sm = STRING_RE.exec(code))) {
              ranges.push({
                from: line.from + sm.index,
                to: line.from + sm.index + sm[0].length,
                cls: 'cm-nginx-string'
              });
            }
            VAR_RE.lastIndex = 0;
            let vm: RegExpExecArray | null;
            while ((vm = VAR_RE.exec(code))) {
              ranges.push({
                from: line.from + vm.index,
                to: line.from + vm.index + vm[0].length,
                cls: 'cm-nginx-var'
              });
            }
            pos = line.to + 1;
            if (line.to >= v.state.doc.length) break;
          }
        }
        // Decorations must be added in order of `from`; we collected matches
        // in document order per line but variables/strings overlap by start
        // position, so sort once at the end before feeding the builder.
        ranges.sort((a, b) => a.from - b.from || a.to - b.to);
        const b = new RangeSetBuilder<Decoration>();
        let last = -1;
        for (const r of ranges) {
          if (r.from < last) continue;
          b.add(r.from, r.to, Decoration.mark({ class: r.cls }));
          last = r.to;
        }
        return b.finish();
      }
    },
    { decorations: (v) => v.decorations }
  );

  const nginxExtensions = [
    lineNumbers(),
    highlightActiveLine(),
    history(),
    nginxHighlighter,
    keymap.of([...defaultKeymap, ...historyKeymap]),
    EditorView.lineWrapping
  ];
</script>

<div class="nginx-editor h-full w-full">
  <CodeEditor bind:value {readOnly} {onChange} extensions={nginxExtensions} />
</div>

<style>
  .nginx-editor :global(.cm-nginx-directive) {
    color: #1d4ed8;
    font-weight: 500;
  }
  :global(.dark) .nginx-editor :global(.cm-nginx-directive) {
    color: #93c5fd;
  }
  .nginx-editor :global(.cm-nginx-block) {
    color: #7c3aed;
    font-weight: 600;
  }
  :global(.dark) .nginx-editor :global(.cm-nginx-block) {
    color: #c4b5fd;
  }
  .nginx-editor :global(.cm-nginx-string) {
    color: #047857;
  }
  :global(.dark) .nginx-editor :global(.cm-nginx-string) {
    color: #6ee7b7;
  }
  .nginx-editor :global(.cm-nginx-var) {
    color: #b45309;
  }
  :global(.dark) .nginx-editor :global(.cm-nginx-var) {
    color: #fcd34d;
  }
  .nginx-editor :global(.cm-nginx-comment) {
    color: #9ca3af;
    font-style: italic;
  }
</style>
