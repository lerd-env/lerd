import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import SourcePath from './SourcePath.svelte';

const openInEditor = vi.fn(async (_path: string, _line: number) => {});
vi.mock('$lib/editor', async (orig) => ({ ...(await orig<object>()), openInEditor: (p: string, l: number) => openInEditor(p, l) }));

describe('SourcePath', () => {
  beforeEach(() => {
    openInEditor.mockClear();
    Object.assign(navigator, { clipboard: { writeText: vi.fn(async () => {}) } });
  });

  it('opens the file at its line', async () => {
    render(SourcePath, { props: { file: '/home/u/app/Models/User.php', line: 42 } });
    screen.getByText('/home/u/app/Models/User.php:42').click();
    await vi.waitFor(() => expect(openInEditor).toHaveBeenCalledWith('/home/u/app/Models/User.php', 42));
  });

  it('opens a file with no line at the top', async () => {
    render(SourcePath, { props: { file: '/home/u/resources/views/home.blade.php' } });
    screen.getByText('/home/u/resources/views/home.blade.php').click();
    await vi.waitFor(() =>
      expect(openInEditor).toHaveBeenCalledWith('/home/u/resources/views/home.blade.php', 1)
    );
  });

  it('copies the file:line reference', async () => {
    const writeText = vi.fn(async () => {});
    Object.assign(navigator, { clipboard: { writeText } });
    render(SourcePath, { props: { file: '/home/u/app/Models/User.php', line: 42 } });
    screen.getByLabelText('Copy path').click();
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith('/home/u/app/Models/User.php:42'));
  });

  it('shortens a long path for display but copies it whole', async () => {
    const writeText = vi.fn(async () => {});
    Object.assign(navigator, { clipboard: { writeText } });
    render(SourcePath, {
      props: { file: '/home/u/Projects/shop/app/Models/User.php', line: 7, short: true }
    });
    expect(screen.getByText('…/app/Models/User.php:7')).toBeInTheDocument();
    screen.getByLabelText('Copy path').click();
    await vi.waitFor(() =>
      expect(writeText).toHaveBeenCalledWith('/home/u/Projects/shop/app/Models/User.php:7')
    );
  });
});
