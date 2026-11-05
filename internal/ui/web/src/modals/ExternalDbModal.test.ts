import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ExternalDbModal from './ExternalDbModal.svelte';

const { apiFetch, closeModal, loadServices } = vi.hoisted(() => ({
  apiFetch: vi.fn(),
  closeModal: vi.fn(),
  loadServices: vi.fn()
}));
vi.mock('$lib/api', async (importOriginal) => ({ ...(await importOriginal<typeof import('$lib/api')>()), apiFetch }));
vi.mock('$stores/modals', async (importOriginal) => ({ ...(await importOriginal<typeof import('$stores/modals')>()), closeModal }));
vi.mock('$stores/services', async (importOriginal) => ({ ...(await importOriginal<typeof import('$stores/services')>()), loadServices }));

const answer = (ok: boolean, body: unknown) =>
  ({ ok, json: async () => body, text: async () => String(body) }) as unknown as Response;

describe('ExternalDbModal', () => {
  beforeEach(() => {
    apiFetch.mockReset();
    closeModal.mockReset();
  });

  it('shows the version the server answers a test with', async () => {
    apiFetch.mockResolvedValue(answer(true, { version: '11.8.8-MariaDB' }));
    render(ExternalDbModal);
    await fireEvent.click(screen.getByText('Test connection'));
    await waitFor(() => expect(screen.getByText('Connected to 11.8.8-MariaDB')).toBeTruthy());
    expect(apiFetch.mock.calls[0][0]).toBe('/api/services/external/test');
    expect(closeModal).not.toHaveBeenCalled();
  });

  it('stays open with the reason when the server refuses', async () => {
    apiFetch.mockResolvedValue(answer(false, 'could not connect: access denied'));
    render(ExternalDbModal);
    await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'dbngin' } });
    await fireEvent.click(screen.getByText('Add server'));
    await waitFor(() => expect(screen.getByText('could not connect: access denied')).toBeTruthy());
    expect(closeModal).not.toHaveBeenCalled();
  });

  it('adds the server and closes', async () => {
    apiFetch.mockResolvedValue(answer(true, { name: 'dbngin' }));
    render(ExternalDbModal);
    await fireEvent.input(screen.getByLabelText('Name'), { target: { value: 'dbngin' } });
    await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 's3cret' } });
    await fireEvent.click(screen.getByText('Add server'));
    await waitFor(() => expect(closeModal).toHaveBeenCalled());
    const body = JSON.parse(apiFetch.mock.calls[0][1].body);
    expect(body).toMatchObject({ name: 'dbngin', engine: 'mysql', port: 3306, password: 's3cret' });
  });

  it('needs a name before it can add', async () => {
    render(ExternalDbModal);
    expect((screen.getByText('Add server').closest('button') as HTMLButtonElement).disabled).toBe(true);
  });
});
