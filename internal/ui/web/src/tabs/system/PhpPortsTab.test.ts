import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import PhpPortsTab from './PhpPortsTab.svelte';

// A hand-rolled store built inside vi.hoisted so it exists before the mock runs.
const status = vi.hoisted(() => {
  let value: { php_fpms: { version: string; ports?: string[] }[] } = { php_fpms: [] };
  const subs = new Set<(v: typeof value) => void>();
  return {
    subscribe(fn: (v: typeof value) => void) {
      subs.add(fn);
      fn(value);
      return () => subs.delete(fn);
    },
    set(v: typeof value) {
      value = v;
      subs.forEach((f) => f(value));
    }
  };
});
const { setFpmPorts } = vi.hoisted(() => ({ setFpmPorts: vi.fn() }));

vi.mock('$stores/status', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, status };
});
vi.mock('$stores/phpVersions', async (orig) => {
  const actual = (await orig()) as object;
  return { ...actual, setFpmPorts };
});

function numberInputs(container: HTMLElement) {
  return container.querySelectorAll('input[type="number"]') as NodeListOf<HTMLInputElement>;
}

describe('PhpPortsTab', () => {
  beforeEach(() => {
    setFpmPorts.mockReset();
    status.set({ php_fpms: [{ version: '8.4', ports: ['9000:9000'] }] });
  });

  it('optimistically applies an added port and persists it', async () => {
    setFpmPorts.mockResolvedValue({ ok: true, ports: ['9000:9000', '9100:9100'] });
    const { container, getByLabelText, getByText } = render(PhpPortsTab, { props: { version: '8.4' } });
    const [host, cont] = numberInputs(container);
    await fireEvent.input(host, { target: { value: '9100' } });
    await fireEvent.input(cont, { target: { value: '9100' } });
    await fireEvent.click(getByLabelText('Add'));
    expect(setFpmPorts).toHaveBeenCalledWith('8.4', ['9000:9000', '9100:9100']);
    expect(getByText('9100')).toBeTruthy();
  });

  it('rolls back to the persisted set when the save fails', async () => {
    setFpmPorts.mockResolvedValue({ ok: false, error: 'nope' });
    const { container, getByLabelText, getByText, queryByText } = render(PhpPortsTab, {
      props: { version: '8.4' }
    });
    const [host, cont] = numberInputs(container);
    await fireEvent.input(host, { target: { value: '9100' } });
    await fireEvent.input(cont, { target: { value: '9100' } });
    await fireEvent.click(getByLabelText('Add'));
    // The optimistic 9100 card is rolled back and the error surfaces.
    expect(queryByText('9100')).toBeNull();
    expect(getByText('nope')).toBeTruthy();
  });
});
