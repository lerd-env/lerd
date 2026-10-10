import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import BackButton from './BackButton.svelte';
import { tab, routeRest } from '$stores/route';

describe('BackButton', () => {
  it('goes from an item back to its section list', async () => {
    tab.set('services');
    routeRest.set('mysql');
    const { getByLabelText } = render(BackButton);
    await fireEvent.click(getByLabelText('Back'));
    expect(location.hash).toBe('#services');
  });
});
