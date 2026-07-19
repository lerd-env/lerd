import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import ConfirmServiceInstallModal from './ConfirmServiceInstallModal.svelte';
import { presets, presetsLoaded } from '$stores/presets';
import { modal } from '$stores/modals';

// Keep the real stores and helpers; only stub the network-bound calls so the
// seeded preset list drives the tests.
const installPresetAndOpen = vi.fn();
vi.mock('$stores/presets', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$stores/presets')>();
  return {
    ...actual,
    loadPresets: vi.fn(),
    installPresetAndOpen: (...args: unknown[]) => installPresetAndOpen(...args)
  };
});

describe('ConfirmServiceInstallModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    presetsLoaded.set(true);
  });

  afterEach(() => {
    presets.set([]);
    presetsLoaded.set(false);
    modal.set({ kind: null });
  });

  it('offers to install a bundled preset', () => {
    presets.set([{ name: 'mongo', description: 'Document database' } as never]);
    modal.set({ kind: 'serviceInstall', serviceInstall: { name: 'mongo' } });
    render(ConfirmServiceInstallModal);
    expect(screen.getByText('Document database')).toBeInTheDocument();
    expect(screen.getByText('Add')).toBeInTheDocument();
  });

  it('installs the preset when the action is clicked', async () => {
    presets.set([{ name: 'mongo' } as never]);
    modal.set({ kind: 'serviceInstall', serviceInstall: { name: 'mongo' } });
    render(ConfirmServiceInstallModal);
    await fireEvent.click(screen.getByText('Add'));
    expect(installPresetAndOpen).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'mongo' }),
      expect.objectContaining({ onSuccess: expect.any(Function) })
    );
  });

  it('explains a custom service the store cannot install', () => {
    presets.set([{ name: 'mongo' } as never]);
    modal.set({ kind: 'serviceInstall', serviceInstall: { name: 'widgets' } });
    render(ConfirmServiceInstallModal);
    expect(screen.getByText(/custom service/i)).toBeInTheDocument();
    expect(screen.queryByText('Add')).not.toBeInTheDocument();
  });
});
