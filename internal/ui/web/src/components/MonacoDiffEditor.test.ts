import { render, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import MonacoDiffEditor from './MonacoDiffEditor.svelte';
import { editorFontSize } from '$stores/editorFont';

// Monaco needs canvas/layout/workers that jsdom lacks, so the lazy loader is
// mocked and the diff editor's wiring is checked against a fake.
const { state } = vi.hoisted(() => ({
  state: {
    diffed: null as (() => void) | null,
    models: [] as { original: string; modified: string }[],
    created: {} as { fontSize?: number; splitViewDefaultRatio?: number },
    fontSize: 0,
    widths: { original: 0, modified: 0 },
    layout: null as (() => void) | null,
    updatedRatio: undefined as number | undefined
  }
}));

vi.mock('$lib/monaco', () => {
  let model: { original: { getValue: () => string; dispose: () => void }; modified: { getValue: () => string; dispose: () => void } } | null = null;
  const side = {
    getLayoutInfo: () => ({ width: state.widths.modified }),
    onDidLayoutChange: (cb: () => void) => {
      state.layout = cb;
      return { dispose() {} };
    },
    onMouseDown: () => ({ dispose() {} }), updateOptions: () => {}, createDecorationsCollection: () => ({ set: () => {} }) };
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
    updateOptions: (o: { fontSize?: number; splitViewDefaultRatio?: number }) => {
      if (o.fontSize !== undefined) state.fontSize = o.fontSize;
      state.updatedRatio = o.splitViewDefaultRatio;
    },
    dispose: () => {}
  };
  const monaco = {
    editor: {
      createDiffEditor: (_el: HTMLElement, opts: { fontSize?: number; splitViewDefaultRatio?: number }) => {
        state.created = opts;
        return editor;
      },
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
    state.layout = null;
    frames = [];
    vi.stubGlobal('requestAnimationFrame', (f: FrameRequestCallback) => frames.push(f));
  });

  it('opens at the split it is given and reports where a drag leaves it', async () => {
    const onSplit = vi.fn();
    const { container } = render(MonacoDiffEditor, { props: { original: 'a', modified: 'b', splitRatio: 0.3, onSplit } });
    await flush();
    expect(state.created.splitViewDefaultRatio).toBe(0.3);
    expect(state.updatedRatio).toBe(0.3);
    Object.defineProperty(container.firstElementChild, 'clientWidth', { value: 1000 });
    // A layout of its own, such as a new editor settling, is not the user's split.
    state.widths = { original: 400, modified: 600 };
    state.layout!();
    expect(onSplit).not.toHaveBeenCalled();

    const sash = document.createElement('div');
    sash.className = 'monaco-sash vertical';
    container.firstElementChild!.append(sash);
    await fireEvent.pointerDown(sash);
    state.widths = { original: 215, modified: 750 };
    state.layout!();
    expect(onSplit).toHaveBeenLastCalledWith(0.25);
    await fireEvent.pointerUp(window);
    state.widths = { original: 500, modified: 500 };
    state.layout!();
    expect(onSplit).toHaveBeenCalledTimes(1);
  });

  it('reports no split while unified or not laid out', async () => {
    const onSplit = vi.fn();
    const { container } = render(MonacoDiffEditor, { props: { original: 'a', modified: 'b', sideBySide: false, onSplit } });
    await flush();
    Object.defineProperty(container.firstElementChild, 'clientWidth', { value: 1000 });
    const sash = document.createElement('div');
    sash.className = 'monaco-sash vertical';
    container.firstElementChild!.append(sash);
    await fireEvent.pointerDown(sash);
    state.widths = { original: 0, modified: 1000 };
    state.layout!();
    expect(onSplit).not.toHaveBeenCalled();
  });

  it('opens at the shared text size and follows it', async () => {
    editorFontSize.set(15);
    render(MonacoDiffEditor, { props: { original: 'a', modified: 'b' } });
    await flush();
    expect(state.created.fontSize).toBe(15);
    editorFontSize.set(17);
    await tick();
    expect(state.fontSize).toBe(17);
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
