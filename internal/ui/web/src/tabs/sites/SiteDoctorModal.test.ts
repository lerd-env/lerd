import { render, screen, cleanup, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, afterEach } from 'vitest';

const loadDoctor = vi.fn();
const loadCommands = vi.fn();
const launchCommand = vi.fn();
const executeDoctorFix = vi.fn();
const runSettled = vi.fn().mockResolvedValue(undefined);

vi.mock('$stores/doctor', () => ({
  loadDoctor: (...a: unknown[]) => loadDoctor(...a)
}));
vi.mock('$stores/commands', () => ({
  loadCommands: (...a: unknown[]) => loadCommands(...a),
  launchCommand: (...a: unknown[]) => launchCommand(...a),
  executeDoctorFix: (...a: unknown[]) => executeDoctorFix(...a),
  runSettled: (...a: unknown[]) => runSettled(...a)
}));

import SiteDoctorModal from './SiteDoctorModal.svelte';
import type { Site } from '$stores/sites';

function site(over: Partial<Site> = {}): Site {
  return { domain: 'acme.test', is_laravel: true, ...over } as Site;
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('SiteDoctorModal', () => {
  it('does not run the checks while closed', () => {
    render(SiteDoctorModal, { props: { open: false, site: site(), branch: '', onclose: () => {} } });
    expect(loadDoctor).not.toHaveBeenCalled();
    expect(loadCommands).not.toHaveBeenCalled();
  });

  it('renders each check title and detail when opened, with a Fix button for fixable findings', async () => {
    loadDoctor.mockResolvedValue({
      checks: [
        { name: 'app_key', status: 'fail', detail: 'APP_KEY is empty', fix: 'key:generate' },
        { name: 'app_debug', status: 'ok' }
      ],
      failures: 1,
      warnings: 0
    });
    loadCommands.mockResolvedValue([
      { name: 'key:generate', label: 'Generate APP_KEY', command: 'php artisan key:generate' }
    ]);

    render(SiteDoctorModal, { props: { open: true, site: site(), branch: '', onclose: () => {} } });

    expect(loadDoctor).toHaveBeenCalled();
    expect(await screen.findByText('Application key')).toBeTruthy();
    expect(screen.getByText('Debug mode')).toBeTruthy();
    expect(screen.getByText('APP_KEY is empty')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Fix' })).toBeTruthy();
  });

  // The slow container execs must not hold back the findings that are already in.
  it('shows each check as it streams in while the rest are still running', async () => {
    loadDoctor.mockImplementation((_d: string, _b: string, onCheck: (c: unknown) => void) => {
      onCheck({ name: 'app_key', status: 'fail', detail: 'APP_KEY is empty' });
      return new Promise(() => {});
    });
    loadCommands.mockResolvedValue([]);

    render(SiteDoctorModal, { props: { open: true, site: site(), branch: '', onclose: () => {} } });

    expect(await screen.findByText('APP_KEY is empty')).toBeTruthy();
    expect(screen.getByTestId('doctor-pending')).toBeTruthy();
    expect(screen.queryByText('All checks passed')).toBeNull();
  });

  // A fix naming a destructive command (migrate:fresh drops every table) must
  // still go through the confirm gate, so Fix launches rather than executing.
  it('launches the fix command so a confirm: true fix still prompts', async () => {
    loadDoctor.mockResolvedValue({
      checks: [{ name: 'migrations', status: 'fail', detail: 'pending migrations', fix: 'migrate:fresh' }],
      failures: 1,
      warnings: 0
    });
    const cmd = { name: 'migrate:fresh', label: 'Drop and re-migrate', command: 'php artisan migrate:fresh', confirm: true };
    loadCommands.mockResolvedValue([cmd]);

    render(SiteDoctorModal, { props: { open: true, site: site(), branch: 'feat-x', onclose: () => {} } });

    await fireEvent.click(await screen.findByRole('button', { name: 'Fix' }));

    expect(launchCommand).toHaveBeenCalledWith('acme.test', cmd, { branch: 'feat-x' });
    expect(runSettled).toHaveBeenCalled();
  });

  // A schema the engine does not hold is created through the doctor fix
  // endpoint: no framework command can make it, and migrations fail without it.
  it('creates a missing database through the fix endpoint', async () => {
    loadDoctor.mockResolvedValue({
      checks: [
        { name: 'server_database', label: 'Database', status: 'fail', detail: 'Database "shop" on mysql does not exist.', fix: 'database_create' }
      ],
      failures: 1,
      warnings: 0
    });
    loadCommands.mockResolvedValue([]);

    render(SiteDoctorModal, { props: { open: true, site: site(), branch: '', onclose: () => {} } });

    await fireEvent.click(await screen.findByRole('button', { name: 'Fix' }));

    expect(executeDoctorFix).toHaveBeenCalledWith('acme.test', 'database_create', 'Create the missing database', '');
    expect(launchCommand).not.toHaveBeenCalled();
  });

  // A unit left behind by a worker the definition dropped is removed on the
  // host, not in the site container, so it goes through the fix endpoint like
  // the vhost one rather than looking for a framework command.
  it('removes stale worker units through the fix endpoint', async () => {
    loadDoctor.mockResolvedValue({
      checks: [
        { name: 'stale_workers', label: 'Worker Units', status: 'warn', detail: 'unit files are still installed for a worker this site no longer declares (jump)', fix: 'stale_workers_remove' }
      ],
      failures: 0,
      warnings: 1
    });
    loadCommands.mockResolvedValue([]);

    render(SiteDoctorModal, { props: { open: true, site: site(), branch: '', onclose: () => {} } });

    await fireEvent.click(await screen.findByRole('button', { name: 'Fix' }));

    expect(executeDoctorFix).toHaveBeenCalledWith('acme.test', 'stale_workers_remove', 'Remove the stale worker units', '');
    expect(launchCommand).not.toHaveBeenCalled();
  });

  it('omits the Fix button when no matching command is available', async () => {
    loadDoctor.mockResolvedValue({
      checks: [{ name: 'storage_link', status: 'warn', detail: 'symlink missing', fix: 'storage:link' }],
      failures: 0,
      warnings: 1
    });
    loadCommands.mockResolvedValue([]);

    render(SiteDoctorModal, { props: { open: true, site: site(), branch: '', onclose: () => {} } });

    expect(await screen.findByText('Storage symlink')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Fix' })).toBeNull();
  });
});
