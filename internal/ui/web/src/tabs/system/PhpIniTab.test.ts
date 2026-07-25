import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import PhpIniTab from './PhpIniTab.svelte';

// Monaco can't run in jsdom.
vi.mock('$lib/monaco', () => ({
  loadMonaco: () =>
    Promise.resolve({
      editor: {
        create: (_el: HTMLElement, opts: { value?: string }) => ({
          getValue: () => opts.value ?? '',
          setValue: () => {},
          onDidChangeModelContent: () => ({ dispose() {} }),
          updateOptions: () => {},
          dispose: () => {}
        }),
        setTheme: () => {},
        defineTheme: () => {}
      }
    }),
  lerdThemeName: () => 'lerd-dark'
}));

const { getPhpIni, loadPhpIniBackups, loadPhpIniBackupContent } = vi.hoisted(() => ({
  getPhpIni: vi.fn(),
  loadPhpIniBackups: vi.fn(),
  loadPhpIniBackupContent: vi.fn()
}));
const { openPhpIniRestoreModal } = vi.hoisted(() => ({ openPhpIniRestoreModal: vi.fn() }));

vi.mock('$stores/phpVersions', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, getPhpIni, loadPhpIniBackups, loadPhpIniBackupContent };
});
vi.mock('$stores/modals', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, openPhpIniRestoreModal };
});

const flush = () => new Promise((r) => setTimeout(r, 0));

describe('PhpIniTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getPhpIni.mockImplementation((scope: string) =>
      Promise.resolve({
        content: scope === 'shared' ? 'memory_limit = 256M' : 'memory_limit = 512M',
        path: scope === 'shared' ? '/etc/php/conf.d/lerd.ini' : `/etc/php/${scope}/php.ini`,
        exists: true
      })
    );
    loadPhpIniBackups.mockResolvedValue({ ok: true, list: [{ name: 'php.ini.bak' }] });
    loadPhpIniBackupContent.mockResolvedValue('memory_limit = 128M');
  });

  it('loads both the version and the shared file side by side', async () => {
    const { getByText } = render(PhpIniTab, { props: { version: '8.4' } });
    await flush();

    expect(getPhpIni.mock.calls.map((c) => c[0]).sort()).toEqual(['8.4', 'shared']);
    expect(getByText('PHP 8.4')).toBeTruthy();
    expect(getByText('Shared (all versions)')).toBeTruthy();
    expect(getByText('/etc/php/8.4/php.ini')).toBeTruthy();
    expect(getByText('/etc/php/conf.d/lerd.ini')).toBeTruthy();
  });

  it('keeps each pane’s actions on its own scope', async () => {
    const { getAllByText } = render(PhpIniTab, { props: { version: '8.4' } });
    await flush();

    // Panes render version-first, so the second Restore belongs to the shared file.
    await fireEvent.click(getAllByText('Restore')[1]);
    await flush();

    expect(loadPhpIniBackupContent).toHaveBeenCalledWith('shared', 'php.ini.bak');
    expect(openPhpIniRestoreModal.mock.calls[0][0]).toMatchObject({
      version: 'shared',
      label: 'the shared file (all versions)'
    });
  });
});
