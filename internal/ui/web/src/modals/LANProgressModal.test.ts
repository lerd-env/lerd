import { render } from '@testing-library/svelte';
import { describe, it, expect, beforeEach } from 'vitest';
import LANProgressModal from './LANProgressModal.svelte';
import { lan } from '$stores/lan';
import { modal } from '$stores/modals';

describe('LANProgressModal', () => {
  beforeEach(() => {
    modal.set({ kind: 'lanProgress', lanAction: 'expose' } as never);
  });

  it('shows the address the dashboard is now reachable on', () => {
    lan.update((v) => ({ ...v, loading: false, error: '', lanIP: '192.168.0.200' }));
    const { container } = render(LANProgressModal);
    expect(container.querySelector('code')?.textContent).toBe('192.168.0.200');
  });

  // The address reaches {@html} through a paraglide message, which interpolates
  // without escaping. A hostile value has to render as text, not as markup.
  it('renders a hostile address inert instead of executing it', () => {
    lan.update((v) => ({
      ...v,
      loading: false,
      error: '',
      lanIP: '<img src=x onerror="alert(1)">'
    }));
    const { container } = render(LANProgressModal);
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('code')?.textContent).toBe('<img src=x onerror="alert(1)">');
  });
});
