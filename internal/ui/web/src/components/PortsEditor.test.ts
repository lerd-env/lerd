import { render, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import PortsEditor from './PortsEditor.svelte';

function numberInputs(container: HTMLElement) {
  return container.querySelectorAll('input[type="number"]') as NodeListOf<HTMLInputElement>;
}

describe('PortsEditor', () => {
  it('adds a valid host:container spec and clears the inputs', async () => {
    const onadd = vi.fn();
    const { container, getByLabelText } = render(PortsEditor, {
      props: { ports: [], onadd, onremove: vi.fn() }
    });
    const [host, cont] = numberInputs(container);
    await fireEvent.input(host, { target: { value: '8080' } });
    await fireEvent.input(cont, { target: { value: '80' } });
    await fireEvent.click(getByLabelText('Add'));
    expect(onadd).toHaveBeenCalledWith('8080:80');
    expect(host.value).toBe('');
    expect(cont.value).toBe('');
  });

  it('rejects an out-of-range port with an error and no add', async () => {
    const onadd = vi.fn();
    const { container, getByLabelText, getByText } = render(PortsEditor, {
      props: { ports: [], onadd, onremove: vi.fn() }
    });
    const [host, cont] = numberInputs(container);
    await fireEvent.input(host, { target: { value: '0' } });
    await fireEvent.input(cont, { target: { value: '80' } });
    await fireEvent.click(getByLabelText('Add'));
    expect(onadd).not.toHaveBeenCalled();
    expect(getByText('Enter a port between 0 and 65535.')).toBeTruthy();
  });

  it('does not re-add a spec already present', async () => {
    const onadd = vi.fn();
    const { container, getByLabelText } = render(PortsEditor, {
      props: { ports: ['8080:80'], onadd, onremove: vi.fn() }
    });
    const [host, cont] = numberInputs(container);
    await fireEvent.input(host, { target: { value: '8080' } });
    await fireEvent.input(cont, { target: { value: '80' } });
    await fireEvent.click(getByLabelText('Add'));
    expect(onadd).not.toHaveBeenCalled();
  });

  it('splits ip:host:container and bare specs for display', () => {
    const { container } = render(PortsEditor, {
      props: { ports: ['127.0.0.1:5433:5432', '9000'], onadd: vi.fn(), onremove: vi.fn() }
    });
    const text = container.textContent ?? '';
    // ip:host:container shows the host (second-to-last) and container (last).
    expect(text).toContain('5433');
    expect(text).toContain('5432');
    // A bare port shows on both sides.
    expect(text).toContain('9000');
  });

  it('removes a spec via its delete button', async () => {
    const onremove = vi.fn();
    const { getAllByLabelText } = render(PortsEditor, {
      props: { ports: ['8080:80'], onadd: vi.fn(), onremove }
    });
    await fireEvent.click(getAllByLabelText('Remove')[0]);
    expect(onremove).toHaveBeenCalledWith('8080:80');
  });
});
