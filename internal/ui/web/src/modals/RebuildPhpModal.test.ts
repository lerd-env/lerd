import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
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
vi.mock('$stores/phpVersions', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, streamPhpRebuild: vi.fn(() => new Promise(() => {})) };
});

import RebuildPhpModal from './RebuildPhpModal.svelte';

describe('RebuildPhpModal', () => {
  it('calls it a rebuild on the container runtime', () => {
    phpRuntimeStore.set('container');
    render(RebuildPhpModal, { props: { version: '8.1' } });
    expect(screen.getByText('Rebuild PHP 8.1')).toBeInTheDocument();
  });

  // There is no image to rebuild on the host: the same action downloads the
  // published build, and calling it a rebuild names work that never happens.
  it('calls it an update on the native runtime', () => {
    phpRuntimeStore.set('native');
    render(RebuildPhpModal, { props: { version: '8.1' } });
    expect(screen.getByText('Update PHP 8.1')).toBeInTheDocument();
    expect(screen.queryByText(/Rebuild/)).toBeNull();
  });
});
