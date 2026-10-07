import { render, screen, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, afterEach } from 'vitest';
import EditorPicker from './EditorPicker.svelte';

function stubEditors() {
  const editors = [
    { id: 'vscode', label: 'Visual Studio Code', installed: true },
    { id: 'zed', label: 'Zed', installed: false },
    { id: 'phpstorm', label: 'PhpStorm', installed: false }
  ];
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ editors, global: '' })))
  );
}

async function listed(value: string): Promise<string[]> {
  render(EditorPicker, { props: { value, onchange: () => {} } });
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  await new Promise((r) => setTimeout(r, 0));
  screen.getByRole('button').click();
  await new Promise((r) => setTimeout(r, 0));
  const menu = document.querySelector('[role="listbox"]') as HTMLElement;
  return Array.from(menu.querySelectorAll('[role="option"]')).map((o) => o.textContent ?? '');
}

describe('EditorPicker', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('lists only the editors this machine can open', async () => {
    stubEditors();
    const items = (await listed('')).join('|');
    expect(items).toContain('Visual Studio Code');
    expect(items).not.toContain('Zed');
    expect(items).not.toContain('PhpStorm');
  });

  // A choice whose editor was since uninstalled still reads as the choice.
  it('keeps the current choice even when it is no longer installed', async () => {
    stubEditors();
    const items = (await listed('phpstorm')).join('|');
    expect(items).toContain('PhpStorm');
    expect(items).not.toContain('Zed');
  });

  // Without a global choice the header has no editor, so it reads as Disabled.
  it('offers Disabled as the empty global choice', async () => {
    stubEditors();
    const items = await listed('');
    expect(items[0]).toContain('Disabled');
  });

  // A saved template is edited by picking Custom… again, prefilled with it.
  it('reopens the custom editor with the saved template', async () => {
    stubEditors();
    const tmpl = 'myeditor --line {line} {file}';
    const items = await listed(tmpl);
    expect(screen.getByRole('button', { name: /myeditor --line \{line\} \{file\}/ })).toBeInTheDocument();
    const custom = Array.from(document.querySelectorAll('[role="option"]')).find((o) => o.textContent?.includes('Custom…')) as HTMLElement;
    custom.click();
    await new Promise((r) => setTimeout(r, 0));
    const input = document.querySelector('input[placeholder*="{file}"]') as HTMLInputElement;
    expect(input?.value).toBe(tmpl);
  });
});
