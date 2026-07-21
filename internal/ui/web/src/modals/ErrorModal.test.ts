import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import ErrorModal from './ErrorModal.svelte';
import { modal, openErrorModal } from '$stores/modals';

describe('ErrorModal', () => {
  beforeEach(() => {
    openErrorModal('Restart failed: port 5173 still in use');
  });

  it('shows the failure message', () => {
    const { getByText } = render(ErrorModal);
    expect(getByText('Restart failed: port 5173 still in use')).toBeTruthy();
  });

  it('takes a title over the generic one', () => {
    openErrorModal('nope', 'Restart dev server');
    const { getByText } = render(ErrorModal);
    expect(getByText('Restart dev server')).toBeTruthy();
  });

  it('closes on dismiss', async () => {
    const { getByText } = render(ErrorModal);
    await fireEvent.click(getByText('Close'));
    expect(get(modal).kind).toBeNull();
  });
});
