import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { it, expect, vi } from 'vitest';
import PluginDiffModal from './PluginDiffModal.svelte';

// Monaco needs canvas and workers jsdom lacks, so record what each editor is created with.
const { created } = vi.hoisted(() => ({ created: [] as { language?: string; value?: string }[] }));

vi.mock('$lib/monaco', () => {
  const fakeEditor = {
    getValue: () => '',
    setValue: () => {},
    onDidChangeModelContent: () => ({ dispose() {} }),
    updateOptions: () => {},
    addCommand: () => {},
    dispose: () => {}
  };
  const monaco = {
    editor: {
      create: (_el: HTMLElement, opts: any) => {
        created.push({ language: opts.language, value: opts.value });
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
