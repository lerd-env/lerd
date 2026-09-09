import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, waitFor, fireEvent, screen } from '@testing-library/svelte';
import RuntimeDetail from './RuntimeDetail.svelte';

describe('RuntimeDetail', () => {
  beforeEach(() => vi.resetModules());

  function settings(phpRuntime: string) {
    globalThis.fetch = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            php_runtime: phpRuntime,
            php_runtime_applies: true,
            worker_exec_mode: 'exec',
            worker_mode_applies: true
          }),
          { status: 200 }
        )
    ) as unknown as typeof fetch;
  }

  it('offers the worker mode in container mode', async () => {
    settings('container');
    const { container } = render(RuntimeDetail);
    await waitFor(() => expect(container.textContent).toContain('Worker'));
  });

  it('hides the worker mode under the native runtime', async () => {
    settings('native');
    const { container } = render(RuntimeDetail);
    await waitFor(() => expect(container.textContent).not.toContain('Worker'));
    expect(container.textContent).toContain('PHP runtime');
  });

  // Picking container while still saved as native must reveal the worker choice
  // straight away, so both can be set and applied in one go rather than making
  // the user apply, wait, and come back for the second decision.
  it('reveals the worker mode as soon as container is selected', async () => {
    settings('native');
    const { container } = render(RuntimeDetail);
    await waitFor(() => expect(container.textContent).not.toContain('Worker'));

    await fireEvent.click(screen.getByRole('button', { name: /^Container/i }));

    await waitFor(() => expect(container.textContent).toContain('Worker'));
  });

  // And picking native back hides it again, without applying anything.
  it('hides the worker mode again when native is reselected', async () => {
    settings('container');
    const { container } = render(RuntimeDetail);
    await waitFor(() => expect(container.textContent).toContain('Worker'));

    await fireEvent.click(screen.getByRole('button', { name: /^Native/i }));

    await waitFor(() => expect(container.textContent).not.toContain('Worker'));
  });
});
