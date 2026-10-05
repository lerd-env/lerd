import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import TraceView from './TraceView.svelte';

const openInEditor = vi.fn();
vi.mock('$lib/editor', async (orig) => ({ ...(await orig<object>()), openInEditor: (...a: unknown[]) => openInEditor(...a) }));
vi.mock('$lib/sourceCode', async (orig) => ({ ...(await orig<object>()), loadSource: () => Promise.resolve([{ n: 31, text: '$x = Cache::get($key);' }]) }));

const trace = [
  { file: '/srv/app/vendor/acme/cache/src/Repo.php', line: 120, func: 'Acme\\Repo->get' },
  { file: '/srv/app/vendor/acme/cache/src/Manager.php', line: 9, func: 'Acme\\Repo->fetch' },
  { file: '/srv/app/app/Cart.php', line: 31, func: 'Acme\\Manager->get' },
  { file: '/srv/app/app/Http/CartController.php', line: 12, func: 'App\\Cart->total' }
];

describe('TraceView', () => {
  it('folds the vendor run, opens it, and steps through the stops with the arrows', async () => {
    render(TraceView, { props: { trace, start: 2, showCode: false } });
    const run = screen.getByRole('button', { name: /2 vendor frames/ });
    await fireEvent.click(run);
    expect(screen.getAllByRole('option')).toHaveLength(4);
    await fireEvent.keyDown(screen.getByRole('listbox'), { key: 'ArrowUp' });
    expect(screen.getByRole('option', { selected: true })).toHaveTextContent('Manager->get()');
  });

  it('opens the vendor run holding the frame it starts on', () => {
    render(TraceView, { props: { trace: trace.slice(0, 2), start: 0, showCode: false } });
    expect(screen.getByRole('option', { selected: true })).toHaveTextContent('Repo->fetch()');
  });

  it('shows the code around the picked line', async () => {
    render(TraceView, { props: { trace, start: 2, showCode: true } });
    const n = await screen.findByRole('button', { name: '31' });
    expect(n.parentElement).toHaveTextContent('$x = Cache::get($key);');
    expect(n.parentElement!.querySelector('.hl-var')).toHaveTextContent('$x');
    await fireEvent.click(screen.getByRole('button', { name: '31' }));
    expect(openInEditor).toHaveBeenCalledWith('/srv/app/app/Cart.php', 31);
  });
});
