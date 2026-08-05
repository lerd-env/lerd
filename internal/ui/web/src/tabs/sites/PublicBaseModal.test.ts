import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import PublicBaseModal from './PublicBaseModal.svelte';
import Harness from './PublicBaseModal.test.svelte';

const props = {
  open: true,
  base: '',
  error: '',
  busy: false,
  onclose: () => {},
  onsubmit: () => {}
};

const field = () => screen.getByLabelText('Base domain') as HTMLInputElement;

describe('PublicBaseModal', () => {
  it('seeds the field from the configured base when it opens', async () => {
    const { rerender } = render(PublicBaseModal, { props: { ...props, open: false, base: 'dev.example.com' } });
    await rerender({ ...props, open: true, base: 'dev.example.com' });
    expect(field().value).toBe('dev.example.com');
  });

  // The share tools refresh while the modal is up; that must not wipe the field.
  it('keeps what the user typed when the configured base refreshes', async () => {
    render(Harness);
    await fireEvent.input(field(), { target: { value: 'lab.example.com' } });
    await fireEvent.click(screen.getByTestId('refresh-base'));
    expect(field().value).toBe('lab.example.com');
  });

  it('submits the cleaned domain', async () => {
    const onsubmit = vi.fn();
    render(PublicBaseModal, { props: { ...props, onsubmit } });
    await fireEvent.input(field(), { target: { value: '.Dev.Example.com.' } });
    await fireEvent.click(screen.getByText('Save'));
    expect(onsubmit).toHaveBeenCalledWith('dev.example.com');
  });
});
