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
  return { ...actual, runTinker: vi.fn().mockResolvedValue({ stdout: '', stderr: '', error: null, mode: 'php', duration_ms: 1 }) };
});

const site = { domain: 'app.test', is_laravel: false } as unknown as Site;

beforeEach(() => {
  localStorage.clear();
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
