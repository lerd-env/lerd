import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import SiteTinkerTab from './SiteTinkerTab.svelte';
import type { Site } from '$stores/sites';

// Monaco needs canvas/layout/workers jsdom can't provide; stub the loader so
// MonacoEditor mounts a no-op editor. The LSP attach is also stubbed so the
// status indicator never tries to talk to a real language server.
vi.mock('$lib/monaco', () => ({
  loadMonaco: () =>
    Promise.resolve({
      editor: {
        create: (_el: HTMLElement, opts: any) => ({
          getValue: () => opts.value ?? '',
          setValue: (v: string) => { opts.value = v; },
          onDidChangeModelContent: () => ({ dispose() {} }),
          onKeyDown: () => {},
          updateOptions: () => {},
          addCommand: () => {},
          focus: () => {},
          dispose: () => {}
        }),
        setTheme: () => {},
        defineTheme: () => {}
      },
      KeyMod: { CtrlCmd: 2048 },
      KeyCode: { Enter: 3 }
    }),
  lerdThemeName: () => 'lerd-dark'
}));

vi.mock('$lib/lsp', () => ({
  attachPhpLsp: () => ({ dispose() {} })
}));

vi.mock('$stores/sites', async () => {
  const actual = await vi.importActual<typeof import('$stores/sites')>('$stores/sites');
  return {
    ...actual,
    runTinker: vi.fn().mockResolvedValue({ stdout: '', stderr: '', error: null, mode: 'php', duration_ms: 1 }),
    fetchTinkerSnippets: vi.fn().mockResolvedValue([]),
    saveTinkerSnippet: vi.fn().mockResolvedValue({ ok: true, snippets: [] }),
    deleteTinkerSnippet: vi.fn().mockResolvedValue({ ok: true, snippets: [] })
  };
});

import { fetchTinkerSnippets, runTinker, saveTinkerSnippet, deleteTinkerSnippet, type TinkerSnippet } from '$stores/sites';

const site = { domain: 'app.test', is_laravel: false } as unknown as Site;

async function tick() {
  await new Promise((r) => setTimeout(r, 0));
}

beforeEach(() => {
  localStorage.clear();
  // The snippet list is fetched at mount and again on every menu open, so
  // per-test data must persist across calls (mockResolvedValue, not Once).
  vi.mocked(fetchTinkerSnippets).mockResolvedValue([]);
  vi.mocked(saveTinkerSnippet).mockClear().mockResolvedValue({ ok: true, snippets: [] });
  vi.mocked(deleteTinkerSnippet).mockClear().mockResolvedValue({ ok: true, snippets: [] });
});

describe('SiteTinkerTab split and full screen', () => {
  it('defaults to a horizontal split', () => {
    render(SiteTinkerTab, { props: { site } });
    // The split container carries data-splitdir for testability.
    const split = document.querySelector('[data-splitdir]');
    expect(split?.getAttribute('data-splitdir')).toBe('horizontal');
  });

  it('flips the split direction when the toggle is clicked', async () => {
    const { getByLabelText } = render(SiteTinkerTab, { props: { site } });
    // While horizontal, the toggle advertises the stacked (vertical) option.
    const toggle = getByLabelText('Split stacked');
    await fireEvent.click(toggle);
    expect(document.querySelector('[data-splitdir]')?.getAttribute('data-splitdir')).toBe('vertical');
  });

  it('persists the split direction across remounts via localStorage', async () => {
    const r1 = render(SiteTinkerTab, { props: { site } });
    await fireEvent.click(r1.getByLabelText('Split stacked'));
    r1.unmount();
    const r2 = render(SiteTinkerTab, { props: { site } });
    expect(document.querySelector('[data-splitdir]')?.getAttribute('data-splitdir')).toBe('vertical');
  });

  it('enters full screen on the maximize button', async () => {
    const { getByLabelText, container } = render(SiteTinkerTab, { props: { site } });
    const root = container.firstElementChild as HTMLElement;
    expect(root.className).not.toContain('fixed');
    await fireEvent.click(getByLabelText('Enter full screen'));
    expect(root.className).toContain('fixed');
  });

  it('keeps the flex column while full screen so the panes fill the viewport', async () => {
    const { getByLabelText, container } = render(SiteTinkerTab, { props: { site } });
    const root = container.firstElementChild as HTMLElement;
    await fireEvent.click(getByLabelText('Enter full screen'));
    for (const cls of ['flex', 'flex-col', 'min-h-0']) expect(root.classList).toContain(cls);
    const split = document.querySelector('[data-splitdir]') as HTMLElement;
    expect(split.classList).toContain('flex-1');
  });

  it('names the site only while full screen', async () => {
    const { getByLabelText, queryByText, getByText } = render(SiteTinkerTab, { props: { site } });
    expect(queryByText('app.test')).not.toBeInTheDocument();
    await fireEvent.click(getByLabelText('Enter full screen'));
    expect(getByText('app.test')).toBeInTheDocument();
  });

  it('names the worktree branch alongside its domain while full screen', async () => {
    const wt = {
      domain: 'app.test',
      is_laravel: false,
      worktrees: [{ branch: 'feature/x', domain: 'feature-x.app.test' }]
    } as unknown as Site;
    const { getByLabelText, getByText } = render(SiteTinkerTab, { props: { site: wt, branch: 'feature/x' } });
    await fireEvent.click(getByLabelText('Enter full screen'));
    expect(getByText('feature-x.app.test')).toBeInTheDocument();
    expect(getByText('feature/x')).toBeInTheDocument();
  });

  it('exits full screen on Escape', async () => {
    const { getByLabelText, container } = render(SiteTinkerTab, { props: { site } });
    const root = container.firstElementChild as HTMLElement;
    await fireEvent.click(getByLabelText('Enter full screen'));
    expect(root.className).toContain('fixed');
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(root.className).not.toContain('fixed');
  });

  it('swaps the maximize button to a minimize button while full screen', async () => {
    const { getByLabelText, queryByLabelText } = render(SiteTinkerTab, { props: { site } });
    await fireEvent.click(getByLabelText('Enter full screen'));
    expect(getByLabelText('Exit full screen (Esc)')).toBeInTheDocument();
    expect(queryByLabelText('Enter full screen')).not.toBeInTheDocument();
  });
});

describe('SiteTinkerTab snippets menu', () => {
  const seedSnippet: TinkerSnippet = {
    name: 'seed.php',
    label: 'Seed demo data',
    source: 'project',
    content: 'Seeder::run();\n'
  };

  async function openMenu(getByLabelText: (t: string) => HTMLElement) {
    await fireEvent.click(getByLabelText('Snippets'));
    await tick();
    return document.querySelector('[data-testid="snippets-menu"]') as HTMLElement;
  }

  it('shows the empty state and disables saving while the editor is empty', async () => {
    const { getByLabelText, getByText } = render(SiteTinkerTab, { props: { site } });
    await tick();
    const menu = await openMenu(getByLabelText);
    expect(menu).not.toBeNull();
    expect(getByText('No snippets yet. Save the editor contents to reuse them here.')).toBeInTheDocument();
    expect((getByText('Save editor as snippet…').closest('button') as HTMLButtonElement).disabled).toBe(true);
  });

  it('loads a snippet straight into an empty editor without running it', async () => {
    vi.mocked(fetchTinkerSnippets).mockResolvedValue([seedSnippet]);
    const { getByLabelText, getByText } = render(SiteTinkerTab, { props: { site } });
    await tick();
    await openMenu(getByLabelText);
    expect(getByText('.lerd/tinker/snippets')).toBeInTheDocument();
    await fireEvent.click(getByText('Seed demo data'));
    await tick();
    // The draft-persisting $effect mirrors the editor value, so the
    // localStorage draft doubles as the "editor now holds it" assertion.
    expect(localStorage.getItem('tinker:app.test:draft')).toBe('Seeder::run();\n');
    expect(vi.mocked(runTinker)).not.toHaveBeenCalled();
  });

  it('asks before replacing a non-empty editor and loads only on confirm', async () => {
    localStorage.setItem('tinker:app.test:draft', 'echo "work in progress";');
    vi.mocked(fetchTinkerSnippets).mockResolvedValue([seedSnippet]);
    const { getByLabelText, getByText, getByRole } = render(SiteTinkerTab, { props: { site } });
    await tick();
    await openMenu(getByLabelText);
    await fireEvent.click(getByText('Seed demo data'));
    await tick();
    // Nothing replaced yet; the confirmation modal is up instead.
    expect(localStorage.getItem('tinker:app.test:draft')).toBe('echo "work in progress";');
    expect(getByRole('heading', { name: 'Load snippet' })).toBeInTheDocument();
    await fireEvent.click(getByText('Load'));
    await tick();
    expect(localStorage.getItem('tinker:app.test:draft')).toBe('Seeder::run();\n');
  });

  it('deletes a snippet only after its confirmation', async () => {
    vi.mocked(fetchTinkerSnippets).mockResolvedValue([seedSnippet]);
    const { getByLabelText, getByText, getByRole } = render(SiteTinkerTab, { props: { site } });
    await tick();
    await openMenu(getByLabelText);
    await fireEvent.click(getByLabelText('Delete snippet'));
    await tick();
    expect(vi.mocked(deleteTinkerSnippet)).not.toHaveBeenCalled();
    expect(getByRole('heading', { name: 'Delete snippet' })).toBeInTheDocument();
    await fireEvent.click(getByText('Delete'));
    await tick();
    expect(vi.mocked(deleteTinkerSnippet)).toHaveBeenCalledWith('app.test', { name: 'seed.php', source: 'project' }, '');
  });

  it('saves the editor contents as a named snippet', async () => {
    localStorage.setItem('tinker:app.test:draft', 'User::count();');
    const { getByLabelText, getByText } = render(SiteTinkerTab, { props: { site } });
    await tick();
    await openMenu(getByLabelText);
    await fireEvent.click(getByText('Save editor as snippet…'));
    await tick();
    const input = document.getElementById('snippet-name') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'count-users' } });
    await fireEvent.click(getByText('Save'));
    await tick();
    expect(vi.mocked(saveTinkerSnippet)).toHaveBeenCalledWith(
      'app.test',
      { name: 'count-users', source: 'project', content: 'User::count();' },
      ''
    );
  });

  it('prefills the save dialog from the last loaded snippet, but not from tinkerwell', async () => {
    const globalSnippet: TinkerSnippet = { name: 'whoami.php', label: 'whoami', source: 'global', content: '1;' };
    const twSnippet: TinkerSnippet = { name: 'legacy.php', label: 'legacy', source: 'tinkerwell', content: '1;' };
    vi.mocked(fetchTinkerSnippets).mockResolvedValue([globalSnippet, twSnippet]);
    const { getByLabelText, getByText } = render(SiteTinkerTab, { props: { site } });
    await tick();

    await openMenu(getByLabelText);
    await fireEvent.click(getByText('whoami'));
    await tick();
    await openMenu(getByLabelText);
    await fireEvent.click(getByText('Save editor as snippet…'));
    await tick();
    let input = document.getElementById('snippet-name') as HTMLInputElement;
    expect(input.value).toBe('whoami');
    expect(getByText('All my sites')).toBeInTheDocument();
    await fireEvent.click(getByText('Cancel'));

    // Loading a tinkerwell snippet must not offer to "update" it: lerd never
    // writes into another tool's directory, so the dialog starts blank.
    await openMenu(getByLabelText);
    await fireEvent.click(getByText('legacy'));
    await tick();
    await openMenu(getByLabelText);
    await fireEvent.click(getByText('Save editor as snippet…'));
    await tick();
    input = document.getElementById('snippet-name') as HTMLInputElement;
    expect(input.value).toBe('');
  });

  it('keeps the save dialog open with the error when saving fails', async () => {
    localStorage.setItem('tinker:app.test:draft', 'User::count();');
    vi.mocked(saveTinkerSnippet).mockResolvedValue({ ok: false, error: 'writing snippet: disk full' });
    const { getByLabelText, getByText } = render(SiteTinkerTab, { props: { site } });
    await tick();
    await openMenu(getByLabelText);
    await fireEvent.click(getByText('Save editor as snippet…'));
    await tick();
    const input = document.getElementById('snippet-name') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'count-users' } });
    await fireEvent.click(getByText('Save'));
    await tick();
    expect(getByText('writing snippet: disk full')).toBeInTheDocument();
    expect(document.getElementById('snippet-name')).not.toBeNull();
    // A retry is possible once the backend recovers.
    vi.mocked(saveTinkerSnippet).mockResolvedValue({ ok: true, snippets: [] });
    await fireEvent.click(getByText('Save'));
    await tick();
    expect(document.getElementById('snippet-name')).toBeNull();
  });
});
