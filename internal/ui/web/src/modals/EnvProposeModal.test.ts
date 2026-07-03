import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import { describe, it, expect, afterEach, vi } from 'vitest';
import EnvProposeModal from './EnvProposeModal.svelte';
import { openEnvProposeModal, closeModal } from '$stores/modals';

afterEach(() => {
  closeModal();
  cleanup();
});

function open(onAdd = vi.fn()) {
  openEnvProposeModal({
    file: '.env',
    entries: [
      { key: 'DB_PORT', value: '5432', required: true },
      { key: 'MAIL_HOST', value: 'smtp', required: false }
    ],
    onAdd
  });
  return onAdd;
}

describe('EnvProposeModal', () => {
  it('shows each missing key with its value', () => {
    open();
    render(EnvProposeModal);
    expect(screen.getByText('DB_PORT')).toBeInTheDocument();
    expect(screen.getByText('=5432')).toBeInTheDocument();
    expect(screen.getByText('MAIL_HOST')).toBeInTheDocument();
    expect(screen.getByText('=smtp')).toBeInTheDocument();
  });

  it('pre-checks required keys and leaves optional ones off', () => {
    open();
    render(EnvProposeModal);
    const boxes = screen.getAllByRole('checkbox') as HTMLInputElement[];
    expect(boxes[0].checked).toBe(true); // DB_PORT (required)
    expect(boxes[1].checked).toBe(false); // MAIL_HOST (optional)
    // The Add button counts only the ticked keys.
    expect(screen.getByRole('button', { name: 'Add 1' })).toBeInTheDocument();
  });

  it('adds exactly the ticked keys', async () => {
    const onAdd = open();
    render(EnvProposeModal);
    const boxes = screen.getAllByRole('checkbox') as HTMLInputElement[];
    await fireEvent.click(boxes[1]); // also tick MAIL_HOST
    await fireEvent.click(screen.getByRole('button', { name: 'Add 2' }));
    expect(onAdd).toHaveBeenCalledWith(['DB_PORT', 'MAIL_HOST']);
  });

  it('disables the add button when nothing is selected', async () => {
    open();
    render(EnvProposeModal);
    await fireEvent.click(screen.getByRole('button', { name: 'Clear' }));
    expect(screen.getByRole('button', { name: 'Add selected' })).toBeDisabled();
  });
});
