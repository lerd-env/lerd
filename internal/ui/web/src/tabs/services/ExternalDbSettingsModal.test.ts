import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import ExternalDbSettingsModal from './ExternalDbSettingsModal.svelte';
import type { Service } from '$stores/services';

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }));
vi.mock('$lib/api', async (importOriginal) => ({ ...(await importOriginal<typeof import('$lib/api')>()), apiFetch }));

const svc = { name: 'dbngin', status: 'active', external: 'host.containers.internal:3306', external_tools: '/usr/bin' } as Service;
const props = { open: true, svc, onclose: vi.fn() };

describe('ExternalDbSettingsModal', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders nothing while closed', () => {
    render(ExternalDbSettingsModal, { props: { ...props, open: false } });
    expect(screen.queryByLabelText('Password')).toBeNull();
  });

  it('shows the saved folder and has nothing to save until something changes', () => {
    render(ExternalDbSettingsModal, { props });
    expect((screen.getByLabelText('Client tools folder') as HTMLInputElement).value).toBe('/usr/bin');
    expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('keeps the form open with the reason when the server refuses', async () => {
    apiFetch.mockResolvedValue({ ok: false, text: async () => 'could not connect: access denied' });
    render(ExternalDbSettingsModal, { props });
    await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'typo' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(screen.getByText('could not connect: access denied')).toBeTruthy());
    expect(screen.getByLabelText('Password')).toBeTruthy();
  });

  it('saves a new folder without asking for the password again', async () => {
    apiFetch.mockResolvedValue({ ok: true });
    render(ExternalDbSettingsModal, { props });
    await fireEvent.input(screen.getByLabelText('Client tools folder'), { target: { value: '/opt/pg/bin' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(screen.getByText('Saved')).toBeTruthy());
    expect(JSON.parse(apiFetch.mock.calls[0][1].body)).toEqual({ name: 'dbngin', password: '', tools: '/opt/pg/bin' });
  });
});
