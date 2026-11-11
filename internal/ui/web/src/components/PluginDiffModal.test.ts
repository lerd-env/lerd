import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { it, expect, vi, beforeEach } from 'vitest';
import PluginDiffModal from './PluginDiffModal.svelte';

// Monaco needs canvas and workers jsdom lacks, so record what each editor is created with.
const { created, ed } = vi.hoisted(() => ({
  created: [] as { language?: string; value?: string }[],
  // The last editor made: its text, and what typing and Ctrl+S would call.
  ed: { value: '', change: null as null | (() => void), save: null as null | (() => void) }
}));

vi.mock('$lib/monaco', () => {
  const fakeEditor = {
    getValue: () => ed.value,
    setValue: (v: string) => {
      ed.value = v;
    },
    onDidChangeModelContent: (cb: () => void) => {
      ed.change = cb;
      return { dispose() {} };
    },
    updateOptions: () => {},
    addCommand: (_key: number, cb: () => void) => {
      ed.save = cb;
    },
    dispose: () => {}
  };
  const monaco = {
    editor: {
      create: (_el: HTMLElement, opts: any) => {
        created.push({ language: opts.language, value: opts.value });
        ed.value = opts.value ?? '';
        ed.change = ed.save = null;
        return fakeEditor;
      },
      setTheme: () => {},
      defineTheme: () => {}
    },
    KeyMod: { CtrlCmd: 0 },
    KeyCode: { KeyS: 0 }
  };
  return { loadMonaco: async () => monaco, default: async () => monaco, applyEditorAccent: () => {}, lerdThemeName: () => 'lerd-light' };
});

// A page may send a second file while the viewer is still open; it must be
// highlighted as its own language, not the first file's.
it('highlights a second viewed file by its own language', async () => {
  const { rerender } = render(PluginDiffModal, { props: { diff: { path: 'a.php', modified: '<?php' }, onclose: () => {} } });
  await vi.waitFor(() => expect(created.at(-1)?.language).toBe('php'));
  await rerender({ diff: { path: 'b.ini', modified: 'memory_limit=1G' }, onclose: () => {} });
  await tick();
  await vi.waitFor(() => expect(created.at(-1)).toEqual({ language: 'ini', value: 'memory_limit=1G' }));
});

beforeEach(() => {
  ed.change = ed.save = null;
});

// Typing into the editor must reach the page: a test that saves untouched text
// would pass just as well with an editor that never lets anyone type.
function typeInto(text: string) {
  ed.value = text;
  ed.change?.();
}

it('saves what was typed, from the Save button', async () => {
  const onsave = vi.fn();
  const { findByText } = render(PluginDiffModal, { props: { diff: { path: 'scrub.sql', modified: 'OLD;' }, onclose: () => {}, onsave } });
  await vi.waitFor(() => expect(ed.change).not.toBeNull());
  typeInto('NEW;');
  (await findByText('Save')).click();
  expect(onsave).toHaveBeenCalledWith('NEW;');
});

it('saves what was typed, from Ctrl+S', async () => {
  const onsave = vi.fn();
  render(PluginDiffModal, { props: { diff: { path: 'scrub.sql', modified: 'OLD;' }, onclose: () => {}, onsave } });
  await vi.waitFor(() => expect(ed.save).not.toBeNull());
  typeInto('TYPED;');
  ed.save!();
  expect(onsave).toHaveBeenCalledWith('TYPED;');
});
