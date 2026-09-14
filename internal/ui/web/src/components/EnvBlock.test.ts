import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import EnvBlock from './EnvBlock.svelte';

describe('EnvBlock', () => {
  beforeEach(() => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn(async () => {}) }
    });
  });

  it('renders one sorted row per var', () => {
    render(EnvBlock, { props: { vars: { B: 'two', A: 'one' } } });
    const keys = [...document.querySelectorAll('code')].map((c) => c.textContent);
    expect(keys).toEqual(['A', 'one', 'B', 'two']);
  });

  it('uses custom label', () => {
    render(EnvBlock, { props: { vars: { K: 'v' }, label: 'Config' } });
    expect(screen.getByText('Config')).toBeInTheDocument();
  });

  it('copies the whole block as KEY=value lines', async () => {
    const writeText = vi.fn(async () => {});
    Object.assign(navigator, { clipboard: { writeText } });
    render(EnvBlock, { props: { vars: { B: 'two', A: 'one' } } });
    screen.getAllByLabelText('Copy')[0].click();
    await Promise.resolve();
    expect(writeText).toHaveBeenCalledWith('A=one\nB=two');
  });

  it('copies a single row as a KEY=value line', async () => {
    const writeText = vi.fn(async () => {});
    Object.assign(navigator, { clipboard: { writeText } });
    render(EnvBlock, { props: { vars: { DB_PASSWORD: 'lerd' } } });
    screen.getAllByLabelText('Copy')[1].click();
    await Promise.resolve();
    expect(writeText).toHaveBeenCalledWith('DB_PASSWORD=lerd');
  });
});
