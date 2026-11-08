import { describe, it, expect, beforeAll } from 'vitest';
import { EditorExtensionsRegistry } from 'monaco-editor/editor/browser/editorExtensions';
import { loadMonaco } from './monaco';

// Monaco's core API ships bare, so the editors only find, fold, suggest and
// hover when the loader registers those features itself.
describe('loadMonaco', () => {
  // jsdom has no matchMedia, which Monaco reads for the colour scheme.
  beforeAll(() => {
    window.matchMedia ??= ((query: string) =>
      ({ matches: false, media: query, addEventListener() {}, removeEventListener() {} }) as unknown as MediaQueryList);
  });

  it('registers the editor features the editors rely on', async () => {
    await loadMonaco();
    const ids = EditorExtensionsRegistry.getEditorContributions().map((c) => c.id);
    expect(ids).toEqual(
      expect.arrayContaining([
        'editor.contrib.findController',
        'editor.contrib.folding',
        'editor.contrib.suggestController',
        'editor.contrib.contentHover',
        'editor.contrib.bracketMatchingController',
      ]),
    );
  });
});
