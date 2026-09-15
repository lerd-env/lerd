import { render, screen } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import { vi } from 'vitest';

const phpRuntimeStore = vi.hoisted(() => {
  let value = 'container';
  const subs = new Set<(v: string) => void>();
  return {
    subscribe(fn: (v: string) => void) {
      fn(value);
      subs.add(fn);
      return () => subs.delete(fn);
    },
    set(v: string) {
      value = v;
      subs.forEach((fn) => fn(value));
    }
  };
});

vi.mock('$stores/phpRuntime', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, phpRuntime: phpRuntimeStore, loadPHPRuntime: vi.fn() };
});
vi.mock('$stores/status', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, loadStatus: vi.fn() };
});

import ConfirmPhpRemoveModal from './ConfirmPhpRemoveModal.svelte';
import { openPhpRemoveModal, closeModal } from '$stores/modals';

describe('ConfirmPhpRemoveModal', () => {
  beforeEach(() => {
    closeModal();
    openPhpRemoveModal({ version: '8.1', siteCount: 0 });
  });

  it('promises a container teardown on the container runtime', () => {
    phpRuntimeStore.set('container');
    render(ConfirmPhpRemoveModal);
    expect(screen.getByText(/PHP-FPM service and container/)).toBeInTheDocument();
  });

  // Nothing containerised is removed under the native runtime, so the copy has
  // to name the host binaries it actually deletes.
  it('promises the host binaries on the native runtime', () => {
    phpRuntimeStore.set('native');
    render(ConfirmPhpRemoveModal);
    expect(screen.getByText(/native binaries/)).toBeInTheDocument();
    // The dashboard's own install refuses under this runtime, so the way back
    // is the command rather than a button that is not there.
    expect(screen.getByText(/lerd use 8\.1/)).toBeInTheDocument();
    expect(screen.queryByText(/container/)).toBeNull();
  });
});
