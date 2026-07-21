import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import Harness from './Popover.test.svelte';

describe('Popover', () => {
  it('renders the panel on <body> so an ancestor stacking context cannot bury it', async () => {
    render(Harness);
    await fireEvent.click(screen.getByLabelText('Open'));

    const panel = screen.getByText('panel item').parentElement!;
    expect(panel.parentElement).toBe(document.body);
    expect(panel.closest('aside')).toBeNull();
  });

  it('removes the portalled panel when it closes', async () => {
    render(Harness);
    await fireEvent.click(screen.getByLabelText('Open'));
    await fireEvent.click(screen.getByText('panel item'));

    expect(screen.queryByText('panel item')).not.toBeInTheDocument();
  });
});
