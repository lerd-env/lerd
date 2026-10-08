import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { m } from '../paraglide/messages.js';
import LensSearch from './LensSearch.svelte';

// Every lens search clears the same way, from the × inside it.
describe('LensSearch', () => {
  it('offers a clear button only while there is text, and empties the search', async () => {
    render(LensSearch, { props: { value: '', placeholder: 'Search' } });
    expect(screen.queryByRole('button', { name: m.queries_clearFilter() })).toBeNull();

    const input = screen.getByPlaceholderText('Search') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'select' } });
    await fireEvent.click(screen.getByRole('button', { name: m.queries_clearFilter() }));
    expect(input.value).toBe('');
  });
});
