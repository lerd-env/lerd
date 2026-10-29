import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import MonacoDiffEditor from './MonacoDiffEditor.svelte';

// Monaco needs canvas/layout/workers that jsdom lacks, so the lazy loader is
// mocked and the diff editor's wiring is checked against a fake.
const { state } = vi.hoisted(() => ({
  state: {
    diffed: null as (() => void) | null,
    models: [] as { original: string; modified: string }[]
  }
}));

vi.mock('$lib/monaco', () => {
  let model: { original: { getValue: () => string; dispose: () => void }; modified: { getValue: () => string; dispose: () => void } } | null = null;
  const side = { onMouseDown: () => ({ dispose() {} }), updateOptions: () => {}, createDecorationsCollection: () => ({ set: () => {} }) };
  const editor = {
    onDidUpdateDiff: (cb: () => void) => {
      state.diffed = cb;
      return { dispose() {} };
    },
    getModel: () => model,
    setModel: (m: typeof model) => {
      model = m;
      state.models.push({ original: m!.original.getValue(), modified: m!.modified.getValue() });
    },
    getLineChanges: () => [],
    getModifiedEditor: () => side,
    updateOptions: () => {},
    dispose: () => {}
  };
  const monaco = {
    editor: {
      createDiffEditor: () => editor,
      createModel: (v: string) => ({ getValue: () => v, dispose: () => {} }),
      setTheme: () => {},
      MouseTargetType: { GUTTER_GLYPH_MARGIN: 2 }
    },
    Range: class {}
  };
  return { loadMonaco: () => Promise.resolve(monaco), lerdThemeName: () => 'lerd-dark', applyEditorAccent: () => {} };
});

let frames: FrameRequestCallback[] = [];
function nextFrame() {
  const run = frames;
  frames = [];
  run.forEach((f) => f(0));
  return tick();
}

// onMount's async loader resolves on the macrotask after render.
const flush = () => new Promise((r) => setTimeout(r, 0));

describe('MonacoDiffEditor', () => {
  beforeEach(() => {
    state.diffed = null;
    state.models = [];
    frames = [];
    vi.stubGlobal('requestAnimationFrame', (f: FrameRequestCallback) => frames.push(f));
  });

  // Monaco paints the whole file before folding it, so nothing shows until the fold has painted.
  it('stays hidden until the diff has landed and a frame has passed', async () => {
    const { container } = render(MonacoDiffEditor, { props: { original: 'a', modified: 'b' } });
    await flush();
    const box = container.firstElementChild as HTMLElement;
    expect(state.models).toEqual([{ original: 'a', modified: 'b' }]);
    expect(box.classList.contains('opacity-0')).toBe(true);

    state.diffed!();
    await tick();
    expect(box.classList.contains('opacity-0')).toBe(true);
    await nextFrame();
    expect(box.classList.contains('opacity-0')).toBe(false);
  });

  it('hides again for the next file until its own diff lands', async () => {
    const { container, rerender } = render(MonacoDiffEditor, { props: { original: 'a', modified: 'b' } });
    await flush();
    state.diffed!();
    await nextFrame();
    const box = container.firstElementChild as HTMLElement;
    expect(box.classList.contains('opacity-0')).toBe(false);

    await rerender({ original: 'c', modified: 'd' });
    expect(state.models.at(-1)).toEqual({ original: 'c', modified: 'd' });
    expect(box.classList.contains('opacity-0')).toBe(true);
    state.diffed!();
    await nextFrame();
    expect(box.classList.contains('opacity-0')).toBe(false);
  });
});
