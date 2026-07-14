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
